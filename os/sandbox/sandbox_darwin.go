//go:build darwin

package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// darwinSandbox macOS Seatbelt 沙箱
//
// 使用 sandbox-exec + SBPL (Sandbox Profile Language) 限制进程。
// 与 Codex 和 Claude Code 使用完全相同的技术。
type darwinSandbox struct {
	cfg Config
}

func newPlatformSandbox(cfg Config) (Sandbox, error) {
	return newDarwinSandbox(cfg), nil
}

func newDarwinSandbox(cfg Config) *darwinSandbox {
	return &darwinSandbox{cfg: cfg}
}

// generateSBPL 生成 Seatbelt Profile Language 策略
func (s *darwinSandbox) generateSBPL() string {
	workspace := s.cfg.Workspace

	var sb strings.Builder
	sb.WriteString("(version 1)\n")
	sb.WriteString("(deny default)\n")

	// 允许进程执行基本操作
	sb.WriteString("(allow process-exec)\n")
	sb.WriteString("(allow process-fork)\n")
	sb.WriteString("(allow sysctl-read)\n")
	sb.WriteString("(allow mach-lookup)\n")
	sb.WriteString("(allow signal)\n")
	sb.WriteString("(allow system-socket)\n")

	// 允许 dyld 将可执行映像 mmap 进内存。
	// macOS 26+ 在 (deny default) 下若不显式授予 file-map-executable,
	// dyld 加载共享缓存/可执行段时会被 SIGABRT, 任何二进制都无法启动。
	sb.WriteString("(allow file-map-executable)\n")

	// 允许读取根目录 inode 本身。
	// 枚举式 file-read* 子路径只授予子项访问权, 不含根目录 "/" 自身;
	// 而 dyld 在路径解析阶段需要 stat/read "/", 缺失会导致进程在
	// dyld 阶段 SIGABRT (macOS 26+ 上整个沙箱不可用的真正根因)。
	sb.WriteString("(allow file-read* (literal \"/\"))\n")

	// 允许读取系统文件 (运行时、库等)
	sb.WriteString("(allow file-read*\n")
	sb.WriteString("  (subpath \"/usr\")\n")
	sb.WriteString("  (subpath \"/bin\")\n")
	sb.WriteString("  (subpath \"/sbin\")\n")
	sb.WriteString("  (subpath \"/Library\")\n")
	sb.WriteString("  (subpath \"/System\")\n")
	sb.WriteString("  (subpath \"/private/var\")\n")
	sb.WriteString("  (subpath \"/private/tmp\")\n")
	sb.WriteString("  (subpath \"/var\")\n")
	sb.WriteString("  (subpath \"/tmp\")\n")
	sb.WriteString("  (subpath \"/etc\")\n")
	sb.WriteString("  (subpath \"/dev\")\n")
	sb.WriteString("  (subpath \"/opt\")\n")

	// Homebrew paths
	sb.WriteString("  (subpath \"/opt/homebrew\")\n")
	sb.WriteString("  (subpath \"/usr/local\")\n")

	// Python/Node 运行时
	home, _ := os.UserHomeDir()
	sb.WriteString(fmt.Sprintf("  (subpath \"%s/.pyenv\")\n", home))
	sb.WriteString(fmt.Sprintf("  (subpath \"%s/.nvm\")\n", home))
	sb.WriteString(fmt.Sprintf("  (subpath \"%s/.local\")\n", home))
	sb.WriteString(")\n")

	// 工作区读写
	sb.WriteString(fmt.Sprintf("(allow file-read* (subpath \"%s\"))\n", workspace))
	sb.WriteString(fmt.Sprintf("(allow file-write* (subpath \"%s\"))\n", workspace))

	// /tmp 读写 (临时文件)
	sb.WriteString("(allow file-write* (subpath \"/tmp\"))\n")
	sb.WriteString("(allow file-write* (subpath \"/private/tmp\"))\n")

	// 额外授权目录：只读放行（用户经数据连接器等显式授权的本地目录，让 code_exec 能读到）。
	// 仅 file-read*（不授写），且写在 DeniedPaths 的 deny 之前——后写的 deny 规则在 seatbelt 里优先生效。
	// 安全：路径来自连接器自由文本，必须先过 isSafeSeatbeltPath——含 `"`/`\`/换行的路径会终止/损坏
	// SBPL 字面量（轻则整张 profile 失效搞瘫所有 code_exec，重则注入 (allow network*) 之类逃逸沙箱），
	// 非绝对路径写进 subpath 也无意义。非法者跳过，绝不污染 profile。
	for _, readable := range s.cfg.ReadablePaths {
		expanded := expandPath(readable)
		if !isSafeSeatbeltPath(expanded) {
			continue
		}
		sb.WriteString(fmt.Sprintf("(allow file-read* (subpath \"%s\"))\n", expanded))
	}

	// 明确拒绝的路径
	for _, denied := range s.cfg.DeniedPaths {
		expanded := expandPath(denied)
		sb.WriteString(fmt.Sprintf("(deny file-read* (subpath \"%s\"))\n", expanded))
		sb.WriteString(fmt.Sprintf("(deny file-write* (subpath \"%s\"))\n", expanded))
	}

	// 网络控制
	if s.cfg.Network {
		sb.WriteString("(allow network*)\n")
	} else {
		sb.WriteString("(deny network*)\n")
		// 允许本地 DNS 和 loopback (某些运行时需要)
		sb.WriteString("(allow network-outbound (to unix-socket))\n")
	}

	return sb.String()
}

// Exec 在 Seatbelt 沙箱内执行命令
func (s *darwinSandbox) Exec(ctx context.Context, command string, args []string) (*ExecResult, error) {
	// 应用 cfg.Timeout: 调用方 ctx 无更早 deadline 时按配置强制超时。
	ctx, cancel := withTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	sbpl := s.generateSBPL()

	sandboxArgs := []string{"-p", sbpl, command}
	sandboxArgs = append(sandboxArgs, args...)

	cmd := exec.CommandContext(ctx, "sandbox-exec", sandboxArgs...)
	cmd.Dir = s.cfg.Workspace

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// 清理危险环境变量
	cmd.Env = cleanEnv(os.Environ())

	err := cmd.Run()
	// 进程被 ctx(含 cfg.Timeout 派生的 deadline)强制终止时, cmd.Run 返回的是
	// *exec.ExitError("signal: killed"), 若仅按 ExitError 当作普通非零退出处理,
	// 会把 err 抹成 nil, 调用方无从得知进程是"超时被杀"还是"正常退出"。
	// 因此优先检查 ctx.Err(): 一旦 ctx 已取消/超时, 显式返回包装错误,
	// 使 cfg.Timeout 的强制终止对调用方可见。
	if ctxErr := ctx.Err(); ctxErr != nil {
		return &ExecResult{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: -1,
		}, fmt.Errorf("sandbox exec terminated by timeout/cancel: %w", ctxErr)
	}
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("sandbox exec failed: %w", err)
		}
	}

	return &ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}, nil
}

// ExecCode 在沙箱内执行代码
func (s *darwinSandbox) ExecCode(ctx context.Context, language, code string) (*ExecResult, error) {
	// 写代码到临时文件
	var ext, cmd string
	switch language {
	case "python", "python3":
		ext = ".py"
		cmd = "python3"
	case "javascript", "node", "js":
		ext = ".js"
		cmd = "node"
	case "go":
		ext = ".go"
		cmd = "go"
	default:
		return nil, fmt.Errorf("unsupported language: %s", language)
	}

	// 使用唯一临时文件名避免并发串扰。
	// 旧实现写死 "_hexclaw_exec.<ext>", 同一 workspace 并发执行多份代码时
	// 会互相覆盖同一物理文件, 且 defer 删除可能误删他人正在使用的文件,
	// 违反"并发执行隔离"安全要求。os.CreateTemp 以 "<前缀>_*<ext>" 模式生成唯一名。
	tmpFile, err := newUniqueCodeFile(s.cfg.Workspace, ext, code)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile)

	if language == "go" {
		return s.Exec(ctx, cmd, []string{"run", tmpFile})
	}
	return s.Exec(ctx, cmd, []string{tmpFile})
}

// cleanEnv 清理危险环境变量
//
// 安全语义: LD_/DYLD_ 作为真正的前缀通配, 拦截所有形如 LD_<X>/DYLD_<X> 的变量
// (如 LD_PRELOAD、LD_AUDIT、LD_LIBRARY_PATH、DYLD_INSERT_LIBRARIES 等),
// 而非仅精确命中枚举出的具体名称。
//
// 历史缺陷: 旧实现对每个条目统一拼接 "="(prefix+"="), 使 "LD_" 退化为只匹配
// "LD_=", 导致 LD_AUDIT/LD_FOO 等任意 LD_<X> 注入向量全部漏网。
func cleanEnv(env []string) []string {
	// dangerousPrefixes 为真正的前缀通配, 命中以其开头的所有变量名。
	dangerousPrefixes := []string{"LD_", "DYLD_"}
	var clean []string
	for _, e := range env {
		// 取出 "=" 之前的变量名再做前缀判断, 避免值中含前缀字面量造成误删。
		name := e
		if idx := strings.IndexByte(e, '='); idx >= 0 {
			name = e[:idx]
		}
		dangerous := false
		for _, prefix := range dangerousPrefixes {
			if strings.HasPrefix(name, prefix) {
				dangerous = true
				break
			}
		}
		if !dangerous {
			clean = append(clean, e)
		}
	}
	return clean
}

// isSafeSeatbeltPath 判断一个路径能否安全写进 SBPL 的 (subpath "...") 字面量。
//
// 要求：① 绝对路径（subpath 只对绝对路径有意义）② 不含会破坏/注入 SBPL 字符串的字符——
// 双引号 `"`（终止字面量→注入）、反斜杠 `\`（SBPL 转义引导；macOS 路径分隔是 `/`，正常路径不含 `\`）、
// 换行/回车/空字符（截断 profile）。非法路径直接拒绝（跳过放行），宁可少授权也不污染整张 profile。
func isSafeSeatbeltPath(p string) bool {
	if p == "" || !strings.HasPrefix(p, "/") {
		return false
	}
	return !strings.ContainsAny(p, "\"\\\n\r\x00")
}

// expandPath 展开路径中的波浪号前缀为当前用户的 home 目录。
//
// 支持:
//   - "~"        -> home (裸波浪号, 用户用它表达 home 目录)
//   - "~/x"、"~\x" -> home/x
//
// 不支持 "~user" 形式 (解析任意其他用户的 home 涉及平台特定的用户库查询,
// 且在 deny 规则里指向他人 home 语义不明确), 原样返回, 由调用方自行决定。
//
// 历史缺陷: 旧实现只处理 "~/" 前缀, 裸 "~" 会被当成字面路径写进 SBPL deny 规则,
// 指向不存在的 "~" 文件, 导致用户"拒绝 home 目录"的意图静默失效。
func expandPath(p string) string {
	if p == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}
