//go:build !windows

package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTrustedPOSIXLauncherRejectsUserWritableCandidates(t *testing.T) {
	directory := t.TempDir()
	candidate := filepath.Join(directory, "sandbox-launcher")
	if err := os.WriteFile(candidate, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveTrustedPOSIXLauncher("sandbox-launcher", []string{candidate}); err == nil {
		t.Fatal("user-writable launcher candidate was trusted")
	}
	if err := os.Chmod(candidate, 0o775); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveTrustedPOSIXLauncher("sandbox-launcher", []string{candidate}); err == nil {
		t.Fatal("group-writable launcher candidate was trusted")
	}
}
