package sandbox

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	errWindowsActiveReadDrainTimeout        = errors.New("windows output read did not stop before handle close")
	errWindowsProcessContainmentUnconfirmed = errors.New("windows sandbox process containment is unconfirmed")
	errWindowsProcessLifecycleUnconfirmed   = errors.New("windows sandbox process lifecycle is unconfirmed")
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

// windowsActiveResource 在阻塞读取期间保留资源所有权。关闭方先阻止新读取，
// 再取消当前读取并等待活动引用归还，最后才能释放底层资源。
type windowsActiveResource[T comparable] struct {
	closeMu  sync.Mutex
	mu       sync.Mutex
	resource T
	closing  bool
	active   int
	drained  chan struct{}
}

func newWindowsActiveResource[T comparable](resource T) *windowsActiveResource[T] {
	return &windowsActiveResource[T]{resource: resource}
}

func (r *windowsActiveResource[T]) value() T {
	if r == nil {
		var zero T
		return zero
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.resource
}

func (r *windowsActiveResource[T]) beginRead() (T, bool) {
	if r == nil {
		var zero T
		return zero, false
	}
	r.mu.Lock()
	var zero T
	if r.closing || r.resource == zero {
		r.mu.Unlock()
		return zero, false
	}
	r.active++
	resource := r.resource
	r.mu.Unlock()
	return resource, true
}

func (r *windowsActiveResource[T]) endRead() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active <= 0 {
		panic("windows active resource read count is unbalanced")
	}
	r.active--
	if r.closing && r.active == 0 && r.drained != nil {
		close(r.drained)
		r.drained = nil
	}
}

func (r *windowsActiveResource[T]) closeAfterReads(
	cancel, release func(T) error,
	timeout time.Duration,
) error {
	if r == nil {
		return nil
	}
	if release == nil {
		return fmt.Errorf("windows resource release function is required")
	}

	r.closeMu.Lock()
	defer r.closeMu.Unlock()

	r.mu.Lock()
	var zero T
	if r.resource == zero {
		r.mu.Unlock()
		return nil
	}
	r.closing = true
	resource := r.resource
	var drained <-chan struct{}
	if r.active > 0 {
		if r.drained == nil {
			r.drained = make(chan struct{})
		}
		drained = r.drained
	}
	r.mu.Unlock()

	var result error
	if drained != nil && cancel != nil {
		result = errors.Join(result, cancel(resource))
	}
	if drained != nil && !waitForWindowsResourceDrain(drained, timeout) {
		return errors.Join(result, fmt.Errorf("%w after %s", errWindowsActiveReadDrainTimeout, timeout))
	}

	r.mu.Lock()
	releaseErr := release(r.resource)
	if releaseErr == nil {
		r.resource = zero
	}
	r.mu.Unlock()
	return errors.Join(result, releaseErr)
}

func waitForWindowsResourceDrain(drained <-chan struct{}, timeout time.Duration) bool {
	if drained == nil {
		return true
	}
	if timeout <= 0 {
		select {
		case <-drained:
			return true
		default:
			return false
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-drained:
		return true
	case <-timer.C:
		return false
	}
}

type windowsQuarantineEntry struct {
	reclaim func(time.Duration) (bool, error)
}

type windowsLifecycleOwner uint8

const (
	windowsLifecycleOwnedByCaller windowsLifecycleOwner = iota
	windowsLifecycleOwnedBySandbox
	windowsLifecycleReclaimed
)

// windowsRetainedLifecycle 为仍需收敛的 Windows 生命周期提供唯一所有权状态机。
// 调用方可先同步回收一次；失败后只能向 Sandbox quarantine 转移一次，回收成功后不可再次转移。
type windowsRetainedLifecycle struct {
	reclaimMu sync.Mutex
	mu        sync.Mutex
	owner     windowsLifecycleOwner
	entry     *windowsQuarantineEntry
	reclaimer func(time.Duration) (bool, error)
}

func newWindowsRetainedLifecycle(
	reclaimer func(time.Duration) (bool, error),
) *windowsRetainedLifecycle {
	lifecycle := &windowsRetainedLifecycle{reclaimer: reclaimer}
	lifecycle.entry = &windowsQuarantineEntry{reclaim: lifecycle.reclaim}
	return lifecycle
}

func (lifecycle *windowsRetainedLifecycle) retain(quarantine *windowsProcessQuarantine) error {
	if lifecycle == nil || lifecycle.reclaimer == nil || lifecycle.entry == nil {
		return fmt.Errorf("windows retained lifecycle is invalid")
	}
	lifecycle.reclaimMu.Lock()
	defer lifecycle.reclaimMu.Unlock()
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	switch lifecycle.owner {
	case windowsLifecycleOwnedBySandbox:
		return fmt.Errorf("windows retained lifecycle is already retained")
	case windowsLifecycleReclaimed:
		return fmt.Errorf("windows retained lifecycle is already reclaimed")
	}
	if err := quarantine.add(lifecycle.entry); err != nil {
		return err
	}
	lifecycle.owner = windowsLifecycleOwnedBySandbox
	return nil
}

func (lifecycle *windowsRetainedLifecycle) reclaim(timeout time.Duration) (bool, error) {
	if lifecycle == nil || lifecycle.reclaimer == nil {
		return false, fmt.Errorf("windows retained lifecycle is invalid")
	}
	lifecycle.reclaimMu.Lock()
	defer lifecycle.reclaimMu.Unlock()

	lifecycle.mu.Lock()
	if lifecycle.owner == windowsLifecycleReclaimed {
		lifecycle.mu.Unlock()
		return true, nil
	}
	lifecycle.mu.Unlock()

	reclaimed, err := lifecycle.reclaimer(timeout)
	if reclaimed {
		lifecycle.mu.Lock()
		lifecycle.owner = windowsLifecycleReclaimed
		lifecycle.mu.Unlock()
	}
	return reclaimed, err
}

// windowsProcessQuarantine 由 Sandbox 统一持有启动失败或执行期未确认退出、释放失败的生命周期。
// 条目只有在确认进程退出且全部资源释放后才会从集合中移除。
type windowsProcessQuarantine struct {
	reclaimMu sync.Mutex
	mu        sync.Mutex
	entries   []*windowsQuarantineEntry
}

func (q *windowsProcessQuarantine) add(entry *windowsQuarantineEntry) error {
	if q == nil {
		return fmt.Errorf("windows process quarantine is unavailable")
	}
	if entry == nil || entry.reclaim == nil {
		return fmt.Errorf("windows quarantine entry is invalid")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, retained := range q.entries {
		if retained == entry {
			return fmt.Errorf("windows quarantine entry is already retained")
		}
	}
	q.entries = append(q.entries, entry)
	return nil
}

func (q *windowsProcessQuarantine) count() int {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.entries)
}

func (q *windowsProcessQuarantine) reclaim(attempts int, timeout time.Duration) (bool, error) {
	if q == nil {
		return true, nil
	}
	if attempts <= 0 {
		return false, fmt.Errorf("windows quarantine reclaim attempts must be positive")
	}
	if timeout <= 0 {
		return false, fmt.Errorf("windows quarantine reclaim timeout must be positive")
	}

	q.reclaimMu.Lock()
	defer q.reclaimMu.Unlock()
	var result error
	for attempt := 1; attempt <= attempts; attempt++ {
		entries := q.snapshot()
		if len(entries) == 0 {
			return true, result
		}
		for _, entry := range entries {
			reclaimed, err := entry.reclaim(timeout)
			if err != nil {
				result = errors.Join(result, fmt.Errorf("reclaim retained Windows lifecycle on attempt %d: %w", attempt, err))
			}
			if reclaimed {
				q.remove(entry)
			}
		}
	}
	if q.count() != 0 {
		result = errors.Join(result, errWindowsProcessLifecycleUnconfirmed)
		return false, result
	}
	return true, result
}

func (q *windowsProcessQuarantine) snapshot() []*windowsQuarantineEntry {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]*windowsQuarantineEntry(nil), q.entries...)
}

func (q *windowsProcessQuarantine) remove(entry *windowsQuarantineEntry) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for index, retained := range q.entries {
		if retained != entry {
			continue
		}
		copy(q.entries[index:], q.entries[index+1:])
		q.entries[len(q.entries)-1] = nil
		q.entries = q.entries[:len(q.entries)-1]
		return
	}
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

type windowsExecutionWaitResult struct {
	state *os.ProcessState
	err   error
}

// windowsExecutionCompletion 只启动一个由进程对象持有的 Wait goroutine，并向 Exec 与
// quarantine 广播同一个不可变结果，避免取消路径遗失 goroutine 和资源所有权。
type windowsExecutionCompletion struct {
	startOnce sync.Once
	mu        sync.Mutex
	done      chan struct{}
	result    windowsExecutionWaitResult
}

func newWindowsExecutionCompletion() *windowsExecutionCompletion {
	return &windowsExecutionCompletion{done: make(chan struct{})}
}

func (completion *windowsExecutionCompletion) start(
	wait func() windowsExecutionWaitResult,
) <-chan struct{} {
	if completion == nil {
		return nil
	}
	completion.startOnce.Do(func() {
		go func() {
			result := windowsExecutionWaitResult{}
			if wait == nil {
				result.err = fmt.Errorf("windows execution wait function is required")
			} else {
				result = wait()
			}
			completion.mu.Lock()
			completion.result = result
			completion.mu.Unlock()
			close(completion.done)
		}()
	})
	return completion.done
}

func (completion *windowsExecutionCompletion) wait(
	timeout time.Duration,
) (windowsExecutionWaitResult, bool) {
	if completion == nil || completion.done == nil {
		return windowsExecutionWaitResult{}, false
	}
	switch {
	case timeout < 0:
		<-completion.done
	case timeout == 0:
		select {
		case <-completion.done:
		default:
			return windowsExecutionWaitResult{}, false
		}
	default:
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-completion.done:
		case <-timer.C:
			return windowsExecutionWaitResult{}, false
		}
	}
	completion.mu.Lock()
	defer completion.mu.Unlock()
	return completion.result, true
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
	_, err := settleWindowsJobConfirmed(lifecycle, exitGrace, terminationTimeout)
	return err
}

func settleWindowsJobConfirmed(
	lifecycle windowsJobLifecycle,
	exitGrace, terminationTimeout time.Duration,
) (bool, error) {
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
	if !exited || err != nil {
		// 未确认进程树清空时保留 Job 句柄，继续约束仍可能存活的后代。
		return false, result
	}
	if err := lifecycle.close(); err != nil {
		result = errors.Join(result, fmt.Errorf("close sandbox job: %w", err))
	}
	return true, result
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
