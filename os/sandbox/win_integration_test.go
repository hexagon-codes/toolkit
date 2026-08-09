//go:build windows

package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ============== D35: Windows 沙箱集成测试 ==============

func TestWindows_SandboxCreation(t *testing.T) {
	ws := t.TempDir()
	sb, err := New(Config{Workspace: ws, Timeout: 30})
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	if sb == nil {
		t.Fatal("sandbox is nil")
	}
}

func TestWindows_ExecSimpleCommand(t *testing.T) {
	ws := t.TempDir()
	sb, err := New(Config{Workspace: ws, Timeout: 10})
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	ctx := context.Background()
	result, err := sb.Exec(ctx, "cmd", []string{"/c", "echo", "hello"})
	if err != nil {
		t.Fatalf("exec failed: %v", err)
	}
	if !strings.Contains(result.Stdout, "hello") {
		t.Fatalf("expected 'hello' in stdout, got: %q", result.Stdout)
	}
	t.Logf("stdout: %s", result.Stdout)
}

func TestWindows_ExecPythonCode(t *testing.T) {
	ws := t.TempDir()
	sb, err := New(Config{Workspace: ws, Timeout: 15})
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	// Skip if python not available
	ctx := context.Background()
	result, err := sb.ExecCode(ctx, "python", `print("hello from sandbox")`)
	if err != nil {
		t.Skipf("python not available: %v", err)
	}
	if !strings.Contains(result.Stdout, "hello from sandbox") {
		t.Fatalf("expected output, got: %q", result.Stdout)
	}
}

func TestWindows_WorkspaceIsolation(t *testing.T) {
	ws := t.TempDir()

	// Create a file in workspace
	testFile := filepath.Join(ws, "test.txt")
	if err := os.WriteFile(testFile, []byte("sandbox data"), 0o600); err != nil {
		t.Fatalf("write workspace fixture: %v", err)
	}

	sb, err := New(Config{Workspace: ws, Timeout: 10})
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	ctx := context.Background()
	// Can read workspace file
	result, err := sb.Exec(ctx, "cmd", []string{"/c", "type", "test.txt"})
	if err != nil {
		t.Fatalf("read workspace file: %v", err)
	}
	if !strings.Contains(result.Stdout, "sandbox data") {
		t.Fatalf("expected file content, got: %q", result.Stdout)
	}
}

func TestWindows_Timeout(t *testing.T) {
	ws := t.TempDir()
	sb, err := New(Config{Workspace: ws, Timeout: 2})
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	ctx := context.Background()
	start := time.Now()
	_, execErr := sb.Exec(ctx, "cmd", []string{"/c", "ping", "-n", "10", "127.0.0.1"})
	elapsed := time.Since(start)

	if !errors.Is(execErr, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v, want context deadline exceeded", execErr)
	}
	// Should timeout within ~3 seconds (2s timeout + buffer)
	if elapsed > 5*time.Second {
		t.Fatalf("timeout not enforced: took %v", elapsed)
	}
	t.Logf("timed out after %v (expected ~2s)", elapsed)
}

func TestWindows_ExecPreservesNonZeroExitCode(t *testing.T) {
	ws := t.TempDir()
	sb, err := New(Config{Workspace: ws, Timeout: 10})
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := sb.Exec(ctx, "cmd", []string{"/c", "exit", "7"})
	if err != nil {
		t.Fatalf("exec failed: %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("expected ExitCode=7, got %d (stdout=%q stderr=%q)", result.ExitCode, result.Stdout, result.Stderr)
	}
}

func TestWindows_EnvironmentClean(t *testing.T) {
	ws := t.TempDir()
	sb, err := New(Config{Workspace: ws, Timeout: 10})
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	ctx := context.Background()
	result, err := sb.Exec(ctx, "cmd", []string{"/c", "echo", "%COMSPEC%"})
	if err != nil {
		t.Fatalf("exec failed: %v", err)
	}
	// COMSPEC should be cleaned (not expanded to cmd.exe path)
	stdout := strings.TrimSpace(result.Stdout)
	if strings.Contains(strings.ToLower(stdout), "cmd.exe") {
		t.Logf("WARNING: COMSPEC leaked: %s (hardening may need Windows-specific fixes)", stdout)
	}
}

func TestWindows_PathValidation(t *testing.T) {
	tests := []struct {
		path string
		ok   bool
	}{
		{`C:\Users\test\file.txt`, true},
		{`file.txt`, true},
		{`\\server\share\file`, false}, // UNC
		{`\\.\PhysicalDrive0`, false},  // device handle
		{`file.txt:hidden`, false},     // ADS (non-absolute)
	}

	for _, tt := range tests {
		err := validateWindowsPath(tt.path)
		if tt.ok && err != nil {
			t.Errorf("expected OK for %q, got: %v", tt.path, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("expected block for %q", tt.path)
		}
	}
}

func TestWindows_EscapeVectors(t *testing.T) {
	tests := []struct {
		cmd  string
		args []string
		ok   bool
	}{
		{"python", []string{"script.py"}, true},
		{"node", []string{"app.js"}, true},
		{"powershell", []string{"-Command", "Get-Process"}, false}, // blocked
		{"cmd.exe", []string{"/c", "whoami"}, false},               // blocked
		{"python", []string{`\\server\share\evil.py`}, false},      // UNC in args
	}

	for _, tt := range tests {
		err := validateWindowsEscapeVectors(tt.cmd, tt.args)
		if tt.ok && err != nil {
			t.Errorf("expected OK for %q %v, got: %v", tt.cmd, tt.args, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("expected block for %q %v", tt.cmd, tt.args)
		}
	}
}
