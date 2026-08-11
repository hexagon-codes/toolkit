//go:build darwin

package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

const (
	darwinUntrustedInitTrigger        = "TOOLKIT_SANDBOX_DARWIN_UNTRUSTED_INIT"
	darwinUntrustedInitExternalMarker = "TOOLKIT_SANDBOX_DARWIN_EXTERNAL_MARKER"
	darwinUntrustedInitInternalMarker = "TOOLKIT_SANDBOX_DARWIN_INTERNAL_MARKER"
)

func init() {
	if os.Getenv(darwinUntrustedInitTrigger) != "1" {
		return
	}
	for _, name := range []string{darwinUntrustedInitExternalMarker, darwinUntrustedInitInternalMarker} {
		if marker := os.Getenv(name); marker != "" {
			_ = os.WriteFile(marker, []byte("caller init executed"), 0o600)
		}
	}
	os.Exit(93)
}

func TestDarwinResourceLauncherDoesNotLoadCallerInitWithPayloadEnvironment(t *testing.T) {
	workspace := t.TempDir()
	externalMarker := filepath.Join(t.TempDir(), "external-marker")
	internalMarker := filepath.Join(workspace, "internal-marker")
	sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: workspace, Timeout: 20})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sandboxInstance.Exec(context.Background(), Command{
		Path: "/usr/bin/true",
		Env: []string{
			"PATH=/usr/bin:/bin",
			"HOME=" + workspace,
			"TMPDIR=" + workspace,
			darwinUntrustedInitTrigger + "=1",
			darwinUntrustedInitExternalMarker + "=" + externalMarker,
			darwinUntrustedInitInternalMarker + "=" + internalMarker,
		},
	})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("Exec() exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
	for _, marker := range []string{externalMarker, internalMarker} {
		if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
			t.Fatalf("caller init marker %q exists: %v", marker, statErr)
		}
	}
}
