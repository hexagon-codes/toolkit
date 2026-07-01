//go:build !windows

package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
)

func runBoundedCommand(ctx context.Context, command string, args []string, dir string, env []string, stdoutLimit, stderrLimit int64) (*ExecResult, error) {
	stdout := newBoundedBuffer(stdoutLimit)
	stderr := newBoundedBuffer(stderrLimit)

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("sandbox exec start failed: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		err = <-done
		return &ExecResult{
			Stdout:          stdout.String(),
			Stderr:          stderr.String(),
			ExitCode:        -1,
			StdoutBytes:     stdout.BytesSeen(),
			StderrBytes:     stderr.BytesSeen(),
			StdoutTruncated: stdout.Truncated(),
			StderrTruncated: stderr.Truncated(),
		}, fmt.Errorf("sandbox exec terminated by timeout/cancel: %w", ctx.Err())
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
		Stdout:          stdout.String(),
		Stderr:          stderr.String(),
		ExitCode:        exitCode,
		StdoutBytes:     stdout.BytesSeen(),
		StderrBytes:     stderr.BytesSeen(),
		StdoutTruncated: stdout.Truncated(),
		StderrTruncated: stderr.Truncated(),
	}, nil
}
