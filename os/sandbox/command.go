package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Command 描述一次无需 shell 拼接的结构化进程执行请求。
type Command struct {
	Path string
	Args []string
	// Dir 为空时使用 Sandbox 的 Workspace；非空目录必须解析后位于 Workspace 内。
	Dir string
	// Env 为 nil 时使用平台最小安全环境；非 nil 时表示完整环境而非增量覆盖。
	Env []string

	workspaceIdentity sandboxPathIdentity
	directoryIdentity sandboxPathIdentity
}

// sandboxPathIdentity 绑定校验时看到的原始绝对路径、规范路径和文件对象。
// 平台后端应在进入不可逆启动边界前调用 revalidateSandboxExecutionPaths；
// 需要更强保证的平台可以在此基础上冻结句柄或文件描述符。
type sandboxPathIdentity struct {
	path      string
	canonical string
	info      os.FileInfo
}

func snapshotSandboxDirectoryIdentity(field, path string) (sandboxPathIdentity, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return sandboxPathIdentity{}, fmt.Errorf("inspect sandbox %s: %w", field, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return sandboxPathIdentity{}, fmt.Errorf("sandbox %s must not be a symbolic link", field)
	}
	if !info.IsDir() {
		return sandboxPathIdentity{}, fmt.Errorf("sandbox %s must be a directory", field)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return sandboxPathIdentity{}, fmt.Errorf("resolve sandbox %s: %w", field, err)
	}
	return sandboxPathIdentity{
		path:      filepath.Clean(path),
		canonical: filepath.Clean(canonical),
		info:      info,
	}, nil
}

func (identity sandboxPathIdentity) revalidate(field string) error {
	if identity.path == "" || identity.info == nil {
		return fmt.Errorf("sandbox %s identity is unavailable", field)
	}
	current, err := os.Lstat(identity.path)
	if err != nil {
		return fmt.Errorf("revalidate sandbox %s: %w", field, err)
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.IsDir() {
		return fmt.Errorf("sandbox %s identity changed", field)
	}
	if !os.SameFile(identity.info, current) {
		return fmt.Errorf("sandbox %s identity changed", field)
	}
	// NTFS 的文件索引可能被新目录复用，SameFile 不足以证明身份未替换；
	// 补充修改时间与大小对比（目录替换必然产生差异）。
	if !identity.info.ModTime().Equal(current.ModTime()) || identity.info.Size() != current.Size() {
		return fmt.Errorf("sandbox %s identity changed", field)
	}
	canonical, err := filepath.EvalSymlinks(identity.path)
	if err != nil {
		return fmt.Errorf("revalidate sandbox %s path: %w", field, err)
	}
	if !sameSandboxPath(identity.canonical, canonical) {
		return fmt.Errorf("sandbox %s identity changed", field)
	}
	return nil
}

func prepareSandboxCommand(cfg Config, requested Command, defaultEnvironment func() ([]string, error)) (Command, error) {
	if requested.Path == "" {
		return Command{}, fmt.Errorf("sandbox command path is required")
	}
	if strings.IndexByte(requested.Path, 0) >= 0 {
		return Command{}, fmt.Errorf("sandbox command path contains NUL")
	}
	prepared, err := snapshotSandboxCommandPaths(cfg, requested)
	if err != nil {
		return Command{}, err
	}
	for index, argument := range prepared.Args {
		if strings.IndexByte(argument, 0) >= 0 {
			return Command{}, fmt.Errorf("sandbox command argument at index %d contains NUL", index)
		}
	}

	if requested.Env == nil {
		if defaultEnvironment == nil {
			return Command{}, fmt.Errorf("sandbox default environment is unavailable")
		}
		prepared.Env, err = defaultEnvironment()
		if err != nil {
			return Command{}, err
		}
	} else {
		prepared.Env = make([]string, len(requested.Env))
		copy(prepared.Env, requested.Env)
	}
	if err := validateSandboxEnvironment(prepared.Env); err != nil {
		return Command{}, err
	}
	return prepared, nil
}

func snapshotSandboxCommandPaths(cfg Config, requested Command) (Command, error) {
	workspaceIdentity := cfg.workspaceIdentity
	if workspaceIdentity.info == nil {
		var err error
		workspaceIdentity, err = snapshotSandboxDirectoryIdentity("workspace", cfg.Workspace)
		if err != nil {
			return Command{}, err
		}
	}
	if err := workspaceIdentity.revalidate("workspace"); err != nil {
		return Command{}, err
	}
	directoryIdentity, err := resolveSandboxCommandDirectory(workspaceIdentity, requested.Dir)
	if err != nil {
		return Command{}, err
	}
	return Command{
		Path:              requested.Path,
		Args:              append([]string(nil), requested.Args...),
		Dir:               directoryIdentity.path,
		Env:               appendSandboxEnvironment(requested.Env),
		workspaceIdentity: workspaceIdentity,
		directoryIdentity: directoryIdentity,
	}, nil
}

func appendSandboxEnvironment(environment []string) []string {
	if environment == nil {
		return nil
	}
	return append([]string(nil), environment...)
}

// revalidateSandboxExecutionPaths 在平台进入载荷启动边界前复核工作区和工作目录对象。
func revalidateSandboxExecutionPaths(command Command) error {
	if err := command.workspaceIdentity.revalidate("workspace"); err != nil {
		return err
	}
	if err := command.directoryIdentity.revalidate("command directory"); err != nil {
		return err
	}
	if !sandboxPathWithin(command.workspaceIdentity.canonical, command.directoryIdentity.canonical) {
		return fmt.Errorf("sandbox command directory must be within the workspace")
	}
	return nil
}

func resolveSandboxCommandDirectory(workspace sandboxPathIdentity, requested string) (sandboxPathIdentity, error) {
	if requested == "" {
		return workspace, nil
	}
	path := requested
	if !filepath.IsAbs(path) {
		path = filepath.Join(workspace.path, path)
	}
	absolute, err := absoluteSandboxPath("command directory", path)
	if err != nil {
		return sandboxPathIdentity{}, err
	}
	identity, err := snapshotSandboxDirectoryIdentity("command directory", absolute)
	if err != nil {
		return sandboxPathIdentity{}, err
	}
	if !sandboxPathWithin(workspace.canonical, identity.canonical) {
		return sandboxPathIdentity{}, fmt.Errorf("sandbox command directory must be within the workspace")
	}
	return identity, nil
}

func validateSandboxEnvironment(env []string) error {
	seen := make(map[string]struct{}, len(env))
	for index, item := range env {
		if strings.IndexByte(item, 0) >= 0 {
			return fmt.Errorf("sandbox environment entry at index %d contains NUL", index)
		}
		name, _, ok := strings.Cut(item, "=")
		if !ok || !validSandboxEnvironmentName(name) {
			return fmt.Errorf("sandbox environment entry at index %d must use NAME=VALUE", index)
		}
		key := name
		if runtime.GOOS == "windows" {
			key = strings.ToUpper(key)
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("sandbox environment variable %q is duplicated", name)
		}
		seen[key] = struct{}{}
		if dangerousSandboxLauncherEnvironment(name) {
			return fmt.Errorf("sandbox environment variable %q is not allowed", name)
		}
	}
	return nil
}

func validSandboxEnvironmentName(name string) bool {
	if name == "" {
		return false
	}
	for index, character := range name {
		if character == '_' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || index > 0 && character >= '0' && character <= '9' {
			continue
		}
		return false
	}
	return true
}

func dangerousSandboxLauncherEnvironment(name string) bool {
	upper := strings.ToUpper(name)
	for _, prefix := range []string{"LD_", "DYLD_", "_RLD_"} {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	switch upper {
	case "GCONV_PATH", "LIBPATH", "LOCPATH", "NLSPATH", "SHLIB_PATH":
		return true
	default:
		return false
	}
}

func sandboxPathWithin(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if runtime.GOOS == "windows" {
		parent = strings.ToLower(parent)
		child = strings.ToLower(child)
	}
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func sameSandboxPath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
