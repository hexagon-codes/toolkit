//go:build windows

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindows_SandboxCreation(t *testing.T) {
	sandboxValue := newWindowsTestSandbox(t, Config{Workspace: t.TempDir(), Timeout: 30, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	if sandboxValue == nil {
		t.Fatal("sandbox is nil")
	}
}

func TestWindows_ExecSimpleCommand(t *testing.T) {
	sandboxValue := newWindowsTestSandbox(t, Config{Workspace: t.TempDir(), Timeout: 10, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	result, err := sandboxValue.Exec(context.Background(), Command{
		Path: canonicalWindowsSystemExecutable(t, "cmd.exe"),
		Args: []string{"/d", "/c", "echo", "hello"},
	})
	if err != nil {
		t.Fatalf("execute structured cmd command: %v", err)
	}
	if !strings.Contains(result.Stdout, "hello") {
		t.Fatalf("stdout = %q, want hello", result.Stdout)
	}
}

func TestWindows_CommandDirOutsideWorkspaceDenied(t *testing.T) {
	sandboxValue := newWindowsTestSandbox(t, Config{Workspace: t.TempDir(), Timeout: 10, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	_, err := sandboxValue.Exec(context.Background(), Command{
		Path: canonicalWindowsSystemExecutable(t, "cmd.exe"),
		Args: []string{"/d", "/c", "cd"},
		Dir:  t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "Command.Dir must remain inside") {
		t.Fatalf("outside Command.Dir error = %v, want workspace containment rejection", err)
	}
}

func TestWindows_CommandDirRejectsAlternateDataStream(t *testing.T) {
	sandboxValue := newWindowsTestSandbox(t, Config{Workspace: t.TempDir(), Timeout: 10, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	_, err := sandboxValue.Exec(context.Background(), Command{
		Path: canonicalWindowsSystemExecutable(t, "cmd.exe"),
		Args: []string{"/d", "/c", "cd"},
		Dir:  `workspace:stream`,
	})
	if err == nil || !strings.Contains(err.Error(), "alternate data streams") {
		t.Fatalf("alternate-stream Command.Dir error = %v, want rejection", err)
	}
}

func TestWindows_CommandPathMustBeCanonicalAbsolute(t *testing.T) {
	sandboxValue := newWindowsTestSandbox(t, Config{Workspace: t.TempDir(), Timeout: 10, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	_, err := sandboxValue.Exec(context.Background(), Command{Path: "cmd.exe", Args: []string{"/c", "exit"}})
	if err == nil || !strings.Contains(err.Error(), "absolute canonical path") {
		t.Fatalf("relative Command.Path error = %v, want canonical-path rejection", err)
	}
}

func TestWindows_ExecPythonCode(t *testing.T) {
	pythonPath, err := exec.LookPath("python")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			t.Skip("python is not installed on this Windows runner")
		}
		t.Fatalf("resolve python: %v", err)
	}
	pythonFile, err := os.Open(pythonPath)
	if err != nil {
		t.Fatalf("open python: %v", err)
	}
	defer pythonFile.Close()
	pythonPath, err = canonicalWindowsPathFromHandle(pythonFile)
	if err != nil {
		t.Fatalf("resolve canonical python path: %v", err)
	}
	sandboxValue := newWindowsTestSandbox(t, Config{Workspace: t.TempDir(), Timeout: 15, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	result, err := sandboxValue.Exec(context.Background(), Command{
		Path: pythonPath,
		Args: []string{"-c", `print("hello from sandbox")`},
	})
	if err != nil {
		t.Fatalf("execute Python code: %v", err)
	}
	if !strings.Contains(result.Stdout, "hello from sandbox") {
		t.Fatalf("stdout = %q, want Python output", result.Stdout)
	}
}

func TestWindows_WorkspaceIsolation(t *testing.T) {
	workspacePath := t.TempDir()
	sandboxValue := newWindowsTestSandbox(t, Config{Workspace: workspacePath, Timeout: 10, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	if err := os.WriteFile(filepath.Join(workspacePath, "test.txt"), []byte("sandbox data"), 0o600); err != nil {
		t.Fatalf("write workspace fixture: %v", err)
	}
	result, err := sandboxValue.Exec(context.Background(), Command{
		Path: canonicalWindowsSystemExecutable(t, "cmd.exe"),
		Args: []string{"/d", "/c", "type", "test.txt"},
	})
	if err != nil {
		t.Fatalf("read workspace file: %v", err)
	}
	if !strings.Contains(result.Stdout, "sandbox data") {
		t.Fatalf("stdout = %q, want workspace content", result.Stdout)
	}
	if _, err := sandboxValue.Exec(context.Background(), Command{
		Path: canonicalWindowsSystemExecutable(t, "cmd.exe"),
		Args: []string{"/d", "/c", "echo", "child-data", ">", "child.txt"},
	}); err != nil {
		t.Fatalf("write workspace file from AppContainer: %v", err)
	}
	written, err := os.ReadFile(filepath.Join(workspacePath, "child.txt"))
	if err != nil {
		t.Fatalf("read AppContainer workspace output: %v", err)
	}
	if !strings.Contains(string(written), "child-data") {
		t.Fatalf("AppContainer workspace output = %q, want child-data", written)
	}
}

func TestWindows_HostPathEscapeDenied(t *testing.T) {
	const sentinel = "WINDOWS_HOST_ESCAPE_SENTINEL_6F7D0C19"
	externalPath := filepath.Join(t.TempDir(), "host-secret.txt")
	if err := os.WriteFile(externalPath, []byte(sentinel), 0o600); err != nil {
		t.Fatalf("create external sentinel: %v", err)
	}
	externalWritePath := filepath.Join(filepath.Dir(externalPath), "sandbox-write-escape.txt")

	workspacePath := t.TempDir()
	if windowsPathsOverlap(workspacePath, externalPath) {
		t.Fatalf("host sentinel fixture unexpectedly overlaps the workspace: workspace=%q sentinel=%q", workspacePath, externalPath)
	}
	sandboxValue := newWindowsTestSandbox(t, Config{Workspace: workspacePath, Timeout: 10, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	payloadPath := installWindowsTestPayload(t, workspacePath, "sandbox-host-read-payload.exe")
	result, err := sandboxValue.Exec(context.Background(), Command{
		Path: payloadPath,
		Args: []string{
			"-test.run=^TestWindows_HostPathEscapePayload$",
			"-test.count=1",
			"--",
			externalPath,
			externalWritePath,
		},
	})
	if err != nil {
		t.Fatalf("execute host-read denial payload: %v", err)
	}
	if result == nil {
		t.Fatal("host-read denial payload returned no result")
	}
	if strings.Contains(result.Stdout, sentinel) || strings.Contains(result.Stderr, sentinel) {
		t.Fatalf("external sentinel leaked: stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stdout, "HOST_READ_DENIED") {
		t.Fatalf("host read was not explicitly denied: stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stdout, "HOST_OVERWRITE_DENIED") {
		t.Fatalf("host overwrite was not explicitly denied: stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stdout, "HOST_WRITE_DENIED") {
		t.Fatalf("host write was not explicitly denied: stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
	if content, err := os.ReadFile(externalPath); err != nil {
		t.Fatalf("re-read external sentinel: %v", err)
	} else if string(content) != sentinel {
		t.Fatalf("external sentinel was modified: got=%q want=%q", content, sentinel)
	}
	if content, err := os.ReadFile(externalWritePath); err == nil {
		t.Fatalf("sandbox wrote outside the workspace: %q", content)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect external write target: %v", err)
	}
}

// TestWindows_HostPathEscapePayload 在复制后的测试载荷中尝试读写宿主路径。
func TestWindows_HostPathEscapePayload(t *testing.T) {
	if !strings.EqualFold(filepath.Base(os.Args[0]), "sandbox-host-read-payload.exe") {
		return
	}
	arguments := argumentsAfterDoubleDash(os.Args)
	if len(arguments) != 2 {
		fmt.Print("HOST_READ_FIXTURE_INVALID")
		return
	}
	content, err := os.ReadFile(arguments[0])
	if err != nil {
		fmt.Print("HOST_READ_DENIED")
	} else {
		fmt.Printf("HOST_READ_LEAK:%s", content)
	}
	if err := os.WriteFile(arguments[0], []byte("HOST_OVERWRITE_LEAK"), 0o600); err != nil {
		fmt.Print(" HOST_OVERWRITE_DENIED")
	} else {
		fmt.Print(" HOST_OVERWRITE_ALLOWED")
	}
	if err := os.WriteFile(arguments[1], []byte("HOST_WRITE_LEAK"), 0o600); err != nil {
		fmt.Print(" HOST_WRITE_DENIED")
		return
	}
	fmt.Print(" HOST_WRITE_ALLOWED")
}

func TestWindows_Timeout(t *testing.T) {
	workspacePath := t.TempDir()
	sandboxValue := newWindowsTestSandbox(t, Config{Workspace: workspacePath, Timeout: 2, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	payloadPath := installWindowsTestPayload(t, workspacePath, "sandbox-timeout-payload.exe")
	started := time.Now()
	result, execErr := sandboxValue.Exec(context.Background(), Command{
		Path: payloadPath,
		Args: []string{"-test.run=^TestWindows_TimeoutPayload$", "-test.count=1"},
	})
	if !errors.Is(execErr, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v, want context deadline exceeded", execErr)
	}
	if elapsed := time.Since(started); elapsed > 8*time.Second {
		t.Fatalf("timeout did not converge within the bound: %s", elapsed)
	}
	if result == nil || result.Limits.ProcessContainment != LimitStatusEnforced {
		t.Fatalf("timeout ProcessContainment status = %+v, want enforced", result)
	}
}

func TestWindows_TimeoutTerminatesProcessContainment(t *testing.T) {
	workspacePath := t.TempDir()
	sandboxValue := newWindowsTestSandbox(t, Config{
		Workspace:            workspacePath,
		Timeout:              2,
		MaxProcesses:         8,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities | CapabilityProcesses,
	})
	payloadPath := installWindowsTestPayload(t, workspacePath, "sandbox-tree-timeout-payload.exe")
	pidPath := filepath.Join(workspacePath, "child.pid")
	result, execErr := sandboxValue.Exec(context.Background(), Command{
		Path: payloadPath,
		Args: []string{
			"-test.run=^TestWindows_ProcessContainmentRootPayload$",
			"-test.count=1",
			"--",
			pidPath,
		},
	})
	if !errors.Is(execErr, context.DeadlineExceeded) {
		t.Fatalf("process-containment timeout error = %v, want context deadline exceeded", execErr)
	}
	if result == nil || result.Limits.ProcessContainment != LimitStatusEnforced {
		t.Fatalf("process-containment timeout report = %+v, want enforced", result)
	}
	pidBytes, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("read child PID fixture: %v", err)
	}
	pidValue, err := strconv.ParseUint(strings.TrimSpace(string(pidBytes)), 10, 32)
	if err != nil {
		t.Fatalf("parse child PID: %v", err)
	}
	process, err := windows.OpenProcess(
		windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false,
		uint32(pidValue),
	)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return
	}
	if err != nil {
		t.Fatalf("open timed-out child process: %v", err)
	}
	defer windows.CloseHandle(process)
	waitResult, err := windows.WaitForSingleObject(process, 0)
	if err != nil {
		t.Fatalf("inspect timed-out child process: %v", err)
	}
	if waitResult != windows.WAIT_OBJECT_0 {
		t.Fatalf("child process %d survived Job termination", pidValue)
	}
}

// TestWindows_ProcessContainmentRootPayload 启动后代后保持根进程存活，供 Job 子树超时测试使用。
func TestWindows_ProcessContainmentRootPayload(t *testing.T) {
	if !strings.EqualFold(filepath.Base(os.Args[0]), "sandbox-tree-timeout-payload.exe") {
		return
	}
	arguments := argumentsAfterDoubleDash(os.Args)
	if len(arguments) != 1 {
		fmt.Print("PROCESS_TREE_FIXTURE_INVALID")
		return
	}
	child := exec.Command(
		os.Args[0],
		"-test.run=^TestWindows_ProcessContainmentChildPayload$",
		"-test.count=1",
	) // #nosec G204 -- 仅重启工作区内已冻结的测试载荷。
	if err := child.Start(); err != nil {
		fmt.Printf("PROCESS_TREE_CHILD_START_FAILED:%v", err)
		return
	}
	if err := os.WriteFile(arguments[0], []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
		fmt.Printf("PROCESS_TREE_PID_WRITE_FAILED:%v", err)
		return
	}
	time.Sleep(30 * time.Second)
}

// TestWindows_ProcessContainmentChildPayload 模拟必须随 Job 一起终止的后代。
func TestWindows_ProcessContainmentChildPayload(t *testing.T) {
	if !strings.EqualFold(filepath.Base(os.Args[0]), "sandbox-tree-timeout-payload.exe") {
		return
	}
	time.Sleep(30 * time.Second)
}

// TestWindows_TimeoutPayload 为超时测试提供确定性阻塞载荷。
func TestWindows_TimeoutPayload(t *testing.T) {
	if !strings.EqualFold(filepath.Base(os.Args[0]), "sandbox-timeout-payload.exe") {
		return
	}
	time.Sleep(30 * time.Second)
}

func TestWindows_ExecPreservesNonZeroExitCode(t *testing.T) {
	sandboxValue := newWindowsTestSandbox(t, Config{Workspace: t.TempDir(), Timeout: 10, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	result, err := sandboxValue.Exec(context.Background(), Command{
		Path: canonicalWindowsSystemExecutable(t, "cmd.exe"),
		Args: []string{"/d", "/c", "exit", "7"},
	})
	if err != nil {
		t.Fatalf("execute non-zero command: %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7", result.ExitCode)
	}
}

func TestWindows_ConcurrentExecOnSameSandbox(t *testing.T) {
	sandboxValue := newWindowsTestSandbox(t, Config{Workspace: t.TempDir(), Timeout: 10, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	cmdPath := canonicalWindowsSystemExecutable(t, "cmd.exe")
	type execution struct {
		result *ExecResult
		err    error
	}
	results := make(chan execution, 2)
	var group sync.WaitGroup
	for _, marker := range []string{"first", "second"} {
		marker := marker
		group.Add(1)
		go func() {
			defer group.Done()
			result, execErr := sandboxValue.Exec(context.Background(), Command{
				Path: cmdPath,
				Args: []string{"/d", "/c", "echo", marker},
			})
			results <- execution{result: result, err: execErr}
		}()
	}
	group.Wait()
	close(results)
	seen := make(map[string]bool)
	for execution := range results {
		if execution.err != nil {
			t.Fatalf("concurrent Exec failed: %v", execution.err)
		}
		if execution.result.Limits.ProcessContainment != LimitStatusEnforced {
			t.Fatalf("concurrent Exec ProcessContainment = %q, want enforced", execution.result.Limits.ProcessContainment)
		}
		for _, marker := range []string{"first", "second"} {
			if strings.Contains(execution.result.Stdout, marker) {
				seen[marker] = true
			}
		}
	}
	if !seen["first"] || !seen["second"] {
		t.Fatalf("concurrent outputs are incomplete: %#v", seen)
	}
}

func TestWindows_PersistentIdentitySurvivesInitializerCrash(t *testing.T) {
	workspacePath := t.TempDir()
	testExecutable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	initializer := exec.Command(testExecutable, "-test.run=^TestWindows_CrashingInitializerPayload$", "-test.count=1") // #nosec G204 -- 仅重启当前测试二进制。
	initializer.Env = append(
		os.Environ(),
		"TOOLKIT_WINDOWS_CRASH_INITIALIZER=1",
		"TOOLKIT_WINDOWS_CRASH_WORKSPACE="+workspacePath,
	)
	if output, err := initializer.CombinedOutput(); err != nil {
		t.Fatalf("run crashing initializer: %v, output=%s", err, output)
	}
	newWindowsTestSandbox(t, Config{Workspace: workspacePath, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
}

// TestWindows_CrashingInitializerPayload 初始化工作区后直接退出，不执行任何恢复逻辑。
func TestWindows_CrashingInitializerPayload(t *testing.T) {
	if os.Getenv("TOOLKIT_WINDOWS_CRASH_INITIALIZER") != "1" {
		return
	}
	if _, err := New(Config{
		Workspace:            os.Getenv("TOOLKIT_WINDOWS_CRASH_WORKSPACE"),
		RequiredCapabilities: UntrustedCodeIsolationCapabilities,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "initialize persistent Windows workspace: %v", err)
		os.Exit(2)
	}
	os.Exit(0)
}

func TestWindows_JobTotalMemoryEnforced(t *testing.T) {
	control := runWindowsJobMemoryScenario(t, 512*1024*1024, "control")
	if !strings.Contains(control.Stdout, "JOB_MEMORY_CONTROL_READY") {
		t.Fatalf("wide Job memory control did not start the payload: stdout=%q stderr=%q", control.Stdout, control.Stderr)
	}

	limited := runWindowsJobMemoryScenario(t, 160*1024*1024, "limit")
	if strings.Contains(limited.Stdout, "JOB_MEMORY_FIXTURE_FAILED") ||
		strings.Contains(limited.Stdout, "JOB_MEMORY_LIMIT_NOT_ENFORCED") {
		t.Fatalf("Job memory limit produced invalid evidence: stdout=%q stderr=%q", limited.Stdout, limited.Stderr)
	}
	if !strings.Contains(limited.Stdout, "JOB_MEMORY_LIMIT_ENFORCED:8") &&
		!strings.Contains(limited.Stdout, "JOB_MEMORY_LIMIT_ENFORCED:1455") {
		t.Fatalf("Job memory limit lacks a Windows allocation error: stdout=%q stderr=%q", limited.Stdout, limited.Stderr)
	}
	if limited.Limits.Memory != LimitStatusEnforced || limited.Limits.ProcessContainment != LimitStatusEnforced {
		t.Fatalf("Job memory report = %+v, want Memory and ProcessContainment enforced", limited.Limits)
	}
}

func TestWindows_JobProcessLimitEnforced(t *testing.T) {
	control := runWindowsJobProcessScenario(t, 4, "control")
	if !strings.Contains(control.Stdout, "JOB_PROCESS_CONTROL_READY") {
		t.Fatalf("wide Job process control did not start the payload: stdout=%q stderr=%q", control.Stdout, control.Stderr)
	}

	limited := runWindowsJobProcessScenario(t, 2, "limit")
	if strings.Contains(limited.Stdout, "JOB_PROCESS_FIXTURE_FAILED") ||
		strings.Contains(limited.Stdout, "JOB_PROCESS_LIMIT_NOT_ENFORCED") {
		t.Fatalf("Job process limit produced invalid evidence: stdout=%q stderr=%q", limited.Stdout, limited.Stderr)
	}
	wantStatus := fmt.Sprintf("JOB_PROCESS_LIMIT_ENFORCED:%d", uint32(windows.ERROR_NOT_ENOUGH_QUOTA))
	if !strings.Contains(limited.Stdout, wantStatus) {
		t.Fatalf("Job process limit lacks ERROR_NOT_ENOUGH_QUOTA: stdout=%q stderr=%q", limited.Stdout, limited.Stderr)
	}
	if limited.Limits.Processes != LimitStatusEnforced || limited.Limits.ProcessContainment != LimitStatusEnforced {
		t.Fatalf("Job process-limit report = %+v, want Processes and ProcessContainment enforced", limited.Limits)
	}
}

func runWindowsJobMemoryScenario(t *testing.T, memoryLimit int64, mode string) *ExecResult {
	t.Helper()
	workspacePath := t.TempDir()
	sandboxValue := newWindowsTestSandbox(t, Config{
		Workspace:            workspacePath,
		Timeout:              15,
		MaxMemoryBytes:       memoryLimit,
		MaxProcesses:         8,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities | CapabilityMemory | CapabilityProcesses,
	})
	payloadPath := installWindowsTestPayload(t, workspacePath, "sandbox-job-memory-payload.exe")
	result, execErr := sandboxValue.Exec(context.Background(), Command{
		Path: payloadPath,
		Args: []string{
			"-test.run=^TestWindows_JobMemoryRootPayload$",
			"-test.count=1",
			"--",
			mode,
			strconv.FormatInt(memoryLimit, 10),
		},
	})
	if execErr != nil {
		t.Fatalf("execute %s Job memory payload: %v", mode, execErr)
	}
	if result == nil || result.ExitCode != 0 {
		t.Fatalf("%s Job memory result = %+v, want exit code 0", mode, result)
	}
	return result
}

func runWindowsJobProcessScenario(t *testing.T, processLimit int, mode string) *ExecResult {
	t.Helper()
	workspacePath := t.TempDir()
	sandboxValue := newWindowsTestSandbox(t, Config{
		Workspace:            workspacePath,
		Timeout:              15,
		MaxProcesses:         processLimit,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities | CapabilityProcesses,
	})
	payloadPath := installWindowsTestPayload(t, workspacePath, "sandbox-job-process-payload.exe")
	result, execErr := sandboxValue.Exec(context.Background(), Command{
		Path: payloadPath,
		Args: []string{
			"-test.run=^TestWindows_JobProcessRootPayload$",
			"-test.count=1",
			"--",
			mode,
			strconv.Itoa(processLimit),
		},
	})
	if execErr != nil {
		t.Fatalf("execute %s Job process payload: %v", mode, execErr)
	}
	if result == nil || result.ExitCode != 0 {
		t.Fatalf("%s Job process result = %+v, want exit code 0", mode, result)
	}
	return result
}

// TestWindows_JobProcessRootPayload 先证明首个子进程可运行，再读取超额进程的精确退出状态。
func TestWindows_JobProcessRootPayload(t *testing.T) {
	if !strings.EqualFold(filepath.Base(os.Args[0]), "sandbox-job-process-payload.exe") {
		return
	}
	arguments := argumentsAfterDoubleDash(os.Args)
	if len(arguments) != 2 || arguments[0] != "control" && arguments[0] != "limit" {
		fmt.Print("JOB_PROCESS_FIXTURE_FAILED:mode")
		return
	}
	mode := arguments[0]
	expectedLimit, err := strconv.Atoi(arguments[1])
	if err != nil {
		fmt.Printf("JOB_PROCESS_FIXTURE_FAILED:limit:%v", err)
		return
	}
	limits, err := queryCurrentWindowsJobLimits()
	if err != nil {
		fmt.Printf("JOB_PROCESS_FIXTURE_FAILED:query-limits:%v", err)
		return
	}
	if limits.BasicLimitInformation.LimitFlags&jobObjectLimitActiveProcess == 0 ||
		limits.BasicLimitInformation.ActiveProcessLimit != uint32(expectedLimit) {
		fmt.Printf(
			"JOB_PROCESS_FIXTURE_FAILED:job-limit:flags=0x%x active=%d want=%d",
			limits.BasicLimitInformation.LimitFlags,
			limits.BasicLimitInformation.ActiveProcessLimit,
			expectedLimit,
		)
		return
	}
	firstReady := "process-child-first.ready"
	secondReady := "process-child-second.ready"
	first := exec.Command(
		os.Args[0],
		"-test.run=^TestWindows_JobProcessChildPayload$",
		"-test.count=1",
		"--",
		firstReady,
	) // #nosec G204 -- 仅重启工作区内已冻结的测试载荷。
	if err := first.Start(); err != nil {
		fmt.Printf("JOB_PROCESS_FIXTURE_FAILED:first-start:%v", err)
		return
	}
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- first.Wait()
		close(firstDone)
	}()
	if ready, waitErr := waitForWindowsPayloadOutcome(firstReady, firstDone, 5*time.Second); !ready {
		fmt.Printf("JOB_PROCESS_FIXTURE_FAILED:first-ready:%v", waitErr)
		killWindowsPayloadChildren([]*exec.Cmd{first}, []<-chan error{firstDone})
		return
	}

	second := exec.Command(
		os.Args[0],
		"-test.run=^TestWindows_JobProcessChildPayload$",
		"-test.count=1",
		"--",
		secondReady,
	) // #nosec G204 -- 仅重启工作区内已冻结的测试载荷。
	if err := second.Start(); err != nil {
		killWindowsPayloadChildren([]*exec.Cmd{first}, []<-chan error{firstDone})
		fmt.Printf("JOB_PROCESS_FIXTURE_FAILED:second-start:%v", err)
		return
	}
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- second.Wait()
		close(secondDone)
	}()
	secondWasReady, secondWaitErr := waitForWindowsPayloadOutcome(secondReady, secondDone, 5*time.Second)
	killWindowsPayloadChildren(
		[]*exec.Cmd{first, second},
		[]<-chan error{firstDone, secondDone},
	)
	if mode == "control" {
		if !secondWasReady {
			fmt.Printf("JOB_PROCESS_FIXTURE_FAILED:control-second:%v", secondWaitErr)
			return
		}
		fmt.Print("JOB_PROCESS_CONTROL_READY")
		return
	}
	if secondWasReady {
		fmt.Print("JOB_PROCESS_LIMIT_NOT_ENFORCED")
		return
	}
	var exitError *exec.ExitError
	if !errors.As(secondWaitErr, &exitError) || second.ProcessState == nil {
		fmt.Printf("JOB_PROCESS_FIXTURE_FAILED:second-wait:%v", secondWaitErr)
		return
	}
	exitStatus := uint32(second.ProcessState.ExitCode())
	if exitStatus != uint32(windows.ERROR_NOT_ENOUGH_QUOTA) {
		fmt.Printf("JOB_PROCESS_FIXTURE_FAILED:second-status:%d", exitStatus)
		return
	}
	fmt.Printf("JOB_PROCESS_LIMIT_ENFORCED:%d", exitStatus)
}

// TestWindows_JobProcessChildPayload 保持 Job 中的活动进程槽位。
func TestWindows_JobProcessChildPayload(t *testing.T) {
	if !strings.EqualFold(filepath.Base(os.Args[0]), "sandbox-job-process-payload.exe") {
		return
	}
	arguments := argumentsAfterDoubleDash(os.Args)
	if len(arguments) != 1 {
		return
	}
	if err := os.WriteFile(arguments[0], []byte("ready"), 0o600); err != nil {
		return
	}
	time.Sleep(30 * time.Second)
}

// TestWindows_JobMemoryRootPayload 使用同一载荷分别证明宽松对照和 Job 总内存拒绝。
func TestWindows_JobMemoryRootPayload(t *testing.T) {
	if !strings.EqualFold(filepath.Base(os.Args[0]), "sandbox-job-memory-payload.exe") {
		return
	}
	arguments := argumentsAfterDoubleDash(os.Args)
	if len(arguments) != 2 || arguments[0] != "control" && arguments[0] != "limit" {
		fmt.Print("JOB_MEMORY_FIXTURE_FAILED:mode")
		return
	}
	mode := arguments[0]
	expectedLimit, err := strconv.ParseUint(arguments[1], 10, 64)
	if err != nil {
		fmt.Printf("JOB_MEMORY_FIXTURE_FAILED:limit:%v", err)
		return
	}
	limits, err := queryCurrentWindowsJobLimits()
	if err != nil {
		fmt.Printf("JOB_MEMORY_FIXTURE_FAILED:query-limits:%v", err)
		return
	}
	memoryFlags := uint32(jobObjectLimitProcessMemory | jobObjectLimitJobMemory)
	if limits.BasicLimitInformation.LimitFlags&memoryFlags != memoryFlags ||
		uint64(limits.ProcessMemoryLimit) != expectedLimit ||
		uint64(limits.JobMemoryLimit) != expectedLimit {
		fmt.Printf(
			"JOB_MEMORY_FIXTURE_FAILED:job-limit:flags=0x%x process=%d job=%d want=%d",
			limits.BasicLimitInformation.LimitFlags,
			limits.ProcessMemoryLimit,
			limits.JobMemoryLimit,
			expectedLimit,
		)
		return
	}
	children := make([]*exec.Cmd, 0, 2)
	waits := make([]<-chan error, 0, 2)
	outcomes := make([]string, 0, 2)
	for index := 0; index < 2; index++ {
		resultPath := filepath.Join(".", fmt.Sprintf("memory-child-%d.result", index))
		child := exec.Command(
			os.Args[0],
			"-test.run=^TestWindows_JobMemoryChildPayload$",
			"-test.count=1",
			"--",
			resultPath,
		) // #nosec G204 -- 仅重启工作区内已冻结的测试载荷。
		if err := child.Start(); err != nil {
			killWindowsPayloadChildren(children, waits)
			fmt.Printf("JOB_MEMORY_FIXTURE_FAILED:child-%d-start:%v", index, err)
			return
		}
		waitDone := make(chan error, 1)
		go func() {
			waitDone <- child.Wait()
			close(waitDone)
		}()
		children = append(children, child)
		waits = append(waits, waitDone)
		if ready, waitErr := waitForWindowsPayloadOutcome(resultPath, waitDone, 5*time.Second); !ready {
			killWindowsPayloadChildren(children, waits)
			fmt.Printf("JOB_MEMORY_FIXTURE_FAILED:child-%d-result:%v", index, waitErr)
			return
		}
		outcome, err := os.ReadFile(resultPath)
		if err != nil {
			killWindowsPayloadChildren(children, waits)
			fmt.Printf("JOB_MEMORY_FIXTURE_FAILED:child-%d-read:%v", index, err)
			return
		}
		outcomes = append(outcomes, strings.TrimSpace(string(outcome)))
	}
	killWindowsPayloadChildren(children, waits)
	if mode == "control" {
		if outcomes[0] != "ready" || outcomes[1] != "ready" {
			fmt.Printf("JOB_MEMORY_FIXTURE_FAILED:control-outcomes:%v", outcomes)
			return
		}
		fmt.Print("JOB_MEMORY_CONTROL_READY")
		return
	}
	if outcomes[0] != "ready" {
		fmt.Printf("JOB_MEMORY_FIXTURE_FAILED:first-outcome:%s", outcomes[0])
		return
	}
	if outcomes[1] == "ready" {
		fmt.Print("JOB_MEMORY_LIMIT_NOT_ENFORCED")
		return
	}
	if outcomes[1] != "denied:8" && outcomes[1] != "denied:1455" {
		fmt.Printf("JOB_MEMORY_FIXTURE_FAILED:second-outcome:%s", outcomes[1])
		return
	}
	fmt.Printf("JOB_MEMORY_LIMIT_ENFORCED:%s", strings.TrimPrefix(outcomes[1], "denied:"))
}

// TestWindows_JobMemoryChildPayload 直接调用 VirtualAlloc，使失败证据绑定到 Windows 内存限额错误码。
func TestWindows_JobMemoryChildPayload(t *testing.T) {
	if !strings.EqualFold(filepath.Base(os.Args[0]), "sandbox-job-memory-payload.exe") {
		return
	}
	arguments := argumentsAfterDoubleDash(os.Args)
	if len(arguments) != 1 {
		return
	}
	const allocationBytes = uintptr(64 * 1024 * 1024)
	address, err := windows.VirtualAlloc(
		0,
		allocationBytes,
		windows.MEM_COMMIT|windows.MEM_RESERVE,
		windows.PAGE_READWRITE,
	)
	if err != nil {
		var errno syscall.Errno
		if !errors.As(err, &errno) ||
			errno != windows.ERROR_NOT_ENOUGH_MEMORY && errno != windows.ERROR_COMMITMENT_LIMIT {
			_ = os.WriteFile(arguments[0], []byte(fmt.Sprintf("fixture-error:%v", err)), 0o600)
			return
		}
		_ = os.WriteFile(arguments[0], []byte(fmt.Sprintf("denied:%d", uint32(errno))), 0o600)
		return
	}
	if err := os.WriteFile(arguments[0], []byte("ready"), 0o600); err != nil {
		_ = windows.VirtualFree(address, 0, windows.MEM_RELEASE)
		return
	}
	time.Sleep(30 * time.Second)
	_ = windows.VirtualFree(address, 0, windows.MEM_RELEASE)
}

func TestWindows_EnvironmentClean(t *testing.T) {
	const sentinel = "WINDOWS_HOST_ENV_SENTINEL_4B928DAE"
	t.Setenv("TOOLKIT_WINDOWS_HOST_SECRET", sentinel)
	workspacePath := t.TempDir()
	sandboxValue := newWindowsTestSandbox(t, Config{Workspace: workspacePath, Timeout: 10, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	payloadPath := installWindowsTestPayload(t, workspacePath, "sandbox-env-payload.exe")
	result, err := sandboxValue.Exec(context.Background(), Command{
		Path: payloadPath,
		Args: []string{"-test.run=^TestWindows_EnvironmentPayload$", "-test.count=1"},
	})
	if err != nil {
		t.Fatalf("execute environment payload: %v", err)
	}
	if strings.Contains(result.Stdout, sentinel) || strings.Contains(result.Stderr, sentinel) {
		t.Fatalf("host environment sentinel leaked: stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
	if !strings.Contains(result.Stdout, "HOST_ENV_CLEAN") {
		t.Fatalf("environment payload did not confirm a clean environment: %q", result.Stdout)
	}
}

// TestWindows_EnvironmentPayload 验证宿主环境哨兵没有被隐式继承。
func TestWindows_EnvironmentPayload(t *testing.T) {
	if !strings.EqualFold(filepath.Base(os.Args[0]), "sandbox-env-payload.exe") {
		return
	}
	if value := os.Getenv("TOOLKIT_WINDOWS_HOST_SECRET"); value != "" {
		fmt.Printf("HOST_ENV_LEAK:%s", value)
		return
	}
	fmt.Print("HOST_ENV_CLEAN")
}

func TestWindows_PathValidation(t *testing.T) {
	for _, path := range []string{`\\server\share\file`, `\\.\PhysicalDrive0`, `file.txt:hidden`} {
		if err := validateWindowsPath(path); err == nil {
			t.Errorf("dangerous Windows path %q was accepted", path)
		}
	}
}

func installWindowsTestPayload(t *testing.T, workspacePath, filename string) string {
	t.Helper()
	testExecutable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	payload, err := os.ReadFile(testExecutable)
	if err != nil {
		t.Fatalf("read test executable: %v", err)
	}
	payloadPath := filepath.Join(workspacePath, filename)
	if err := os.WriteFile(payloadPath, payload, 0o700); err != nil {
		t.Fatalf("write test payload: %v", err)
	}
	file, err := os.Open(payloadPath)
	if err != nil {
		t.Fatalf("open test payload: %v", err)
	}
	defer file.Close()
	canonicalPath, err := canonicalWindowsPathFromHandle(file)
	if err != nil {
		t.Fatalf("resolve test payload handle: %v", err)
	}
	return canonicalPath
}

func canonicalWindowsSystemExecutable(t *testing.T, filename string) string {
	t.Helper()
	systemDirectory, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatalf("resolve Windows system directory: %v", err)
	}
	file, err := os.Open(filepath.Join(systemDirectory, filename))
	if err != nil {
		t.Fatalf("open Windows system executable: %v", err)
	}
	defer file.Close()
	path, err := canonicalWindowsPathFromHandle(file)
	if err != nil {
		t.Fatalf("resolve Windows system executable: %v", err)
	}
	return path
}

func argumentsAfterDoubleDash(arguments []string) []string {
	for index, argument := range arguments {
		if argument == "--" {
			return arguments[index+1:]
		}
	}
	return nil
}

func waitForWindowsPayloadFile(path string, processDone <-chan error, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		select {
		case <-processDone:
			return false
		case <-ticker.C:
		case <-deadline.C:
			return false
		}
	}
}

func waitForWindowsPayloadOutcome(path string, processDone <-chan error, timeout time.Duration) (bool, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("inspect Windows payload outcome: %w", err)
		}
		select {
		case waitErr, ok := <-processDone:
			if !ok {
				return false, fmt.Errorf("Windows payload exited without a wait result")
			}
			return false, waitErr
		case <-ticker.C:
		case <-deadline.C:
			return false, fmt.Errorf("Windows payload outcome timed out after %s", timeout)
		}
	}
}

func queryCurrentWindowsJobLimits() (jobObjectExtendedLimitInformation, error) {
	var limits jobObjectExtendedLimitInformation
	if err := windows.QueryInformationJobObject(
		0,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), // #nosec G103 -- 结构体布局与 JOBOBJECT_EXTENDED_LIMIT_INFORMATION ABI 一致。
		uint32(unsafe.Sizeof(limits)),
		nil,
	); err != nil {
		return jobObjectExtendedLimitInformation{}, err
	}
	runtime.KeepAlive(&limits)
	return limits, nil
}

func killWindowsPayloadChildren(children []*exec.Cmd, waits []<-chan error) {
	for _, child := range children {
		if child != nil && child.Process != nil {
			_ = child.Process.Kill()
		}
	}
	for _, waitDone := range waits {
		select {
		case <-waitDone:
		case <-time.After(2 * time.Second):
		}
	}
}
