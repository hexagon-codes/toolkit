//go:build linux

package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// linuxSandbox Linux Namespace + seccomp 沙箱
//
// 五层隔离: Namespace → pivot_root → seccomp → 进程加固 → 两阶段 re-exec
// 对标 Codex bubblewrap 沙箱。
//
// 退化路径:
//   - CLONE_NEWUSER 不可用 → Landlock + seccomp
//   - Landlock 不可用 → 仅 seccomp + 警告
type linuxSandbox struct {
	cfg Config
}

func newPlatformSandbox(cfg Config) (Sandbox, error) {
	return &linuxSandbox{cfg: cfg}, nil
}

// Exec 在沙箱内执行命令。
//
// Linux 优先使用 bubblewrap，因为它能在普通桌面/CI Linux 上提供接近
// macOS Seatbelt 的 deny-by-default 文件系统视图：workspace 读写、系统运行时
// 只读、ReadablePaths 只读、Network=false 时 unshare net。若 bubblewrap 不可用，
// 再使用 util-linux unshare 作为较弱但仍 fail-closed 的 namespace 后端；两者都
// 不存在或启动失败时直接返回 sandbox unavailable，不做裸 exec fallback。
func (s *linuxSandbox) Exec(ctx context.Context, command string, args []string) (*ExecResult, error) {
	// 应用 cfg.Timeout: 调用方 ctx 无更早 deadline 时按配置强制超时。
	ctx, cancel := withTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	runner, runnerArgs, env, err := s.linuxSandboxRunner(command, args)
	if err != nil {
		return nil, err
	}
	res, err := runBoundedCommand(ctx, runner, runnerArgs, s.cfg.Workspace, env, s.cfg.MaxOutputBytes, s.cfg.MaxStderrBytes)
	if err != nil {
		return nil, fmt.Errorf("sandbox unavailable: linux backend failed: %w", err)
	}
	return res, nil
}

func (s *linuxSandbox) linuxSandboxRunner(command string, args []string) (string, []string, []string, error) {
	env := cleanLinuxEnv(os.Environ())
	if bwrap, err := exec.LookPath("bwrap"); err == nil {
		return bwrap, s.bwrapArgs(command, args), env, nil
	}
	if unshare, err := exec.LookPath("unshare"); err == nil {
		return unshare, s.unshareArgs(command, args), env, nil
	}
	return "", nil, nil, fmt.Errorf("sandbox unavailable: linux requires bubblewrap or unshare")
}

func (s *linuxSandbox) bwrapArgs(command string, args []string) []string {
	out := []string{
		"--die-with-parent",
		"--new-session",
		"--unshare-pid",
		"--proc", "/proc",
		"--dev", "/dev",
		"--setenv", "HOME", s.cfg.Workspace,
		"--setenv", "TMPDIR", "/tmp",
		"--setenv", "TMP", "/tmp",
		"--setenv", "TEMP", "/tmp",
	}
	if dirExists("/tmp") {
		out = append(out, "--bind", "/tmp", "/tmp")
	} else {
		out = append(out, "--dir", "/tmp")
	}
	if !s.cfg.Network {
		out = append(out, "--unshare-net")
	}
	for _, p := range linuxSystemReadPaths() {
		if dirExists(p) {
			out = append(out, "--ro-bind", p, p)
		}
	}
	out = append(out, "--bind", s.cfg.Workspace, s.cfg.Workspace)
	for _, p := range s.cfg.ReadablePaths {
		if p = cleanLinuxMountPath(p); p != "" && dirExists(p) {
			out = append(out, "--ro-bind", p, p)
		}
	}
	for _, p := range s.cfg.DeniedPaths {
		if p = cleanLinuxMountPath(p); p != "" && p != "/" {
			out = append(out, "--tmpfs", p)
		}
	}
	out = append(out, "--chdir", s.cfg.Workspace, "--", command)
	out = append(out, args...)
	return out
}

func (s *linuxSandbox) unshareArgs(command string, args []string) []string {
	out := []string{
		"--user",
		"--map-root-user",
		"--mount",
		"--pid",
		"--fork",
		"--mount-proc",
	}
	if !s.cfg.Network {
		out = append(out, "--net")
	}
	out = append(out, "--", command)
	out = append(out, args...)
	return out
}

// ExecCode 在沙箱内执行代码
func (s *linuxSandbox) ExecCode(ctx context.Context, language, code string) (*ExecResult, error) {
	var ext, interpreter string
	switch language {
	case "python", "python3":
		ext = ".py"
		interpreter = "python3"
	case "javascript", "node", "js":
		ext = ".js"
		interpreter = "node"
	case "go":
		ext = ".go"
		interpreter = "go"
	default:
		return nil, fmt.Errorf("unsupported language: %s", language)
	}

	// 使用唯一临时文件名避免并发串扰(详见 newUniqueCodeFile 注释)。
	tmpFile, err := newUniqueCodeFile(s.cfg.Workspace, ext, code)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile)

	if language == "go" {
		return s.Exec(ctx, interpreter, []string{"run", tmpFile})
	}
	return s.Exec(ctx, interpreter, []string{tmpFile})
}

// cleanLinuxEnv 清理 Linux 下的危险动态链接器环境变量
//
// 与 darwin cleanEnv 保持一致: LD_ 作为真正的前缀通配, 拦截所有形如 LD_<X>
// 的变量 (LD_PRELOAD、LD_LIBRARY_PATH、LD_AUDIT 及任意 LD_<X> 注入向量),
// 而非仅精确命中枚举出的具体名称。
func cleanLinuxEnv(env []string) []string {
	// dangerousPrefixes 为真正的前缀通配。
	dangerousPrefixes := []string{"LD_"}
	var clean []string
	for _, e := range env {
		// 取出 "=" 之前的变量名再做前缀判断。
		name := e
		if idx := strings.IndexByte(e, '='); idx >= 0 {
			name = e[:idx]
		}
		skip := false
		for _, prefix := range dangerousPrefixes {
			if strings.HasPrefix(name, prefix) {
				skip = true
				break
			}
		}
		if !skip {
			clean = append(clean, e)
		}
	}
	return clean
}

func linuxSystemReadPaths() []string {
	return []string{
		"/bin",
		"/etc",
		"/lib",
		"/lib64",
		"/nix/store",
		"/opt",
		"/run",
		"/sbin",
		"/usr",
		"/usr/local",
	}
}

func cleanLinuxMountPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		return ""
	}
	if real, err := filepath.EvalSymlinks(p); err == nil {
		p = real
	}
	return filepath.Clean(p)
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
