//go:build windows && windows_security

package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsApplicationControlPolicyIsNotBypassed(t *testing.T) {
	executableFixture := requireWindowsPolicyFixture(t, "TOOLKIT_WINDOWS_POLICY_BLOCKED_EXE")
	scriptFixture := requireWindowsPolicyFixture(t, "TOOLKIT_WINDOWS_POLICY_BLOCKED_SCRIPT")

	workspacePath := t.TempDir()
	sandboxValue := newWindowsTestSandbox(t, Config{
		Workspace:            workspacePath,
		Timeout:              15,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities,
	})
	windowsValue := windowsBackendForTest(t, sandboxValue)

	t.Run("executable", func(t *testing.T) {
		allowedPath := copyWindowsPolicyFixture(t, executableFixture, workspacePath, "toolkit-policy-allowed.exe")
		blockedPath := copyWindowsPolicyFixture(t, executableFixture, workspacePath, "toolkit-policy-blocked.exe")
		if err := windowsValue.workspace.auditAndAuthorizeTree(); err != nil {
			t.Fatalf("authorize Windows executable policy fixtures: %v", err)
		}
		assertWindowsCommandAllowed(t, sandboxValue, Command{Path: allowedPath}, "")
		assertWindowsEffectivePolicyDenied(t, blockedPath)
		assertWindowsApplicationControlDenied(t, sandboxValue, Command{Path: blockedPath})
	})

	t.Run("PowerShell script", func(t *testing.T) {
		allowedPath := copyWindowsPolicyFixture(t, scriptFixture, workspacePath, "toolkit-policy-allowed.ps1")
		blockedPath := copyWindowsPolicyFixture(t, scriptFixture, workspacePath, "toolkit-policy-blocked.ps1")
		if err := windowsValue.workspace.auditAndAuthorizeTree(); err != nil {
			t.Fatalf("authorize Windows script policy fixtures: %v", err)
		}
		powerShellPath := canonicalWindowsSystemExecutable(t, "WindowsPowerShell\\v1.0\\powershell.exe")
		commandFor := func(scriptPath string) Command {
			return Command{
				Path: powerShellPath,
				Args: []string{
					"-NoLogo",
					"-NoProfile",
					"-NonInteractive",
					"-ExecutionPolicy",
					"Bypass",
					"-File",
					scriptPath,
				},
			}
		}
		assertWindowsCommandAllowed(t, sandboxValue, commandFor(allowedPath), "TOOLKIT_POLICY_EXECUTED")
		assertWindowsEffectivePolicyDenied(t, blockedPath)
		assertWindowsApplicationControlDenied(t, sandboxValue, commandFor(blockedPath))
	})
}

func requireWindowsPolicyFixture(t *testing.T, name string) string {
	t.Helper()
	path := os.Getenv(name)
	if path == "" {
		t.Fatalf("required Windows application-control fixture is not configured: %s", name)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("inspect Windows application-control fixture %s: %v", name, err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("Windows application-control fixture %s is not a regular file", name)
	}
	return path
}

func copyWindowsPolicyFixture(t *testing.T, sourcePath, workspacePath, name string) string {
	t.Helper()
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read Windows application-control fixture: %v", err)
	}
	destinationPath := filepath.Join(workspacePath, name)
	if err := os.WriteFile(destinationPath, content, 0o700); err != nil {
		t.Fatalf("write Windows application-control fixture: %v", err)
	}
	file, err := os.Open(destinationPath)
	if err != nil {
		t.Fatalf("open Windows application-control fixture: %v", err)
	}
	defer file.Close()
	canonicalPath, err := canonicalWindowsPathFromHandle(file)
	if err != nil {
		t.Fatalf("canonicalize Windows application-control fixture: %v", err)
	}
	return canonicalPath
}

func assertWindowsEffectivePolicyDenied(t *testing.T, path string) {
	t.Helper()
	powerShellPath := canonicalWindowsSystemExecutable(t, "WindowsPowerShell\\v1.0\\powershell.exe")
	command := exec.Command(
		powerShellPath,
		"-NoLogo",
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		"$decision=(Get-AppLockerPolicy -Effective | Test-AppLockerPolicy -Path $args[0] -User 'Everyone').PolicyDecision; [Console]::Out.Write($decision)",
		path,
	) // #nosec G204 -- 路径作为独立参数传入固定的策略查询脚本。
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("query effective Windows application-control policy: %v, output=%s", err, output)
	}
	if decision := strings.TrimSpace(string(output)); !strings.EqualFold(decision, "Denied") {
		t.Fatalf("Windows application-control decision for %q = %q, want Denied", path, decision)
	}
}

func assertWindowsCommandAllowed(t *testing.T, sandboxValue Sandbox, command Command, requiredOutput string) {
	t.Helper()
	result, err := sandboxValue.Exec(context.Background(), command)
	if err != nil {
		t.Fatalf("execute allowed Windows application-control fixture: %v", err)
	}
	if result == nil || result.ExitCode != 0 {
		t.Fatalf("allowed Windows application-control fixture result = %+v, want exit code 0", result)
	}
	if requiredOutput != "" && !strings.Contains(result.Stdout, requiredOutput) {
		t.Fatalf("allowed Windows application-control fixture stdout = %q, want %q", result.Stdout, requiredOutput)
	}
	if requiredOutput == "" && strings.TrimSpace(result.Stdout) == "" {
		t.Fatal("allowed Windows executable fixture produced no output")
	}
}

func assertWindowsApplicationControlDenied(t *testing.T, sandboxValue Sandbox, command Command) {
	t.Helper()
	result, execErr := sandboxValue.Exec(context.Background(), command)
	if execErr == nil && result != nil && result.ExitCode == 0 {
		t.Fatal("Windows application-control policy was bypassed")
	}
	if result == nil && execErr == nil {
		t.Fatal("Windows application-control denial returned no result or error")
	}
	detail := fmt.Sprint(execErr)
	if result != nil {
		detail += " stdout=" + result.Stdout + " stderr=" + result.Stderr
	}
	if strings.Contains(detail, "TOOLKIT_POLICY_EXECUTED") {
		t.Fatalf("blocked Windows application-control fixture executed: %s", detail)
	}
}
