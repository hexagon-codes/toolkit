//go:build !darwin && !linux && !windows

package sandbox

import (
	"context"
	"testing"
)

func TestBasicCapabilitiesDoNotReportCreationWithinOneProcessBudget(t *testing.T) {
	tests := []struct {
		name         string
		maxProcesses int
		wantCreation bool
	}{
		{name: "unlimited", wantCreation: true},
		{name: "one process", maxProcesses: 1, wantCreation: false},
		{name: "two processes", maxProcesses: 2, wantCreation: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newBasicSandbox(Config{Network: NetworkHost, MaxProcesses: test.maxProcesses})
			available, err := backend.sandboxCapabilities(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if got := available.Has(CapabilityProcessCreation); got != test.wantCreation {
				t.Fatalf("CapabilityProcessCreation = %t, want %t; capabilities=%s", got, test.wantCreation, available)
			}
		})
	}
}
