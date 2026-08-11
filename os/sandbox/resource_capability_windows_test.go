//go:build windows

package sandbox

import (
	"context"
	"testing"
)

func TestWindowsCapabilitiesReportOnlyConfiguredResourceLimits(t *testing.T) {
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
		{name: "memory", config: Config{MaxMemoryBytes: 1}, want: base | CapabilityMemory | CapabilityProcessCreation},
		{name: "one process", config: Config{MaxProcesses: 1}, want: base | CapabilityProcesses},
		{
			name: "memory and two processes",
			config: Config{
				MaxMemoryBytes: 1,
				MaxProcesses:   2,
			},
			want: base | CapabilityMemory | CapabilityProcesses | CapabilityProcessCreation,
		},
		{name: "unsupported storage", config: Config{MaxWorkspaceBytes: 1}, want: base | CapabilityProcessCreation},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &windowsSandbox{cfg: test.config, workspace: &windowsWorkspace{}}
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
