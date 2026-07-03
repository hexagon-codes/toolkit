package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStorageLimitsEnforceMaxArtifactBytes(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "large.bin"), []byte("1234567890"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	err := enforceSandboxStorageLimits(Config{
		Workspace:        ws,
		MaxArtifactBytes: 5,
	})
	if err == nil || !strings.Contains(err.Error(), "MaxArtifactBytes") {
		t.Fatalf("MaxArtifactBytes should reject oversized artifact, got %v", err)
	}
}

func TestStorageLimitsEnforceMaxWorkspaceBytes(t *testing.T) {
	ws := t.TempDir()
	for _, name := range []string{"a.bin", "b.bin"} {
		if err := os.WriteFile(filepath.Join(ws, name), []byte("12345"), 0644); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}

	err := enforceSandboxStorageLimits(Config{
		Workspace:         ws,
		MaxWorkspaceBytes: 8,
		MaxArtifactBytes:  10,
	})
	if err == nil || !strings.Contains(err.Error(), "MaxWorkspaceBytes") {
		t.Fatalf("MaxWorkspaceBytes should reject oversized workspace, got %v", err)
	}
}
