//go:build windows

package sandbox

import (
	"fmt"
	"os"
	"strings"
)

// Phase 8 D34: Security hardening + escape prevention

// cleanWindowsEnv 构造沙箱进程的最小环境，并创建隔离的临时目录。
func cleanWindowsEnv(workspace string) ([]string, error) {
	dangerousVars := map[string]bool{
		"COMSPEC":                true,
		"PSMODULEPATH":           true,
		"PROCESSOR_ARCHITECTURE": true,
		"USERNAME":               true,
		"USERDOMAIN":             true,
		"APPDATA":                true,
		"LOCALAPPDATA":           true,
		"USERPROFILE":            true,
		"HOMEPATH":               true,
		"HOMEDRIVE":              true,
	}

	// 仅保留运行时启动所需的系统变量。
	var clean []string
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToUpper(parts[0])
		if dangerousVars[key] {
			continue
		}
		if key == "PATH" || key == "SYSTEMROOT" || key == "SYSTEMDRIVE" || key == "WINDIR" {
			clean = append(clean, env)
		}
	}

	temporaryDir := workspace + "\\_tmp"
	appDataDir := workspace + "\\_appdata"
	localAppDataDir := workspace + "\\_localappdata"
	for _, dir := range []string{temporaryDir, appDataDir, localAppDataDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create sandbox environment directory %q: %w", dir, err)
		}
	}

	// 用工作区内目录覆盖用户身份与临时目录变量。
	clean = append(clean,
		"TEMP="+temporaryDir,
		"TMP="+temporaryDir,
		"USERPROFILE="+workspace,
		"HOMEPATH="+workspace,
		"APPDATA="+appDataDir,
		"LOCALAPPDATA="+localAppDataDir,
	)
	return clean, nil
}

// validateWindowsEscapeVectors checks for common escape attempts in command/args.
func validateWindowsEscapeVectors(command string, args []string) error {
	all := append([]string{command}, args...)
	for _, s := range all {
		if err := validateWindowsPath(s); err != nil {
			return err
		}
		// Block PowerShell invocation
		lower := strings.ToLower(s)
		if strings.Contains(lower, "powershell") || strings.Contains(lower, "pwsh") {
			return fmt.Errorf("PowerShell invocation blocked: %s", s)
		}
		// Block cmd.exe invocation
		if strings.Contains(lower, "cmd.exe") || strings.Contains(lower, "cmd /") {
			return fmt.Errorf("cmd.exe invocation blocked: %s", s)
		}
	}
	return nil
}
