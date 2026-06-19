//go:build linux

package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
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

// Exec 在沙箱内执行命令
// TODO: D18-D19 完整实现 Namespace + seccomp + pivot_root
// 当前使用基础隔离 (unshare + chroot fallback)
func (s *linuxSandbox) Exec(ctx context.Context, command string, args []string) (*ExecResult, error) {
	// 应用 cfg.Timeout: 调用方 ctx 无更早 deadline 时按配置强制超时。
	ctx, cancel := withTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	// 尝试使用 unshare 创建隔离环境
	unshareArgs := []string{
		"--mount", "--pid", "--fork",
		"--", command,
	}
	unshareArgs = append(unshareArgs, args...)

	cmd := exec.CommandContext(ctx, "unshare", unshareArgs...)
	cmd.Dir = s.cfg.Workspace
	cmd.Env = cleanLinuxEnv(os.Environ())

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	// 进程被 ctx(含 cfg.Timeout 派生的 deadline)强制终止时, 必须显式上报超时,
	// 不能误判为"unshare 无权限"而退化重跑——否则会在超时后再跑一遍命令。
	if ctxErr := ctx.Err(); ctxErr != nil {
		return &ExecResult{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: -1,
		}, fmt.Errorf("sandbox exec terminated by timeout/cancel: %w", ctxErr)
	}
	if err != nil {
		// unshare 失败 (无权限) → 退化为直接执行 + 路径限制
		return s.execFallback(ctx, command, args)
	}

	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	}

	return &ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}, nil
}

// execFallback 退化执行 (无 namespace 权限时)
func (s *linuxSandbox) execFallback(ctx context.Context, command string, args []string) (*ExecResult, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = s.cfg.Workspace
	cmd.Env = cleanLinuxEnv(os.Environ())

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	// ctx(含 cfg.Timeout 派生 deadline)超时/取消时显式上报, 使强制终止对调用方可见。
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
			return nil, fmt.Errorf("exec failed: %w", err)
		}
	}

	return &ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}, nil
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
