package sandbox

import (
	"errors"
	"strings"
	"testing"
)

func TestFilesystemContainmentUnavailableSentinelContract(t *testing.T) {
	wrapped := errors.Join(errors.New("backend probe failed"), ErrFilesystemContainmentUnavailable)
	if !errors.Is(wrapped, ErrFilesystemContainmentUnavailable) {
		t.Fatal("filesystem containment sentinel must support errors.Is")
	}
	if ErrFilesystemContainmentUnavailable.Error() != "sandbox filesystem containment unavailable" {
		t.Fatalf("filesystem containment sentinel = %q", ErrFilesystemContainmentUnavailable)
	}
}

func TestLinuxBackendHasNoWeakFilesystemFallback(t *testing.T) {
	linuxSource := mustReadSandboxSource(t, "sandbox_linux.go")
	for _, forbidden := range []string{
		"linuxBackendUnshare",
		"linuxUnshareBackendUsable",
		"linuxUnsharePolicyScript",
		"func (s *linuxSandbox) unshareArgs",
		`exec.LookPath("unshare")`,
	} {
		if strings.Contains(linuxSource, forbidden) {
			t.Errorf("Linux backend still contains weak fallback %q", forbidden)
		}
	}
	if !strings.Contains(linuxSource, "linux requires usable bubblewrap") {
		t.Fatal("Linux backend must fail closed when bubblewrap is unavailable")
	}

	contractSource := mustReadSandboxSource(t, "sandbox.go")
	if strings.Contains(contractSource, "LimitStatusWeak") || strings.Contains(contractSource, `"weak"`) {
		t.Fatal("public limit contract must not advertise removed weak containment")
	}
}
