// Package sandbox 提供跨平台进程沙箱
//
// 三平台隔离策略:
//   - macOS: Seatbelt (sandbox-exec + SBPL 策略)
//   - Linux: Namespace + seccomp + pivot_root (Phase 7 D18-D19)
//   - Windows: Restricted Token + ACL + Job Object (Phase 8 D29-D35)
//
// 默认 ON，零外部依赖，对齐 Codex 沙箱能力。
package sandbox

import (
	"context"
	"fmt"
	"os"
	"time"
)

// Config 沙箱配置
type Config struct {
	Workspace   string   `yaml:"workspace"`    // 工作区目录 (可读写)
	Timeout     int      `yaml:"timeout"`      // 超时秒数，默认 60
	DeniedPaths []string `yaml:"denied_paths"` // 禁止访问的路径
	Network     bool     `yaml:"network"`      // 是否允许网络，默认 false
}

// ExecResult 沙箱执行结果
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
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
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60
	}

	return newPlatformSandbox(cfg)
}

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
		_ = f.Close()
		_ = os.Remove(name)
		return "", fmt.Errorf("write temp code: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("close temp code file: %w", err)
	}
	return name, nil
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
