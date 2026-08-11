//go:build darwin

package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwinTrustedBuildProfileAllowsChildrenWithoutClaimingContainment(t *testing.T) {
	backend := newDarwinSandbox(Config{
		Workspace:            t.TempDir(),
		ExecutionProfile:     ExecutionProfileTrustedBuild,
		RequiredCapabilities: TrustedBuildIsolationCapabilities,
	})
	profile := backend.generateSBPL()
	if !strings.Contains(profile, "(allow process-fork)") || strings.Contains(profile, "(deny process-fork)") {
		t.Fatalf("trusted build profile does not allow child processes:\n%s", profile)
	}
	available, err := backend.sandboxCapabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !available.Has(CapabilityProcessCreation) {
		t.Fatalf("trusted build capabilities = %s, want process-creation", available)
	}
	if available.Has(CapabilityProcessContainment) {
		t.Fatalf("trusted build capabilities = %s, must not claim process-containment", available)
	}
}

func TestDarwinTrustedBuildRunsFrozenGoCompilerChild(t *testing.T) {
	goBinary := requireDarwinRuntime(t, "go")
	sandboxInstance, err := New(Config{
		Workspace:            t.TempDir(),
		ExecutionProfile:     ExecutionProfileTrustedBuild,
		RequiredCapabilities: TrustedBuildIsolationCapabilities,
		Timeout:              30,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := sandboxInstance.Close(); closeErr != nil {
			t.Errorf("Close() error = %v", closeErr)
		}
	})

	result, err := sandboxInstance.Exec(context.Background(), Command{
		Path: goBinary,
		Args: []string{"tool", "compile", "-V=full"},
	})
	if err != nil {
		t.Fatalf("trusted Go compiler command failed: %v", err)
	}
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, "compile version") {
		t.Fatalf("trusted Go compiler result: exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
	if result.Limits.ProcessContainment != LimitStatusUnsupported {
		t.Fatalf("trusted build ProcessContainment = %q, want unsupported", result.Limits.ProcessContainment)
	}
}

func TestDarwinRejectsProcessCreationAndContainmentBeforePayload(t *testing.T) {
	workspace := t.TempDir()
	marker := filepath.Join(workspace, "must-not-exist")
	sandboxInstance, err := New(Config{
		Workspace:        workspace,
		ExecutionProfile: ExecutionProfileTrustedBuild,
		RequiredCapabilities: TrustedBuildIsolationCapabilities |
			CapabilityProcessContainment,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sandboxInstance.Close() })

	_, err = sandboxInstance.Exec(context.Background(), Command{
		Path: "/usr/bin/touch",
		Args: []string{marker},
	})
	if !errors.Is(err, ErrRequiredCapabilitiesUnavailable) {
		t.Fatalf("Exec() error = %v, want ErrRequiredCapabilitiesUnavailable", err)
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("payload marker state = %v, want not created", statErr)
	}
}

func TestDarwinRequiredCapabilitiesCannotSelectExecutionProfile(t *testing.T) {
	untrusted := newDarwinSandbox(Config{
		Workspace:        t.TempDir(),
		ExecutionProfile: ExecutionProfileUntrusted,
		RequiredCapabilities: TrustedBuildIsolationCapabilities |
			CapabilityProcessContainment,
	})
	untrustedPolicy := untrusted.generateSBPL()
	if !strings.Contains(untrustedPolicy, "(deny process-fork)") || strings.Contains(untrustedPolicy, "(allow process-fork)") {
		t.Fatalf("untrusted profile was changed by required capabilities:\n%s", untrustedPolicy)
	}
	untrustedCapabilities, err := untrusted.sandboxCapabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if untrustedCapabilities.Has(CapabilityProcessCreation) ||
		!untrustedCapabilities.Has(CapabilityProcessContainment) {
		t.Fatalf("untrusted capabilities = %s, want containment without process creation", untrustedCapabilities)
	}

	trusted := newDarwinSandbox(Config{
		Workspace:            t.TempDir(),
		ExecutionProfile:     ExecutionProfileTrustedBuild,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities | CapabilityProcessCreation,
	})
	trustedPolicy := trusted.generateSBPL()
	if !strings.Contains(trustedPolicy, "(allow process-fork)") || strings.Contains(trustedPolicy, "(deny process-fork)") {
		t.Fatalf("trusted profile was changed by required capabilities:\n%s", trustedPolicy)
	}
}

func TestDarwinProcessCreationRespectsProfileAndTotalBudget(t *testing.T) {
	tests := []struct {
		name      string
		profile   ExecutionProfile
		processes int
		want      bool
	}{
		{name: "untrusted unlimited", profile: ExecutionProfileUntrusted, want: false},
		{name: "untrusted budget two", profile: ExecutionProfileUntrusted, processes: 2, want: false},
		{name: "trusted unlimited", profile: ExecutionProfileTrustedBuild, want: true},
		{name: "trusted budget one", profile: ExecutionProfileTrustedBuild, processes: 1, want: false},
		{name: "trusted budget two", profile: ExecutionProfileTrustedBuild, processes: 2, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newDarwinSandbox(Config{
				Workspace:        t.TempDir(),
				ExecutionProfile: test.profile,
				MaxProcesses:     test.processes,
			})
			available, err := backend.sandboxCapabilities(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if got := available.Has(CapabilityProcessCreation); got != test.want {
				t.Fatalf("CapabilityProcessCreation = %t, want %t; capabilities=%s", got, test.want, available)
			}
		})
	}
}
