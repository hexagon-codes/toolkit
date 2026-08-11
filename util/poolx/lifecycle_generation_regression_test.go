package poolx

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const poolxBarrierTimeout = 2 * time.Second

func TestPoolSubmitWithContextCancellationWakesRegisteredWaiter(t *testing.T) {
	blockerStarted := make(chan struct{})
	releaseBlocker := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseBlocker) })
	}

	p := New("context-cancel-wakeup", WithMaxWorkers(1), WithAutoScale(false))
	t.Cleanup(func() {
		release()
		p.Release()
	})
	if err := p.Submit(func() {
		close(blockerStarted)
		<-releaseBlocker
	}); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	waitForSignal(t, blockerStarted, "blocking task did not start")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	result := make(chan error, 1)
	go func() {
		result <- p.SubmitWithContext(ctx, func(context.Context) {})
	}()
	waitForAtomicValue(t, &p.generation.Load().blockingCount, 1, "context waiter was not registered")

	cancel()
	if err := waitForResult(t, result, "context waiter did not wake after cancellation"); !errors.Is(err, context.Canceled) {
		t.Fatalf("SubmitWithContext() error = %v, want context.Canceled", err)
	}
}

func TestPoolWithFuncInvokeWithTimeoutWakesAtDeadline(t *testing.T) {
	blockerStarted := make(chan struct{})
	releaseBlocker := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseBlocker) })
	}

	p := NewPoolWithFunc("context-timeout-wakeup", func(arg any) {
		if arg == "block" {
			close(blockerStarted)
			<-releaseBlocker
		}
	}, WithMaxWorkers(1), WithAutoScale(false))
	t.Cleanup(func() {
		release()
		p.Release()
	})
	if err := p.Invoke("block"); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	waitForSignal(t, blockerStarted, "blocking invocation did not start")

	result := make(chan error, 1)
	go func() {
		result <- p.InvokeWithTimeout("timed", 50*time.Millisecond)
	}()
	waitForAtomicValue(t, &p.generation.Load().blockingCount, 1, "timed waiter was not registered")

	if err := waitForResult(t, result, "InvokeWithTimeout() did not return at its deadline"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("InvokeWithTimeout() error = %v, want context.DeadlineExceeded", err)
	}
}

func TestPriorityQueuePopWaitCancellationWakesRegisteredWaiter(t *testing.T) {
	queue := NewPriorityQueue(1)
	done := make(chan struct{})
	result := make(chan func(), 1)
	go func() {
		result <- queue.PopWait(done)
	}()
	waitForPriorityQueueWaiter(t, queue)

	close(done)
	select {
	case task := <-result:
		if task != nil {
			t.Fatal("PopWait() returned a task after cancellation")
		}
	case <-time.After(poolxBarrierTimeout):
		t.Fatal("PopWait() did not wake after cancellation")
	}
}

func TestPoolReleaseLinearizesAgainstSubmissionPausedInHook(t *testing.T) {
	hookEntered := make(chan struct{})
	releaseHook := make(chan struct{})
	taskRan := make(chan struct{})
	hooks := NewHookBuilder().BeforeSubmit(func(*TaskInfo) {
		close(hookEntered)
		<-releaseHook
	}).Build()
	p := New("release-submit-linearization",
		WithMaxWorkers(1),
		WithAutoScale(false),
		WithHooks(hooks),
	)
	t.Cleanup(p.Release)

	result := make(chan error, 1)
	go func() {
		result <- p.Submit(func() { close(taskRan) })
	}()
	waitForSignal(t, hookEntered, "before-submit hook was not entered")

	p.Release()
	close(releaseHook)
	err := waitForResult(t, result, "paused submission did not return")
	if !errors.Is(err, ErrPoolClosed) {
		if err == nil {
			waitForSignal(t, taskRan, "submission succeeded after Release() but its task did not run")
		}
		t.Fatalf("Submit() error = %v, want ErrPoolClosed", err)
	}
	select {
	case <-taskRan:
		t.Fatal("task ran after Release() returned")
	default:
	}
}

func TestPoolRebootUsesFreshGenerationWhileTimedOutTaskFinishes(t *testing.T) {
	oldStarted := make(chan struct{})
	releaseOld := make(chan struct{})
	oldFinished := make(chan struct{})
	newFinished := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseOld) })
	}

	p := New("reboot-generation", WithMaxWorkers(1), WithAutoScale(false))
	t.Cleanup(func() {
		release()
		p.Release()
	})
	if err := p.Submit(func() {
		close(oldStarted)
		<-releaseOld
		close(oldFinished)
	}); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	oldGeneration := p.generation.Load()
	waitForSignal(t, oldStarted, "old-generation task did not start")
	if err := p.ReleaseTimeout(0); !errors.Is(err, ErrTimeout) {
		t.Fatalf("ReleaseTimeout() error = %v, want ErrTimeout", err)
	}

	p.Reboot()
	newGeneration := p.generation.Load()
	if !p.TrySubmit(func() { close(newFinished) }) {
		t.Fatal("new generation could not create its own worker")
	}
	waitForSignal(t, newFinished, "new-generation task did not finish")
	p.Release()
	if got := newGeneration.metrics.CompletedTasks.Load(); got != 1 {
		t.Fatalf("new-generation completed tasks = %d, want 1", got)
	}

	release()
	waitForSignal(t, oldFinished, "old-generation task did not finish")
	waitForSignal(t, oldGeneration.done, "old generation did not retire")
	if got := newGeneration.metrics.CompletedTasks.Load(); got != 1 {
		t.Fatalf("new-generation completed tasks after old retirement = %d, want 1", got)
	}
}

func TestPoolWithFuncRebootUsesFreshGenerationWhileTimedOutTaskFinishes(t *testing.T) {
	oldStarted := make(chan struct{})
	releaseOld := make(chan struct{})
	oldFinished := make(chan struct{})
	newFinished := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseOld) })
	}

	p := NewPoolWithFunc("func-reboot-generation", func(arg any) {
		switch arg {
		case "old":
			close(oldStarted)
			<-releaseOld
			close(oldFinished)
		case "new":
			close(newFinished)
		}
	}, WithMaxWorkers(1), WithAutoScale(false))
	t.Cleanup(func() {
		release()
		p.Release()
	})
	if err := p.Invoke("old"); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	oldGeneration := p.generation.Load()
	waitForSignal(t, oldStarted, "old-generation invocation did not start")
	if err := p.ReleaseTimeout(0); !errors.Is(err, ErrTimeout) {
		t.Fatalf("ReleaseTimeout() error = %v, want ErrTimeout", err)
	}

	p.Reboot()
	newGeneration := p.generation.Load()
	if !p.TryInvoke("new") {
		t.Fatal("new function-pool generation could not create its own worker")
	}
	waitForSignal(t, newFinished, "new-generation invocation did not finish")
	p.Release()
	if got := newGeneration.metrics.CompletedTasks.Load(); got != 1 {
		t.Fatalf("new-generation completed tasks = %d, want 1", got)
	}

	release()
	waitForSignal(t, oldFinished, "old-generation invocation did not finish")
	waitForSignal(t, oldGeneration.done, "old function-pool generation did not retire")
	if got := newGeneration.metrics.CompletedTasks.Load(); got != 1 {
		t.Fatalf("new-generation completed tasks after old function-pool retirement = %d, want 1", got)
	}
}

func TestPoolConcurrentTuneUsesAtomicCapacitySnapshot(t *testing.T) {
	p := New("concurrent-tune", WithMaxWorkers(4), WithAutoScale(false))
	t.Cleanup(p.Release)
	runConcurrentCapacityReaders(t, p.Tune, p.Cap, p.Free)
}

func TestPoolWithFuncConcurrentTuneUsesAtomicCapacitySnapshot(t *testing.T) {
	p := NewPoolWithFunc("func-concurrent-tune", func(any) {}, WithMaxWorkers(4), WithAutoScale(false))
	t.Cleanup(p.Release)
	runConcurrentCapacityReaders(t, p.Tune, p.Cap, p.Free)
}

func TestPoolTuneDownRetiresBusyWorkersAtNewCapacity(t *testing.T) {
	started := make(chan struct{}, 2)
	releaseTasks := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseTasks) })
	}
	p := New("tune-down-busy", WithMaxWorkers(2), WithAutoScale(false))
	t.Cleanup(func() {
		release()
		p.Release()
	})
	for range 2 {
		if err := p.Submit(func() {
			started <- struct{}{}
			<-releaseTasks
		}); err != nil {
			t.Fatalf("Submit() error = %v", err)
		}
	}
	waitForSignal(t, started, "first busy worker did not start")
	waitForSignal(t, started, "second busy worker did not start")

	p.Tune(1)
	release()
	waitForRunningAtMost(t, p.Running, 1, "busy workers remained above tuned capacity")
}

func TestPoolWithFuncTuneDownRetiresBusyWorkersAtNewCapacity(t *testing.T) {
	started := make(chan struct{}, 2)
	releaseTasks := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseTasks) })
	}
	p := NewPoolWithFunc("func-tune-down-busy", func(any) {
		started <- struct{}{}
		<-releaseTasks
	}, WithMaxWorkers(2), WithAutoScale(false))
	t.Cleanup(func() {
		release()
		p.Release()
	})
	for range 2 {
		if err := p.Invoke("work"); err != nil {
			t.Fatalf("Invoke() error = %v", err)
		}
	}
	waitForSignal(t, started, "first busy function worker did not start")
	waitForSignal(t, started, "second busy function worker did not start")

	p.Tune(1)
	release()
	waitForRunningAtMost(t, p.Running, 1, "busy function workers remained above tuned capacity")
}

func TestSetPanicHandlerPublishesImmutableSnapshot(t *testing.T) {
	original := DefaultPool()
	p := New("panic-handler-snapshot", WithMaxWorkers(8), WithAutoScale(false), WithPanicHandler(func(any) {}))
	if err := SetDefaultPool(p); err != nil {
		p.Release()
		t.Fatalf("SetDefaultPool() error = %v", err)
	}
	t.Cleanup(func() {
		if err := SetDefaultPool(original); err != nil {
			t.Errorf("restore default pool: %v", err)
		}
		p.Release()
	})

	start := make(chan struct{})
	var tasks sync.WaitGroup
	const taskCount = 512
	tasks.Add(taskCount)
	submitDone := make(chan struct{})
	go func() {
		defer close(submitDone)
		<-start
		for index := 0; index < taskCount; index++ {
			if err := p.Submit(func() {
				defer tasks.Done()
				panic("expected")
			}); err != nil {
				for range taskCount - index {
					tasks.Done()
				}
				return
			}
		}
	}()

	setDone := make(chan struct{})
	go func() {
		defer close(setDone)
		<-start
		for iteration := 0; iteration < taskCount; iteration++ {
			if iteration%2 == 0 {
				SetPanicHandler(func(any) {})
			} else {
				SetPanicHandler(nil)
			}
		}
	}()

	close(start)
	waitForSignal(t, submitDone, "panic task submission did not finish")
	waitForSignal(t, setDone, "panic handler updates did not finish")
	tasksDone := make(chan struct{})
	go func() {
		tasks.Wait()
		close(tasksDone)
	}()
	waitForSignal(t, tasksDone, "panic tasks did not finish")
}

func runConcurrentCapacityReaders(t *testing.T, tune func(int32), capacity func() int32, free func() int32) {
	t.Helper()
	start := make(chan struct{})
	var callers sync.WaitGroup
	var observed atomic.Int64
	for range 8 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			<-start
			for range 4096 {
				observed.Add(int64(capacity()))
				observed.Add(int64(free()))
			}
		}()
	}
	callers.Add(1)
	go func() {
		defer callers.Done()
		<-start
		for iteration := 0; iteration < 4096; iteration++ {
			if iteration%2 == 0 {
				tune(2)
			} else {
				tune(8)
			}
		}
	}()
	close(start)
	callers.Wait()
	if observed.Load() == 0 {
		t.Fatal("capacity readers did not execute")
	}
}

func waitForAtomicValue(t *testing.T, value *atomic.Int32, want int32, failure string) {
	t.Helper()
	deadline := time.Now().Add(poolxBarrierTimeout)
	for value.Load() != want {
		if time.Now().After(deadline) {
			t.Fatal(failure)
		}
		runtime.Gosched()
	}
}

func waitForRunningAtMost(t *testing.T, running func() int32, maximum int32, failure string) {
	t.Helper()
	deadline := time.Now().Add(poolxBarrierTimeout)
	for running() > maximum {
		if time.Now().After(deadline) {
			t.Fatal(failure)
		}
		runtime.Gosched()
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(poolxBarrierTimeout):
		t.Fatal(failure)
	}
}

func waitForPriorityQueueWaiter(t *testing.T, queue *PriorityQueue) {
	t.Helper()
	deadline := time.Now().Add(poolxBarrierTimeout)
	for {
		queue.lock.Lock()
		waiting := len(queue.waiters.waiters)
		queue.lock.Unlock()
		if waiting == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("priority queue waiter was not registered")
		}
		runtime.Gosched()
	}
}

func waitForResult(t *testing.T, result <-chan error, failure string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(poolxBarrierTimeout):
		t.Fatal(failure)
		return nil
	}
}
