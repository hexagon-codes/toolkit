//go:build linux

package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

type linuxLimitExecutionOutcome struct {
	result *ExecResult
	err    error
}

func TestLinuxBoundaryProbeRootDropsCapabilitiesButRejectsIneffectiveProcessLimit(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root Linux boundary probe only")
	}
	requireUsableLinuxBwrap(t)
	bwrap, err := linuxBwrapPath()
	if err != nil {
		t.Skipf("trusted bubblewrap is unavailable: %v", err)
	}
	result := runLinuxBwrapProbe(bwrap, false)
	if result.Isolation != LimitStatusEnforced {
		t.Fatalf("root Linux isolation probe = %+v, want isolation enforced", result)
	}
}

func TestLinuxBoundaryProbeNonRootProvesProcessLimitWithEAGAIN(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("non-root Linux boundary probe only")
	}
	bwrap, err := linuxBwrapPath()
	if err != nil {
		t.Skipf("trusted bubblewrap is unavailable: %v", err)
	}
	result := runLinuxBwrapProbe(bwrap, false)
	if result.Isolation != LimitStatusEnforced {
		t.Fatalf("non-root Linux boundary probe = %+v, want isolation enforced", result)
	}
}

func TestLinuxRootExecReportsProcessesNotRequestedWithoutFalseEnforcement(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root Linux execution only")
	}
	requireUsableLinuxBwrap(t)
	workspace := t.TempDir()
	sandboxInstance, err := New(Config{
		Workspace: workspace,
		Network:   NetworkDisabled,
		RequiredCapabilities: CapabilityFilesystem |
			CapabilityNetwork |
			CapabilityProcessContainment |
			CapabilityOutput,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sandboxInstance.Exec(context.Background(), Command{Path: "/usr/bin/true"})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("Exec() exit=%d stderr=%q", result.ExitCode, result.Stderr)
	}
	if result.Limits.Memory != LimitStatusNotRequested || result.Limits.Processes != LimitStatusNotRequested {
		t.Fatalf("root Linux requested limits = %+v, want memory and processes not_requested", result.Limits)
	}
	if result.Limits.Filesystem != LimitStatusEnforced ||
		result.Limits.Network != LimitStatusEnforced ||
		result.Limits.ProcessContainment != LimitStatusEnforced {
		t.Fatalf("root Linux isolation report = %+v", result.Limits)
	}
}

func TestLinuxNativeProcessBudgetCapabilityAndReportMatrix(t *testing.T) {
	requireUsableLinuxBwrap(t)
	tests := []struct {
		budget       int
		wantCreation bool
		wantReject   bool
	}{
		{budget: 0, wantCreation: true},
		{budget: 1, wantReject: true},
		{budget: 2, wantCreation: true, wantReject: true},
	}

	for _, test := range tests {
		t.Run("budget="+strconv.Itoa(test.budget), func(t *testing.T) {
			workspace := t.TempDir()
			marker := filepath.Join(workspace, "payload-started")
			required := UntrustedCodeIsolationCapabilities
			if test.budget > 0 {
				required |= CapabilityProcesses
			}
			sandboxInstance, err := New(Config{
				Workspace:            workspace,
				Network:              NetworkDisabled,
				MaxProcesses:         test.budget,
				RequiredCapabilities: required,
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if closeErr := sandboxInstance.Close(); closeErr != nil {
					t.Errorf("Close() error = %v", closeErr)
				}
			})

			available, err := AvailableCapabilities(context.Background(), sandboxInstance)
			if err != nil {
				t.Fatalf("AvailableCapabilities() error = %v", err)
			}
			if available.Has(CapabilityProcesses) {
				t.Fatalf("available capabilities = %s, want exact process budget omitted", available)
			}
			if got := available.Has(CapabilityProcessCreation); got != test.wantCreation {
				t.Fatalf("process creation capability = %t, want %t for budget %d", got, test.wantCreation, test.budget)
			}

			result, execErr := sandboxInstance.Exec(context.Background(), Command{
				Path: "/bin/sh",
				Args: []string{"-c", `printf started >"$1"`, "process-budget", marker},
			})
			if test.wantReject {
				if !errors.Is(execErr, ErrRequiredCapabilitiesUnavailable) {
					t.Fatalf("Exec() error = %v, want exact process budget rejection", execErr)
				}
				if result != nil {
					t.Fatalf("Exec() result = %+v, want nil before payload launch", result)
				}
				if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("payload started before process capability acceptance: %v", statErr)
				}
				return
			}
			if execErr != nil {
				t.Fatalf("Exec() error = %v", execErr)
			}
			if result == nil || result.Limits.Processes != LimitStatusNotRequested ||
				result.Limits.ProcessContainment != LimitStatusEnforced {
				t.Fatalf("unlimited process report = %+v, want processes not_requested and containment enforced", result)
			}
			if _, statErr := os.Stat(marker); statErr != nil {
				t.Fatalf("unlimited payload marker: %v", statErr)
			}
		})
	}
}

func TestLinuxNativeMemoryCapabilityMatchesAppliedPrlimit(t *testing.T) {
	requireUsableLinuxBwrap(t)
	const maxMemoryBytes = int64(512 * 1024 * 1024)
	workspace := t.TempDir()
	sandboxInstance, err := New(Config{
		Workspace:            workspace,
		Network:              NetworkDisabled,
		MaxMemoryBytes:       maxMemoryBytes,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities | CapabilityMemory,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := sandboxInstance.Close(); closeErr != nil {
			t.Errorf("Close() error = %v", closeErr)
		}
	})

	available, err := AvailableCapabilities(context.Background(), sandboxInstance)
	if err != nil {
		t.Fatalf("AvailableCapabilities() error = %v", err)
	}
	if !available.Has(CapabilityMemory) {
		t.Fatalf("available capabilities = %s, want memory after prlimit preflight", available)
	}
	result, err := sandboxInstance.Exec(context.Background(), Command{
		Path: "/bin/sh",
		Args: []string{"-c", `awk '$1 == "Max" && $2 == "address" && $3 == "space" { print $4; exit }' /proc/self/limits`},
	})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if got, want := strings.TrimSpace(result.Stdout), strconv.FormatInt(maxMemoryBytes, 10); got != want {
		t.Fatalf("payload RLIMIT_AS = %q, want %q", got, want)
	}
	if result.Limits.Memory != LimitStatusEnforced || result.Limits.Processes != LimitStatusNotRequested {
		t.Fatalf("Linux resource report = %+v, want memory enforced and processes not_requested", result.Limits)
	}
}

func TestLinuxRootRequiredProcessesRejectsBeforePayload(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root Linux capability rejection only")
	}
	if _, err := linuxBwrapPath(); err != nil {
		t.Skipf("trusted bubblewrap is unavailable: %v", err)
	}
	workspace := t.TempDir()
	marker := filepath.Join(workspace, "payload-started")
	sandboxInstance, err := New(Config{
		Workspace:            workspace,
		Network:              NetworkDisabled,
		MaxProcesses:         1,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities | CapabilityProcesses,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sandboxInstance.Exec(context.Background(), Command{
		Path: "/bin/sh",
		Args: []string{"-c", `printf started >"$1"`, "process-capability", marker},
	})
	if !errors.Is(err, ErrRequiredCapabilitiesUnavailable) {
		t.Fatalf("Exec() error = %v, want required capability rejection", err)
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("payload started before capability rejection: %v", statErr)
	}
}

func TestLinuxNonRootExecSeparatesAvailableProcessesCapabilityFromRequest(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("non-root Linux execution only")
	}
	if _, err := linuxBwrapPath(); err != nil {
		t.Skipf("trusted bubblewrap is unavailable: %v", err)
	}
	workspace := t.TempDir()
	sandboxInstance, err := New(Config{
		Workspace:            workspace,
		Network:              NetworkDisabled,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sandboxInstance.Exec(context.Background(), Command{Path: "/usr/bin/true"})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 || result.Limits.Memory != LimitStatusNotRequested || result.Limits.Processes != LimitStatusNotRequested {
		t.Fatalf("non-root Linux execution = %+v stderr=%q", result, result.Stderr)
	}
}

func TestLinuxNonRootConfiguredProcessLimitReachesEAGAIN(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("non-root Linux process enforcement only")
	}
	requireUsableLinuxBwrap(t)
	workspace := t.TempDir()
	marker := filepath.Join(workspace, "payload-started")
	sandboxInstance, err := New(Config{
		Workspace:            workspace,
		MaxProcesses:         2,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities | CapabilityProcesses,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sandboxInstance.Close() })
	_, err = sandboxInstance.Exec(context.Background(), Command{
		Path: "/bin/sh",
		Args: []string{"-c", `printf started >"$1"`, "exact-process-budget", marker},
	})
	if !errors.Is(err, ErrRequiredCapabilitiesUnavailable) {
		t.Fatalf("Exec() error = %v, want exact process budget rejection", err)
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("payload marker exists before capability acceptance: %v", statErr)
	}
}

func TestLinuxNonRootRequestedLimitReportSurvivesDeadline(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("non-root Linux limit report only")
	}
	requireUsableLinuxBwrap(t)
	workspace := t.TempDir()
	marker := filepath.Join(workspace, "deadline-started")
	sandboxInstance := newLinuxRequestedLimitSandbox(t, workspace)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, execErr := sandboxInstance.Exec(ctx, Command{
		Path: "/bin/sh",
		Args: []string{"-c", `printf started >"$1"; exec /usr/bin/sleep 30`, "limit-report-deadline", marker},
	})
	if !errors.Is(execErr, context.DeadlineExceeded) {
		t.Fatalf("Exec() error = %v, want context deadline exceeded", execErr)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("deadline payload did not start: %v", err)
	}
	assertLinuxRequestedLimitReport(t, result)
}

func TestLinuxNonRootRequestedLimitReportSurvivesCancellation(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("non-root Linux limit report only")
	}
	requireUsableLinuxBwrap(t)
	workspace := t.TempDir()
	marker := filepath.Join(workspace, "cancel-started")
	sandboxInstance := newLinuxRequestedLimitSandbox(t, workspace)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan linuxLimitExecutionOutcome, 1)
	go func() {
		result, execErr := sandboxInstance.Exec(ctx, Command{
			Path: "/bin/sh",
			Args: []string{"-c", `printf started >"$1"; exec /usr/bin/sleep 30`, "limit-report-cancel", marker},
		})
		done <- linuxLimitExecutionOutcome{result: result, err: execErr}
	}()

	waitForLinuxPayloadMarker(t, marker, done, cancel)
	cancel()
	select {
	case outcome := <-done:
		if !errors.Is(outcome.err, context.Canceled) {
			t.Fatalf("Exec() error = %v, want context canceled", outcome.err)
		}
		assertLinuxRequestedLimitReport(t, outcome.result)
	case <-time.After(5 * time.Second):
		t.Fatal("canceled Linux execution did not return")
	}
}

func newLinuxRequestedLimitSandbox(t *testing.T, workspace string) Sandbox {
	t.Helper()
	sandboxInstance, err := New(Config{
		Workspace:            workspace,
		Network:              NetworkDisabled,
		MaxMemoryBytes:       1 << 62,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities | CapabilityMemory,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := sandboxInstance.Close(); closeErr != nil {
			t.Errorf("Close() error = %v", closeErr)
		}
	})
	return sandboxInstance
}

func assertLinuxRequestedLimitReport(t *testing.T, result *ExecResult) {
	t.Helper()
	if result == nil {
		t.Fatal("Exec() result is nil")
	}
	if result.Limits.Memory != LimitStatusEnforced || result.Limits.Processes != LimitStatusNotRequested {
		t.Fatalf("Linux requested limits = %+v, want memory enforced and processes not_requested", result.Limits)
	}
}

func waitForLinuxPayloadMarker(
	t *testing.T,
	marker string,
	done <-chan linuxLimitExecutionOutcome,
	cancel context.CancelFunc,
) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		if _, err := os.Stat(marker); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			cancel()
			t.Fatalf("inspect cancellation marker: %v", err)
		}
		select {
		case outcome := <-done:
			t.Fatalf("Linux execution returned before cancellation: result=%+v error=%v", outcome.result, outcome.err)
		case <-ticker.C:
		case <-timer.C:
			cancel()
			t.Fatal("Linux cancellation payload did not start")
		}
	}
}

func TestLinuxRequiredProcessesRejectsBeforePayloadWhenProbeIsUnsupported(t *testing.T) {
	workspace := t.TempDir()
	marker := filepath.Join(workspace, "payload-started")
	backend := &linuxSandbox{
		cfg: Config{
			Workspace:            workspace,
			Network:              NetworkDisabled,
			MaxProcesses:         1,
			RequiredCapabilities: CapabilityProcesses,
		},
		resolveBwrap: func() (string, error) { return "/usr/bin/bwrap", nil },
		probeBwrap: func(string, bool) linuxBwrapProbeResult {
			return linuxBwrapProbeResult{Isolation: LimitStatusEnforced}
		},
	}
	available, err := backend.sandboxCapabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	err = requireSandboxCapabilities(CapabilityProcesses, available)
	if !errors.Is(err, ErrRequiredCapabilitiesUnavailable) {
		t.Fatalf("required process capability error = %v", err)
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("payload marker exists before capability acceptance: %v", statErr)
	}
}

func TestLinuxBwrapArgsDropCapabilitiesAndIsolateNamespaces(t *testing.T) {
	workspace := t.TempDir()
	if err := initializePOSIXRuntimeDirectories(workspace); err != nil {
		t.Fatal(err)
	}
	command := linuxPreparedTestCommand(t, "/usr/bin/true", workspace)

	for _, network := range []bool{false, true} {
		t.Run("network="+strconv.FormatBool(network), func(t *testing.T) {
			sandboxInstance := &linuxSandbox{cfg: Config{Workspace: workspace, Network: NetworkMode(network)}}
			args, err := sandboxInstance.bwrapArgs(command)
			if err != nil {
				t.Fatal(err)
			}
			for _, flag := range []string{
				"--unshare-user",
				"--unshare-pid",
				"--unshare-ipc",
				"--unshare-uts",
				"--disable-userns",
				"--assert-userns-disabled",
			} {
				if !slices.Contains(args, flag) {
					t.Errorf("bubblewrap arguments do not contain %s", flag)
				}
			}
			if !linuxArgumentPairExists(args, "--cap-drop", "ALL") {
				t.Error("bubblewrap arguments must drop all capabilities")
			}
			if got := slices.Contains(args, "--unshare-net"); got != !network {
				t.Errorf("--unshare-net present = %t, want %t", got, !network)
			}
		})
	}
}

func TestLinuxBwrapProbeRejectsExitZeroWithoutSecurityEvidence(t *testing.T) {
	launcher := filepath.Join(t.TempDir(), "bwrap")
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if result := runLinuxBwrapProbe(launcher, false); result.Isolation != "" {
		t.Fatalf("bubblewrap probe accepted an exit-zero launcher without security evidence: %+v", result)
	}
}

func TestLinuxDeniedPathMustExistBeforeLaunch(t *testing.T) {
	workspace := t.TempDir()
	if err := initializePOSIXRuntimeDirectories(workspace); err != nil {
		t.Fatal(err)
	}
	denied := filepath.Join(workspace, "future-secret")
	sandboxInstance := &linuxSandbox{cfg: Config{
		Workspace:   workspace,
		DeniedPaths: []string{denied},
	}}
	_, err := sandboxInstance.bwrapArgs(linuxPreparedTestCommand(t, "/usr/bin/true", workspace))
	if err == nil || !strings.Contains(err.Error(), "denied path must exist before launch") {
		t.Fatalf("bwrapArgs() error = %v, want missing denied path rejection", err)
	}
	if _, statErr := os.Lstat(denied); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("missing denied path was created: %v", statErr)
	}
}

func TestLinuxMissingDeniedPathRejectsPayloadBeforeLaunch(t *testing.T) {
	workspace := t.TempDir()
	denied := filepath.Join(workspace, "future-secret")
	sandboxInstance, err := New(Config{
		Workspace:            workspace,
		DeniedPaths:          []string{denied},
		RequiredCapabilities: UntrustedCodeIsolationCapabilities,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sandboxInstance.Exec(context.Background(), Command{Path: "/usr/bin/true"})
	if err == nil {
		t.Fatal("Exec() accepted a missing denied path")
	}
	if _, statErr := os.Lstat(denied); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("missing denied path was created: %v", statErr)
	}
}

func TestLinuxDeniedPathCreatedAfterNewRemainsHiddenAndReadOnly(t *testing.T) {
	requireUsableLinuxBwrap(t)
	workspace := t.TempDir()
	denied := filepath.Join(workspace, "future-secret")
	sandboxInstance, err := New(Config{
		Workspace:            workspace,
		DeniedPaths:          []string{denied},
		RequiredCapabilities: UntrustedCodeIsolationCapabilities,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(denied, []byte("host-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := sandboxInstance.Exec(context.Background(), Command{
		Path: "/bin/sh",
		Args: []string{"-c", `
if test "$(cat "$1")" = host-secret; then exit 81; fi
if (printf escaped >"$1") 2>/dev/null; then exit 82; fi
printf blocked
`, "denied-path", denied},
	})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "blocked" {
		t.Fatalf("denied path escaped: exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
	content, err := os.ReadFile(denied)
	if err != nil || string(content) != "host-secret" {
		t.Fatalf("denied host file changed: content=%q error=%v", content, err)
	}
}

func TestLinuxDeniedDirectoryCannotBeReadOrWritten(t *testing.T) {
	requireUsableLinuxBwrap(t)
	workspace := t.TempDir()
	denied := filepath.Join(workspace, "secret-directory")
	if err := os.Mkdir(denied, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(denied, "sentinel")
	if err := os.WriteFile(sentinel, []byte("host-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	sandboxInstance, err := New(Config{
		Workspace:            workspace,
		DeniedPaths:          []string{denied},
		RequiredCapabilities: UntrustedCodeIsolationCapabilities,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sandboxInstance.Exec(context.Background(), Command{
		Path: "/bin/sh",
		Args: []string{"-c", `
if cat "$1/sentinel" >/dev/null 2>&1; then exit 81; fi
if (printf escaped >"$1/created") 2>/dev/null; then exit 82; fi
printf blocked
`, "denied-directory", denied},
	})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "blocked" {
		t.Fatalf("denied directory escaped: exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
	content, err := os.ReadFile(sentinel)
	if err != nil || string(content) != "host-secret" {
		t.Fatalf("denied directory sentinel changed: content=%q error=%v", content, err)
	}
	if _, err := os.Stat(filepath.Join(denied, "created")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("denied directory received a new file: %v", err)
	}
}

func TestLinuxWorkspaceRejectsHardlinkToExternalFile(t *testing.T) {
	workspace := t.TempDir()
	if err := initializePOSIXRuntimeDirectories(workspace); err != nil {
		t.Fatal(err)
	}
	externalRoot := t.TempDir()
	sentinel := filepath.Join(externalRoot, "sentinel")
	if err := os.WriteFile(sentinel, []byte("host-safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(workspace, "external-link")
	if err := os.Link(sentinel, link); err != nil {
		t.Skipf("hardlinks are unavailable: %v", err)
	}

	sandboxInstance := linuxInjectedBoundaryTestSandbox(workspace)
	_, err := sandboxInstance.planLinuxSandboxExecution(linuxPreparedTestCommand(t, "/usr/bin/true", workspace))
	if err == nil || !strings.Contains(err.Error(), "links outside the workspace") {
		t.Fatalf("linuxSandboxRunner() error = %v, want external hardlink rejection", err)
	}
	content, readErr := os.ReadFile(sentinel)
	if readErr != nil || string(content) != "host-safe" {
		t.Fatalf("external sentinel changed: content=%q error=%v", content, readErr)
	}
}

func TestLinuxWorkspaceRejectsGroupOrWorldWritableEntries(t *testing.T) {
	workspace := t.TempDir()
	if err := initializePOSIXRuntimeDirectories(workspace); err != nil {
		t.Fatal(err)
	}
	unsafePath := filepath.Join(workspace, "unsafe")
	if err := os.WriteFile(unsafePath, []byte("unsafe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unsafePath, 0o666); err != nil {
		t.Fatal(err)
	}
	sandboxInstance := linuxInjectedBoundaryTestSandbox(workspace)
	_, err := sandboxInstance.planLinuxSandboxExecution(linuxPreparedTestCommand(t, "/usr/bin/true", workspace))
	if err == nil || !strings.Contains(err.Error(), "group or world writable") {
		t.Fatalf("planLinuxSandboxExecution() error = %v, want writable entry rejection", err)
	}
}

func TestLinuxPreLaunchIdentityRecheckRejectsDirectoryReplacement(t *testing.T) {
	workspace := t.TempDir()
	if err := initializePOSIXRuntimeDirectories(workspace); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(workspace, "command-dir")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	sandboxInstance := linuxInjectedBoundaryTestSandbox(workspace)
	oldHook := linuxPreLaunchAuditHook
	linuxPreLaunchAuditHook = func() {
		original := dir + ".original"
		if err := os.Rename(dir, original); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { linuxPreLaunchAuditHook = oldHook })

	command := linuxPreparedTestCommand(t, "/usr/bin/true", workspace)
	command.Dir = dir
	execution, err := sandboxInstance.planLinuxSandboxExecution(command)
	if err == nil {
		err = execution.preflight(t.Context())
	}
	if err == nil || !strings.Contains(err.Error(), "command directory changed before launch") {
		t.Fatalf("linuxSandboxRunner() error = %v, want command directory identity rejection", err)
	}
}

func TestLinuxPreLaunchIdentityRecheckRejectsCommandReplacement(t *testing.T) {
	workspace := t.TempDir()
	if err := initializePOSIXRuntimeDirectories(workspace); err != nil {
		t.Fatal(err)
	}
	commandPath := filepath.Join(workspace, "payload")
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	sandboxInstance := linuxInjectedBoundaryTestSandbox(workspace)
	oldHook := linuxPreLaunchAuditHook
	linuxPreLaunchAuditHook = func() {
		original := commandPath + ".original"
		if err := os.Rename(commandPath, original); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nexit 81\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { linuxPreLaunchAuditHook = oldHook })

	execution, err := sandboxInstance.planLinuxSandboxExecution(linuxPreparedTestCommand(t, commandPath, workspace))
	if err == nil {
		err = execution.preflight(t.Context())
	}
	if err == nil || !strings.Contains(err.Error(), "command changed before launch") {
		t.Fatalf("preflight error = %v, want command identity rejection", err)
	}
}

func TestLinuxPreLaunchAuditRejectsNewExternalHardlink(t *testing.T) {
	workspace := t.TempDir()
	if err := initializePOSIXRuntimeDirectories(workspace); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "sentinel")
	if err := os.WriteFile(external, []byte("host-safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	sandboxInstance := linuxInjectedBoundaryTestSandbox(workspace)
	oldHook := linuxPreLaunchAuditHook
	linuxPreLaunchAuditHook = func() {
		if err := os.Link(external, filepath.Join(workspace, "late-hardlink")); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { linuxPreLaunchAuditHook = oldHook })

	execution, err := sandboxInstance.planLinuxSandboxExecution(linuxPreparedTestCommand(t, "/usr/bin/true", workspace))
	if err == nil {
		err = execution.preflight(t.Context())
	}
	if err == nil || !strings.Contains(err.Error(), "links outside the workspace") {
		t.Fatalf("preflight error = %v, want late external hardlink rejection", err)
	}
	content, readErr := os.ReadFile(external)
	if readErr != nil || string(content) != "host-safe" {
		t.Fatalf("external sentinel changed: content=%q error=%v", content, readErr)
	}
}

func linuxInjectedBoundaryTestSandbox(workspace string) *linuxSandbox {
	return &linuxSandbox{
		cfg: Config{Workspace: workspace},
		resolveBwrap: func() (string, error) {
			return "/usr/bin/bwrap", nil
		},
		probeBwrap: func(string, bool) linuxBwrapProbeResult {
			return linuxBwrapProbeResult{Isolation: LimitStatusEnforced}
		},
	}
}

func linuxArgumentPairExists(arguments []string, name, value string) bool {
	for index := 0; index+1 < len(arguments); index++ {
		if arguments[index] == name && arguments[index+1] == value {
			return true
		}
	}
	return false
}
