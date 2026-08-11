//go:build windows

package sandbox

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows 进程启动器组合稳定 LowBox 身份、Job Object 与精确句柄继承启动子进程。

var (
	procResumeThread = modKernel32.NewProc("ResumeThread")
)

const (
	windowsJobExitGracePeriod   = 250 * time.Millisecond
	windowsJobTerminationLimit  = 5 * time.Second
	windowsJobPollInterval      = 10 * time.Millisecond
	windowsOutputDrainTimeLimit = 5 * time.Second
)

type jobObjectBasicAccountingInformation struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

type windowsOutputBuffer struct {
	mu     sync.Mutex
	buffer *boundedBuffer
}

func newWindowsOutputBuffer(limit int64) *windowsOutputBuffer {
	return &windowsOutputBuffer{buffer: newBoundedBuffer(limit)}
}

func (b *windowsOutputBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *windowsOutputBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func (b *windowsOutputBuffer) BytesSeen() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.BytesSeen()
}

func (b *windowsOutputBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Truncated()
}

type windowsSandboxedProcess struct {
	proc          *ownedWindowsResource[*os.Process]
	job           *ownedWindowsResource[syscall.Handle]
	stdout        *windowsOutputBuffer
	stderr        *windowsOutputBuffer
	stdoutReader  *windowsActiveResource[syscall.Handle]
	stderrReader  *windowsActiveResource[syscall.Handle]
	stdoutDone    chan error
	stderrDone    chan error
	completion    *windowsExecutionCompletion
	retention     *windowsRetainedLifecycle
	mu            sync.Mutex
	treeConfirmed bool
}

type windowsLaunchResources struct {
	token        *ownedWindowsResource[syscall.Token]
	lowBoxToken  *ownedWindowsResource[syscall.Token]
	thread       *ownedWindowsResource[syscall.Handle]
	process      *ownedWindowsResource[syscall.Handle]
	stdinReader  *ownedWindowsResource[syscall.Handle]
	stdoutReader *ownedWindowsResource[syscall.Handle]
	stdoutWriter *ownedWindowsResource[syscall.Handle]
	stderrReader *ownedWindowsResource[syscall.Handle]
	stderrWriter *ownedWindowsResource[syscall.Handle]
	job          *ownedWindowsResource[syscall.Handle]
}

// windowsLaunchOps 由单个 Windows Sandbox 实例持有，禁止测试通过可变全局函数注入故障。
type windowsLaunchOps struct {
	assignProcessToJob func(syscall.Handle, syscall.Handle) error
	findProcess        func(int) (*os.Process, error)
	resumeThread       func(syscall.Handle) error
	settleFailure      func(bool, *ownedWindowsResource[syscall.Handle], *ownedWindowsResource[syscall.Handle], time.Duration) (bool, error)
	releaseProcess     func(*os.Process) error
	closeToken         func(syscall.Token) error
	closeHandle        func(syscall.Handle) error
	settlementLimit    time.Duration
}

func newWindowsLaunchOps() windowsLaunchOps {
	return windowsLaunchOps{
		assignProcessToJob: assignProcessToJob,
		findProcess:        os.FindProcess,
		resumeThread:       resumeWindowsThread,
		settleFailure:      settleFailedWindowsLaunch,
		releaseProcess:     releaseWindowsProcess,
		closeToken:         closeWindowsToken,
		closeHandle:        closeWindowsHandle,
		settlementLimit:    windowsExecutionSettlementLimit,
	}
}

func (ops windowsLaunchOps) validate() error {
	switch {
	case ops.assignProcessToJob == nil:
		return fmt.Errorf("windows assign-process-to-job operation is required")
	case ops.findProcess == nil:
		return fmt.Errorf("windows find-process operation is required")
	case ops.resumeThread == nil:
		return fmt.Errorf("windows resume-thread operation is required")
	case ops.settleFailure == nil:
		return fmt.Errorf("windows failed-launch settlement operation is required")
	case ops.releaseProcess == nil:
		return fmt.Errorf("windows process release operation is required")
	case ops.closeToken == nil:
		return fmt.Errorf("windows token close operation is required")
	case ops.closeHandle == nil:
		return fmt.Errorf("windows handle close operation is required")
	case ops.settlementLimit <= 0:
		return fmt.Errorf("windows execution settlement limit must be positive")
	default:
		return nil
	}
}

type windowsProcessLauncher func(
	Config,
	*windowsWorkspace,
	*windowsExecutablePlan,
	*windowsDirectoryPlan,
	[]string,
	[]string,
	*windowsProcessQuarantine,
	windowsLaunchOps,
) (*windowsSandboxedProcess, error)

func (resources *windowsLaunchResources) ownedResources() windowsLaunchOwnedResources[syscall.Token, syscall.Handle] {
	if resources == nil {
		return windowsLaunchOwnedResources[syscall.Token, syscall.Handle]{}
	}
	return windowsLaunchOwnedResources[syscall.Token, syscall.Handle]{
		token:        resources.token,
		lowBoxToken:  resources.lowBoxToken,
		thread:       resources.thread,
		process:      resources.process,
		stdinReader:  resources.stdinReader,
		stdoutReader: resources.stdoutReader,
		stdoutWriter: resources.stdoutWriter,
		stderrReader: resources.stderrReader,
		stderrWriter: resources.stderrWriter,
		job:          resources.job,
	}
}

// take 将资源的唯一所有权转移给新的 Windows 生命周期对象。
func (r *ownedWindowsResource[T]) take() T {
	if r == nil {
		var zero T
		return zero
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	resource := r.resource
	var zero T
	r.resource = zero
	return resource
}

func (r *windowsActiveResource[T]) isClosing() bool {
	if r == nil {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closing
}

// launchSandboxedProcess 创建并启动具备完整隔离层的进程。
func launchSandboxedProcess(
	cfg Config,
	workspace *windowsWorkspace,
	executable *windowsExecutablePlan,
	workingDirectory *windowsDirectoryPlan,
	args []string,
	env []string,
	quarantine *windowsProcessQuarantine,
	ops windowsLaunchOps,
) (process *windowsSandboxedProcess, resultErr error) {
	if workspace == nil || executable == nil || workingDirectory == nil {
		return nil, fmt.Errorf("windows launch plans are required")
	}
	if quarantine == nil {
		return nil, fmt.Errorf("windows process quarantine is required")
	}
	if err := ops.validate(); err != nil {
		return nil, err
	}
	// 1. 创建受限令牌。
	token, err := createSandboxToken()
	if err != nil {
		return nil, fmt.Errorf("create sandbox token: %w", err)
	}
	tokenOwner := newOwnedWindowsResource(token)
	launchResourcesTransferred := false
	defer func() {
		if !launchResourcesTransferred {
			resultErr = errors.Join(resultErr, tokenOwner.release(closeWindowsToken))
		}
	}()

	// 2. 使用工作区稳定身份创建 LowBox/AppContainer 令牌。
	lowBoxToken, err := createLowBoxToken(tokenOwner.value(), workspace.appContainerSID, cfg.Network)
	if err != nil {
		return nil, fmt.Errorf("create lowbox token: %w", err)
	}
	lowBoxTokenOwner := newOwnedWindowsResource(lowBoxToken)
	defer func() {
		if !launchResourcesTransferred {
			resultErr = errors.Join(resultErr, lowBoxTokenOwner.release(closeWindowsToken))
		}
	}()

	// 3. 创建 Job Object。
	jobHandle, err := createSandboxJobObject(cfg.MaxMemoryBytes, cfg.MaxProcesses)
	if err != nil {
		return nil, fmt.Errorf("create job object: %w", err)
	}
	job := newOwnedWindowsResource(jobHandle)

	// 4. Command.Path 仅作为 argv[0] 和非空 ApplicationName，不参与 PATH 解析。
	argv := make([]string, 0, len(args)+1)
	argv = append(argv, executable.applicationName)
	argv = append(argv, args...)
	cmdLine := windows.ComposeCommandLine(argv)
	cmdLineW, err := syscall.UTF16PtrFromString(cmdLine)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("encode command line: %w", err), cleanupWindowsLaunch(job))
	}
	applicationNameW, err := syscall.UTF16PtrFromString(executable.applicationName)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("encode application name: %w", err), cleanupWindowsLaunch(job))
	}
	workingDirectoryW, err := syscall.UTF16PtrFromString(workingDirectory.path)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("encode working directory: %w", err), cleanupWindowsLaunch(job))
	}
	envBlock, err := windowsEnvBlock(env)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("build environment block: %w", err), cleanupWindowsLaunch(job))
	}
	var envPtr *uint16
	if len(envBlock) > 0 {
		envPtr = &envBlock[0]
	}

	// 5. 创建标准输入、标准输出与标准错误管道。
	var stdinRHandle, stdinWHandle syscall.Handle
	var stdoutRHandle, stdoutWHandle syscall.Handle
	var stderrRHandle, stderrWHandle syscall.Handle
	sa := syscall.SecurityAttributes{Length: uint32(unsafe.Sizeof(syscall.SecurityAttributes{})), InheritHandle: 1}
	if pipeErr := syscall.CreatePipe(&stdinRHandle, &stdinWHandle, &sa, 0); pipeErr != nil {
		return nil, errors.Join(fmt.Errorf("create stdin pipe: %w", pipeErr), cleanupWindowsLaunch(job))
	}
	stdinR := newOwnedWindowsResource(stdinRHandle)
	stdinW := newOwnedWindowsResource(stdinWHandle)
	if pipeErr := syscall.CreatePipe(&stdoutRHandle, &stdoutWHandle, &sa, 0); pipeErr != nil {
		return nil, errors.Join(fmt.Errorf("create stdout pipe: %w", pipeErr), cleanupWindowsLaunch(stdinR, stdinW, job))
	}
	stdoutR := newOwnedWindowsResource(stdoutRHandle)
	stdoutW := newOwnedWindowsResource(stdoutWHandle)
	if pipeErr := syscall.CreatePipe(&stderrRHandle, &stderrWHandle, &sa, 0); pipeErr != nil {
		return nil, errors.Join(fmt.Errorf("create stderr pipe: %w", pipeErr), cleanupWindowsLaunch(stdinR, stdinW, stdoutR, stdoutW, job))
	}
	stderrR := newOwnedWindowsResource(stderrRHandle)
	stderrW := newOwnedWindowsResource(stderrWHandle)

	// 父进程不提供交互式输入，提前关闭写端，让子进程读取标准输入时得到 EOF。
	if closeErr := stdinW.release(closeWindowsHandle); closeErr != nil {
		return nil, errors.Join(
			fmt.Errorf("close parent stdin writer: %w", closeErr),
			cleanupWindowsLaunch(stdinR, stdinW, stdoutR, stdoutW, stderrR, stderrW, job),
		)
	}

	// 6. 使用精确继承列表，禁止沙箱进程获得宿主的其他可继承句柄。
	inheritedHandles := []windows.Handle{
		windows.Handle(stdinR.value()),
		windows.Handle(stdoutW.value()),
		windows.Handle(stderrW.value()),
	}
	attributeList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("create process attribute list: %w", err),
			cleanupWindowsLaunch(stdinR, stdoutR, stdoutW, stderrR, stderrW, job),
		)
	}
	defer attributeList.Delete()
	attributeErr := attributeList.Update(
		windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST,
		unsafe.Pointer(&inheritedHandles[0]), // #nosec G103 -- 非空句柄数组在同步创建调用返回前保持存活。
		uintptr(len(inheritedHandles))*unsafe.Sizeof(inheritedHandles[0]),
	)
	if attributeErr != nil {
		return nil, errors.Join(
			fmt.Errorf("set inherited handle list: %w", attributeErr),
			cleanupWindowsLaunch(stdinR, stdoutR, stdoutW, stderrR, stderrW, job),
		)
	}
	si := windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Cb:        uint32(unsafe.Sizeof(windows.StartupInfoEx{})),
			Flags:     windows.STARTF_USESTDHANDLES,
			StdInput:  inheritedHandles[0],
			StdOutput: inheritedHandles[1],
			StdErr:    inheritedHandles[2],
		},
		ProcThreadAttributeList: attributeList.List(),
	}

	// 7. 启动前再次按句柄核验可执行文件与工作目录。
	if revalidateErr := executable.revalidate(); revalidateErr != nil {
		return nil, errors.Join(revalidateErr, cleanupWindowsLaunch(stdinR, stdoutR, stdoutW, stderrR, stderrW, job))
	}
	if revalidateErr := workingDirectory.revalidate(); revalidateErr != nil {
		return nil, errors.Join(revalidateErr, cleanupWindowsLaunch(stdinR, stdoutR, stdoutW, stderrR, stderrW, job))
	}

	// 8. 以挂起状态创建进程，确保进入 Job 后才开始执行用户代码。
	var pi windows.ProcessInformation
	creationFlags := uint32(windows.CREATE_SUSPENDED |
		windows.CREATE_NO_WINDOW |
		windows.CREATE_UNICODE_ENVIRONMENT |
		windows.EXTENDED_STARTUPINFO_PRESENT)
	err = windows.CreateProcessAsUser(
		windows.Token(lowBoxTokenOwner.value()),
		applicationNameW,
		cmdLineW,
		nil,
		nil,
		true,
		creationFlags,
		envPtr,
		workingDirectoryW,
		&si.StartupInfo,
		&pi,
	)
	runtime.KeepAlive(envBlock)
	runtime.KeepAlive(applicationNameW)
	runtime.KeepAlive(cmdLineW)
	runtime.KeepAlive(workingDirectoryW)
	runtime.KeepAlive(inheritedHandles)
	runtime.KeepAlive(executable.file)
	runtime.KeepAlive(workingDirectory.file)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("CreateProcessAsUser: %w", err),
			cleanupWindowsLaunch(stdinR, stdoutR, stdoutW, stderrR, stderrW, job),
		)
	}
	thread := newOwnedWindowsResource(syscall.Handle(pi.Thread))
	childProcess := newOwnedWindowsResource(syscall.Handle(pi.Process))
	launchResources := &windowsLaunchResources{
		token:        tokenOwner,
		lowBoxToken:  lowBoxTokenOwner,
		thread:       thread,
		process:      childProcess,
		stdinReader:  stdinR,
		stdoutReader: stdoutR,
		stdoutWriter: stdoutW,
		stderrReader: stderrR,
		stderrWriter: stderrW,
		job:          job,
	}

	// 进程创建成功后的六个阶段由实例级操作表驱动，任一失败都只经过一次统一所有权转移。
	proc, err := runWindowsLaunchProtocol(windowsLaunchProtocolOps[*os.Process]{
		assignJob: func() error {
			return ops.assignProcessToJob(job.value(), childProcess.value())
		},
		closeCreationTokens: func() error {
			if closeErr := releaseOwnedWindowsResources(ops.closeToken, lowBoxTokenOwner, tokenOwner); closeErr != nil {
				return fmt.Errorf("close process creation tokens: %w", closeErr)
			}
			return nil
		},
		closeInheritedPipes: func() error {
			if closeErr := releaseOwnedWindowsResources(ops.closeHandle, stdinR, stdoutW, stderrW); closeErr != nil {
				return fmt.Errorf("close inherited pipe handles: %w", closeErr)
			}
			return nil
		},
		findProcess: func() (*os.Process, error) {
			opened, openErr := ops.findProcess(int(pi.ProcessId))
			if openErr != nil {
				return nil, fmt.Errorf("open child process: %w", openErr)
			}
			return opened, nil
		},
		resumeThread: func() error {
			return ops.resumeThread(thread.value())
		},
		closeLaunchHandles: func() error {
			if closeErr := releaseOwnedWindowsResources(ops.closeHandle, thread, childProcess); closeErr != nil {
				return fmt.Errorf("close process launch handles: %w", closeErr)
			}
			return nil
		},
		onFailure: func(failure windowsLaunchFailure[*os.Process]) error {
			launchResourcesTransferred = true
			return quarantineFailedWindowsLaunch(
				quarantine,
				failure.assignedToJob,
				failure.process,
				launchResources,
				ops,
			)
		},
	})
	if err != nil {
		return nil, err
	}

	// 读取标准输出与标准错误。
	stdout := newWindowsOutputBuffer(cfg.MaxOutputBytes)
	stderr := newWindowsOutputBuffer(cfg.MaxStderrBytes)
	stdoutReader := newWindowsActiveResource(stdoutR.take())
	stderrReader := newWindowsActiveResource(stderrR.take())
	wp := &windowsSandboxedProcess{
		proc:         newOwnedWindowsResource(proc),
		job:          job,
		stdout:       stdout,
		stderr:       stderr,
		stdoutReader: stdoutReader,
		stderrReader: stderrReader,
		stdoutDone:   make(chan error, 1),
		stderrDone:   make(chan error, 1),
		completion:   newWindowsExecutionCompletion(),
	}
	wp.retention = newWindowsRetainedLifecycle(wp.reclaim)
	go readHandle(stdout, stdoutReader, wp.stdoutDone)
	go readHandle(stderr, stderrReader, wp.stderrDone)
	return wp, nil
}

func (p *windowsSandboxedProcess) Wait() (*os.ProcessState, error) {
	done := p.startWait()
	if done == nil {
		return nil, fmt.Errorf("sandbox process completion is not initialized")
	}
	<-done
	result, finished := p.completion.wait(0)
	if !finished {
		return nil, fmt.Errorf("sandbox process completion is unavailable")
	}
	return result.state, result.err
}

func (p *windowsSandboxedProcess) startWait() <-chan struct{} {
	if p == nil || p.completion == nil {
		return nil
	}
	return p.completion.start(func() windowsExecutionWaitResult {
		state, err := p.waitLifecycle()
		return windowsExecutionWaitResult{state: state, err: err}
	})
}

func (p *windowsSandboxedProcess) waitLifecycle() (*os.ProcessState, error) {
	if p.proc == nil || p.proc.value() == nil {
		return nil, fmt.Errorf("sandbox process is not initialized")
	}
	return runWindowsWaitLifecycle(windowsWaitLifecycle[*os.ProcessState]{
		waitProcess: func() (*os.ProcessState, error) {
			process := p.proc.value()
			if process == nil {
				return nil, fmt.Errorf("sandbox process is unavailable")
			}
			return process.Wait()
		},
		settleJob: p.settleJob,
		waitOutput: func() error {
			return waitForWindowsOutput(
				p.stdoutDone,
				p.stderrDone,
				windowsOutputDrainTimeLimit,
				p.abortOutputReaders,
			)
		},
		cleanup: p.cleanup,
	})
}

func (p *windowsSandboxedProcess) Kill() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.job != nil {
		job := p.job.value()
		if job == 0 {
			return nil
		}
		return terminateJob(job, 1)
	}
	if p.proc == nil || p.proc.value() == nil {
		return nil
	}
	err := p.proc.value().Kill()
	if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.EINVAL) {
		return nil
	}
	return err
}

func (p *windowsSandboxedProcess) cleanup() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	result := errors.Join(
		p.stdoutReader.closeAfterReads(nil, closeWindowsHandle, windowsOutputDrainTimeLimit),
		p.stderrReader.closeAfterReads(nil, closeWindowsHandle, windowsOutputDrainTimeLimit),
		p.proc.release(releaseWindowsProcess),
	)
	if p.treeConfirmed {
		result = errors.Join(result, p.job.release(closeWindowsHandle))
	}
	return result
}

func (p *windowsSandboxedProcess) settleJob() error {
	_, err := p.settleJobWithin(windowsJobExitGracePeriod, windowsJobTerminationLimit)
	return err
}

func (p *windowsSandboxedProcess) settleJobWithin(
	exitGrace, terminationTimeout time.Duration,
) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.treeConfirmed {
		return true, nil
	}
	confirmed, err := settleOwnedWindowsJob(p.job, exitGrace, terminationTimeout)
	p.treeConfirmed = confirmed
	if !confirmed {
		err = errors.Join(err, errWindowsProcessContainmentUnconfirmed)
	}
	return confirmed, err
}

func settleOwnedWindowsJob(
	job *ownedWindowsResource[syscall.Handle],
	exitGrace, terminationTimeout time.Duration,
) (bool, error) {
	handle := job.value()
	if handle == 0 {
		return false, fmt.Errorf("sandbox job handle is unavailable")
	}
	return settleWindowsJobConfirmed(windowsJobLifecycle{
		wait: func(timeout time.Duration) (bool, error) {
			return waitForWindowsJob(handle, timeout)
		},
		terminate: func() error {
			return terminateJob(handle, 1)
		},
		close: func() error {
			return job.release(closeWindowsHandle)
		},
	}, exitGrace, terminationTimeout)
}

func (p *windowsSandboxedProcess) processContainmentConfirmed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.treeConfirmed
}

func (p *windowsSandboxedProcess) lifecycleReleased() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.treeConfirmed &&
		p.proc.value() == nil &&
		p.job.value() == 0 &&
		p.stdoutReader.value() == 0 &&
		p.stderrReader.value() == 0
}

func (p *windowsSandboxedProcess) retain(quarantine *windowsProcessQuarantine) error {
	if p == nil || p.retention == nil {
		return fmt.Errorf("windows execution lifecycle retention is unavailable")
	}
	return p.retention.retain(quarantine)
}

func (p *windowsSandboxedProcess) reclaim(timeout time.Duration) (bool, error) {
	if p == nil || p.completion == nil {
		return false, fmt.Errorf("windows execution lifecycle is unavailable")
	}
	p.startWait()
	var result error
	if _, finished := p.completion.wait(0); !finished || !p.processContainmentConfirmed() {
		if err := p.Kill(); err != nil {
			result = errors.Join(result, fmt.Errorf("terminate retained Windows process tree: %w", err))
		}
	}
	wait, finished := p.completion.wait(timeout)
	if !finished {
		return false, errors.Join(
			result,
			fmt.Errorf("retained Windows process lifecycle did not finish within %s", timeout),
		)
	}
	result = errors.Join(result, wait.err)
	if !p.processContainmentConfirmed() {
		confirmed, settleErr := p.settleJobWithin(0, timeout)
		result = errors.Join(result, settleErr)
		if !confirmed {
			return false, errors.Join(result, errWindowsProcessContainmentUnconfirmed)
		}
	}
	result = errors.Join(result, p.cleanup())
	if !p.lifecycleReleased() {
		return false, errors.Join(result, fmt.Errorf("retained Windows process resources are not fully released"))
	}
	return true, result
}

func (p *windowsSandboxedProcess) abortOutputReaders() error {
	return errors.Join(
		cancelAndReleaseWindowsReader(p.stdoutReader),
		cancelAndReleaseWindowsReader(p.stderrReader),
	)
}

func readHandle(
	buf *windowsOutputBuffer,
	handle *windowsActiveResource[syscall.Handle],
	done chan<- error,
) {
	var result error
	defer func() {
		done <- errors.Join(result, handle.closeAfterReads(nil, closeWindowsHandle, windowsOutputDrainTimeLimit))
		close(done)
	}()
	tmp := make([]byte, 4096)
	for {
		h, ok := handle.beginRead()
		if !ok {
			return
		}
		var n uint32
		err := syscall.ReadFile(h, tmp, &n, nil)
		handle.endRead()
		if err != nil {
			if errors.Is(err, syscall.ERROR_BROKEN_PIPE) ||
				(handle.isClosing() &&
					(errors.Is(err, windows.ERROR_OPERATION_ABORTED) || errors.Is(err, windows.ERROR_INVALID_HANDLE))) {
				return
			}
			result = fmt.Errorf("read child output: %w", err)
			return
		}
		if n == 0 {
			return
		}
		if _, err := buf.Write(tmp[:n]); err != nil {
			result = fmt.Errorf("buffer child output: %w", err)
			return
		}
	}
}

func closeWindowsHandle(handle syscall.Handle) error {
	return syscall.CloseHandle(handle)
}

func closeWindowsToken(token syscall.Token) error {
	return token.Close()
}

func terminateOwnedWindowsProcessWithin(
	process *ownedWindowsResource[syscall.Handle],
	timeout time.Duration,
) (bool, error) {
	handle := process.value()
	if handle == 0 {
		return false, fmt.Errorf("sandbox process handle is unavailable")
	}
	var result error
	if err := syscall.TerminateProcess(handle, 1); err != nil {
		result = errors.Join(result, fmt.Errorf("terminate sandbox process: %w", err))
	}
	exited, err := waitForWindowsProcess(handle, timeout)
	if err != nil {
		result = errors.Join(result, fmt.Errorf("wait for terminated sandbox process: %w", err))
		return false, result
	}
	if !exited {
		result = errors.Join(result, fmt.Errorf("sandbox process did not exit within %s", timeout))
		return false, result
	}
	return true, result
}

func cleanupWindowsLaunch(handles ...*ownedWindowsResource[syscall.Handle]) error {
	return releaseOwnedWindowsResources(closeWindowsHandle, handles...)
}

func quarantineFailedWindowsLaunch(
	quarantine *windowsProcessQuarantine,
	assignedToJob bool,
	proc *os.Process,
	resources *windowsLaunchResources,
	ops windowsLaunchOps,
) error {
	lifecycle, ownership, lifecycleErr := newWindowsLaunchRetainedLifecycle(
		assignedToJob,
		proc,
		resources,
		ops,
	)
	if lifecycleErr != nil {
		return errors.Join(lifecycleErr, errWindowsProcessContainmentUnconfirmed)
	}
	return settleOrRetainWindowsLaunchFailure(
		quarantine,
		lifecycle,
		ownership.processContainmentConfirmed,
		nil,
		windowsJobTerminationLimit,
	)
}

func newWindowsLaunchRetainedLifecycle(
	assignedToJob bool,
	proc *os.Process,
	resources *windowsLaunchResources,
	ops windowsLaunchOps,
) (*windowsRetainedLifecycle, *windowsLaunchOwnership[syscall.Token, syscall.Handle, *os.Process], error) {
	if resources == nil {
		return nil, nil, fmt.Errorf("windows launch resources are required")
	}
	if err := ops.validate(); err != nil {
		return nil, nil, err
	}
	ownership := &windowsLaunchOwnership[syscall.Token, syscall.Handle, *os.Process]{
		assignedToJob:  assignedToJob,
		process:        newOwnedWindowsResource(proc),
		resources:      resources.ownedResources(),
		settle:         ops.settleFailure,
		releaseProcess: ops.releaseProcess,
		closeToken:     ops.closeToken,
		closeHandle:    ops.closeHandle,
	}
	return newWindowsRetainedLifecycle(ownership.reclaim), ownership, nil
}

func settleFailedWindowsLaunch(
	assignedToJob bool,
	process, job *ownedWindowsResource[syscall.Handle],
	timeout time.Duration,
) (bool, error) {
	if assignedToJob {
		confirmed, err := settleOwnedWindowsJob(job, 0, timeout)
		if !confirmed {
			err = errors.Join(err, errWindowsProcessContainmentUnconfirmed)
		}
		return confirmed, err
	}
	return terminateOwnedWindowsProcessWithin(process, timeout)
}

func releaseWindowsProcess(proc *os.Process) error {
	if proc == nil {
		return nil
	}
	err := proc.Release()
	// Windows Process.Wait 会以 released 状态完成句柄释放，后续 Release 返回 EINVAL。
	if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.EINVAL) {
		return nil
	}
	return err
}

func waitForWindowsJob(job syscall.Handle, timeout time.Duration) (bool, error) {
	return waitForWindowsJobProcesses(func() (uint32, error) {
		var info jobObjectBasicAccountingInformation
		if err := windows.QueryInformationJobObject(
			windows.Handle(job),
			windows.JobObjectBasicAccountingInformation,
			uintptr(unsafe.Pointer(&info)), // #nosec G103 -- 结构体布局与 JOBOBJECT_BASIC_ACCOUNTING_INFORMATION ABI 一致。
			uint32(unsafe.Sizeof(info)),
			nil,
		); err != nil {
			return 0, fmt.Errorf("query sandbox job accounting: %w", err)
		}
		runtime.KeepAlive(&info)
		return info.ActiveProcesses, nil
	}, timeout, windowsJobPollInterval)
}

func waitForWindowsProcess(process syscall.Handle, timeout time.Duration) (bool, error) {
	event, err := windows.WaitForSingleObject(windows.Handle(process), windowsWaitMilliseconds(timeout))
	if err != nil {
		return false, err
	}
	switch event {
	case windows.WAIT_OBJECT_0:
		return true, nil
	case uint32(windows.WAIT_TIMEOUT):
		return false, nil
	default:
		return false, fmt.Errorf("unexpected process wait result: %d", event)
	}
}

func windowsWaitMilliseconds(timeout time.Duration) uint32 {
	if timeout <= 0 {
		return 0
	}
	milliseconds := (timeout + time.Millisecond - 1) / time.Millisecond
	if milliseconds >= time.Duration(windows.INFINITE) {
		return windows.INFINITE - 1
	}
	return uint32(milliseconds)
}

func cancelAndReleaseWindowsReader(reader *windowsActiveResource[syscall.Handle]) error {
	return reader.closeAfterReads(func(handle syscall.Handle) error {
		cancelErr := windows.CancelIoEx(windows.Handle(handle), nil)
		if errors.Is(cancelErr, windows.ERROR_NOT_FOUND) {
			return nil
		}
		if cancelErr != nil {
			return fmt.Errorf("cancel child output read: %w", cancelErr)
		}
		return nil
	}, closeWindowsHandle, windowsOutputDrainTimeLimit)
}

func resumeWindowsThread(thread syscall.Handle) error {
	result, _, callErr := procResumeThread.Call(uintptr(thread))
	const failureResult uintptr = 0xFFFFFFFF
	if result == failureResult {
		return fmt.Errorf("ResumeThread: %w", callErr)
	}
	return nil
}

func windowsEnvBlock(env []string) ([]uint16, error) {
	if len(env) == 0 {
		return []uint16{0, 0}, nil
	}
	var block []uint16
	for _, e := range env {
		if strings.ContainsRune(e, '\x00') {
			return nil, fmt.Errorf("environment entry contains NUL")
		}
		block = append(block, utf16.Encode([]rune(e))...)
		block = append(block, 0)
	}
	block = append(block, 0)
	return block, nil
}
