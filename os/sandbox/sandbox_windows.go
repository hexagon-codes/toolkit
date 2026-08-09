//go:build windows

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// windowsSandbox 通过 Win32 四层机制实现沙箱隔离：
//  1. 受限令牌：移除权限并使用不可信完整性级别
//  2. AppContainer ACL：工作区可读写、授权路径只读、拒绝路径不可访问
//  3. Job Object：限制内存、进程数和界面能力
//  4. Low Box Token：由内核执行网络隔离
type windowsSandbox struct {
	cfg Config
}

func newPlatformSandbox(cfg Config) (Sandbox, error) {
	if err := os.MkdirAll(cfg.Workspace, 0o700); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}

	return &windowsSandbox{cfg: cfg}, nil
}

func (s *windowsSandbox) Exec(ctx context.Context, command string, args []string) (*ExecResult, error) {
	if err := validateExecContext(ctx); err != nil {
		return nil, err
	}
	if err := enforceSandboxStorageLimits(s.cfg); err != nil {
		return nil, err
	}

	// Validate escape vectors
	if err := validateWindowsEscapeVectors(command, args); err != nil {
		return nil, fmt.Errorf("security check failed: %w", err)
	}
	if err := validateWindowsPath(command); err != nil {
		return nil, err
	}

	ctx, cancel := withTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	// Try full sandbox launch
	proc, err := launchSandboxedProcess(s.cfg, command, args)
	if err != nil {
		return nil, fmt.Errorf("sandbox unavailable: windows sandbox backend failed: %w", err)
	}

	// Wait for process
	type waitResult struct {
		state *os.ProcessState
		err   error
	}
	done := make(chan waitResult, 1)
	go func() {
		state, err := proc.Wait()
		done <- waitResult{state: state, err: err}
	}()

	select {
	case wait := <-done:
		exitCode := 0
		if wait.state != nil {
			exitCode = wait.state.ExitCode()
		} else if wait.err != nil {
			exitCode = 1
		}
		res := &ExecResult{
			Stdout:          proc.stdout.String(),
			Stderr:          proc.stderr.String(),
			ExitCode:        exitCode,
			StdoutBytes:     proc.stdout.BytesSeen(),
			StderrBytes:     proc.stderr.BytesSeen(),
			StdoutTruncated: proc.stdout.Truncated(),
			StderrTruncated: proc.stderr.Truncated(),
			Limits:          windowsLimitReport(),
		}
		limitErr := enforceSandboxStorageLimits(s.cfg)
		if wait.err != nil {
			return res, errors.Join(fmt.Errorf("sandbox exec wait failed: %w", wait.err), limitErr)
		}
		if wait.state == nil {
			return res, errors.Join(fmt.Errorf("sandbox exec wait failed: process state is unavailable"), limitErr)
		}
		if limitErr != nil {
			return res, limitErr
		}
		return res, nil
	case <-ctx.Done():
		killErr := proc.Kill()
		wait := <-done
		stderr := proc.stderr.String()
		if stderr == "" {
			stderr = "process canceled"
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				stderr = "process timed out"
			}
		}
		stderrBytes := proc.stderr.BytesSeen()
		if stderrBytes == 0 {
			stderrBytes = int64(len(stderr))
		}
		res := &ExecResult{
			Stdout:          proc.stdout.String(),
			Stderr:          stderr,
			ExitCode:        -1,
			StdoutBytes:     proc.stdout.BytesSeen(),
			StderrBytes:     stderrBytes,
			StdoutTruncated: proc.stdout.Truncated(),
			StderrTruncated: proc.stderr.Truncated(),
			Limits:          windowsLimitReport(),
		}
		resultErr := fmt.Errorf("sandbox exec terminated by timeout/cancel: %w", ctx.Err())
		if killErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("kill sandbox process: %w", killErr))
		}
		if wait.err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("sandbox exec wait failed: %w", wait.err))
		}
		if limitErr := enforceSandboxStorageLimits(s.cfg); limitErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("storage limit check failed: %w", limitErr))
		}
		return res, resultErr
	}
}

// windowsLimitReport 报告 Windows 后端各资源限制项的实际执行状态。
//
// Job Object 真实执行 memory/processes 上限(创建失败时 Exec 直接报错, 不会
// 走到结果构造), Storage walk 检查与有界输出缓冲由本包纯用户态实现,
// 受限令牌 + ACL(workspace RW / ReadablePaths RO / DeniedPaths deny)提供
// deny-by-default 文件系统隔离, 均恒 enforced。
func windowsLimitReport() LimitReport {
	return LimitReport{
		Memory:     LimitStatusEnforced,
		Processes:  LimitStatusEnforced,
		Storage:    LimitStatusEnforced,
		Output:     LimitStatusEnforced,
		Filesystem: LimitStatusEnforced,
	}
}

func (s *windowsSandbox) ExecCode(ctx context.Context, language, code string) (result *ExecResult, err error) {
	if validationErr := validateExecContext(ctx); validationErr != nil {
		return nil, validationErr
	}
	var ext, runner string
	lang := strings.ToLower(language)
	switch lang {
	case "python", "python3":
		ext, runner = ".py", "python"
	case "javascript", "js", "node":
		ext, runner = ".js", "node"
	case "go", "golang":
		ext, runner = ".go", "go"
	default:
		return nil, fmt.Errorf("unsupported language: %s", language)
	}

	tmpFile, err := newUniqueCodeFile(s.cfg.Workspace, ext, code)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, removeCodeFile(tmpFile)) }()

	var args []string
	if lang == "go" || lang == "golang" {
		args = []string{"run", tmpFile}
	} else {
		args = []string{tmpFile}
	}

	runCfg := s.cfg
	if resolvedRunner, runtimePath := resolveWindowsRuntimeForExecCode(runner); resolvedRunner != "" {
		runner = resolvedRunner
		if runtimePath != "" {
			runCfg.ReadablePaths = appendUniqueWindowsPath(runCfg.ReadablePaths, runtimePath)
		}
	}

	runSandbox := *s
	runSandbox.cfg = runCfg
	return runSandbox.Exec(ctx, runner, args)
}

func resolveWindowsRuntimeForExecCode(runner string) (resolvedRunner, readablePath string) {
	resolvedRunner = runner
	path, err := exec.LookPath(runner)
	if err != nil {
		return resolvedRunner, ""
	}
	if path == "" {
		return resolvedRunner, ""
	}
	resolvedRunner = path
	return resolvedRunner, filepath.Dir(path)
}

func appendUniqueWindowsPath(paths []string, path string) []string {
	if path == "" {
		return paths
	}
	clean := filepath.Clean(path)
	for _, existing := range paths {
		if strings.EqualFold(filepath.Clean(existing), clean) {
			return paths
		}
	}
	return append(paths, clean)
}
