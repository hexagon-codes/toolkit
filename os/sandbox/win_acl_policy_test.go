//go:build windows

package sandbox

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsWorkspaceUsesStableAppContainerIdentity(t *testing.T) {
	workspacePath := t.TempDir()
	firstSandbox := newWindowsTestSandbox(t, Config{Workspace: workspacePath, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	secondSandbox := newWindowsTestSandbox(t, Config{Workspace: workspacePath, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	first := windowsBackendForTest(t, firstSandbox)
	second := windowsBackendForTest(t, secondSandbox)
	if !bytes.Equal(first.workspace.appContainerSID, second.workspace.appContainerSID) {
		t.Fatal("the same workspace produced different AppContainer identities")
	}
}

func TestWindowsWorkspaceRejectsNonEmptyUninitializedDirectory(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspacePath, "host-file.txt"), []byte("host"), 0o600); err != nil {
		t.Fatalf("create host fixture: %v", err)
	}
	_, err := New(Config{Workspace: workspacePath, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	if err == nil {
		t.Fatal("New accepted a non-empty uninitialized workspace")
	}
}

func TestWindowsWorkspaceRejectsRootJunction(t *testing.T) {
	junctionParent := t.TempDir()
	targetPath := t.TempDir()
	junctionPath := filepath.Join(junctionParent, "workspace-junction")
	command := exec.CommandContext(
		context.Background(),
		canonicalWindowsSystemExecutable(t, "cmd.exe"),
		"/d",
		"/c",
		"mklink",
		"/J",
		junctionPath,
		targetPath,
	) // #nosec G204 -- 仅用于创建固定的 Windows 根 junction 测试夹具。
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create Windows root junction fixture: %v, output=%s", err, output)
	}

	_, err := New(Config{Workspace: junctionPath, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	if err == nil || (!strings.Contains(err.Error(), "reparse point") && !strings.Contains(err.Error(), "symbolic link")) {
		t.Fatalf("root junction error = %v, want direct link rejection", err)
	}
	entries, readErr := os.ReadDir(targetPath)
	if readErr != nil {
		t.Fatalf("inspect root junction target after rejection: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("root junction target changed before rejection: %v", entries)
	}
}

func TestWindowsWorkspaceRootGuardRejectsJunction(t *testing.T) {
	junctionPath := filepath.Join(t.TempDir(), "workspace-junction")
	targetPath := t.TempDir()
	command := exec.CommandContext(
		context.Background(),
		canonicalWindowsSystemExecutable(t, "cmd.exe"),
		"/d",
		"/c",
		"mklink",
		"/J",
		junctionPath,
		targetPath,
	) // #nosec G204 -- 仅用于验证 Windows raw root guard 的 reparse point 拒绝。
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create Windows root-guard junction fixture: %v, output=%s", err, output)
	}

	guard, _, _, err := openWindowsWorkspaceRootGuard(junctionPath)
	if guard != nil {
		_ = guard.Close()
		t.Fatal("raw Windows workspace guard returned a handle for a junction")
	}
	if err == nil || !strings.Contains(err.Error(), "reparse point") {
		t.Fatalf("raw root guard error = %v, want reparse-point rejection", err)
	}
}

func TestWindowsWorkspaceRejectsExternalHardLink(t *testing.T) {
	workspacePath := t.TempDir()
	sandboxValue := newWindowsTestSandbox(t, Config{Workspace: workspacePath, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	externalFile := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(externalFile, []byte("outside"), 0o600); err != nil {
		t.Fatalf("create external hard-link source: %v", err)
	}
	if err := os.Link(externalFile, filepath.Join(workspacePath, "linked.txt")); err != nil {
		t.Fatalf("create external hard link: %v", err)
	}
	_, err := sandboxValue.Exec(context.Background(), Command{
		Path: canonicalWindowsSystemExecutable(t, "cmd.exe"),
		Args: []string{"/d", "/c", "exit", "0"},
	})
	if err == nil || !strings.Contains(err.Error(), "hard links") {
		t.Fatalf("hard-link Exec error = %v, want rejection", err)
	}
}

func TestWindowsWorkspaceRejectsReparsePoint(t *testing.T) {
	workspacePath := t.TempDir()
	sandboxValue := newWindowsTestSandbox(t, Config{Workspace: workspacePath, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	externalDirectory := t.TempDir()
	linkPath := filepath.Join(workspacePath, "outside-link")
	if err := os.Symlink(externalDirectory, linkPath); err != nil {
		t.Fatalf("create Windows reparse-point fixture: %v", err)
	}
	_, err := sandboxValue.Exec(context.Background(), Command{
		Path: canonicalWindowsSystemExecutable(t, "cmd.exe"),
		Args: []string{"/d", "/c", "exit", "0"},
	})
	if err == nil || !strings.Contains(err.Error(), "reparse point") {
		t.Fatalf("reparse-point Exec error = %v, want rejection", err)
	}
}

func TestWindowsWorkspaceRejectsInternalReparsePoint(t *testing.T) {
	workspacePath := t.TempDir()
	sandboxValue := newWindowsTestSandbox(t, Config{Workspace: workspacePath, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	targetPath := filepath.Join(workspacePath, "internal-target")
	if err := os.Mkdir(targetPath, 0o700); err != nil {
		t.Fatalf("create internal reparse target: %v", err)
	}
	linkPath := filepath.Join(workspacePath, "internal-link")
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Fatalf("create internal reparse-point fixture: %v", err)
	}
	_, err := sandboxValue.Exec(context.Background(), Command{
		Path: canonicalWindowsSystemExecutable(t, "cmd.exe"),
		Args: []string{"/d", "/c", "exit", "0"},
	})
	if err == nil || !strings.Contains(err.Error(), "reparse point") {
		t.Fatalf("internal reparse-point Exec error = %v, want rejection", err)
	}
}

func TestWindowsWorkspaceRejectsJunction(t *testing.T) {
	workspacePath := t.TempDir()
	sandboxValue := newWindowsTestSandbox(t, Config{Workspace: workspacePath, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	externalDirectory := t.TempDir()
	junctionPath := filepath.Join(workspacePath, "outside-junction")
	command := exec.CommandContext(
		context.Background(),
		canonicalWindowsSystemExecutable(t, "cmd.exe"),
		"/d",
		"/c",
		"mklink",
		"/J",
		junctionPath,
		externalDirectory,
	) // #nosec G204 -- 仅用于创建固定的 Windows junction 测试夹具。
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create Windows junction fixture: %v, output=%s", err, output)
	}
	_, err := sandboxValue.Exec(context.Background(), Command{
		Path: canonicalWindowsSystemExecutable(t, "cmd.exe"),
		Args: []string{"/d", "/c", "exit", "0"},
	})
	if err == nil || !strings.Contains(err.Error(), "reparse point") {
		t.Fatalf("junction Exec error = %v, want rejection", err)
	}
}

func TestWindowsValidatePathRejectsAbsoluteADS(t *testing.T) {
	if err := validateWindowsPath(`C:\Users\test\file.txt:hidden`); err == nil {
		t.Fatal("absolute alternate data stream path was accepted")
	}
}

func TestWindowsDeniedPathsRejectsAccessibleDescendant(t *testing.T) {
	workspacePath := t.TempDir()
	sandboxValue := newWindowsTestSandbox(t, Config{Workspace: workspacePath, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	windowsValue := windowsBackendForTest(t, sandboxValue)

	deniedRoot := t.TempDir()
	childPath := filepath.Join(deniedRoot, "allowed-child.txt")
	if err := os.WriteFile(childPath, []byte("child"), 0o600); err != nil {
		t.Fatalf("create denied-path child fixture: %v", err)
	}
	child, err := os.Open(childPath)
	if err != nil {
		t.Fatalf("open denied-path child fixture: %v", err)
	}
	if aclErr := setPersistentWindowsWorkspaceACL(
		child,
		windowsValue.workspace.ownerSID,
		windowsValue.workspace.appContainerSID,
	); aclErr != nil {
		_ = child.Close()
		t.Fatalf("grant child AppContainer fixture access: %v", aclErr)
	}
	if closeErr := child.Close(); closeErr != nil {
		t.Fatalf("close denied-path child fixture: %v", closeErr)
	}

	_, err = New(Config{
		Workspace:            workspacePath,
		DeniedPaths:          []string{deniedRoot},
		RequiredCapabilities: UntrustedCodeIsolationCapabilities,
	})
	if err == nil || !strings.Contains(err.Error(), "allowed-child.txt") {
		t.Fatalf("DeniedPaths descendant audit error = %v, want child-specific rejection", err)
	}
}
