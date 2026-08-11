//go:build !windows

package sandbox

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const posixRuntimeDirectory = ".sandbox-runtime"

var posixPreservedEnvironmentNames = []string{
	"LANG",
	"LC_ALL",
	"LC_CTYPE",
	"LC_COLLATE",
	"LC_MESSAGES",
	"LC_MONETARY",
	"LC_NUMERIC",
	"LC_TIME",
	"TERM",
	"COLORTERM",
	"NO_COLOR",
	"TZ",
	"__CF_USER_TEXT_ENCODING",
	"SYSTEM_VERSION_COMPAT",
}

func buildPOSIXSandboxEnv(workspace string, hostEnv []string, executablePath string) ([]string, error) {
	if executablePath == "" {
		executablePath = defaultPOSIXSandboxPath()
	}
	if err := initializePOSIXRuntimeDirectories(workspace); err != nil {
		return nil, err
	}

	base := filepath.Join(workspace, posixRuntimeDirectory)
	home := filepath.Join(base, "home")
	temporary := filepath.Join(base, "tmp")
	cache := filepath.Join(base, "cache")
	config := filepath.Join(base, "config")
	data := filepath.Join(base, "data")
	state := filepath.Join(base, "state")
	goRoot := filepath.Join(base, "go")

	hostValues := make(map[string]string, len(posixPreservedEnvironmentNames))
	for _, item := range hostEnv {
		name, value, ok := strings.Cut(item, "=")
		if ok && preservePOSIXEnvironment(name) {
			hostValues[name] = value
		}
	}
	result := make([]string, 0, len(hostValues)+24)
	for _, name := range posixPreservedEnvironmentNames {
		if value, exists := hostValues[name]; exists {
			result = append(result, name+"="+value)
		}
	}
	result = append(result,
		"PATH="+executablePath,
		"HOME="+home,
		"TMPDIR="+temporary,
		"TMP="+temporary,
		"TEMP="+temporary,
		"XDG_CACHE_HOME="+cache,
		"XDG_CONFIG_HOME="+config,
		"XDG_DATA_HOME="+data,
		"XDG_STATE_HOME="+state,
		"CFFIXED_USER_HOME="+home,
		"PYTHONPYCACHEPREFIX="+filepath.Join(cache, "python"),
		"PYTHON_HISTORY="+filepath.Join(state, "python_history"),
		"NODE_REPL_HISTORY="+filepath.Join(state, "node_repl_history"),
		"HISTFILE=/dev/null",
		"GOPATH="+filepath.Join(goRoot, "path"),
		"GOCACHE="+filepath.Join(goRoot, "cache"),
		"GOMODCACHE="+filepath.Join(goRoot, "mod"),
		"GOTMPDIR="+filepath.Join(goRoot, "tmp"),
		"GOENV="+filepath.Join(goRoot, "env"),
		"GOWORK=off",
		"GOTOOLCHAIN=local",
		"NPM_CONFIG_CACHE="+filepath.Join(cache, "npm"),
		"NPM_CONFIG_USERCONFIG="+filepath.Join(config, "npmrc"),
		"NPM_CONFIG_PREFIX="+filepath.Join(base, "npm"),
		"COREPACK_HOME="+filepath.Join(cache, "corepack"),
		"YARN_CACHE_FOLDER="+filepath.Join(cache, "yarn"),
		"PIP_CACHE_DIR="+filepath.Join(cache, "pip"),
		"PIP_CONFIG_FILE=/dev/null",
	)
	return result, nil
}

func initializePOSIXRuntimeDirectories(workspace string) error {
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return fmt.Errorf("open sandbox workspace for runtime environment: %w", err)
	}
	defer root.Close() //nolint:errcheck // 只读目录描述符的关闭错误不影响已完成的初始化。

	for _, relative := range posixRuntimeRelativeDirectories() {
		if err := root.MkdirAll(relative, 0o700); err != nil {
			return fmt.Errorf("create sandbox runtime directory %q: %w", relative, err)
		}
		info, err := root.Stat(relative)
		if err != nil {
			return fmt.Errorf("inspect sandbox runtime directory %q: %w", relative, err)
		}
		if !info.IsDir() || info.Mode().Perm() != 0o700 {
			return fmt.Errorf("sandbox runtime directory %q must be a private directory with mode 0700", relative)
		}
	}
	return nil
}

func posixRuntimeRelativeDirectories() []string {
	base := posixRuntimeDirectory
	return []string{
		base,
		filepath.Join(base, "home"),
		filepath.Join(base, "tmp"),
		filepath.Join(base, "cache"),
		filepath.Join(base, "cache", "npm"),
		filepath.Join(base, "cache", "pip"),
		filepath.Join(base, "cache", "corepack"),
		filepath.Join(base, "cache", "yarn"),
		filepath.Join(base, "config"),
		filepath.Join(base, "data"),
		filepath.Join(base, "state"),
		filepath.Join(base, "go"),
		filepath.Join(base, "go", "path"),
		filepath.Join(base, "go", "cache"),
		filepath.Join(base, "go", "mod"),
		filepath.Join(base, "go", "tmp"),
		filepath.Join(base, "npm"),
	}
}

func preservePOSIXEnvironment(name string) bool {
	for _, allowed := range posixPreservedEnvironmentNames {
		if name == allowed {
			return true
		}
	}
	return false
}

func defaultPOSIXSandboxPath() string {
	return strings.Join([]string{"/usr/local/bin", "/usr/bin", "/bin", "/usr/local/sbin", "/usr/sbin", "/sbin"}, string(os.PathListSeparator))
}

func cleanBasicEnv(workspace string, env []string) ([]string, error) {
	return buildPOSIXSandboxEnv(workspace, env, defaultPOSIXSandboxPath())
}

func resolvePOSIXCommandExecutable(path, dir string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("sandbox command path is required")
	}
	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	return freezePOSIXExecutable(candidate)
}

func resolvePOSIXShebangExecutable(path, dir string, env []string) (string, error) {
	if filepath.IsAbs(path) || strings.ContainsRune(path, filepath.Separator) {
		return resolvePOSIXCommandExecutable(path, dir)
	}
	searchPath := posixEnvironmentValue(env, "PATH")
	for _, directory := range filepath.SplitList(searchPath) {
		if !filepath.IsAbs(directory) {
			continue
		}
		resolved, err := freezePOSIXExecutable(filepath.Join(directory, path))
		if err == nil {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("resolve sandbox interpreter %q", path)
}

func freezePOSIXExecutable(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve sandbox executable %q: %w", path, err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve sandbox executable %q: %w", path, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect sandbox executable %q: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("sandbox executable %q must be a regular executable file", path)
	}
	return filepath.Clean(resolved), nil
}

func posixEnvironmentValue(env []string, name string) string {
	prefix := name + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix)
		}
	}
	return ""
}

type posixShebangFile interface {
	Read([]byte) (int, error)
	Stat() (os.FileInfo, error)
	Close() error
}

func inspectPOSIXShebangCommands(script string) ([]string, error) {
	return inspectPOSIXShebangCommandsWithOpen(script, openPOSIXShebangFile)
}

func openPOSIXShebangFile(path string) (posixShebangFile, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func inspectPOSIXShebangCommandsWithOpen(
	script string,
	openFile func(string) (posixShebangFile, error),
) ([]string, error) {
	if openFile == nil {
		return nil, fmt.Errorf("POSIX shebang opener is unavailable")
	}
	file, err := openFile(script)
	if err != nil {
		return nil, fmt.Errorf("open POSIX shebang %q: %w", script, err)
	}
	openedBefore, statErr := file.Stat()
	if statErr != nil {
		return nil, errors.Join(
			fmt.Errorf("inspect opened POSIX shebang %q: %w", script, statErr),
			closePOSIXShebangFile(script, file),
		)
	}
	buffer := make([]byte, 4096)
	n, readErr := file.Read(buffer)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, errors.Join(
			fmt.Errorf("read POSIX shebang %q: %w", script, readErr),
			closePOSIXShebangFile(script, file),
		)
	}
	openedAfter, statErr := file.Stat()
	closeErr := closePOSIXShebangFile(script, file)
	if statErr != nil || closeErr != nil {
		return nil, errors.Join(
			wrapPOSIXShebangError("inspect opened POSIX shebang", script, statErr),
			closeErr,
		)
	}
	pathInfo, err := os.Lstat(script)
	if err != nil {
		return nil, fmt.Errorf("reinspect POSIX shebang %q: %w", script, err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 ||
		!samePOSIXShebangFileInfo(openedBefore, openedAfter) ||
		!samePOSIXShebangFileInfo(openedBefore, pathInfo) {
		return nil, fmt.Errorf("POSIX shebang %q changed during inspection", script)
	}
	if n < 2 || string(buffer[:2]) != "#!" {
		return nil, nil
	}
	line, _, _ := strings.Cut(string(buffer[2:n]), "\n")
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return nil, nil
	}
	commands := []string{fields[0]}
	if filepath.Base(fields[0]) != "env" {
		return commands, nil
	}
	for _, field := range fields[1:] {
		if strings.HasPrefix(field, "-") || strings.ContainsRune(field, '=') {
			continue
		}
		return append(commands, field), nil
	}
	return commands, nil
}

func samePOSIXShebangFileInfo(left, right os.FileInfo) bool {
	return left != nil && right != nil &&
		os.SameFile(left, right) &&
		left.Mode() == right.Mode() &&
		left.Size() == right.Size() &&
		left.ModTime().Equal(right.ModTime())
}

func closePOSIXShebangFile(script string, file posixShebangFile) error {
	if err := file.Close(); err != nil {
		return fmt.Errorf("close POSIX shebang %q: %w", script, err)
	}
	return nil
}

func wrapPOSIXShebangError(action, script string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s %q: %w", action, script, err)
}

// readPOSIXShebangCommands 由 Linux 构建使用；Darwin 直接消费带错误通道的严格版本。
func readPOSIXShebangCommands(script string) []string { //nolint:unused // Darwin 构建看不到 Linux 调用点。
	commands, err := inspectPOSIXShebangCommands(script)
	if err != nil {
		// Linux 调用方当前没有独立错误通道；返回不可解析解释器令其在规划阶段失败关闭。
		return []string{"sandbox-shebang-inspection-failed"}
	}
	return commands
}
