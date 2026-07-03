//go:build windows

package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// windowsSandbox implements Sandbox using Win32 five-layer isolation:
//  1. Restricted Token — strip privileges, Untrusted IL
//  2. AppContainer ACL — workspace RW, ReadablePaths RO, DeniedPaths deny
//  3. Job Object — memory/process limits + UI restrictions
//  4. Low Box Token — kernel-level network isolation
//  5. Alternate Desktop — GUI isolation
type windowsSandbox struct {
	cfg    Config
	policy WindowsSandboxPolicy
}

func newPlatformSandbox(cfg Config) (Sandbox, error) {
	if err := os.MkdirAll(cfg.Workspace, 0755); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}

	return &windowsSandbox{
		cfg:    cfg,
		policy: DefaultWindowsPolicy(),
	}, nil
}

func (s *windowsSandbox) Exec(ctx context.Context, command string, args []string) (*ExecResult, error) {
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

	timeout := time.Duration(s.cfg.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
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
		if err := enforceSandboxStorageLimits(s.cfg); err != nil {
			return res, err
		}
		return res, nil
	case <-ctx.Done():
		_ = proc.Kill()
		<-done
		stderr := proc.stderr.String()
		if stderr == "" {
			stderr = "process timed out"
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
		if limitErr := enforceSandboxStorageLimits(s.cfg); limitErr != nil {
			// 存储限额错误也用 %w 包装, 保证 ErrStorageLimitExceeded 哨兵可被 errors.Is 命中
			return res, fmt.Errorf("sandbox exec terminated by timeout/cancel: %w; storage limit check failed: %w", ctx.Err(), limitErr)
		}
		return res, ctx.Err()
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

func (s *windowsSandbox) ExecCode(ctx context.Context, language, code string) (*ExecResult, error) {
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
	defer os.Remove(tmpFile)

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

func resolveWindowsRuntimeForExecCode(runner string) (resolvedRunner string, readablePath string) {
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
