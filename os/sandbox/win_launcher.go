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

// Windows 进程启动器组合令牌、ACL、Job Object 与网络隔离层启动子进程。

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
	proc         *os.Process
	job          *ownedWindowsResource[syscall.Handle]
	acl          *windowsACLPolicy
	stdout       *windowsOutputBuffer
	stderr       *windowsOutputBuffer
	stdoutReader *ownedWindowsResource[syscall.Handle]
	stderrReader *ownedWindowsResource[syscall.Handle]
	stdoutDone   chan error
	stderrDone   chan error
	mu           sync.Mutex
}

// launchSandboxedProcess 创建并启动具备完整隔离层的进程。
func launchSandboxedProcess(cfg Config, command string, args []string) (process *windowsSandboxedProcess, resultErr error) {
	// 1. 创建受限令牌。
	token, err := createSandboxToken()
	if err != nil {
		return nil, fmt.Errorf("create sandbox token: %w", err)
	}
	tokenOwner := newOwnedWindowsResource(token)
	defer func() {
		resultErr = errors.Join(resultErr, tokenOwner.release(closeWindowsToken))
	}()

	// 2. 创建 LowBox/AppContainer 令牌，使文件 ACL 与网络隔离使用同一身份。
	lowBoxToken, appContainerSID, err := createLowBoxToken(tokenOwner.value(), cfg.Network)
	if err != nil {
		return nil, fmt.Errorf("create lowbox token: %w", err)
	}
	lowBoxTokenOwner := newOwnedWindowsResource(lowBoxToken)
	defer func() {
		resultErr = errors.Join(resultErr, lowBoxTokenOwner.release(closeWindowsToken))
	}()

	// 3. 应用工作区读写、额外路径只读和拒绝路径 ACL。
	aclPolicy, err := applyWindowsACLPolicy(cfg, appContainerSID)
	if err != nil {
		return nil, fmt.Errorf("apply ACL policy: %w", err)
	}

	// 4. 创建 Job Object。
	jobHandle, err := createSandboxJobObject(memoryLimitMB(cfg.MaxMemoryBytes), cfg.MaxProcesses)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("create job object: %w", err), aclPolicy.restoreACL())
	}
	job := newOwnedWindowsResource(jobHandle)

	// 5. 构造命令行与环境块。
	cmdLine := buildCommandLine(command, args)
	cmdLineW, err := syscall.UTF16PtrFromString(cmdLine)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("encode command line: %w", err), cleanupWindowsLaunch(aclPolicy, job))
	}
	workspaceW, err := syscall.UTF16PtrFromString(cfg.Workspace)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("encode workspace: %w", err), cleanupWindowsLaunch(aclPolicy, job))
	}
	cleanEnv, err := cleanWindowsEnv(cfg.Workspace)
	if err != nil {
		return nil, errors.Join(err, cleanupWindowsLaunch(aclPolicy, job))
	}
	envBlock, err := windowsEnvBlock(cleanEnv)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("build environment block: %w", err), cleanupWindowsLaunch(aclPolicy, job))
	}
	var envPtr *uint16
	if len(envBlock) > 0 {
		envPtr = &envBlock[0]
	}

	// 6. 创建标准输入、标准输出与标准错误管道。
	var stdinRHandle, stdinWHandle syscall.Handle
	var stdoutRHandle, stdoutWHandle syscall.Handle
	var stderrRHandle, stderrWHandle syscall.Handle
	sa := syscall.SecurityAttributes{Length: uint32(unsafe.Sizeof(syscall.SecurityAttributes{})), InheritHandle: 1}
	if pipeErr := syscall.CreatePipe(&stdinRHandle, &stdinWHandle, &sa, 0); pipeErr != nil {
		return nil, errors.Join(fmt.Errorf("create stdin pipe: %w", pipeErr), cleanupWindowsLaunch(aclPolicy, job))
	}
	stdinR := newOwnedWindowsResource(stdinRHandle)
	stdinW := newOwnedWindowsResource(stdinWHandle)
	if pipeErr := syscall.CreatePipe(&stdoutRHandle, &stdoutWHandle, &sa, 0); pipeErr != nil {
		return nil, errors.Join(fmt.Errorf("create stdout pipe: %w", pipeErr), cleanupWindowsLaunch(aclPolicy, stdinR, stdinW, job))
	}
	stdoutR := newOwnedWindowsResource(stdoutRHandle)
	stdoutW := newOwnedWindowsResource(stdoutWHandle)
	if pipeErr := syscall.CreatePipe(&stderrRHandle, &stderrWHandle, &sa, 0); pipeErr != nil {
		return nil, errors.Join(fmt.Errorf("create stderr pipe: %w", pipeErr), cleanupWindowsLaunch(aclPolicy, stdinR, stdinW, stdoutR, stdoutW, job))
	}
	stderrR := newOwnedWindowsResource(stderrRHandle)
	stderrW := newOwnedWindowsResource(stderrWHandle)

	// 父进程不提供交互式输入，提前关闭写端，让子进程读取标准输入时得到 EOF。
	if closeErr := stdinW.release(closeWindowsHandle); closeErr != nil {
		return nil, errors.Join(
			fmt.Errorf("close parent stdin writer: %w", closeErr),
			cleanupWindowsLaunch(aclPolicy, stdinR, stdinW, stdoutR, stdoutW, stderrR, stderrW, job),
		)
	}

	// 7. 使用精确继承列表，禁止沙箱进程获得宿主的其他可继承句柄。
	inheritedHandles := []windows.Handle{
		windows.Handle(stdinR.value()),
		windows.Handle(stdoutW.value()),
		windows.Handle(stderrW.value()),
	}
	attributeList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("create process attribute list: %w", err),
			cleanupWindowsLaunch(aclPolicy, stdinR, stdoutR, stdoutW, stderrR, stderrW, job),
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
			cleanupWindowsLaunch(aclPolicy, stdinR, stdoutR, stdoutW, stderrR, stderrW, job),
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

	// 8. 以挂起状态创建进程，确保进入 Job 后才开始执行用户代码。
	var pi windows.ProcessInformation
	creationFlags := uint32(windows.CREATE_SUSPENDED |
		windows.CREATE_NO_WINDOW |
		windows.CREATE_UNICODE_ENVIRONMENT |
		windows.EXTENDED_STARTUPINFO_PRESENT)
	err = windows.CreateProcessAsUser(
		windows.Token(lowBoxTokenOwner.value()),
		nil,
		cmdLineW,
		nil,
		nil,
		true,
		creationFlags,
		envPtr,
		workspaceW,
		&si.StartupInfo,
		&pi,
	)
	runtime.KeepAlive(envBlock)
	runtime.KeepAlive(cmdLineW)
	runtime.KeepAlive(workspaceW)
	runtime.KeepAlive(inheritedHandles)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("CreateProcessAsUser: %w", err),
			cleanupWindowsLaunch(aclPolicy, stdinR, stdoutR, stdoutW, stderrR, stderrW, job),
		)
	}
	thread := newOwnedWindowsResource(syscall.Handle(pi.Thread))
	childProcess := newOwnedWindowsResource(syscall.Handle(pi.Process))

	// 进程仍处于挂起态时立即交给 Job，缩短未受 Job 生命周期约束的窗口。
	if assignErr := assignProcessToJob(job.value(), childProcess.value()); assignErr != nil {
		return nil, errors.Join(
			assignErr,
			terminateOwnedWindowsProcess(childProcess),
			cleanupWindowsLaunch(aclPolicy, thread, childProcess, stdinR, stdoutR, stdoutW, stderrR, stderrW, job),
		)
	}

	// 创建完成后令牌不再参与子进程生命周期，立即关闭以缩短特权句柄存活时间。
	if closeTokenErr := releaseOwnedWindowsResources(closeWindowsToken, lowBoxTokenOwner, tokenOwner); closeTokenErr != nil {
		return nil, errors.Join(
			fmt.Errorf("close process creation tokens: %w", closeTokenErr),
			settleOwnedWindowsJob(job, 0, windowsJobTerminationLimit),
			cleanupWindowsLaunch(aclPolicy, thread, childProcess, stdinR, stdoutR, stdoutW, stderrR, stderrW, job),
		)
	}

	// 父进程释放子进程侧的管道句柄，避免自身阻止 EOF。
	if closePipeErr := releaseOwnedWindowsResources(closeWindowsHandle, stdinR, stdoutW, stderrW); closePipeErr != nil {
		return nil, errors.Join(
			fmt.Errorf("close inherited pipe handles: %w", closePipeErr),
			settleOwnedWindowsJob(job, 0, windowsJobTerminationLimit),
			cleanupWindowsLaunch(aclPolicy, thread, childProcess, stdinR, stdoutR, stdoutW, stderrR, stderrW, job),
		)
	}

	proc, err := os.FindProcess(int(pi.ProcessId))
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("open child process: %w", err),
			settleOwnedWindowsJob(job, 0, windowsJobTerminationLimit),
			cleanupWindowsLaunch(aclPolicy, thread, childProcess, stdoutR, stderrR, job),
		)
	}

	// 9. 恢复进程运行，并释放 CreateProcess 返回的原始句柄。
	if err := resumeWindowsThread(thread.value()); err != nil {
		return nil, errors.Join(
			err,
			settleOwnedWindowsJob(job, 0, windowsJobTerminationLimit),
			proc.Release(),
			cleanupWindowsLaunch(aclPolicy, thread, childProcess, stdoutR, stderrR, job),
		)
	}
	if err := releaseOwnedWindowsResources(closeWindowsHandle, thread, childProcess); err != nil {
		return nil, errors.Join(
			fmt.Errorf("close process launch handles: %w", err),
			settleOwnedWindowsJob(job, 0, windowsJobTerminationLimit),
			proc.Release(),
			cleanupWindowsLaunch(aclPolicy, thread, childProcess, stdoutR, stderrR, job),
		)
	}

	// 读取标准输出与标准错误。
	stdout := newWindowsOutputBuffer(cfg.MaxOutputBytes)
	stderr := newWindowsOutputBuffer(cfg.MaxStderrBytes)
	wp := &windowsSandboxedProcess{
		proc:         proc,
		job:          job,
		acl:          aclPolicy,
		stdout:       stdout,
		stderr:       stderr,
		stdoutReader: stdoutR,
		stderrReader: stderrR,
		stdoutDone:   make(chan error, 1),
		stderrDone:   make(chan error, 1),
	}
	go readHandle(stdout, stdoutR, wp.stdoutDone)
	go readHandle(stderr, stderrR, wp.stderrDone)
	return wp, nil
}

func (p *windowsSandboxedProcess) Wait() (*os.ProcessState, error) {
	if p.proc == nil {
		return nil, fmt.Errorf("sandbox process is not initialized")
	}
	return runWindowsWaitLifecycle(windowsWaitLifecycle[*os.ProcessState]{
		waitProcess: p.proc.Wait,
		settleJob:   p.settleJob,
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
	if p.proc == nil {
		return nil
	}
	err := p.proc.Kill()
	if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.EINVAL) {
		return nil
	}
	return err
}

func (p *windowsSandboxedProcess) cleanup() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	result := releaseOwnedWindowsResources(closeWindowsHandle, p.job, p.stdoutReader, p.stderrReader)
	if p.acl != nil {
		restoreErr := p.acl.restoreACL()
		result = errors.Join(result, restoreErr)
		if restoreErr == nil {
			p.acl = nil
		}
	}
	return result
}

func (p *windowsSandboxedProcess) settleJob() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	return settleOwnedWindowsJob(p.job, windowsJobExitGracePeriod, windowsJobTerminationLimit)
}

func settleOwnedWindowsJob(
	job *ownedWindowsResource[syscall.Handle],
	exitGrace, terminationTimeout time.Duration,
) error {
	handle := job.value()
	if handle == 0 {
		return nil
	}
	return settleWindowsJob(windowsJobLifecycle{
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

func (p *windowsSandboxedProcess) abortOutputReaders() error {
	return errors.Join(
		cancelAndReleaseWindowsReader(p.stdoutReader),
		cancelAndReleaseWindowsReader(p.stderrReader),
	)
}

func readHandle(
	buf *windowsOutputBuffer,
	handle *ownedWindowsResource[syscall.Handle],
	done chan<- error,
) {
	var result error
	defer func() {
		done <- errors.Join(result, handle.release(closeWindowsHandle))
		close(done)
	}()
	tmp := make([]byte, 4096)
	for {
		h := handle.value()
		if h == 0 {
			return
		}
		var n uint32
		err := syscall.ReadFile(h, tmp, &n, nil)
		if err != nil {
			if errors.Is(err, syscall.ERROR_BROKEN_PIPE) ||
				(handle.value() == 0 &&
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

func terminateOwnedWindowsProcess(process *ownedWindowsResource[syscall.Handle]) error {
	handle := process.value()
	if handle == 0 {
		return nil
	}
	return terminateWindowsProcess(windowsProcessTerminationLifecycle{
		terminate: func() error {
			return syscall.TerminateProcess(handle, 1)
		},
		wait: func(timeout time.Duration) (bool, error) {
			return waitForWindowsProcess(handle, timeout)
		},
	}, windowsJobTerminationLimit)
}

func cleanupWindowsLaunch(
	policy *windowsACLPolicy,
	handles ...*ownedWindowsResource[syscall.Handle],
) error {
	err := releaseOwnedWindowsResources(closeWindowsHandle, handles...)
	if policy != nil {
		err = errors.Join(err, policy.restoreACL())
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

func cancelAndReleaseWindowsReader(reader *ownedWindowsResource[syscall.Handle]) error {
	return reader.releaseAfter(func(handle syscall.Handle) error {
		cancelErr := windows.CancelIoEx(windows.Handle(handle), nil)
		if errors.Is(cancelErr, windows.ERROR_NOT_FOUND) {
			return nil
		}
		if cancelErr != nil {
			return fmt.Errorf("cancel child output read: %w", cancelErr)
		}
		return nil
	}, closeWindowsHandle)
}

func resumeWindowsThread(thread syscall.Handle) error {
	result, _, callErr := procResumeThread.Call(uintptr(thread))
	const failureResult uintptr = 0xFFFFFFFF
	if result == failureResult {
		return fmt.Errorf("ResumeThread: %w", callErr)
	}
	return nil
}

func buildCommandLine(command string, args []string) string {
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, command)
	parts = append(parts, args...)
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " "
		}
		result += quoteWindowsArg(p)
	}
	return result
}

func quoteWindowsArg(arg string) string {
	if arg == "" {
		return `""`
	}
	needsQuote := false
	for _, r := range arg {
		if r == ' ' || r == '\t' || r == '"' || r == '\\' {
			needsQuote = true
			break
		}
	}
	if !needsQuote {
		return arg
	}
	var out []rune
	out = append(out, '"')
	backslashes := 0
	for _, r := range arg {
		switch r {
		case '\\':
			backslashes++
		case '"':
			for i := 0; i < backslashes*2+1; i++ {
				out = append(out, '\\')
			}
			out = append(out, '"')
			backslashes = 0
		default:
			for i := 0; i < backslashes; i++ {
				out = append(out, '\\')
			}
			backslashes = 0
			out = append(out, r)
		}
	}
	for i := 0; i < backslashes*2; i++ {
		out = append(out, '\\')
	}
	out = append(out, '"')
	return string(out)
}

func windowsEnvBlock(env []string) ([]uint16, error) {
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

func memoryLimitMB(limitBytes int64) int {
	if limitBytes <= 0 {
		return 256
	}
	mb := int(limitBytes / (1024 * 1024))
	if mb < 1 {
		return 1
	}
	return mb
}
