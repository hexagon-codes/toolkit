//go:build windows

package sandbox

import (
	"context"
	"errors"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func windowsBackendForTest(t *testing.T, sandboxValue Sandbox) *windowsSandbox {
	t.Helper()
	if backend, ok := sandboxValue.(*windowsSandbox); ok {
		return backend
	}
	wrapper, ok := sandboxValue.(*capabilitySandbox)
	if !ok {
		t.Fatalf("sandbox type = %T, want Windows capability wrapper", sandboxValue)
	}
	backend, ok := wrapper.backend.(*windowsSandbox)
	if !ok {
		t.Fatalf("sandbox backend type = %T, want *windowsSandbox", wrapper.backend)
	}
	return backend
}

func TestWindowsRestrictedTokenKeepsApplicationControlEnabled(t *testing.T) {
	const sandboxInertFlag = uint32(0x2)

	flags := restrictedTokenFlags()
	if flags&sandboxInertFlag != 0 {
		t.Fatalf("restricted token flags %#x include SANDBOX_INERT", flags)
	}
	if flags != disableMaxPrivilege {
		t.Fatalf("restricted token flags = %#x, want DISABLE_MAX_PRIVILEGE only", flags)
	}

	token, err := createSandboxToken()
	if err != nil {
		t.Fatalf("create restricted token without SANDBOX_INERT: %v", err)
	}
	if err := token.Close(); err != nil {
		t.Fatalf("close restricted token: %v", err)
	}
}

func TestWindowsNewRejectsReadablePathsUntilBrokeredMappingsExist(t *testing.T) {
	_, err := New(Config{
		Workspace:            t.TempDir(),
		ReadablePaths:        []string{t.TempDir()},
		RequiredCapabilities: UntrustedCodeIsolationCapabilities,
	})
	if err == nil {
		t.Fatal("Windows New accepted ReadablePaths without a brokered mapping")
	}
	if !strings.Contains(err.Error(), "ReadablePaths are unsupported") {
		t.Fatalf("ReadablePaths error = %v, want explicit unsupported error", err)
	}
}

func TestWindowsCommandPreservesExplicitEmptyEnvironment(t *testing.T) {
	prepared, err := (&windowsSandbox{}).prepareCommand(Command{
		Path: `C:\Windows\System32\cmd.exe`,
		Env:  []string{},
	})
	if err != nil {
		t.Fatalf("prepare Windows command with an explicit empty environment: %v", err)
	}
	if prepared.Env == nil {
		t.Fatal("explicit empty Windows Command.Env was converted to nil")
	}
	if len(prepared.Env) != 0 {
		t.Fatalf("explicit empty Windows Command.Env length = %d, want 0", len(prepared.Env))
	}
}

func TestWindowsNetworkPolicyMatrix(t *testing.T) {
	tests := []struct {
		name        string
		network     NetworkMode
		unsupported bool
	}{
		{name: "deny all network", network: NetworkDisabled},
		{name: "complete host network is unavailable", network: NetworkHost, unsupported: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sandboxValue, err := New(Config{
				Workspace:            t.TempDir(),
				Network:              test.network,
				RequiredCapabilities: UntrustedCodeIsolationCapabilities,
			})
			if test.unsupported {
				if !errors.Is(err, ErrUnsupportedNetworkPolicy) {
					t.Fatalf("network policy error = %v, want ErrUnsupportedNetworkPolicy", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("supported network policy failed: %v", err)
			}
			registerWindowsTestSandboxCleanup(t, sandboxValue)
		})
	}
}

func TestWindowsCapabilitiesMatchEnforcedBoundary(t *testing.T) {
	sandboxValue := newWindowsTestSandbox(t, Config{
		Workspace:            t.TempDir(),
		Network:              NetworkDisabled,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities,
	})
	capabilities, err := AvailableCapabilities(context.Background(), sandboxValue)
	if err != nil {
		t.Fatalf("read Windows sandbox capabilities: %v", err)
	}
	want := CapabilityFilesystem |
		CapabilityNetwork |
		CapabilityProcessContainment |
		CapabilityOutput |
		CapabilityProcessCreation
	if capabilities != want {
		t.Fatalf("Windows capabilities = %s, want %s", capabilities, want)
	}
	if capabilities.Has(CapabilityStorage) {
		t.Fatal("Windows capabilities unexpectedly include realtime storage quota")
	}
}

func TestWindowsLowBoxRejectsHostNetworkBeforeTokenUse(t *testing.T) {
	_, err := createLowBoxToken(0, nil, NetworkHost)
	if !errors.Is(err, ErrUnsupportedNetworkPolicy) {
		t.Fatalf("NetworkHost LowBox error = %v, want ErrUnsupportedNetworkPolicy", err)
	}
}

func TestWindowsStorageCapabilityIsUnsupportedWithoutRuntimeQuota(t *testing.T) {
	report := windowsLimitReport(Config{MaxWorkspaceBytes: 1}, false)
	if report.Network != LimitStatusEnforced {
		t.Fatalf("Windows Network status = %q, want enforced", report.Network)
	}
	if report.Storage != LimitStatusUnsupported {
		t.Fatalf("Windows Storage status = %q, want unsupported", report.Storage)
	}
	if report.ProcessContainment != LimitStatusUnsupported {
		t.Fatalf("unconfirmed Windows ProcessContainment status = %q, want unsupported", report.ProcessContainment)
	}
	if report.Processes != LimitStatusNotRequested {
		t.Fatalf("Windows Processes status = %q, want not_requested without an explicit limit", report.Processes)
	}
	if confirmed := windowsLimitReport(Config{}, true).ProcessContainment; confirmed != LimitStatusEnforced {
		t.Fatalf("confirmed Windows ProcessContainment status = %q, want enforced", confirmed)
	}
}

func TestWindowsJobLimitInformationIncludesAggregateAndPerProcessMemory(t *testing.T) {
	const memoryBytes = int64(256*1024*1024 + 127)
	info, err := windowsJobLimitInformation(memoryBytes, 7)
	if err != nil {
		t.Fatalf("build Job limits: %v", err)
	}
	flags := info.BasicLimitInformation.LimitFlags
	for _, required := range []uint32{
		jobObjectLimitProcessMemory,
		jobObjectLimitJobMemory,
		jobObjectLimitActiveProcess,
		jobObjectLimitKillOnClose,
	} {
		if flags&required == 0 {
			t.Errorf("Job limit flags %#x do not include %#x", flags, required)
		}
	}
	if info.ProcessMemoryLimit != uintptr(memoryBytes) || info.JobMemoryLimit != uintptr(memoryBytes) {
		t.Fatalf(
			"memory limits are not byte-exact: process=%d job=%d want=%d",
			info.ProcessMemoryLimit,
			info.JobMemoryLimit,
			memoryBytes,
		)
	}
	if info.BasicLimitInformation.ActiveProcessLimit != 7 {
		t.Fatalf("active process limit = %d, want 7", info.BasicLimitInformation.ActiveProcessLimit)
	}
}

func TestWindowsAppContainerWorkspacePermissionsExcludeACLControl(t *testing.T) {
	permissions := uint32(windowsAppContainerWorkspacePermissions())
	dangerous := uint32(windows.WRITE_DAC | windows.WRITE_OWNER | windows.GENERIC_ALL | windows.GENERIC_WRITE)
	if permissions&dangerous != 0 {
		t.Fatalf("AppContainer workspace permissions %#x include ACL control mask %#x", permissions, permissions&dangerous)
	}
	required := uint32(
		windows.FILE_GENERIC_READ |
			windows.FILE_GENERIC_WRITE |
			windows.FILE_GENERIC_EXECUTE |
			windows.DELETE |
			windowsFileDeleteChild,
	)
	if permissions&required != required {
		t.Fatalf("AppContainer workspace permissions %#x do not include required mask %#x", permissions, required)
	}
}
