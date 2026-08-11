package sandbox

import (
	"errors"
	"strings"
	"testing"
)

func TestRequiredResourceCapabilitiesRejectUnconfiguredLimits(t *testing.T) {
	tests := []struct {
		name       string
		capability CapabilitySet
	}{
		{name: "memory", capability: CapabilityMemory},
		{name: "processes", capability: CapabilityProcesses},
		{name: "storage", capability: CapabilityStorage},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := Config{
				MaxOutputBytes:       1,
				MaxStderrBytes:       1,
				RequiredCapabilities: UntrustedCodeIsolationCapabilities | test.capability,
			}
			err := validateRequiredCapabilityContract(config)
			if !errors.Is(err, ErrInvalidCapabilityContract) {
				t.Fatalf("validateRequiredCapabilityContract() error = %v, want ErrInvalidCapabilityContract", err)
			}
			if !strings.Contains(err.Error(), test.capability.String()) {
				t.Fatalf("validateRequiredCapabilityContract() error = %q, want capability %q", err, test.capability)
			}
		})
	}
}

func TestOutputCapabilityValidationUsesNormalizedSafeLimits(t *testing.T) {
	config, err := validateSandboxConfigSemantics(Config{
		Workspace:            t.TempDir(),
		RequiredCapabilities: UntrustedCodeIsolationCapabilities,
	})
	if err != nil {
		t.Fatalf("validateSandboxConfigSemantics() error = %v", err)
	}
	if config.MaxOutputBytes <= 0 || config.MaxStderrBytes <= 0 {
		t.Fatalf("normalized output limits = stdout %d, stderr %d; want positive values", config.MaxOutputBytes, config.MaxStderrBytes)
	}
	if !config.RequiredCapabilities.Has(CapabilityOutput) {
		t.Fatalf("normalized required capabilities = %s, want output", config.RequiredCapabilities)
	}
}
