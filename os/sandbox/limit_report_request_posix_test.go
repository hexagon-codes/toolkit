//go:build !windows

package sandbox

import (
	"slices"
	"testing"
)

func TestPosixLimitReportDistinguishesOptionalLimitRequests(t *testing.T) {
	capabilities := posixExecutionCapabilities{
		Filesystem:         LimitStatusEnforced,
		Network:            LimitStatusEnforced,
		Processes:          LimitStatusEnforced,
		ProcessContainment: LimitStatusEnforced,
	}

	withoutRequests := posixLimitReport(Config{}, capabilities)
	for name, status := range map[string]LimitStatus{
		"memory":    withoutRequests.Memory,
		"processes": withoutRequests.Processes,
		"storage":   withoutRequests.Storage,
	} {
		if status != LimitStatusNotRequested {
			t.Errorf("unrequested %s status = %q, want %q", name, status, LimitStatusNotRequested)
		}
	}

	withRequests := posixLimitReport(Config{
		MaxMemoryBytes:    1,
		MaxProcesses:      1,
		MaxWorkspaceBytes: 1,
	}, capabilities)
	if withRequests.Processes != LimitStatusEnforced {
		t.Errorf("requested processes status = %q, want %q", withRequests.Processes, LimitStatusEnforced)
	}
	if withRequests.Storage != LimitStatusUnsupported {
		t.Errorf("requested storage status = %q, want %q", withRequests.Storage, LimitStatusUnsupported)
	}
	probedMemory, _ := posixResourceLimitCapabilities(posixExecutionCapabilities{})
	if withRequests.Memory != probedMemory {
		t.Errorf("requested memory status = %q, want probed capability %q", withRequests.Memory, probedMemory)
	}
}

func TestPosixProcessBudgetDoesNotUsePerUIDRlimitApproximation(t *testing.T) {
	want := Command{
		Path: "/usr/bin/true",
		Args: []string{"process-budget"},
		Dir:  t.TempDir(),
		Env:  []string{"PATH=/usr/bin:/bin"},
	}
	got, err := posixResourceLimitedCommand(want, Config{MaxProcesses: 2})
	if err != nil {
		t.Fatalf("posixResourceLimitedCommand() error = %v", err)
	}
	if got.Path != want.Path || got.Dir != want.Dir ||
		!slices.Equal(got.Args, want.Args) || !slices.Equal(got.Env, want.Env) {
		t.Fatalf("process-only POSIX command = %+v, want unchanged %+v", got, want)
	}
}
