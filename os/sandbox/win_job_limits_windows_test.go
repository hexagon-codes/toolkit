//go:build windows

package sandbox

import (
	"strings"
	"testing"
)

func TestWindowsJobLimitInformationOptionalLimitQuadrants(t *testing.T) {
	const memoryBytes = int64(256*1024*1024 + 127)
	const processLimit = 7

	tests := []struct {
		name             string
		memoryBytes      int64
		processLimit     int
		wantFlags        uint32
		wantMemoryBytes  uintptr
		wantProcessLimit uint32
	}{
		{
			name:      "no optional limits",
			wantFlags: jobObjectLimitKillOnClose,
		},
		{
			name:            "memory only",
			memoryBytes:     memoryBytes,
			wantFlags:       jobObjectLimitKillOnClose | jobObjectLimitProcessMemory | jobObjectLimitJobMemory,
			wantMemoryBytes: uintptr(memoryBytes),
		},
		{
			name:             "processes only",
			processLimit:     processLimit,
			wantFlags:        jobObjectLimitKillOnClose | jobObjectLimitActiveProcess,
			wantProcessLimit: processLimit,
		},
		{
			name:             "memory and processes",
			memoryBytes:      memoryBytes,
			processLimit:     processLimit,
			wantFlags:        jobObjectLimitKillOnClose | jobObjectLimitProcessMemory | jobObjectLimitJobMemory | jobObjectLimitActiveProcess,
			wantMemoryBytes:  uintptr(memoryBytes),
			wantProcessLimit: processLimit,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info, err := windowsJobLimitInformation(test.memoryBytes, test.processLimit)
			if err != nil {
				t.Fatalf("windowsJobLimitInformation() error = %v", err)
			}
			if got := info.BasicLimitInformation.LimitFlags; got != test.wantFlags {
				t.Errorf("limit flags = %#x, want %#x", got, test.wantFlags)
			}
			if got := info.ProcessMemoryLimit; got != test.wantMemoryBytes {
				t.Errorf("process memory limit = %d, want %d", got, test.wantMemoryBytes)
			}
			if got := info.JobMemoryLimit; got != test.wantMemoryBytes {
				t.Errorf("Job memory limit = %d, want %d", got, test.wantMemoryBytes)
			}
			if got := info.BasicLimitInformation.ActiveProcessLimit; got != test.wantProcessLimit {
				t.Errorf("active process limit = %d, want %d", got, test.wantProcessLimit)
			}
		})
	}
}

func TestWindowsJobLimitInformationRejectsNegativeInternalLimits(t *testing.T) {
	for _, test := range []struct {
		name         string
		memoryBytes  int64
		processLimit int
		wantMessage  string
	}{
		{
			name:        "negative memory",
			memoryBytes: -1,
			wantMessage: "job object memory limit must not be negative",
		},
		{
			name:         "negative processes",
			processLimit: -1,
			wantMessage:  "job object process limit must not be negative",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := windowsJobLimitInformation(test.memoryBytes, test.processLimit)
			if err == nil || !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("windowsJobLimitInformation() error = %v, want %q", err, test.wantMessage)
			}
		})
	}
}

func TestWindowsLimitReportMatchesOptionalLimitQuadrants(t *testing.T) {
	tests := []struct {
		name          string
		config        Config
		wantMemory    LimitStatus
		wantProcesses LimitStatus
	}{
		{
			name:          "no optional limits",
			wantMemory:    LimitStatusNotRequested,
			wantProcesses: LimitStatusNotRequested,
		},
		{
			name:          "memory only",
			config:        Config{MaxMemoryBytes: 1},
			wantMemory:    LimitStatusEnforced,
			wantProcesses: LimitStatusNotRequested,
		},
		{
			name:          "processes only",
			config:        Config{MaxProcesses: 1},
			wantMemory:    LimitStatusNotRequested,
			wantProcesses: LimitStatusEnforced,
		},
		{
			name: "memory and processes",
			config: Config{
				MaxMemoryBytes: 1,
				MaxProcesses:   1,
			},
			wantMemory:    LimitStatusEnforced,
			wantProcesses: LimitStatusEnforced,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := windowsLimitReport(test.config, true)
			if report.Memory != test.wantMemory {
				t.Errorf("memory status = %q, want %q", report.Memory, test.wantMemory)
			}
			if report.Processes != test.wantProcesses {
				t.Errorf("processes status = %q, want %q", report.Processes, test.wantProcesses)
			}
		})
	}
}
