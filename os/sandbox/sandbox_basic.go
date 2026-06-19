//go:build !darwin && !linux && !windows

package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
)

// basicSandbox 基础沙箱 (无 OS 隔离，仅路径限制 + 超时)
//
// 用于 Windows (Phase 8 前) 和不支持沙箱的平台。
type basicSandbox struct {
	cfg Config
}

func newPlatformSandbox(cfg Config) (Sandbox, error) {
	return &basicSandbox{cfg: cfg}, nil
}

func newBasicSandbox(cfg Config) *basicSandbox {
	return &basicSandbox{cfg: cfg}
}

func (s *basicSandbox) Exec(ctx context.Context, command string, args []string) (*ExecResult, error) {
	// 应用 cfg.Timeout: 调用方 ctx 无更早 deadline 时按配置强制超时。
	ctx, cancel := withTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = s.cfg.Workspace

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

func (s *basicSandbox) ExecCode(ctx context.Context, language, code string) (*ExecResult, error) {
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
