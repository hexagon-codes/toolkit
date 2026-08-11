//go:build !darwin && !linux && !windows

package sandbox

import (
	"context"
	"os"
)

// basicSandbox 基础沙箱 (无 OS 隔离，仅路径限制 + 超时)
//
// 仅用于没有专用强隔离后端的平台，并如实报告文件系统与进程树能力不受支持。
type basicSandbox struct {
	cfg        Config
	executions posixExecutionRegistry
}

func newPlatformSandbox(cfg Config) (Sandbox, error) {
	return &basicSandbox{cfg: cfg}, nil
}

func newBasicSandbox(cfg Config) *basicSandbox {
	return &basicSandbox{cfg: cfg}
}

func (s *basicSandbox) sandboxCapabilities(context.Context) (CapabilitySet, error) {
	available := CapabilityOutput
	if processCreationFitsBudget(s.cfg.MaxProcesses) {
		available |= CapabilityProcessCreation
	}
	if s.cfg.Network == NetworkHost {
		available |= CapabilityNetwork
	}
	return available, nil
}

func (s *basicSandbox) Exec(ctx context.Context, requested Command) (*ExecResult, error) {
	if err := validateExecContext(ctx); err != nil {
		return nil, err
	}
	if err := s.executions.ensureReady(); err != nil {
		return nil, err
	}
	// 应用 cfg.Timeout: 调用方 ctx 无更早 deadline 时按配置强制超时。
	ctx, cancel := withTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	command, err := prepareSandboxCommand(s.cfg, requested, func() ([]string, error) {
		return cleanBasicEnv(s.cfg.Workspace, os.Environ())
	})
	if err != nil {
		return nil, err
	}
	command.Path, err = resolvePOSIXCommandExecutable(command.Path, command.Dir)
	if err != nil {
		return nil, err
	}
	// basic 后端无 OS 级文件系统隔离 → unsupported(降级信号如实上报)。
	return s.executions.runBoundedCommand(ctx, command, s.cfg, posixExecutionCapabilities{
		Filesystem:         LimitStatusUnsupported,
		ProcessContainment: LimitStatusUnsupported,
	})
}
