// Package sandbox 提供面向本地代码执行的进程、文件和网络隔离能力。
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const maxCrossPlatformProcessLimit = 1<<31 - 1

var (
	// ErrUnsupportedNetworkPolicy 表示当前运行环境无法落实所请求的网络策略。
	ErrUnsupportedNetworkPolicy = errors.New("sandbox: requested network policy is not enforceable")
	// ErrInvalidContext 表示执行调用没有提供有效上下文。
	ErrInvalidContext = errors.New("sandbox: context must not be nil")
)

// Config 沙箱配置
type Config struct {
	Workspace   string   `yaml:"workspace"`    // 工作区目录 (可读写)
	Timeout     int      `yaml:"timeout"`      // 超时秒数，默认 60
	DeniedPaths []string `yaml:"denied_paths"` // 禁止访问的路径
	// ReadablePaths 额外授予「只读」访问的宿主路径（在 Workspace 之外）。
	// 用途：用户经数据连接器等显式授权的本地目录，需让沙箱内代码 (code_exec) 能读到。
	// 语义：deny-default 沙箱里为每个路径追加只读放行（darwin: file-read* subpath）；
	// 不授予写权限（写仅限 Workspace）。DeniedPaths 的 deny 规则写在放行之后、保持优先。
	ReadablePaths []string `yaml:"readable_paths"`
	Network       bool     `yaml:"network"` // 是否允许网络，默认 false
	// DenyLoopback 在 Network=true 时额外禁止访问本机回环地址（127.0.0.1 / ::1 /
	// localhost）。无法真实执行该策略的平台必须返回 ErrUnsupportedNetworkPolicy。
	DenyLoopback bool `yaml:"deny_loopback"`

	// 面向 Agent 代码执行的保守资源上限。平台无法落实的限制必须通过能力报告显式暴露，
	// 不得静默伪装为已经生效。
	MaxOutputBytes    int64 `yaml:"max_output_bytes"`
	MaxStderrBytes    int64 `yaml:"max_stderr_bytes"`
	MaxWorkspaceBytes int64 `yaml:"max_workspace_bytes"`
	MaxArtifactBytes  int64 `yaml:"max_artifact_bytes"`
	MaxMemoryBytes    int64 `yaml:"max_memory_bytes"`
	// MaxProcesses 进程数上限。注意语义差异: POSIX 上通过 RLIMIT_NPROC 实现,
	// 是「per-UID 进程总数 ≤ 当前同 UID 进程数 + MaxProcesses」的浮动增量,
	// 与 Windows Job Object 的 per-job 精确上限语义不同。
	MaxProcesses int `yaml:"max_processes"`
}

// LimitStatus 标记某项资源限制在当前平台/后端是否真实生效
type LimitStatus string

const (
	// LimitStatusEnforced 表示资源限制已被当前后端真实执行。
	LimitStatusEnforced LimitStatus = "enforced"
	// LimitStatusUnsupported 表示当前平台或后端无法执行该资源限制。
	LimitStatusUnsupported LimitStatus = "unsupported"
	// LimitStatusWeak 表示该项在当前后端「有部分约束但非强隔离」——
	// 后端存在且执行了限制动作, 但不满足 deny-by-default 语义。
	// 典型: linux unshare 兜底不 pivot_root, 仅掩蔽 DeniedPaths, 其余宿主
	// 文件系统对载荷仍可见/可写。上层应据此判定是否可承载机密任务。
	LimitStatusWeak LimitStatus = "weak"
)

// LimitReport 逐项报告资源限制的实际执行状态（能力缺口显式上报，不许静默假装）
type LimitReport struct {
	Memory    LimitStatus
	Processes LimitStatus
	Storage   LimitStatus // MaxWorkspaceBytes/MaxArtifactBytes 的 walk 检查
	Output    LimitStatus // stdout/stderr 有界缓冲
	// Filesystem 文件系统隔离(deny-by-default containment)的实际强度:
	//   enforced    — 强隔离(darwin Seatbelt / linux bubblewrap / windows ACL);
	//   weak        — 弱隔离(linux unshare 兜底: 仅掩蔽 DeniedPaths, 非 deny-by-default);
	//   unsupported — 无 OS 级文件系统隔离(basic 后端)。
	// 降级(weak/unsupported)信号供上层(code_exec)决策是否拒载机密任务。
	Filesystem LimitStatus
}

// ExecResult 沙箱执行结果
type ExecResult struct {
	Stdout          string
	Stderr          string
	ExitCode        int
	StdoutBytes     int64
	StderrBytes     int64
	StdoutTruncated bool
	StderrTruncated bool
	// Limits 逐项报告本次执行中资源限制的实际执行状态。
	// 平台不支持的项(如 darwin 无法下调内存 rlimit)标 unsupported,
	// 由调用方作为能力缺口显式上报, 不许静默假装已生效。
	Limits LimitReport
}

// Sandbox 沙箱接口
type Sandbox interface {
	// Exec 在沙箱内执行命令
	Exec(ctx context.Context, command string, args []string) (*ExecResult, error)

	// ExecCode 在沙箱内执行代码 (language: python/javascript/go)
	ExecCode(ctx context.Context, language, code string) (*ExecResult, error)
}

// New 创建当前平台的沙箱实例
func New(cfg Config) (Sandbox, error) {
	if cfg.Workspace == "" {
		return nil, fmt.Errorf("sandbox workspace is required")
	}
	if cfg.Timeout < 0 {
		return nil, fmt.Errorf("sandbox timeout must not be negative")
	}
	if cfg.MaxOutputBytes < 0 || cfg.MaxStderrBytes < 0 || cfg.MaxWorkspaceBytes < 0 ||
		cfg.MaxArtifactBytes < 0 || cfg.MaxMemoryBytes < 0 || cfg.MaxProcesses < 0 {
		return nil, fmt.Errorf("sandbox resource limits must not be negative")
	}
	if cfg.MaxProcesses > maxCrossPlatformProcessLimit {
		return nil, fmt.Errorf("sandbox max processes exceeds the cross-platform limit")
	}

	workspace, err := normalizeSandboxWorkspace(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	cfg.Workspace = workspace
	if cfg.ReadablePaths, err = normalizeSandboxPaths("readable path", cfg.ReadablePaths); err != nil {
		return nil, err
	}
	if cfg.DeniedPaths, err = normalizeSandboxPaths("denied path", cfg.DeniedPaths); err != nil {
		return nil, err
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60
	}
	if cfg.MaxOutputBytes <= 0 {
		cfg.MaxOutputBytes = 64 * 1024
	}
	if cfg.MaxStderrBytes <= 0 {
		cfg.MaxStderrBytes = 64 * 1024
	}
	if cfg.MaxWorkspaceBytes <= 0 {
		cfg.MaxWorkspaceBytes = 1024 * 1024 * 1024
	}
	if cfg.MaxArtifactBytes <= 0 {
		cfg.MaxArtifactBytes = 50 * 1024 * 1024
	}
	if cfg.MaxMemoryBytes <= 0 {
		cfg.MaxMemoryBytes = 256 * 1024 * 1024
	}
	if cfg.MaxProcesses <= 0 {
		cfg.MaxProcesses = 64
	}
	return newPlatformSandbox(cfg)
}

func normalizeSandboxWorkspace(rawPath string) (string, error) {
	path, err := absoluteSandboxPath("workspace", rawPath)
	if err != nil {
		return "", err
	}
	if isFilesystemRoot(path) {
		return "", fmt.Errorf("sandbox workspace must not be a filesystem root")
	}
	if mkdirErr := os.MkdirAll(path, 0o700); mkdirErr != nil {
		return "", fmt.Errorf("create sandbox workspace: %w", mkdirErr)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve sandbox workspace: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect sandbox workspace: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("sandbox workspace must be a directory")
	}
	return filepath.Clean(resolved), nil
}

func normalizeSandboxPaths(field string, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	normalized := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for index, rawPath := range paths {
		if rawPath == "" {
			return nil, fmt.Errorf("sandbox %s at index %d is empty", field, index)
		}
		expanded := expandPath(rawPath)
		if !filepath.IsAbs(expanded) {
			return nil, fmt.Errorf("sandbox %s %q must be absolute", field, rawPath)
		}
		path, err := absoluteSandboxPath(field, expanded)
		if err != nil {
			return nil, err
		}
		if isFilesystemRoot(path) {
			return nil, fmt.Errorf("sandbox %s must not be a filesystem root", field)
		}
		path, err = resolveExistingPathPrefix(path)
		if err != nil {
			return nil, fmt.Errorf("resolve sandbox %s %q: %w", field, rawPath, err)
		}
		key := path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, path)
	}
	return normalized, nil
}

func absoluteSandboxPath(field, rawPath string) (string, error) {
	expanded := expandPath(rawPath)
	if strings.ContainsAny(expanded, "\x00\r\n\"") || (runtime.GOOS == "darwin" && strings.Contains(expanded, `\`)) {
		return "", fmt.Errorf("sandbox %s contains unsupported characters", field)
	}
	path, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("resolve sandbox %s: %w", field, err)
	}
	return filepath.Clean(path), nil
}

func resolveExistingPathPrefix(path string) (string, error) {
	current := filepath.Clean(path)
	missing := make([]string, 0)
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(path), nil
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func isFilesystemRoot(path string) bool {
	volume := filepath.VolumeName(path)
	root := volume + string(filepath.Separator)
	return filepath.Clean(path) == filepath.Clean(root)
}

// expandPath 展开当前用户家目录的波浪号前缀。
func expandPath(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

func validateExecContext(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalidContext
	}
	return nil
}

type boundedBuffer struct {
	limit     int64
	total     int64
	truncated bool
	buf       []byte
}

func newBoundedBuffer(limit int64) *boundedBuffer {
	if limit <= 0 {
		limit = 64 * 1024
	}
	return &boundedBuffer{limit: limit}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	b.total += int64(n)
	remaining := b.limit - int64(len(b.buf))
	if remaining > 0 {
		if int64(len(p)) > remaining {
			b.buf = append(b.buf, p[:remaining]...)
			b.truncated = true
		} else {
			b.buf = append(b.buf, p...)
		}
	} else if n > 0 {
		b.truncated = true
	}
	return n, nil
}

func (b *boundedBuffer) String() string   { return string(b.buf) }
func (b *boundedBuffer) BytesSeen() int64 { return b.total }
func (b *boundedBuffer) Truncated() bool  { return b.truncated || b.total > b.limit }

// newUniqueCodeFile 在 dir 下创建带唯一后缀的代码临时文件并写入 code。
//
// 文件名形如 "_hexclaw_exec_<随机>.<ext>", 保证并发调用之间彼此隔离,
// 不会因固定文件名而互相覆盖或被对方的 defer 误删。返回绝对路径。
//
// 设计动机: 旧实现在 darwin/linux/basic 三个 ExecCode 路径均写死固定文件名
// "_hexclaw_exec.<ext>", 同一 workspace 并发执行多份代码时会互相覆盖同一物理文件,
// 且任一 defer os.Remove 可能误删他人正在使用的文件, 违反"并发执行隔离"安全要求。
// 本函数用 os.CreateTemp 以 "<前缀>_*<ext>" 模式生成唯一名, 跨平台统一隔离。
func newUniqueCodeFile(dir, ext, code string) (string, error) {
	// CreateTemp 的 pattern 中 "*" 会被替换为唯一随机串, 其余原样保留;
	// 将 "*" 置于扩展名之前以保留正确的文件后缀 (go run 等依赖 .go 后缀)。
	f, err := os.CreateTemp(dir, "_hexclaw_exec_*"+ext)
	if err != nil {
		return "", fmt.Errorf("create temp code file: %w", err)
	}
	name := f.Name()
	if _, err := f.WriteString(code); err != nil {
		return "", fmt.Errorf("write temp code: %w", errors.Join(err, f.Close(), os.Remove(name)))
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close temp code file: %w", errors.Join(err, os.Remove(name)))
	}
	return name, nil
}

func removeCodeFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// withTimeout 依据 cfg.Timeout(秒)为执行派生一个截止时间。
//
// 设计动机: Config.Timeout 字段承诺"超时秒数, 默认 60", 但历史实现仅 windows
// 路径使用了它, darwin/linux/basic 三个 POSIX 路径完全无视该字段, 形成跨平台
// 行为不一致的"死配置"。本函数统一兜底: 当 timeoutSec > 0 且调用方传入的 ctx
// 没有更早的 deadline 时, 派生一个 timeoutSec 秒后触发的 deadline,
// 使 cfg.Timeout 真正生效并强制终止超时进程。
//
// 调用方必须 defer 调用返回的 cancel 以释放计时器资源。
func withTimeout(ctx context.Context, timeoutSec int) (context.Context, context.CancelFunc) {
	if timeoutSec <= 0 {
		return ctx, func() {}
	}
	limit := time.Duration(timeoutSec) * time.Second
	// 若调用方 ctx 已有更早(或相同)的 deadline, 则尊重调用方, 不再缩短。
	if dl, ok := ctx.Deadline(); ok && time.Until(dl) <= limit {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, limit)
}
