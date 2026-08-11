//go:build !windows

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
)

const (
	posixOutputFixtureModeEnv = "TOOLKIT_SANDBOX_POSIX_OUTPUT_MODE"
	posixOutputFixtureReady   = "TOOLKIT_SANDBOX_POSIX_OUTPUT_READY"
	posixOutputFixturePID     = "TOOLKIT_SANDBOX_POSIX_OUTPUT_PID"
	posixPrestartMarkerEnv    = "TOOLKIT_SANDBOX_POSIX_PRESTART_MARKER"
)

func TestPOSIXDetachedDescendantOutputDoesNotOutliveReturn(t *testing.T) {
	result, execErr, childPID, elapsed := runPOSIXOutputFixture(t, false, true, 100*time.Millisecond)
	if result == nil {
		t.Fatal("canceled execution returned a nil result")
	}
	if !errors.Is(execErr, context.Canceled) || !errors.Is(execErr, ErrOutputDrainTimeout) {
		t.Fatalf("execution error = %v, want cancellation and output drain timeout", execErr)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("canceled execution took %s", elapsed)
	}
	waitForPOSIXPIDExit(t, childPID, 2*time.Second)
}

func TestPOSIXNormalRootExitWithInheritedOutputIsBounded(t *testing.T) {
	result, execErr, childPID, elapsed := runPOSIXOutputFixture(t, true, false, 100*time.Millisecond)
	if result == nil || result.ExitCode != 0 {
		t.Fatalf("execution result = %+v", result)
	}
	if !errors.Is(execErr, ErrOutputDrainTimeout) {
		t.Fatalf("execution error = %v, want ErrOutputDrainTimeout", execErr)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("normal root exit with inherited output took %s", elapsed)
	}
	waitForPOSIXPIDExit(t, childPID, 2*time.Second)
}

func TestPOSIXRootExitTerminatesSilentSameGroupDescendant(t *testing.T) {
	result, execErr, childPID, elapsed := runPOSIXFixtureMode(
		t,
		"root-exit-silent",
		false,
		100*time.Millisecond,
		posixExecutionCapabilities{ProcessContainment: LimitStatusEnforced},
	)
	if result == nil {
		t.Fatal("execution returned a nil result")
	}
	if !errors.Is(execErr, ErrProcessGroupSurvivedRoot) {
		t.Fatalf("execution error = %v, want ErrProcessGroupSurvivedRoot", execErr)
	}
	if errors.Is(execErr, ErrOutputDrainTimeout) || errors.Is(execErr, ErrProcessGroupSettlement) {
		t.Fatalf("silent descendant did not converge cleanly: %v", execErr)
	}
	if result.Limits.ProcessContainment != LimitStatusEnforced {
		t.Fatalf("ProcessContainment = %q, want enforced after verified settlement", result.Limits.ProcessContainment)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("silent descendant settlement took %s", elapsed)
	}
	waitForPOSIXPIDExit(t, childPID, 2*time.Second)
}

func TestPOSIXSetsidSilentDescendantCannotBeReportedContained(t *testing.T) {
	result, execErr, childPID, elapsed := runPOSIXFixtureMode(
		t,
		"root-exit-silent-setsid",
		false,
		100*time.Millisecond,
		posixExecutionCapabilities{ProcessContainment: LimitStatusUnsupported},
	)
	if execErr != nil {
		t.Fatalf("execution error = %v", execErr)
	}
	if result == nil || result.Limits.ProcessContainment != LimitStatusUnsupported {
		t.Fatalf("execution result = %+v, want unsupported containment", result)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("setsid silent descendant result took %s", elapsed)
	}
	if err := syscall.Kill(childPID, 0); err != nil {
		t.Fatalf("setsid fixture exited before the unsupported boundary was observed: %v", err)
	}
	if err := syscall.Kill(childPID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		t.Fatal(err)
	}
	waitForPOSIXPIDExit(t, childPID, 2*time.Second)
}

func TestPOSIXCancellationClosesOwnedOutputReadersAndDoesNotLeak(t *testing.T) {
	// 先预热测试二进制和 runtime poller，随后比较固定次数前后的稳定资源量。
	_, _, warmupPID, _ := runPOSIXOutputFixture(t, true, false, 20*time.Millisecond)
	waitForPOSIXPIDExit(t, warmupPID, 2*time.Second)
	runtime.GC()
	baselineFDs := countPOSIXTestFileDescriptors(t)
	baselineGoroutines := runtime.NumGoroutine()

	for range 8 {
		_, execErr, childPID, _ := runPOSIXOutputFixture(t, true, false, 20*time.Millisecond)
		if !errors.Is(execErr, ErrOutputDrainTimeout) {
			t.Fatalf("execution error = %v, want ErrOutputDrainTimeout", execErr)
		}
		waitForPOSIXPIDExit(t, childPID, 2*time.Second)
	}
	runtime.GC()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if countPOSIXTestFileDescriptors(t) <= baselineFDs+1 && runtime.NumGoroutine() <= baselineGoroutines+2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf(
		"repeated POSIX output settlement leaked resources: fds=%d baseline=%d goroutines=%d baseline=%d",
		countPOSIXTestFileDescriptors(t), baselineFDs, runtime.NumGoroutine(), baselineGoroutines,
	)
}

func TestBoundedBufferConcurrentSnapshot(t *testing.T) {
	buffer := newBoundedBuffer(1024)
	const writers = 8
	const writes = 1000
	var group sync.WaitGroup
	group.Add(writers * 2)
	for range writers {
		go func() {
			defer group.Done()
			for range writes {
				if _, err := buffer.Write([]byte("payload")); err != nil {
					t.Errorf("Write() error = %v", err)
					return
				}
			}
		}()
		go func() {
			defer group.Done()
			for range writes {
				snapshot := buffer.Snapshot()
				if int64(len(snapshot.Text)) > 1024 || snapshot.BytesSeen < int64(len(snapshot.Text)) {
					t.Errorf("inconsistent snapshot = %+v", snapshot)
					return
				}
			}
		}()
	}
	group.Wait()
	snapshot := buffer.Snapshot()
	wantBytes := int64(writers * writes * len("payload"))
	if snapshot.BytesSeen != wantBytes || !snapshot.Truncated || len(snapshot.Text) != 1024 {
		t.Fatalf("final snapshot = %+v, want bytes=%d truncated length=1024", snapshot, wantBytes)
	}
}

func TestBoundedBufferBytesSeenSaturatesAtMaxInt64(t *testing.T) {
	buffer := newBoundedBuffer(16)
	const maxInt64 = int64(^uint64(0) >> 1)
	buffer.total = maxInt64 - 1
	if n, err := buffer.Write([]byte("ab")); n != 2 || err != nil {
		t.Fatalf("Write() = (%d, %v), want (2, nil)", n, err)
	}
	snapshot := buffer.Snapshot()
	if snapshot.BytesSeen != maxInt64 || !snapshot.Truncated {
		t.Fatalf("overflow snapshot = %+v, want saturated and truncated", snapshot)
	}
	if _, err := buffer.Write([]byte("more")); err != nil {
		t.Fatal(err)
	}
	if got := buffer.Snapshot().BytesSeen; got != maxInt64 {
		t.Fatalf("saturated byte count = %d, want %d", got, maxInt64)
	}
}

func TestPOSIXPrestartCancellationCannotCreateMarker(t *testing.T) {
	workspace := t.TempDir()
	marker := filepath.Join(workspace, "payload-ran")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{})
	release := make(chan struct{})
	type outcome struct {
		result *ExecResult
		err    error
	}
	done := make(chan outcome, 1)
	var executions posixExecutionRegistry
	go func() {
		result, execErr := executions.runBoundedCommandWithOptions(
			ctx,
			Command{
				Path: executable,
				Args: []string{"-test.run=^TestPOSIXPrestartMarkerPayload$", "-test.count=1"},
				Dir:  workspace,
				Env:  append(os.Environ(), posixPrestartMarkerEnv+"="+marker),
			},
			Config{Workspace: workspace, MaxOutputBytes: 1024, MaxStderrBytes: 1024},
			posixExecutionCapabilities{},
			posixExecutionOptions{
				sysProcAttr: &syscall.SysProcAttr{Setpgid: true},
				beforeStart: func() {
					close(entered)
					<-release
				},
			},
		)
		done <- outcome{result: result, err: execErr}
	}()
	<-entered
	cancel()
	close(release)
	got := <-done
	if got.result != nil || !errors.Is(got.err, context.Canceled) {
		t.Fatalf("prestart cancellation = (%+v, %v), want nil result and context cancellation", got.result, got.err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("prestart-canceled payload created marker: %v", err)
	}
}

func TestPOSIXTerminationSettlementPreservesReapAndCopyErrors(t *testing.T) {
	copyErr := errors.New("copy sentinel")
	execution := &posixCommandExecution{
		stdoutReader: nil,
		stderrReader: nil,
		waitDone:     make(chan error),
		copyDone:     make(chan posixCopyResult, 2),
	}
	execution.copyDone <- posixCopyResult{stream: "stdout", err: copyErr}
	execution.copyDone <- posixCopyResult{stream: "stderr"}
	settlement := execution.settleAfterCancellation(10*time.Millisecond, time.Second)
	if !settlement.reapTimedOut || settlement.waitReceived {
		t.Fatalf("settlement = %+v, want an unreaped root", settlement)
	}
	if !errors.Is(settlement.err, ErrProcessReapTimeout) || !errors.Is(settlement.err, copyErr) {
		t.Fatalf("settlement error = %v, want reap and copy errors", settlement.err)
	}
}

func TestPOSIXRetainedExecutionRejectsNewWorkAndCloseRetries(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "while :; do sleep 60; done")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	reaped := false
	t.Cleanup(func() {
		if reaped {
			return
		}
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	waitDone := make(chan error, 1)
	registry := &posixExecutionRegistry{
		retained: []*posixRetainedExecution{{
			cmd:      cmd,
			waitDone: waitDone,
			killProcessGroup: func(pid int) error {
				return syscall.Kill(-pid, syscall.SIGKILL)
			},
		}},
	}
	if _, err := registry.begin(); !errors.Is(err, ErrPOSIXExecutionUnsettled) {
		t.Fatalf("begin() error = %v, want ErrPOSIXExecutionUnsettled", err)
	}
	if err := registry.Close(20 * time.Millisecond); !errors.Is(err, ErrProcessReapTimeout) {
		t.Fatalf("first Close() error = %v, want ErrProcessReapTimeout", err)
	}
	waitErr := cmd.Wait()
	reaped = true
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		t.Fatalf("Wait() error = %v, want ExitError", waitErr)
	}
	waitDone <- waitErr
	if err := registry.Close(time.Second); err != nil {
		t.Fatalf("retry Close() error = %v", err)
	}
	if _, err := registry.begin(); !errors.Is(err, ErrSandboxClosed) {
		t.Fatalf("begin() after Close error = %v, want ErrSandboxClosed", err)
	}
}

func TestSandboxTimeoutOverflowRejectedBeforeWorkspaceMutation(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "must-not-exist")
	_, err := New(Config{
		Workspace:            workspace,
		Timeout:              int(^uint(0) >> 1),
		RequiredCapabilities: UntrustedCodeIsolationCapabilities,
	})
	if err == nil || !strings.Contains(err.Error(), "duration limit") {
		t.Fatalf("New() error = %v, want duration limit rejection", err)
	}
	if _, statErr := os.Stat(workspace); !os.IsNotExist(statErr) {
		t.Fatalf("overflowing timeout mutated workspace: %v", statErr)
	}
}

func TestPOSIXInheritedOutputPayload(t *testing.T) {
	mode := os.Getenv(posixOutputFixtureModeEnv)
	if mode == "" {
		t.Skip("POSIX inherited-output payload is inactive")
	}
	if mode == "child" {
		payload := []byte(strings.Repeat("x", 4096) + "\n")
		for {
			if _, err := os.Stdout.Write(payload); err != nil {
				return
			}
			if _, err := os.Stderr.Write(payload); err != nil {
				return
			}
		}
	}
	if mode == "child-silent" {
		_ = os.Stdout.Close()
		_ = os.Stderr.Close()
		for {
			time.Sleep(time.Hour)
		}
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(executable, "-test.run=^TestPOSIXInheritedOutputPayload$", "-test.count=1") // #nosec G204 -- 测试仅重启当前二进制。
	childMode := "child"
	if strings.HasPrefix(mode, "root-exit-silent") {
		childMode = "child-silent"
	}
	child.Env = append(os.Environ(), posixOutputFixtureModeEnv+"="+childMode)
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if mode != "root-exit-silent" {
		child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv(posixOutputFixturePID), []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv(posixOutputFixtureReady), []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(mode, "root-exit") {
		return
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestPOSIXPrestartMarkerPayload(t *testing.T) {
	marker := os.Getenv(posixPrestartMarkerEnv)
	if marker == "" {
		t.Skip("POSIX prestart marker payload is inactive")
	}
	if err := os.WriteFile(marker, []byte("ran"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runPOSIXOutputFixture(t *testing.T, rootExits, cancelRoot bool, drainLimit time.Duration) (*ExecResult, error, int, time.Duration) {
	t.Helper()
	mode := "root-block"
	if rootExits {
		mode = "root-exit"
	}
	return runPOSIXFixtureMode(t, mode, cancelRoot, drainLimit, posixExecutionCapabilities{
		ProcessContainment: LimitStatusUnsupported,
	})
}

func runPOSIXFixtureMode(
	t *testing.T,
	mode string,
	cancelRoot bool,
	drainLimit time.Duration,
	capabilities posixExecutionCapabilities,
) (*ExecResult, error, int, time.Duration) {
	t.Helper()
	workspace := t.TempDir()
	ready := filepath.Join(workspace, "ready")
	pidFile := filepath.Join(workspace, "child.pid")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if cancelRoot {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()
	type outcome struct {
		result *ExecResult
		err    error
	}
	done := make(chan outcome, 1)
	var executions posixExecutionRegistry
	started := time.Now()
	go func() {
		result, execErr := executions.runBoundedCommandWithOptions(
			ctx,
			Command{
				Path: executable,
				Args: []string{"-test.run=^TestPOSIXInheritedOutputPayload$", "-test.count=1"},
				Dir:  workspace,
				Env: append(os.Environ(),
					posixOutputFixtureModeEnv+"="+mode,
					posixOutputFixtureReady+"="+ready,
					posixOutputFixturePID+"="+pidFile,
				),
			},
			Config{Workspace: workspace, MaxOutputBytes: 1024, MaxStderrBytes: 1024},
			capabilities,
			posixExecutionOptions{
				sysProcAttr:      &syscall.SysProcAttr{Setpgid: true},
				waitLimit:        time.Second,
				outputDrainLimit: drainLimit,
			},
		)
		done <- outcome{result: result, err: execErr}
	}()
	waitForPosixFixture(t, ready, 3*time.Second)
	childPID := readPOSIXFixturePID(t, pidFile)
	t.Cleanup(func() { _ = syscall.Kill(childPID, syscall.SIGKILL) })
	if cancelRoot {
		cancel()
	}
	select {
	case got := <-done:
		return got.result, got.err, childPID, time.Since(started)
	case <-time.After(3 * time.Second):
		t.Fatal("POSIX output fixture did not settle")
		return nil, nil, childPID, 0
	}
}

func readPOSIXFixturePID(t *testing.T, path string) int {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil || pid <= 0 {
		t.Fatalf("invalid fixture pid %q: %v", content, err)
	}
	return pid
}

func waitForPOSIXPIDExit(t *testing.T, pid int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("detached output fixture pid %d survived closed readers", pid)
}

func countPOSIXTestFileDescriptors(t *testing.T) int {
	t.Helper()
	directory, err := os.Open("/dev/fd")
	if err != nil {
		t.Fatalf("open /dev/fd: %v", err)
	}
	defer directory.Close()
	entries, err := directory.Readdirnames(-1)
	if err != nil {
		t.Fatalf("inspect /dev/fd: %v", err)
	}
	return len(entries)
}

func ExampleErrOutputDrainTimeout() {
	fmt.Println(ErrOutputDrainTimeout)
	// Output: sandbox: output drain timed out
}
