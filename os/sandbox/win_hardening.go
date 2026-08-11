//go:build windows

package sandbox

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/windows"
)

// cleanWindowsEnv 仅使用内核解析出的系统目录、已冻结的可执行目录和工作区目录。
// 不继承宿主 PATH、用户配置、凭据或任意环境变量。
func cleanWindowsEnv(workspace *windowsWorkspace, applicationName string) ([]string, error) {
	if workspace == nil || workspace.root == nil {
		return nil, fmt.Errorf("Windows workspace is not initialized")
	}
	systemDirectory, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, fmt.Errorf("resolve Windows system directory: %w", err)
	}
	windowsDirectory, err := windows.GetWindowsDirectory()
	if err != nil {
		return nil, fmt.Errorf("resolve Windows directory: %w", err)
	}

	executableDirectory := filepath.Dir(applicationName)
	pathEntries := []string{executableDirectory}
	if !strings.EqualFold(filepath.Clean(executableDirectory), filepath.Clean(systemDirectory)) {
		pathEntries = append(pathEntries, systemDirectory)
	}
	workspacePath := workspace.canonicalPath
	volume := filepath.VolumeName(workspacePath)
	homePath := strings.TrimPrefix(workspacePath, volume)
	if homePath == "" {
		homePath = `\`
	}

	return []string{
		"PATH=" + strings.Join(pathEntries, ";"),
		"PATHEXT=.COM;.EXE;.BAT;.CMD",
		"SYSTEMROOT=" + windowsDirectory,
		"WINDIR=" + windowsDirectory,
		"SYSTEMDRIVE=" + filepath.VolumeName(windowsDirectory),
		"TEMP=" + filepath.Join(workspacePath, "_tmp"),
		"TMP=" + filepath.Join(workspacePath, "_tmp"),
		"USERPROFILE=" + workspacePath,
		"HOMEDRIVE=" + volume,
		"HOMEPATH=" + homePath,
		"APPDATA=" + filepath.Join(workspacePath, "_appdata"),
		"LOCALAPPDATA=" + filepath.Join(workspacePath, "_localappdata"),
		"GOCACHE=" + filepath.Join(workspacePath, "_gocache"),
		"GOMODCACHE=" + filepath.Join(workspacePath, "_gomodcache"),
		"GOPATH=" + filepath.Join(workspacePath, "_gopath"),
		"GOENV=off",
		"GOTOOLCHAIN=local",
	}, nil
}

// validateWindowsCommandEnv 将 Command.Env 视为完整环境块，只校验显式条目，绝不合并宿主环境。
func validateWindowsCommandEnv(workspace *windowsWorkspace, env []string) ([]string, error) {
	clean := append([]string(nil), env...)
	seen := make(map[string]struct{}, len(clean))
	trustedRoots, trustedRootsErr := trustedWindowsExecutableRoots()
	windowsDirectory, windowsDirectoryErr := windows.GetWindowsDirectory()
	for _, entry := range clean {
		if strings.ContainsAny(entry, "\x00\r\n") {
			return nil, fmt.Errorf("windows Command.Env contains unsupported characters")
		}
		separator := strings.IndexByte(entry, '=')
		if separator <= 0 {
			return nil, fmt.Errorf("windows Command.Env entry must use KEY=VALUE form")
		}
		key := strings.ToUpper(entry[:separator])
		value := entry[separator+1:]
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("windows Command.Env contains duplicate key %s", key)
		}
		seen[key] = struct{}{}

		switch key {
		case "TEMP", "TMP", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "GOCACHE", "GOMODCACHE", "GOPATH":
			if err := validateWindowsPath(value); err != nil {
				return nil, fmt.Errorf("windows Command.Env %s contains an unsupported path: %w", key, err)
			}
			if !filepath.IsAbs(value) || !windowsPathWithin(workspace.canonicalPath, value) {
				return nil, fmt.Errorf("windows Command.Env %s must remain inside the sandbox workspace", key)
			}
		case "PATH":
			if trustedRootsErr != nil {
				return nil, trustedRootsErr
			}
			for _, pathEntry := range filepath.SplitList(value) {
				if err := validateWindowsPath(pathEntry); err != nil {
					return nil, fmt.Errorf("windows Command.Env PATH contains an unsupported path: %w", err)
				}
				if pathEntry == "" || !filepath.IsAbs(pathEntry) {
					return nil, fmt.Errorf("windows Command.Env PATH entries must be absolute")
				}
				if !windowsPathWithin(workspace.canonicalPath, pathEntry) && !windowsPathWithinAny(trustedRoots, pathEntry) {
					return nil, fmt.Errorf("windows Command.Env PATH entry is outside trusted roots: %s", pathEntry)
				}
			}
		case "SYSTEMROOT", "WINDIR":
			if windowsDirectoryErr != nil {
				return nil, windowsDirectoryErr
			}
			if err := validateWindowsPath(value); err != nil {
				return nil, fmt.Errorf("windows Command.Env %s contains an unsupported path: %w", key, err)
			}
			if !strings.EqualFold(filepath.Clean(value), filepath.Clean(windowsDirectory)) {
				return nil, fmt.Errorf("windows Command.Env %s must use the Windows directory", key)
			}
		case "SYSTEMDRIVE":
			if windowsDirectoryErr != nil {
				return nil, windowsDirectoryErr
			}
			if !strings.EqualFold(value, filepath.VolumeName(windowsDirectory)) {
				return nil, fmt.Errorf("windows Command.Env SYSTEMDRIVE is invalid")
			}
		case "COMSPEC":
			if trustedRootsErr != nil {
				return nil, trustedRootsErr
			}
			if err := validateWindowsPath(value); err != nil {
				return nil, fmt.Errorf("windows Command.Env COMSPEC contains an unsupported path: %w", err)
			}
			if !filepath.IsAbs(value) || !windowsPathWithinAny(trustedRoots, value) {
				return nil, fmt.Errorf("windows Command.Env COMSPEC is outside trusted roots")
			}
		}
	}
	sort.Slice(clean, func(first, second int) bool {
		return strings.ToUpper(clean[first]) < strings.ToUpper(clean[second])
	})
	return clean, nil
}
