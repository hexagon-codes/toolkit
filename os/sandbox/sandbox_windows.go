//go:build windows

package sandbox

import (
	"context"
	"fmt"
	"os"
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
		return &ExecResult{
			Stdout:          proc.stdout.String(),
			Stderr:          proc.stderr.String(),
			ExitCode:        exitCode,
			StdoutBytes:     proc.stdout.BytesSeen(),
			StderrBytes:     proc.stderr.BytesSeen(),
			StdoutTruncated: proc.stdout.Truncated(),
			StderrTruncated: proc.stderr.Truncated(),
		}, nil
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
		return &ExecResult{
			Stdout:          proc.stdout.String(),
			Stderr:          stderr,
			ExitCode:        -1,
			StdoutBytes:     proc.stdout.BytesSeen(),
			StderrBytes:     stderrBytes,
			StdoutTruncated: proc.stdout.Truncated(),
			StderrTruncated: proc.stderr.Truncated(),
		}, ctx.Err()
	}
}

func (s *windowsSandbox) ExecCode(ctx context.Context, language, code string) (*ExecResult, error) {
	var ext, runner string
	switch strings.ToLower(language) {
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
	if language == "go" || language == "golang" {
		args = []string{"run", tmpFile}
	} else {
		args = []string{tmpFile}
	}
	return s.Exec(ctx, runner, args)
}
