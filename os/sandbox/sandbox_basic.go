//go:build !darwin && !linux && !windows

package sandbox

import (
	"context"
	"fmt"
	"os"
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

	return runBoundedCommand(ctx, command, args, s.cfg.Workspace, cleanBasicEnv(os.Environ()), s.cfg.MaxOutputBytes, s.cfg.MaxStderrBytes)
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

func cleanBasicEnv(env []string) []string {
	return env
}
