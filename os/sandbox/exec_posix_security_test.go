//go:build !windows

package sandbox

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

const posixSetsidPayloadEnv = "TOOLKIT_SANDBOX_TEST_SETSID_PAYLOAD"

func TestRunBoundedCommandSetsidRootCancellationReturnsWithinBound(t *testing.T) {
	workspace := t.TempDir()
	ready := filepath.Join(workspace, "ready")
	testExecutable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), posixSetsidPayloadEnv+"=1", "TOOLKIT_SANDBOX_TEST_READY="+ready)
	ctx, cancel := context.WithCancel(context.Background())
	type execution struct {
		result *ExecResult
		err    error
	}
	done := make(chan execution, 1)
	go func() {
		result, execErr := runBoundedCommandWithSysProcAttr(
			ctx,
			Command{
				Path: testExecutable,
				Args: []string{"-test.run=^TestPosixSetsidRootPayload$", "-test.count=1"},
				Dir:  workspace,
				Env:  env,
			},
			Config{Workspace: workspace, MaxOutputBytes: 1024, MaxStderrBytes: 1024},
			posixExecutionCapabilities{Filesystem: LimitStatusUnsupported, ProcessContainment: LimitStatusUnsupported},
			&syscall.SysProcAttr{Setsid: true},
		)
		done <- execution{result: result, err: execErr}
	}()
	waitForPosixFixture(t, ready, 5*time.Second)

	started := time.Now()
	cancel()
	select {
	case got := <-done:
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("runBoundedCommandWithSysProcAttr() error = %v, want context cancellation", got.err)
		}
		if got.result == nil || got.result.Limits.ProcessContainment != LimitStatusUnsupported {
			t.Fatalf("ProcessContainment report = %+v, want unsupported", got.result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("setsid root cancellation did not return within the termination bound")
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("setsid root cancellation took %s", elapsed)
	}
}

func TestTerminatePosixCommandFallsBackToDirectRootKill(t *testing.T) {
	testExecutable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(t.TempDir(), "ready")
	cmd := exec.CommandContext(context.Background(), testExecutable, "-test.run=^TestPosixSetsidRootPayload$", "-test.count=1") // #nosec G204 -- 测试只重启当前测试二进制。
	cmd.Env = append(os.Environ(), posixSetsidPayloadEnv+"=1", "TOOLKIT_SANDBOX_TEST_READY="+ready)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	reaped := false
	t.Cleanup(func() {
		if reaped {
			return
		}
		_ = cmd.Process.Kill()
		select {
		case <-waitDone:
		case <-time.After(2 * time.Second):
			t.Errorf("cleanup could not reap POSIX root pid %d", pid)
		}
	})
	waitForPosixFixture(t, ready, 5*time.Second)

	waitErr, terminationErr := terminatePosixCommand(cmd, waitDone, func(int) error {
		return syscall.ESRCH
	}, time.Second)
	if errors.Is(waitErr, ErrProcessReapTimeout) {
		t.Fatalf("direct root kill did not reap pid %d: %v", pid, waitErr)
	}
	if terminationErr != nil {
		t.Fatalf("idempotent process-group ESRCH produced diagnostics: %v", terminationErr)
	}
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		t.Fatalf("wait error = %v, want ExitError proving a completed Wait", waitErr)
	}
	waitStatus, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !waitStatus.Signaled() || waitStatus.Signal() != syscall.SIGKILL {
		t.Fatalf("wait status = %v, want SIGKILL", exitErr.Sys())
	}
	if cmd.ProcessState == nil || cmd.ProcessState.Pid() != pid {
		t.Fatalf("process state = %+v, want reaped pid %d", cmd.ProcessState, pid)
	}
	reaped = true
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("pid %d remains observable after Wait: %v", pid, err)
	}
}

func TestPosixTerminationLimitReportDowngradesContainmentAndPreservesErrors(t *testing.T) {
	killSentinel := errors.New("kill sentinel")
	waitSentinel := errors.New("wait sentinel")
	tests := []struct {
		name           string
		waitErr        error
		terminationErr error
		wantWait       bool
		wantKill       bool
	}{
		{name: "kill", terminationErr: killSentinel, wantKill: true},
		{name: "wait", waitErr: waitSentinel, wantWait: true},
		{name: "combined", waitErr: waitSentinel, terminationErr: killSentinel, wantWait: true, wantKill: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report, err := posixTerminationLimitReport(Config{MaxProcesses: 1}, posixExecutionCapabilities{
				Processes:          LimitStatusEnforced,
				ProcessContainment: LimitStatusEnforced,
			}, test.waitErr, test.terminationErr)
			if report.ProcessContainment != LimitStatusUnsupported {
				t.Fatalf("ProcessContainment status = %q, want unsupported", report.ProcessContainment)
			}
			if report.Processes != LimitStatusEnforced {
				t.Fatalf("Processes status = %q, want independent enforced status", report.Processes)
			}
			if test.wantWait && !errors.Is(err, waitSentinel) {
				t.Fatalf("termination error = %v, want wait sentinel", err)
			}
			if test.wantKill && !errors.Is(err, killSentinel) {
				t.Fatalf("termination error = %v, want kill sentinel", err)
			}
		})
	}
}

func TestPosixTerminationLimitReportKeepsContainmentAfterConfirmedReap(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/usr/bin/false")
	waitErr := command.Run()
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		t.Fatalf("fixture wait error = %v, want ExitError", waitErr)
	}
	report, err := posixTerminationLimitReport(
		Config{},
		posixExecutionCapabilities{ProcessContainment: LimitStatusEnforced},
		waitErr,
		nil,
	)
	if report.ProcessContainment != LimitStatusEnforced {
		t.Fatalf("ProcessContainment status = %q, want enforced after confirmed reap", report.ProcessContainment)
	}
	if !errors.Is(err, waitErr) {
		t.Fatalf("termination error = %v, want complete wait error chain", err)
	}
}

func TestPosixSetsidRootPayload(t *testing.T) {
	if os.Getenv(posixSetsidPayloadEnv) != "1" {
		t.Skip("setsid payload is inactive")
	}
	ready := os.Getenv("TOOLKIT_SANDBOX_TEST_READY")
	if err := os.WriteFile(ready, []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func waitForPosixFixture(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("fixture %q was not created within %s", path, timeout)
}
