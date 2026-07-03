//go:build linux

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
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
// Linux 优先使用 bubblewrap，因为它在普通桌面/CI Linux 上提供接近 macOS Seatbelt 的
// deny-by-default 文件系统视图：workspace 读写、系统运行时只读、ReadablePaths 只读、
// Network=false 时 unshare net。
//
// 若 bubblewrap 不可用，仅剩 util-linux unshare 弱兜底：它用 user+mount namespace，
// 但不 pivot_root 到最小 rootfs，宿主完整挂载视图对载荷仍可见/可写，策略脚本只掩蔽
// DeniedPaths、ro-bind ReadablePaths——属 allow-first / fail-open 的弱隔离，并非
// deny-by-default。因此：配置带机密性要求（DeniedPaths 非空）时对 unshare 兜底
// fail-closed 拒绝执行，不静默降级；无机密性要求时才用 unshare，并在
// ExecResult.Limits.Filesystem 上标 weak 把降级信号交给上层决策。两者都不存在或
// 启动失败时直接返回 sandbox unavailable，不做裸 exec fallback。
func (s *linuxSandbox) Exec(ctx context.Context, command string, args []string) (*ExecResult, error) {
	// 应用 cfg.Timeout: 调用方 ctx 无更早 deadline 时按配置强制超时。
	ctx, cancel := withTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	runner, runnerArgs, env, containment, err := s.linuxSandboxRunner(command, args)
	if err != nil {
		return nil, err
	}
	res, err := runBoundedCommand(ctx, runner, runnerArgs, s.cfg, env, containment)
	if err != nil {
		// 存储限额违规是策略结果而非后端故障: 执行本身可能已成功, 必须保留
		// ExecResult(stdout/stderr)并原样透传携带 ErrStorageLimitExceeded 哨兵
		// 的错误, 不得误包装成 sandbox unavailable。
		if errors.Is(err, ErrStorageLimitExceeded) {
			return res, err
		}
		return nil, fmt.Errorf("sandbox unavailable: linux backend failed: %w", err)
	}
	return res, nil
}

func (s *linuxSandbox) linuxSandboxRunner(command string, args []string) (runner string, runnerArgs, env []string, containment LimitStatus, err error) {
	env = cleanLinuxEnv(os.Environ())
	bwrap, bwrapErr := exec.LookPath("bwrap")
	unshare, unshareErr := exec.LookPath("unshare")
	unshareAvailable := unshareErr == nil && linuxUnshareBackendUsable(unshare)

	// confidential: 调用方显式声明受保护路径即表明在意机密性; 弱兜底无法保证
	// deny-by-default, 对其 fail-closed(详见 selectLinuxSandboxBackend)。
	confidential := len(s.cfg.DeniedPaths) > 0
	backend, containment, err := selectLinuxSandboxBackend(bwrapErr == nil, unshareAvailable, confidential)
	if err != nil {
		return "", nil, nil, containment, err
	}

	switch backend {
	case linuxBackendBwrap:
		return bwrap, s.bwrapArgs(command, args), env, containment, nil
	case linuxBackendUnshare:
		return unshare, s.unshareArgs(command, args), env, containment, nil
	default:
		return "", nil, nil, containment, fmt.Errorf("sandbox unavailable: linux requires bubblewrap or unshare")
	}
}

var (
	linuxUnshareProbeOnce sync.Once
	linuxUnshareProbeOK   bool
)

func linuxUnshareBackendUsable(unshare string) bool {
	if unshare == "" {
		return false
	}
	linuxUnshareProbeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, unshare,
			"--user",
			"--map-root-user",
			"--mount",
			"--pid",
			"--fork",
			"--mount-proc",
			"--",
			"/bin/sh",
			"-c",
			"exit 0",
		)
		cmd.Env = cleanLinuxEnv(os.Environ())
		linuxUnshareProbeOK = cmd.Run() == nil
	})
	return linuxUnshareProbeOK
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
	out = append(out, "--tmpfs", "/tmp")
	if !s.cfg.Network {
		out = append(out, "--unshare-net")
	}
	// 能力缺口（诚实标注）：DenyLoopback 在 linux 尚未实现。Network=true 时进程
	// 共享宿主 net namespace，回环仍可达——细粒度「允许外网、禁回环」需 unshare-net +
	// slirp/pasta 或 nftables 出站过滤，本平台暂缺。调用方（如 hexclaw code_exec）在
	// linux 上不能依赖沙箱屏蔽本机管理端口；darwin 经 seatbelt 已强制。
	_ = s.cfg.DenyLoopback
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

	if len(s.cfg.ReadablePaths) > 0 || len(s.cfg.DeniedPaths) > 0 {
		out = append(out, "--", "/bin/sh", "-eu", "-c", linuxUnsharePolicyScript, "hexclaw-unshare-policy", "--readable")
		for _, p := range s.cfg.ReadablePaths {
			if p = cleanLinuxMountPath(p); p != "" && pathExists(p) {
				out = append(out, p)
			}
		}
		out = append(out, "--denied")
		for _, p := range s.cfg.DeniedPaths {
			if p = cleanLinuxMountPath(p); p != "" && p != "/" && pathExists(p) {
				out = append(out, p)
			}
		}
		out = append(out, "--cmd", command)
		out = append(out, args...)
		return out
	}

	out = append(out, "--", command)
	out = append(out, args...)
	return out
}

const linuxUnsharePolicyScript = `
mode=
while [ "$#" -gt 0 ]; do
	case "$1" in
	--readable|--denied)
		mode="$1"
		shift
		;;
	--cmd)
		shift
		exec "$@"
		;;
	*)
		case "$mode" in
		--readable)
			if [ -e "$1" ]; then
				mount --bind "$1" "$1"
				mount -o remount,bind,ro "$1"
			fi
			;;
		--denied)
			if [ -d "$1" ]; then
				mount -t tmpfs -o size=1m,mode=000 tmpfs "$1"
			elif [ -e "$1" ]; then
				mount --bind /dev/null "$1"
			fi
			;;
		*)
			exit 126
			;;
		esac
		shift
		;;
	esac
done
exit 127
`

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
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return filepath.Clean(p)
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
