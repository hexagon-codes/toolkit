package sandbox

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ownedWindowsResource 显式记录单个 Windows 资源的唯一所有权。
// 释放成功后才清零；释放失败时保留原值，允许后续 cleanup 精确重试。
type ownedWindowsResource[T comparable] struct {
	mu       sync.Mutex
	resource T
}

func newOwnedWindowsResource[T comparable](resource T) *ownedWindowsResource[T] {
	return &ownedWindowsResource[T]{resource: resource}
}

func (r *ownedWindowsResource[T]) value() T {
	if r == nil {
		var zero T
		return zero
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.resource
}

func (r *ownedWindowsResource[T]) release(release func(T) error) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	var zero T
	if r.resource == zero {
		return nil
	}
	if err := release(r.resource); err != nil {
		return err
	}
	r.resource = zero
	return nil
}

// releaseAfter 在同一所有权临界区内完成释放前操作与最终释放，避免裸资源值
// 在两步之间被其他 goroutine 关闭并复用。仅最终释放成功后才清零所有权。
func (r *ownedWindowsResource[T]) releaseAfter(before, release func(T) error) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	var zero T
	if r.resource == zero {
		return nil
	}
	var beforeErr error
	if before != nil {
		beforeErr = before(r.resource)
	}
	releaseErr := release(r.resource)
	if releaseErr == nil {
		r.resource = zero
	}
	return errors.Join(beforeErr, releaseErr)
}

func releaseOwnedWindowsResources[T comparable](
	release func(T) error,
	resources ...*ownedWindowsResource[T],
) error {
	var result error
	for _, resource := range resources {
		result = errors.Join(result, resource.release(release))
	}
	return result
}

// windowsJobLifecycle 将平台调用注入为可在非 Windows 主机验证的生命周期合同。
type windowsJobLifecycle struct {
	wait      func(time.Duration) (bool, error)
	terminate func() error
	close     func() error
}

type windowsProcessTerminationLifecycle struct {
	terminate func() error
	wait      func(time.Duration) (bool, error)
}

// windowsWaitLifecycle 将进程等待、Job 收敛、输出排空和最终清理固定为单向顺序。
// 各阶段即使失败也继续执行后续清理，并通过 errors.Join 保留完整错误链。
type windowsWaitLifecycle[T any] struct {
	waitProcess func() (T, error)
	settleJob   func() error
	waitOutput  func() error
	cleanup     func() error
}

func runWindowsWaitLifecycle[T any](lifecycle windowsWaitLifecycle[T]) (T, error) {
	state, waitErr := lifecycle.waitProcess()
	jobErr := lifecycle.settleJob()
	outputErr := lifecycle.waitOutput()
	cleanupErr := lifecycle.cleanup()
	return state, errors.Join(waitErr, jobErr, outputErr, cleanupErr)
}

// terminateWindowsProcess 发起强制终止后继续等待进程对象进入退出态，避免在
// TerminateProcess 的异步终止尚未收敛时释放句柄或恢复沙箱边界。
func terminateWindowsProcess(lifecycle windowsProcessTerminationLifecycle, timeout time.Duration) error {
	var result error
	if err := lifecycle.terminate(); err != nil {
		result = errors.Join(result, fmt.Errorf("terminate sandbox process: %w", err))
	}
	exited, err := lifecycle.wait(timeout)
	if err != nil {
		result = errors.Join(result, fmt.Errorf("wait for terminated sandbox process: %w", err))
	} else if !exited {
		result = errors.Join(result, fmt.Errorf("sandbox process did not exit within %s", timeout))
	}
	return result
}

// settleWindowsJob 先给正常后代一个有界退出窗口；仅当 Job 仍有活动进程或等待失败时，
// 才显式终止整个 Job。无论前序结果如何，最后都关闭 Job，使 KILL_ON_JOB_CLOSE 成为兜底。
func settleWindowsJob(lifecycle windowsJobLifecycle, exitGrace, terminationTimeout time.Duration) error {
	var result error
	exited, err := lifecycle.wait(exitGrace)
	if err != nil {
		result = errors.Join(result, fmt.Errorf("wait for sandbox job exit: %w", err))
	}
	if err != nil || !exited {
		if terminateErr := lifecycle.terminate(); terminateErr != nil {
			result = errors.Join(result, fmt.Errorf("terminate sandbox job: %w", terminateErr))
		}
		exited, err = lifecycle.wait(terminationTimeout)
		if err != nil {
			result = errors.Join(result, fmt.Errorf("wait for terminated sandbox job: %w", err))
		} else if !exited {
			result = errors.Join(result, fmt.Errorf("sandbox job did not exit within %s", terminationTimeout))
		}
	}
	if err := lifecycle.close(); err != nil {
		result = errors.Join(result, fmt.Errorf("close sandbox job: %w", err))
	}
	return result
}

// waitForWindowsJobProcesses 轮询 Job 的活动进程计数，避免把 Job 的 signaled 状态
// 误当成“进程树已退出”。查询至少执行一次，随后在给定预算内有界等待。
func waitForWindowsJobProcesses(
	activeProcesses func() (uint32, error),
	timeout, pollInterval time.Duration,
) (bool, error) {
	if activeProcesses == nil {
		return false, fmt.Errorf("active process query is required")
	}

	deadline := time.Now().Add(timeout)
	for {
		active, err := activeProcesses()
		if err != nil {
			return false, err
		}
		if active == 0 {
			return true, nil
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false, nil
		}
		delay := pollInterval
		if delay <= 0 || delay > remaining {
			delay = remaining
		}
		timer := time.NewTimer(delay)
		<-timer.C
	}
}

// waitForWindowsOutput 等待两个读取器完成；超时后先调用 abort 关闭读取端，
// 再给读取器同样长度的有界收敛窗口，确保调用方不会在管道 EOF 上无限等待。
func waitForWindowsOutput(
	stdoutDone, stderrDone <-chan error,
	timeout time.Duration,
	abort func() error,
) error {
	var result error
	wait := func(limit time.Duration) bool {
		timer := time.NewTimer(limit)
		defer timer.Stop()
		for stdoutDone != nil || stderrDone != nil {
			select {
			case err := <-stdoutDone:
				result = errors.Join(result, err)
				stdoutDone = nil
			case err := <-stderrDone:
				result = errors.Join(result, err)
				stderrDone = nil
			case <-timer.C:
				return false
			}
		}
		return true
	}

	if wait(timeout) {
		return result
	}
	result = errors.Join(result, fmt.Errorf("wait for child output timed out after %s", timeout))
	if abort != nil {
		result = errors.Join(result, abort())
	}
	if !wait(timeout) {
		result = errors.Join(result, fmt.Errorf("child output readers did not stop within %s", timeout))
	}
	return result
}
