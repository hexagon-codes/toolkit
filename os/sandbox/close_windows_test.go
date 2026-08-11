//go:build windows

package sandbox

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestWindowsSandboxCloseReleasesPersistentWorkspaceHandles(t *testing.T) {
	sandboxInstance, err := New(Config{
		Workspace:            t.TempDir(),
		Network:              NetworkDisabled,
		RequiredCapabilities: CapabilityFilesystem | CapabilityOutput,
	})
	if err != nil {
		t.Fatal(err)
	}
	registerWindowsTestSandboxCleanup(t, sandboxInstance)
	backend := windowsBackendForTest(t, sandboxInstance)
	root := backend.workspace.root
	rootGuard := backend.workspace.rootGuard

	if err := sandboxInstance.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := root.Open("."); err == nil {
		t.Fatal("os.Root remained usable after Close")
	}
	if _, err := rootGuard.Stat(); err == nil {
		t.Fatal("root guard remained usable after Close")
	}
	if err := sandboxInstance.Close(); err != nil {
		t.Fatalf("repeated Close() error = %v", err)
	}
	if _, err := sandboxInstance.Exec(context.Background(), Command{}); !errors.Is(err, ErrSandboxClosed) {
		t.Fatalf("Exec() after Close error = %v, want ErrSandboxClosed", err)
	}
}

func TestWindowsWorkspaceClosePreservesEveryHandleError(t *testing.T) {
	workspace, err := prepareWindowsWorkspace(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if closeErr := workspace.root.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if closeErr := workspace.rootGuard.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	err = workspace.close()
	if !errors.Is(err, os.ErrClosed) {
		t.Fatalf("workspace close error = %v, want os.ErrClosed", err)
	}
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("workspace close error type = %T, want joined error", err)
	}
	// os.Root.Close 幂等（二次调用返回 nil），只有 rootGuard 的二次关闭报 ErrClosed。
	if got := len(joined.Unwrap()); got != 1 {
		t.Fatalf("workspace close error count = %d, want 1", got)
	}
}

func TestWindowsSandboxCloseRetriesAndReclaimsQuarantinedLaunch(t *testing.T) {
	sandboxInstance, err := New(Config{
		Workspace:            t.TempDir(),
		Network:              NetworkDisabled,
		RequiredCapabilities: CapabilityFilesystem | CapabilityOutput,
	})
	if err != nil {
		t.Fatal(err)
	}
	registerWindowsTestSandboxCleanup(t, sandboxInstance)
	backend := windowsBackendForTest(t, sandboxInstance)
	attempts := 0
	lifecycle := newWindowsRetainedLifecycle(func(timeout time.Duration) (bool, error) {
		if timeout != windowsJobTerminationLimit {
			t.Errorf("quarantine timeout = %s, want %s", timeout, windowsJobTerminationLimit)
		}
		attempts++
		return attempts == windowsQuarantineReclaimAttempts, nil
	})
	if err := lifecycle.retain(backend.quarantine); err != nil {
		t.Fatal(err)
	}

	if err := sandboxInstance.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if attempts != windowsQuarantineReclaimAttempts {
		t.Fatalf("quarantine attempts = %d, want %d", attempts, windowsQuarantineReclaimAttempts)
	}
	if backend.quarantine.count() != 0 {
		t.Fatalf("quarantine count = %d, want 0", backend.quarantine.count())
	}
	if backend.workspace != nil {
		t.Fatal("workspace remained owned after quarantine reclamation")
	}
	if err := sandboxInstance.Close(); err != nil {
		t.Fatalf("repeated Close() error = %v", err)
	}
	if attempts != windowsQuarantineReclaimAttempts {
		t.Fatalf("repeated Close retried quarantine: %d", attempts)
	}
}

func TestWindowsSandboxCloseKeepsWorkspaceProtectedUntilRetriedReclaim(t *testing.T) {
	sandboxInstance := newWindowsTestSandbox(t, Config{
		Workspace:            t.TempDir(),
		Network:              NetworkDisabled,
		RequiredCapabilities: CapabilityFilesystem | CapabilityOutput,
	})
	backend := windowsBackendForTest(t, sandboxInstance)
	attempts := 0
	lifecycle := newWindowsRetainedLifecycle(func(time.Duration) (bool, error) {
		attempts++
		return attempts > windowsQuarantineReclaimAttempts, nil
	})
	if err := lifecycle.retain(backend.quarantine); err != nil {
		t.Fatal(err)
	}

	firstErr := sandboxInstance.Close()
	if !errors.Is(firstErr, errWindowsProcessLifecycleUnconfirmed) {
		t.Fatalf("first Close() error = %v, want unconfirmed lifecycle", firstErr)
	}
	if backend.workspace == nil {
		t.Fatal("workspace was released before the retained lifecycle was confirmed")
	}
	if _, err := backend.workspace.rootGuard.Stat(); err != nil {
		t.Fatalf("workspace root guard was not preserved after failed Close(): %v", err)
	}

	if err := sandboxInstance.Close(); err != nil {
		t.Fatalf("retried Close() error = %v", err)
	}
	if attempts != windowsQuarantineReclaimAttempts+1 {
		t.Fatalf("reclaim attempts = %d, want %d", attempts, windowsQuarantineReclaimAttempts+1)
	}
	if backend.workspace != nil {
		t.Fatal("workspace remained owned after retried lifecycle reclamation")
	}
}
