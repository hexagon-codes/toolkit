//go:build windows

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const windowsExecutionSettlementLimit = 16 * time.Second

// windowsSandbox 使用稳定 AppContainer 身份、持久私有工作区 DACL、冻结句柄和 Job Object。
type windowsSandbox struct {
	cfg        Config
	workspace  *windowsWorkspace
	execMu     sync.Mutex
	poisoned   error
	quarantine *windowsProcessQuarantine
	launchOps  windowsLaunchOps
	launcher   windowsProcessLauncher
}

func (*windowsSandbox) sandboxCloseRetryable() {}

func newPlatformSandbox(cfg Config) (Sandbox, error) {
	if len(cfg.ReadablePaths) != 0 {
		return nil, fmt.Errorf("Windows ReadablePaths are unsupported without brokered read-only mappings")
	}
	if cfg.Network != NetworkDisabled {
		return nil, fmt.Errorf(
			"%w: Windows cannot provide the complete host network view",
			ErrUnsupportedNetworkPolicy,
		)
	}

	workspace, err := prepareWindowsWorkspace(cfg)
	if err != nil {
		return nil, err
	}
	cfg.Workspace = workspace.canonicalPath
	if err := verifyWindowsDeniedPaths(cfg, workspace); err != nil {
		return nil, errors.Join(err, workspace.close())
	}
	return &windowsSandbox{
		cfg:        cfg,
		workspace:  workspace,
		quarantine: &windowsProcessQuarantine{},
		launchOps:  newWindowsLaunchOps(),
		launcher:   launchSandboxedProcess,
	}, nil
}

func (s *windowsSandbox) sandboxCapabilities(ctx context.Context) (CapabilitySet, error) {
	if err := validateExecContext(ctx); err != nil {
		return 0, err
	}
	if s == nil || s.workspace == nil {
		return 0, fmt.Errorf("Windows sandbox is not initialized")
	}
	if s.cfg.Network != NetworkDisabled {
		return 0, fmt.Errorf("%w: Windows cannot provide the complete host network view", ErrUnsupportedNetworkPolicy)
	}
	available := CapabilityFilesystem |
		CapabilityNetwork |
		CapabilityProcessContainment |
		CapabilityOutput
	if s.cfg.MaxMemoryBytes > 0 {
		available |= CapabilityMemory
	}
	if s.cfg.MaxProcesses > 0 {
		available |= CapabilityProcesses
	}
	if processCreationFitsBudget(s.cfg.MaxProcesses) {
		available |= CapabilityProcessCreation
	}
	return available, nil
}

func (s *windowsSandbox) Exec(ctx context.Context, requested Command) (*ExecResult, error) {
	if err := validateExecContext(ctx); err != nil {
		return nil, err
	}
	s.execMu.Lock()
	defer s.execMu.Unlock()
	result, _, err := s.execLocked(ctx, requested)
	return result, err
}

func (s *windowsSandbox) execLocked(
	ctx context.Context,
	requested Command,
) (*ExecResult, bool, error) {
	if s.poisoned != nil {
		return nil, false, fmt.Errorf("Windows sandbox is unavailable after an unconfirmed process-containment shutdown: %w", s.poisoned)
	}
	command, err := s.prepareCommand(requested)
	if err != nil {
		return nil, true, err
	}
	if err := ctx.Err(); err != nil {
		return nil, true, fmt.Errorf("sandbox exec context is already done: %w", err)
	}
	if err := enforceWindowsWorkspaceLimits(s.workspace, s.cfg); err != nil {
		return nil, true, err
	}

	executable, err := resolveWindowsExecutable(s.workspace, command.Path)
	if err != nil {
		return nil, true, err
	}
	workingDirectory, err := resolveWindowsWorkingDirectory(s.workspace, command.Dir)
	if err != nil {
		return nil, true, errors.Join(err, executable.close())
	}
	env := command.Env
	if env == nil {
		env, err = cleanWindowsEnv(s.workspace, executable.applicationName)
		if err != nil {
			return nil, true, errors.Join(err, executable.close(), workingDirectory.close())
		}
	}
	if err := validateSandboxEnvironment(env); err != nil {
		return nil, true, errors.Join(err, executable.close(), workingDirectory.close())
	}
	env, err = validateWindowsCommandEnv(s.workspace, env)
	if err != nil {
		return nil, true, errors.Join(err, executable.close(), workingDirectory.close())
	}

	ctx, cancel := withTimeout(ctx, s.cfg.Timeout)
	defer cancel()
	if launchOpsErr := s.launchOps.validate(); launchOpsErr != nil {
		return nil, true, errors.Join(launchOpsErr, executable.close(), workingDirectory.close())
	}
	if s.launcher == nil {
		return nil, true, errors.Join(
			fmt.Errorf("windows process launcher is required"),
			executable.close(),
			workingDirectory.close(),
		)
	}
	proc, err := s.launcher(
		s.cfg,
		s.workspace,
		executable,
		workingDirectory,
		command.Args,
		env,
		s.quarantine,
		s.launchOps,
	)
	planCloseErr := errors.Join(executable.close(), workingDirectory.close())
	if err == nil && proc == nil {
		err = fmt.Errorf("windows process launcher returned no process")
	}
	if err != nil {
		err = errors.Join(fmt.Errorf("sandbox unavailable: Windows sandbox backend failed: %w", err), planCloseErr)
		if errors.Is(err, errWindowsProcessContainmentUnconfirmed) ||
			errors.Is(err, errWindowsProcessLifecycleUnconfirmed) {
			s.poisoned = err
			return nil, false, err
		}
		return nil, true, err
	}

	done := proc.startWait()

	select {
	case <-done:
		wait, lifecycleFinished := proc.completion.wait(0)
		treeConfirmed := proc.processContainmentConfirmed()
		result := windowsExecResult(proc, wait.state, "", windowsLimitReport(s.cfg, treeConfirmed))
		resultErr := planCloseErr
		if !lifecycleFinished {
			resultErr = errors.Join(resultErr, fmt.Errorf("sandbox exec wait lifecycle result is unavailable"))
		}
		if wait.err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("sandbox exec wait failed: %w", wait.err))
		}
		if wait.state == nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("sandbox exec wait failed: process state is unavailable"))
		}
		if !treeConfirmed {
			resultErr = errors.Join(resultErr, errWindowsProcessContainmentUnconfirmed)
		}
		if !lifecycleFinished || !treeConfirmed || !proc.lifecycleReleased() {
			return result, false, s.retainExecution(proc, resultErr)
		}
		if limitErr := enforceWindowsWorkspaceLimits(s.workspace, s.cfg); limitErr != nil {
			resultErr = errors.Join(resultErr, limitErr)
		}
		return result, true, resultErr

	case <-ctx.Done():
		killErr := proc.Kill()
		wait, lifecycleFinished := proc.completion.wait(s.launchOps.settlementLimit)
		treeConfirmed := lifecycleFinished && proc.processContainmentConfirmed()
		stderr := proc.stderr.String()
		if stderr == "" {
			stderr = "process canceled"
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				stderr = "process timed out"
			}
		}
		result := windowsExecResult(proc, wait.state, stderr, windowsLimitReport(s.cfg, treeConfirmed))
		result.ExitCode = -1
		resultErr := errors.Join(
			fmt.Errorf("sandbox exec terminated by timeout/cancel: %w", ctx.Err()),
			planCloseErr,
		)
		if killErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("kill sandbox process tree: %w", killErr))
		}
		if !lifecycleFinished {
			resultErr = errors.Join(resultErr, fmt.Errorf("windows process lifecycle did not finish within %s", s.launchOps.settlementLimit))
		}
		if wait.err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("sandbox exec wait failed: %w", wait.err))
		}
		if !treeConfirmed {
			resultErr = errors.Join(resultErr, errWindowsProcessContainmentUnconfirmed)
		}
		if !treeConfirmed || !proc.lifecycleReleased() {
			return result, false, s.retainExecution(proc, resultErr)
		}
		if limitErr := enforceWindowsWorkspaceLimits(s.workspace, s.cfg); limitErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("storage limit check failed: %w", limitErr))
		}
		return result, true, resultErr
	}
}

func (s *windowsSandbox) retainExecution(
	proc *windowsSandboxedProcess,
	executionErr error,
) error {
	result := retainWindowsExecutionLifecycle(
		s.quarantine,
		proc.retain,
		proc.processContainmentConfirmed(),
		executionErr,
	)
	s.poisoned = result
	return result
}

func (s *windowsSandbox) prepareCommand(requested Command) (Command, error) {
	if requested.Path == "" {
		return Command{}, fmt.Errorf("sandbox command path is required")
	}
	if strings.IndexByte(requested.Path, 0) >= 0 {
		return Command{}, fmt.Errorf("sandbox command path contains NUL")
	}
	command := Command{
		Path: requested.Path,
		Args: append([]string(nil), requested.Args...),
		Dir:  requested.Dir,
	}
	for index, argument := range command.Args {
		if strings.IndexByte(argument, 0) >= 0 {
			return Command{}, fmt.Errorf("sandbox command argument at index %d contains NUL", index)
		}
	}
	if requested.Env != nil {
		command.Env = make([]string, len(requested.Env))
		copy(command.Env, requested.Env)
		if err := validateSandboxEnvironment(command.Env); err != nil {
			return Command{}, err
		}
	}
	return command, nil
}

func windowsExecResult(
	proc *windowsSandboxedProcess,
	state *os.ProcessState,
	fallbackStderr string,
	limits LimitReport,
) *ExecResult {
	exitCode := 0
	if state != nil {
		exitCode = state.ExitCode()
	}
	stderr := proc.stderr.String()
	stderrBytes := proc.stderr.BytesSeen()
	if stderr == "" && fallbackStderr != "" {
		stderr = fallbackStderr
		if stderrBytes == 0 {
			stderrBytes = int64(len(fallbackStderr))
		}
	}
	return &ExecResult{
		Stdout:          proc.stdout.String(),
		Stderr:          stderr,
		ExitCode:        exitCode,
		StdoutBytes:     proc.stdout.BytesSeen(),
		StderrBytes:     stderrBytes,
		StdoutTruncated: proc.stdout.Truncated(),
		StderrTruncated: proc.stderr.Truncated(),
		Limits:          limits,
	}
}

// windowsLimitReport 只对本次已证明的进程生命周期收敛结果报告 Enforced。
func windowsLimitReport(cfg Config, processContainmentConfirmed bool) LimitReport {
	processContainment := LimitStatusUnsupported
	if processContainmentConfirmed {
		processContainment = LimitStatusEnforced
	}
	return LimitReport{
		Network:            LimitStatusEnforced,
		Memory:             requestedLimitStatus(cfg.MaxMemoryBytes > 0, LimitStatusEnforced),
		Processes:          requestedLimitStatus(cfg.MaxProcesses > 0, LimitStatusEnforced),
		ProcessContainment: processContainment,
		Storage: requestedLimitStatus(
			cfg.MaxWorkspaceBytes > 0 || cfg.MaxArtifactBytes > 0,
			LimitStatusUnsupported,
		),
		Output:     LimitStatusEnforced,
		Filesystem: LimitStatusEnforced,
	}
}
