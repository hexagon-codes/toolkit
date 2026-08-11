//go:build linux

package sandbox

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

const linuxTestMemoryLimit = 256 * 1024 * 1024

func TestLinuxCapabilitiesReportOnlyConfiguredResourceLimits(t *testing.T) {
	base := CapabilityFilesystem |
		CapabilityNetwork |
		CapabilityProcessContainment |
		CapabilityOutput
	tests := []struct {
		name   string
		config Config
		want   CapabilitySet
	}{
		{name: "no optional limits", want: base | CapabilityProcessCreation},
		{name: "memory", config: Config{MaxMemoryBytes: linuxTestMemoryLimit}, want: base | CapabilityMemory | CapabilityProcessCreation},
		{name: "one process", config: Config{MaxProcesses: 1}, want: base},
		{
			name: "memory and two processes",
			config: Config{
				MaxMemoryBytes: linuxTestMemoryLimit,
				MaxProcesses:   2,
			},
			want: base | CapabilityMemory | CapabilityProcessCreation,
		},
		{name: "unsupported storage", config: Config{MaxWorkspaceBytes: 1}, want: base | CapabilityProcessCreation},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &linuxSandbox{
				cfg:            test.config,
				resolveBwrap:   func() (string, error) { return "/usr/bin/bwrap", nil },
				resolvePrlimit: func() (string, error) { return "/usr/bin/prlimit", nil },
				probeBwrap: func(string, bool) linuxBwrapProbeResult {
					return linuxBwrapProbeResult{
						Isolation: LimitStatusEnforced,
					}
				},
			}
			got, err := backend.sandboxCapabilities(context.Background())
			if err != nil {
				t.Fatalf("sandboxCapabilities() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("sandboxCapabilities() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestLinuxMemoryCapabilityRequiresUsablePrlimitPreflight(t *testing.T) {
	backend := &linuxSandbox{
		cfg:            Config{MaxMemoryBytes: linuxTestMemoryLimit},
		resolveBwrap:   func() (string, error) { return "/usr/bin/bwrap", nil },
		resolvePrlimit: func() (string, error) { return "", errors.New("prlimit unavailable") },
		probeBwrap: func(string, bool) linuxBwrapProbeResult {
			return linuxBwrapProbeResult{Isolation: LimitStatusEnforced}
		},
	}
	got, err := backend.sandboxCapabilities(context.Background())
	if err != nil {
		t.Fatalf("sandboxCapabilities() error = %v", err)
	}
	if got.Has(CapabilityMemory) {
		t.Fatalf("sandboxCapabilities() = %s, want memory omitted after failed prlimit preflight", got)
	}
}

func TestLinuxMemoryCapabilityRequiresSuccessfulPrlimitExecution(t *testing.T) {
	backend := &linuxSandbox{
		cfg:            Config{MaxMemoryBytes: linuxTestMemoryLimit},
		resolveBwrap:   func() (string, error) { return "/usr/bin/bwrap", nil },
		resolvePrlimit: func() (string, error) { return "/usr/bin/false", nil },
		probeBwrap: func(string, bool) linuxBwrapProbeResult {
			return linuxBwrapProbeResult{Isolation: LimitStatusEnforced}
		},
	}
	got, err := backend.sandboxCapabilities(context.Background())
	if err != nil {
		t.Fatalf("sandboxCapabilities() error = %v", err)
	}
	if got.Has(CapabilityMemory) {
		t.Fatalf("sandboxCapabilities() = %s, want memory omitted after executable prlimit probe failed", got)
	}
	probe := backend.inspectLinuxResourceBackend()
	if probe.Memory != LimitStatusUnsupported || probe.memoryErr == nil {
		t.Fatalf("resource probe = %+v, want observable unsupported memory", probe)
	}
}

func TestLinuxPrlimitCapabilityProbeDoesNotChangeHostRlimit(t *testing.T) {
	var before unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_AS, &before); err != nil {
		t.Fatal(err)
	}
	if err := verifyLinuxPrlimitMemoryLimit(t.Context(), "/usr/bin/prlimit", linuxTestMemoryLimit); err != nil {
		t.Fatal(err)
	}
	var after unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_AS, &after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("host RLIMIT_AS changed during child probe: before=%+v after=%+v", before, after)
	}
}
