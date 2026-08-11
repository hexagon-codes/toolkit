//go:build darwin

package sandbox

import (
	"context"
	"strings"
	"testing"
)

func TestDarwinNetworkModeProfileIsExplicit(t *testing.T) {
	hostProfile := newDarwinSandbox(Config{Network: NetworkHost, Workspace: t.TempDir()}).generateSBPL()
	if !strings.Contains(hostProfile, "(allow network*)") || strings.Contains(hostProfile, "remote ip") {
		t.Fatalf("host network profile is not explicit:\n%s", hostProfile)
	}
	disabledProfile := newDarwinSandbox(Config{Network: NetworkDisabled, Workspace: t.TempDir()}).generateSBPL()
	if !strings.Contains(disabledProfile, "(deny network*)") || strings.Contains(disabledProfile, "(allow network*)") {
		t.Fatalf("disabled network profile is not explicit:\n%s", disabledProfile)
	}
}

func TestDarwinNetworkHostSeatbeltSyntaxValid(t *testing.T) {
	s := newDarwinSandbox(Config{Network: NetworkHost, Timeout: 10, Workspace: t.TempDir()})
	res, err := s.Exec(context.Background(), Command{Path: "/bin/echo", Args: []string{"ok"}})
	if err != nil {
		t.Fatalf("host network Seatbelt profile must execute: %v", err)
	}
	if !strings.Contains(res.Stdout, "ok") {
		t.Fatalf("sandbox-exec stdout=%q stderr=%q", res.Stdout, res.Stderr)
	}
}
