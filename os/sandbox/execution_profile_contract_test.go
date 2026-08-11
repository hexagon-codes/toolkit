package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecutionProfileContract(t *testing.T) {
	if got := ExecutionProfileUntrusted.String(); got != "untrusted" {
		t.Fatalf("ExecutionProfileUntrusted.String() = %q, want untrusted", got)
	}
	if got := ExecutionProfileTrustedBuild.String(); got != "trusted-build" {
		t.Fatalf("ExecutionProfileTrustedBuild.String() = %q, want trusted-build", got)
	}
	if ExecutionProfileUntrusted != 0 {
		t.Fatalf("ExecutionProfileUntrusted = %d, want zero-value safe default", ExecutionProfileUntrusted)
	}
}

func TestExecutionProfileValidationPrecedesWorkspaceMutation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		message string
	}{
		{
			name: "unknown profile",
			config: Config{
				ExecutionProfile:     ExecutionProfile(255),
				RequiredCapabilities: UntrustedCodeIsolationCapabilities,
			},
			message: "execution profile",
		},
		{
			name: "trusted build cannot create a child within a one-process budget",
			config: Config{
				ExecutionProfile: ExecutionProfileTrustedBuild,
				MaxProcesses:     1,
				RequiredCapabilities: TrustedBuildIsolationCapabilities |
					CapabilityProcesses,
			},
			message: "at least 2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := filepath.Join(t.TempDir(), "workspace")
			config := test.config
			config.Workspace = workspace
			_, err := New(config)
			if !errors.Is(err, ErrInvalidCapabilityContract) {
				t.Fatalf("New() error = %v, want ErrInvalidCapabilityContract", err)
			}
			if !strings.Contains(err.Error(), test.message) {
				t.Fatalf("New() error = %q, want %q", err, test.message)
			}
			if _, statErr := os.Stat(workspace); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("workspace state after rejected profile = %v, want not created", statErr)
			}
		})
	}
}

func TestExecutionProfileAddsMandatoryProcessCapability(t *testing.T) {
	tests := []struct {
		name      string
		profile   ExecutionProfile
		available CapabilitySet
		missing   CapabilitySet
	}{
		{
			name:      "untrusted requires containment",
			profile:   ExecutionProfileUntrusted,
			available: CapabilityFilesystem | CapabilityOutput,
			missing:   CapabilityProcessContainment,
		},
		{
			name:      "trusted build requires process creation",
			profile:   ExecutionProfileTrustedBuild,
			available: CapabilityFilesystem | CapabilityOutput,
			missing:   CapabilityProcessCreation,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace, identity, err := snapshotSandboxWorkspace(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			backend := &capabilityRecordingBackend{available: test.available}
			sandboxInstance := &capabilitySandbox{
				backend: backend,
				cfg: Config{
					Workspace:            workspace,
					ExecutionProfile:     test.profile,
					RequiredCapabilities: CapabilityFilesystem | CapabilityOutput,
					workspaceIdentity:    identity,
				},
			}
			_, err = sandboxInstance.Exec(context.Background(), Command{Path: "/usr/bin/true"})
			if !errors.Is(err, ErrRequiredCapabilitiesUnavailable) {
				t.Fatalf("Exec() error = %v, want ErrRequiredCapabilitiesUnavailable", err)
			}
			if !strings.Contains(err.Error(), test.missing.String()) {
				t.Fatalf("Exec() error = %q, want missing capability %q", err, test.missing)
			}
			if backend.executed {
				t.Fatal("payload executed before the profile capability was accepted")
			}
		})
	}
}
