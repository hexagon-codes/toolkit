//go:build windows && windows_security

package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestWindowsNativePostCreationFaultOwnership(t *testing.T) {
	for _, stage := range allWindowsLaunchStages() {
		stage := stage
		t.Run(stage.String(), func(t *testing.T) {
			workspace := t.TempDir()
			sandboxValue := newWindowsTestSandbox(t, Config{
				Workspace:            workspace,
				Network:              NetworkDisabled,
				Timeout:              15,
				RequiredCapabilities: UntrustedCodeIsolationCapabilities,
			})
			backend := windowsBackendForTest(t, sandboxValue)
			payload := installWindowsTestPayload(t, workspace, "sandbox-launch-fault-payload.exe")
			marker := filepath.Join(workspace, "payload.marker")
			injected := errors.New("injected Windows post-creation failure")
			injectionCalls := 0
			backend.launchOps = windowsLaunchOpsWithStageFailure(
				backend.launchOps,
				stage,
				injected,
				&injectionCalls,
			)

			_, err := sandboxValue.Exec(context.Background(), Command{
				Path: payload,
				Args: []string{
					"-test.run=^TestWindowsLaunchFaultPayload$",
					"-test.count=1",
					"--",
					marker,
				},
			})
			if !errors.Is(err, injected) {
				t.Fatalf("Exec() error = %v, want injected failure", err)
			}
			if injectionCalls != 1 {
				t.Fatalf("injected operation calls = %d, want 1", injectionCalls)
			}
			if got := backend.quarantine.count(); got != 0 {
				t.Fatalf("quarantine count = %d, want synchronous reclamation", got)
			}
			if stage < windowsLaunchStageCloseLaunchHandles {
				if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("suspended payload marker error = %v, want os.ErrNotExist", statErr)
				}
			}
		})
	}
}

func TestWindowsNativePostCreationFaultCloseRetriesQuarantine(t *testing.T) {
	for _, stage := range allWindowsLaunchStages() {
		stage := stage
		t.Run(stage.String(), func(t *testing.T) {
			workspace := t.TempDir()
			sandboxValue := newWindowsTestSandbox(t, Config{
				Workspace:            workspace,
				Network:              NetworkDisabled,
				Timeout:              15,
				RequiredCapabilities: UntrustedCodeIsolationCapabilities,
			})
			backend := windowsBackendForTest(t, sandboxValue)
			payload := installWindowsTestPayload(t, workspace, "sandbox-launch-retained-payload.exe")
			marker := filepath.Join(workspace, "retained.marker")
			injected := errors.New("injected Windows post-creation failure")
			settlementFailure := errors.New("injected Windows lifecycle settlement failure")
			injectionCalls := 0
			ops := windowsLaunchOpsWithStageFailure(
				backend.launchOps,
				stage,
				injected,
				&injectionCalls,
			)
			settlementCalls := 0
			defaultSettlement := ops.settleFailure
			ops.settleFailure = func(
				assignedToJob bool,
				process, job *ownedWindowsResource[syscall.Handle],
				timeout time.Duration,
			) (bool, error) {
				settlementCalls++
				if settlementCalls <= windowsQuarantineReclaimAttempts+1 {
					return false, settlementFailure
				}
				return defaultSettlement(assignedToJob, process, job, timeout)
			}
			backend.launchOps = ops

			_, execErr := sandboxValue.Exec(context.Background(), Command{
				Path: payload,
				Args: []string{
					"-test.run=^TestWindowsLaunchFaultPayload$",
					"-test.count=1",
					"--",
					marker,
				},
			})
			for _, want := range []error{
				injected,
				settlementFailure,
				errWindowsProcessLifecycleUnconfirmed,
			} {
				if !errors.Is(execErr, want) {
					t.Fatalf("Exec() error = %v, want %v", execErr, want)
				}
			}
			if injectionCalls != 1 || backend.quarantine.count() != 1 {
				t.Fatalf(
					"injection calls/quarantine count = %d/%d, want 1/1",
					injectionCalls,
					backend.quarantine.count(),
				)
			}

			firstCloseErr := sandboxValue.Close()
			if !errors.Is(firstCloseErr, errWindowsProcessLifecycleUnconfirmed) {
				t.Fatalf("first Close() error = %v, want lifecycle error", firstCloseErr)
			}
			if backend.quarantine.count() != 1 || backend.workspace == nil {
				t.Fatalf(
					"first Close() quarantine/workspace = %d/%t, want 1/preserved",
					backend.quarantine.count(),
					backend.workspace != nil,
				)
			}

			if closeErr := sandboxValue.Close(); closeErr != nil {
				t.Fatalf("retried Close() error = %v", closeErr)
			}
			if backend.quarantine.count() != 0 || backend.workspace != nil {
				t.Fatalf(
					"retried Close() quarantine/workspace = %d/%t, want 0/released",
					backend.quarantine.count(),
					backend.workspace != nil,
				)
			}
			if settlementCalls != windowsQuarantineReclaimAttempts+2 {
				t.Fatalf(
					"settlement calls = %d, want %d",
					settlementCalls,
					windowsQuarantineReclaimAttempts+2,
				)
			}
		})
	}
}

func TestWindowsNativeRetainExecutionOwnership(t *testing.T) {
	t.Run("normal-wait", func(t *testing.T) {
		workspace := t.TempDir()
		sandboxValue := newWindowsTestSandbox(t, Config{
			Workspace:            workspace,
			Network:              NetworkDisabled,
			Timeout:              15,
			RequiredCapabilities: UntrustedCodeIsolationCapabilities,
		})
		backend := windowsBackendForTest(t, sandboxValue)
		payload := installWindowsTestPayload(t, workspace, "sandbox-normal-wait-retention.exe")
		waitFailure := errors.New("injected normal wait failure")
		reclaimCalls := 0
		process := newWindowsRetentionTestProcess(func() windowsExecutionWaitResult {
			return windowsExecutionWaitResult{err: waitFailure}
		}, &reclaimCalls)
		backend.launcher = func(
			Config,
			*windowsWorkspace,
			*windowsExecutablePlan,
			*windowsDirectoryPlan,
			[]string,
			[]string,
			*windowsProcessQuarantine,
			windowsLaunchOps,
		) (*windowsSandboxedProcess, error) {
			return process, nil
		}

		execDone := make(chan error, 1)
		go func() {
			_, err := sandboxValue.Exec(context.Background(), Command{Path: payload})
			execDone <- err
		}()
		var err error
		select {
		case err = <-execDone:
		case <-time.After(5 * time.Second):
			t.Fatal("normal Windows execution did not return")
		}
		assertWindowsExecutionWasRetained(t, backend, reclaimCalls, err, waitFailure)
		if closeErr := sandboxValue.Close(); closeErr != nil {
			t.Fatalf("Close() retained normal wait: %v", closeErr)
		}
		if reclaimCalls != 1 || backend.quarantine.count() != 0 {
			t.Fatalf("reclaim calls/count = %d/%d, want 1/0", reclaimCalls, backend.quarantine.count())
		}
	})

	t.Run("timeout-cancel", func(t *testing.T) {
		workspace := t.TempDir()
		sandboxValue := newWindowsTestSandbox(t, Config{
			Workspace:            workspace,
			Network:              NetworkDisabled,
			Timeout:              15,
			RequiredCapabilities: UntrustedCodeIsolationCapabilities,
		})
		backend := windowsBackendForTest(t, sandboxValue)
		backend.launchOps.settlementLimit = time.Millisecond
		payload := installWindowsTestPayload(t, workspace, "sandbox-cancel-retention.exe")
		waitRelease := make(chan struct{})
		reclaimCalls := 0
		process := newWindowsRetentionTestProcess(func() windowsExecutionWaitResult {
			<-waitRelease
			return windowsExecutionWaitResult{}
		}, &reclaimCalls)
		launched := make(chan struct{})
		backend.launcher = func(
			Config,
			*windowsWorkspace,
			*windowsExecutablePlan,
			*windowsDirectoryPlan,
			[]string,
			[]string,
			*windowsProcessQuarantine,
			windowsLaunchOps,
		) (*windowsSandboxedProcess, error) {
			close(launched)
			return process, nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var releaseWait sync.Once
		releaseCompletion := func() {
			releaseWait.Do(func() { close(waitRelease) })
		}
		t.Cleanup(releaseCompletion)
		execDone := make(chan error, 1)
		go func() {
			_, err := sandboxValue.Exec(ctx, Command{Path: payload})
			execDone <- err
		}()
		select {
		case <-launched:
		case <-time.After(5 * time.Second):
			t.Fatal("injected Windows launcher did not start")
		}
		cancel()
		var err error
		select {
		case err = <-execDone:
		case <-time.After(5 * time.Second):
			t.Fatal("canceled Windows execution did not return")
		}
		releaseCompletion()
		assertWindowsExecutionWasRetained(t, backend, reclaimCalls, err, context.Canceled)
		if closeErr := sandboxValue.Close(); closeErr != nil {
			t.Fatalf("Close() retained canceled wait: %v", closeErr)
		}
		if reclaimCalls != 1 || backend.quarantine.count() != 0 {
			t.Fatalf("reclaim calls/count = %d/%d, want 1/0", reclaimCalls, backend.quarantine.count())
		}
	})
}

func newWindowsRetentionTestProcess(
	wait func() windowsExecutionWaitResult,
	reclaimCalls *int,
) *windowsSandboxedProcess {
	process := &windowsSandboxedProcess{
		stdout:     newWindowsOutputBuffer(1024),
		stderr:     newWindowsOutputBuffer(1024),
		completion: newWindowsExecutionCompletion(),
	}
	process.completion.start(wait)
	process.retention = newWindowsRetainedLifecycle(func(time.Duration) (bool, error) {
		*reclaimCalls = *reclaimCalls + 1
		return true, nil
	})
	return process
}

func assertWindowsExecutionWasRetained(
	t *testing.T,
	backend *windowsSandbox,
	reclaimCalls int,
	err error,
	pathErr error,
) {
	t.Helper()
	for _, want := range []error{
		pathErr,
		errWindowsProcessLifecycleUnconfirmed,
		errWindowsProcessContainmentUnconfirmed,
	} {
		if !errors.Is(err, want) {
			t.Fatalf("Exec() error = %v, want %v", err, want)
		}
	}
	if backend.poisoned == nil || backend.quarantine.count() != 1 || reclaimCalls != 0 {
		t.Fatalf(
			"poisoned/quarantine/reclaim = %t/%d/%d, want true/1/0",
			backend.poisoned != nil,
			backend.quarantine.count(),
			reclaimCalls,
		)
	}
}

func TestWindowsLaunchFaultPayload(t *testing.T) {
	if filepath.Base(os.Args[0]) != "sandbox-launch-fault-payload.exe" &&
		filepath.Base(os.Args[0]) != "sandbox-launch-retained-payload.exe" {
		return
	}
	arguments := argumentsAfterDoubleDash(os.Args)
	if len(arguments) != 1 {
		return
	}
	if err := os.WriteFile(arguments[0], []byte("started"), 0o600); err != nil {
		t.Fatalf("write payload marker: %v", err)
	}
}

func windowsLaunchOpsWithStageFailure(
	ops windowsLaunchOps,
	stage windowsLaunchStage,
	injected error,
	injectionCalls *int,
) windowsLaunchOps {
	var mu sync.Mutex
	failOnce := func() error {
		mu.Lock()
		defer mu.Unlock()
		if *injectionCalls != 0 {
			return nil
		}
		*injectionCalls = *injectionCalls + 1
		return injected
	}

	switch stage {
	case windowsLaunchStageAssignJob:
		ops.assignProcessToJob = func(syscall.Handle, syscall.Handle) error {
			return failOnce()
		}
	case windowsLaunchStageCloseCreationTokens:
		defaultClose := ops.closeToken
		ops.closeToken = func(token syscall.Token) error {
			if err := failOnce(); err != nil {
				return err
			}
			return defaultClose(token)
		}
	case windowsLaunchStageCloseInheritedPipes:
		defaultClose := ops.closeHandle
		ops.closeHandle = func(handle syscall.Handle) error {
			if err := failOnce(); err != nil {
				return err
			}
			return defaultClose(handle)
		}
	case windowsLaunchStageFindProcess:
		ops.findProcess = func(int) (*os.Process, error) {
			return nil, failOnce()
		}
	case windowsLaunchStageResumeThread:
		ops.resumeThread = func(syscall.Handle) error {
			return failOnce()
		}
	case windowsLaunchStageCloseLaunchHandles:
		defaultClose := ops.closeHandle
		closeCalls := 0
		ops.closeHandle = func(handle syscall.Handle) error {
			closeCalls++
			if closeCalls == 4 {
				return failOnce()
			}
			return defaultClose(handle)
		}
	}
	return ops
}
