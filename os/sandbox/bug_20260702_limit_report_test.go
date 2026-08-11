package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// bug-20260702 正向契约：平台能力与本次请求事实必须分开报告。
func TestBug20260702_LimitReportMatchesRequestAndPlatformCapability(t *testing.T) {
	networkMode := NetworkHost
	if runtime.GOOS == "windows" {
		networkMode = NetworkDisabled
	}
	sb, err := New(Config{
		Workspace:            t.TempDir(),
		Network:              networkMode,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := sb.Close(); closeErr != nil {
			t.Errorf("Close: %v", closeErr)
		}
	})
	reporter, ok := sb.(CapabilityReporter)
	if !ok {
		t.Fatal("sandbox does not expose CapabilityReporter")
	}
	available, err := reporter.Capabilities(context.Background())
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}

	command, args := "/bin/sh", []string{"-c", "true"}
	if runtime.GOOS == "windows" {
		command = os.Getenv("ComSpec")
		if command == "" {
			command = filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
		}
		absoluteCommand, absoluteErr := filepath.Abs(command)
		if absoluteErr != nil {
			t.Fatalf("resolve canonical Windows command: %v", absoluteErr)
		}
		canonicalCommand, canonicalErr := filepath.EvalSymlinks(absoluteCommand)
		if canonicalErr != nil {
			t.Fatalf("canonicalize Windows command: %v", canonicalErr)
		}
		command, args = filepath.Clean(canonicalCommand), []string{"/d", "/c", "exit", "0"}
	}
	res, err := sb.Exec(context.Background(), Command{Path: command, Args: args})
	if err != nil {
		skipIfSandboxBackendUnavailable(t, err)
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("Exec exit code = %d, stderr=%q", res.ExitCode, res.Stderr)
	}

	for name, status := range map[string]LimitStatus{
		"memory":    res.Limits.Memory,
		"processes": res.Limits.Processes,
		"storage":   res.Limits.Storage,
	} {
		if status != LimitStatusNotRequested {
			t.Errorf("Limits.%s = %q, want %q", name, status, LimitStatusNotRequested)
		}
	}

	checks := []struct {
		name       string
		status     LimitStatus
		capability CapabilitySet
	}{
		{name: "network", status: res.Limits.Network, capability: CapabilityNetwork},
		{name: "process-containment", status: res.Limits.ProcessContainment, capability: CapabilityProcessContainment},
		{name: "output", status: res.Limits.Output, capability: CapabilityOutput},
		{name: "filesystem", status: res.Limits.Filesystem, capability: CapabilityFilesystem},
	}
	for _, check := range checks {
		want := LimitStatusUnsupported
		if available.Has(check.capability) {
			want = LimitStatusEnforced
		}
		if check.status != want {
			t.Errorf("Limits.%s = %q, want %q from capabilities %s", check.name, check.status, want, available)
		}
	}
}

// 支持的平台上限额报告与实际行为一致: Output 报 enforced 时, 超限输出必须真被截断。
func TestBug20260702_LimitReportOutputEnforcedMatchesBehavior(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows 无 /bin/sh, 输出截断行为由 POSIX 平台验证")
	}
	sb, err := New(Config{
		Workspace:            t.TempDir(),
		Network:              NetworkHost,
		RequiredCapabilities: CapabilityFilesystem | CapabilityNetwork | CapabilityOutput,
		MaxOutputBytes:       16,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res, err := sb.Exec(context.Background(), Command{
		Path: "/usr/bin/printf",
		Args: []string{"%s", strings.Repeat("0", 4096)},
	})
	if err != nil {
		skipIfSandboxBackendUnavailable(t, err)
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("Exec exit code = %d, stderr=%q", res.ExitCode, res.Stderr)
	}
	if res.Limits.Output != LimitStatusEnforced {
		t.Fatalf("Limits.Output = %q, want enforced", res.Limits.Output)
	}
	if !res.StdoutTruncated {
		t.Fatal("Limits.Output is enforced but 4096 bytes were not truncated to 16 bytes")
	}
	if int64(len(res.Stdout)) > 16 {
		t.Fatalf("retained stdout length = %d, want at most 16", len(res.Stdout))
	}
}

func skipIfSandboxBackendUnavailable(t *testing.T, err error) {
	t.Helper()
	if runtime.GOOS == "linux" && (errors.Is(err, ErrRequiredCapabilitiesUnavailable) || errors.Is(err, ErrFilesystemContainmentUnavailable)) {
		t.Skipf("Linux runner has no available OS sandbox backend: %v", err)
	}
}
