//go:build windows

package sandbox

import (
	"fmt"
	"os"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// Phase 8 D32: Windows process launcher
//
// Combines all isolation layers to launch a sandboxed process:
//   Token + ACL + Job Object + Network + Desktop → CreateProcessAsUser

var (
	procCreateProcessAsUserW2 = modAdvapi32.NewProc("CreateProcessAsUserW")
	procResumeThread          = modKernel32.NewProc("ResumeThread")
)

type windowsSandboxedProcess struct {
	proc       *os.Process
	job        syscall.Handle
	acl        *windowsACLPolicy
	stdout     *boundedBuffer
	stderr     *boundedBuffer
	stdoutDone chan struct{}
	stderrDone chan struct{}
	cleaned    bool
}

// launchSandboxedProcess creates and starts a process with full sandbox isolation.
func launchSandboxedProcess(cfg Config, command string, args []string) (*windowsSandboxedProcess, error) {
	// 1. Create restricted token
	token, err := createSandboxToken()
	if err != nil {
		return nil, fmt.Errorf("create sandbox token: %w", err)
	}
	defer token.Close()

	// 2. Create LowBox/AppContainer token. This is used for both filesystem ACL
	// grants and network isolation, so the process identity matches the DACLs.
	lowBoxToken, appContainerSID, err := createLowBoxToken(token, cfg.Network)
	if err != nil {
		return nil, fmt.Errorf("create lowbox token: %w", err)
	}
	defer lowBoxToken.Close()
	finalToken := lowBoxToken

	// 3. Apply ACL policy: workspace RW, ReadablePaths RO, DeniedPaths deny.
	aclPolicy, err := applyWindowsACLPolicy(cfg, appContainerSID)
	if err != nil {
		return nil, fmt.Errorf("apply ACL policy: %w", err)
	}

	// 4. Create Job Object
	jobHandle, err := createSandboxJobObject(memoryLimitMB(cfg.MaxMemoryBytes), cfg.MaxProcesses)
	if err != nil {
		_ = aclPolicy.restoreACL()
		return nil, fmt.Errorf("create job object: %w", err)
	}

	// 5. Create isolated desktop
	desktop, err := createIsolatedDesktop(fmt.Sprintf("hexclaw_sandbox_%d", os.Getpid()))
	if err != nil {
		syscall.CloseHandle(jobHandle)
		_ = aclPolicy.restoreACL()
		return nil, fmt.Errorf("create isolated desktop: %w", err)
	} else {
		defer desktop.Close()
	}

	// 6. Build command line
	cmdLine := buildCommandLine(command, args)
	cmdLineW, _ := syscall.UTF16PtrFromString(cmdLine)
	workspaceW, _ := syscall.UTF16PtrFromString(cfg.Workspace)
	envBlock, err := windowsEnvBlock(cleanWindowsEnv(cfg.Workspace))
	if err != nil {
		syscall.CloseHandle(jobHandle)
		_ = aclPolicy.restoreACL()
		return nil, fmt.Errorf("build environment block: %w", err)
	}
	var envPtr uintptr
	if len(envBlock) > 0 {
		envPtr = uintptr(unsafe.Pointer(&envBlock[0]))
	}

	// 7. Setup STARTUPINFO with isolated desktop
	var si syscall.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = syscall.STARTF_USESTDHANDLES
	if desktop != nil {
		desktopNameW, _ := syscall.UTF16PtrFromString(desktop.DesktopName())
		si.Desktop = desktopNameW
	}

	// 8. Create stdout/stderr pipes
	var stdoutR, stdoutW, stderrR, stderrW syscall.Handle
	sa := syscall.SecurityAttributes{Length: uint32(unsafe.Sizeof(syscall.SecurityAttributes{})), InheritHandle: 1}
	if err := syscall.CreatePipe(&stdoutR, &stdoutW, &sa, 0); err != nil {
		syscall.CloseHandle(jobHandle)
		_ = aclPolicy.restoreACL()
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	if err := syscall.CreatePipe(&stderrR, &stderrW, &sa, 0); err != nil {
		syscall.CloseHandle(stdoutR)
		syscall.CloseHandle(stdoutW)
		syscall.CloseHandle(jobHandle)
		_ = aclPolicy.restoreACL()
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}
	si.StdOutput = stdoutW
	si.StdErr = stderrW

	// 9. CreateProcessAsUser
	var pi syscall.ProcessInformation
	const CREATE_SUSPENDED = 0x00000004
	const CREATE_NEW_CONSOLE = 0x00000010
	const CREATE_UNICODE_ENVIRONMENT = 0x00000400

	r, _, callErr := procCreateProcessAsUserW2.Call(
		uintptr(finalToken),
		0, // application name (use cmdLine)
		uintptr(unsafe.Pointer(cmdLineW)),
		0, 0, // security attributes
		1, // inherit handles
		CREATE_SUSPENDED|CREATE_NEW_CONSOLE|CREATE_UNICODE_ENVIRONMENT,
		envPtr,
		uintptr(unsafe.Pointer(workspaceW)),
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	)
	if r == 0 {
		syscall.CloseHandle(stdoutR)
		syscall.CloseHandle(stdoutW)
		syscall.CloseHandle(stderrR)
		syscall.CloseHandle(stderrW)
		syscall.CloseHandle(jobHandle)
		_ = aclPolicy.restoreACL()
		return nil, fmt.Errorf("CreateProcessAsUser: %w", callErr)
	}

	// Close write ends (parent doesn't need them)
	syscall.CloseHandle(stdoutW)
	syscall.CloseHandle(stderrW)

	// 10. Assign to Job Object
	if err := assignProcessToJob(jobHandle, pi.Process); err != nil {
		syscall.TerminateProcess(pi.Process, 1)
		syscall.CloseHandle(pi.Thread)
		syscall.CloseHandle(pi.Process)
		syscall.CloseHandle(stdoutR)
		syscall.CloseHandle(stderrR)
		syscall.CloseHandle(jobHandle)
		_ = aclPolicy.restoreACL()
		return nil, err
	}

	// 11. Resume the process
	procResumeThread.Call(uintptr(pi.Thread))
	syscall.CloseHandle(pi.Thread)
	syscall.CloseHandle(pi.Process)

	// Read stdout/stderr
	stdout := newBoundedBuffer(cfg.MaxOutputBytes)
	stderr := newBoundedBuffer(cfg.MaxStderrBytes)
	wp := &windowsSandboxedProcess{
		job:        jobHandle,
		acl:        aclPolicy,
		stdout:     stdout,
		stderr:     stderr,
		stdoutDone: make(chan struct{}),
		stderrDone: make(chan struct{}),
	}
	go readHandle(stdout, stdoutR, wp.stdoutDone)
	go readHandle(stderr, stderrR, wp.stderrDone)

	proc, _ := os.FindProcess(int(pi.ProcessId))
	wp.proc = proc
	return wp, nil
}

func (p *windowsSandboxedProcess) Wait() (*os.ProcessState, error) {
	state, err := p.proc.Wait()
	<-p.stdoutDone
	<-p.stderrDone
	p.cleanup()
	return state, err
}

func (p *windowsSandboxedProcess) Kill() error {
	if p.job != 0 {
		_ = terminateJob(p.job, 1)
	}
	return p.proc.Kill()
}

func (p *windowsSandboxedProcess) cleanup() {
	if p.cleaned {
		return
	}
	p.cleaned = true
	if p.job != 0 {
		syscall.CloseHandle(p.job)
		p.job = 0
	}
	if p.acl != nil {
		_ = p.acl.restoreACL()
		p.acl = nil
	}
}

func readHandle(buf *boundedBuffer, h syscall.Handle, done chan<- struct{}) {
	defer close(done)
	defer syscall.CloseHandle(h)
	tmp := make([]byte, 4096)
	for {
		var n uint32
		err := syscall.ReadFile(h, tmp, &n, nil)
		if err != nil || n == 0 {
			break
		}
		buf.Write(tmp[:n])
	}
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
