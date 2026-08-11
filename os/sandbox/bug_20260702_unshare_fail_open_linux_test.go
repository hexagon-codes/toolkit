//go:build linux

package sandbox

import (
	"errors"
	"strings"
	"testing"
)

func TestLinuxRunnerFailsClosedWithoutBubblewrap(t *testing.T) {
	sandboxInstance := &linuxSandbox{
		cfg: Config{Workspace: t.TempDir()},
		resolveBwrap: func() (string, error) {
			return "", errors.New("bubblewrap unavailable")
		},
	}

	execution, err := sandboxInstance.planLinuxSandboxExecution(Command{Path: "/usr/bin/true", Dir: sandboxInstance.cfg.Workspace})
	if err == nil || !strings.Contains(err.Error(), "requires usable bubblewrap") {
		t.Fatalf("linuxSandboxRunner() error = %v, want bubblewrap fail-closed error", err)
	}
	if !errors.Is(err, ErrFilesystemContainmentUnavailable) {
		t.Fatalf("linuxSandboxRunner() error = %v, want filesystem containment sentinel", err)
	}
	if execution.capabilities.Filesystem != "" || execution.capabilities.ProcessContainment != "" {
		t.Fatalf("planLinuxSandboxExecution() capabilities = %+v, want empty on rejection", execution.capabilities)
	}
}

func TestLinuxRunnerReportsEnforcedWithBubblewrap(t *testing.T) {
	bwrap, err := linuxBwrapPath()
	if err != nil || !linuxBwrapBackendUsable(bwrap, false) {
		t.Skip("usable bubblewrap is unavailable")
	}
	sandboxInstance := &linuxSandbox{cfg: Config{Workspace: t.TempDir()}}

	env, err := cleanLinuxEnv(sandboxInstance.cfg.Workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := sandboxInstance.planLinuxSandboxExecution(Command{
		Path: "/usr/bin/true",
		Dir:  sandboxInstance.cfg.Workspace,
		Env:  env,
	})
	if err != nil {
		t.Fatalf("linuxSandboxRunner() error = %v", err)
	}
	if execution.command.Path != bwrap {
		t.Fatalf("planLinuxSandboxExecution() runner = %q, want %q", execution.command.Path, bwrap)
	}
	if execution.capabilities.Filesystem != LimitStatusEnforced || execution.capabilities.ProcessContainment != LimitStatusEnforced {
		t.Fatalf("planLinuxSandboxExecution() capabilities = %+v, want enforced", execution.capabilities)
	}
	if execution.capabilities.Processes != LimitStatusUnsupported {
		t.Fatalf("planLinuxSandboxExecution() Processes = %q, want unsupported tree budget", execution.capabilities.Processes)
	}
}
