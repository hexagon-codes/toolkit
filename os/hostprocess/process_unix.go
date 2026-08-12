//go:build unix

package hostprocess

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

func run(ctx context.Context, spec Spec, result Result) (Result, error) {
	cmd := exec.Command(spec.Executable, spec.Args...) //nolint:noctx // 本次进程组由下方取消与回收路径管理。
	cmd.Dir, cmd.Env = spec.Dir, spec.Env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = spec.Stdin, spec.Stdout, spec.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := cmd.Start(); err != nil {
		return result, fmt.Errorf("start host process: %w", err)
	}
	result.Started, result.PID = true, cmd.Process.Pid
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	var waitErr error
	select {
	case waitErr = <-wait:
	case <-ctx.Done():
		// PID 同时是本次新进程组的 ID，不向其他进程名或全局进程集合发信号。
		terminateErr := syscall.Kill(-result.PID, syscall.SIGTERM)
		if errors.Is(terminateErr, syscall.ESRCH) {
			terminateErr = nil
		}
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case waitErr = <-wait:
		case <-timer.C:
			killErr := syscall.Kill(-result.PID, syscall.SIGKILL)
			if errors.Is(killErr, syscall.ESRCH) {
				killErr = nil
			}
			terminateErr = errors.Join(terminateErr, killErr)
			waitErr = <-wait
		}
		// 根进程可以先响应 SIGTERM，组内其他成员仍须结束，才能封存输出。
		killErr := syscall.Kill(-result.PID, syscall.SIGKILL)
		if errors.Is(killErr, syscall.ESRCH) {
			killErr = nil
		}
		waitErr = errors.Join(ctx.Err(), terminateErr, killErr, waitErr)
	}
	result.Settled = true
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	// 根退出和取消可同时就绪；无论select选中哪一支，都核对本组后代状态。
	groupErr := syscall.Kill(-result.PID, 0)
	if groupErr == nil || errors.Is(groupErr, syscall.EPERM) {
		if ctx.Err() != nil {
			killErr := syscall.Kill(-result.PID, syscall.SIGKILL)
			if !errors.Is(killErr, syscall.ESRCH) {
				waitErr = errors.Join(waitErr, killErr)
			}
			groupErr = syscall.Kill(-result.PID, 0)
			// 退出回收期间可能暂不可探测，取消收口须等待本组真正消失。
			deadline := time.Now().Add(500 * time.Millisecond)
			for (groupErr == nil || errors.Is(groupErr, syscall.EPERM)) && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
				groupErr = syscall.Kill(-result.PID, 0)
			}
		}
		result.BackgroundProcessesRunning = groupErr == nil || errors.Is(groupErr, syscall.EPERM)
	}
	if groupErr != nil && !errors.Is(groupErr, syscall.ESRCH) && !errors.Is(groupErr, syscall.EPERM) {
		return result, errors.Join(waitErr, fmt.Errorf("inspect host process group: %w", groupErr))
	}
	return result, errors.Join(waitErr, ctx.Err())
}
