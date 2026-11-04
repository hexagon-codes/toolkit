package poolx

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNonPositiveWorkerExpiryIsNormalized(t *testing.T) {
	p := New("review-expiry", WithWorkerExpiry(0))
	defer p.Release()

	pf := NewPoolWithFunc("review-expiry-func", func(any) {}, WithWorkerExpiry(-time.Second))
	defer pf.Release()

	want := DefaultConfig().WorkerExpiry
	if p.config.WorkerExpiry != want {
		t.Fatalf("Pool WorkerExpiry = %v, want %v", p.config.WorkerExpiry, want)
	}
	if pf.config.WorkerExpiry != want {
		t.Fatalf("PoolWithFunc WorkerExpiry = %v, want %v", pf.config.WorkerExpiry, want)
	}

	// 让 cleaner goroutine 有机会启动；未归一化时会在后台 panic。
	time.Sleep(20 * time.Millisecond)
}

func TestInvokeWithContextRejectsNilContext(t *testing.T) {
	block := make(chan struct{})
	p := NewPoolWithFunc(
		"review-nil-context",
		func(any) { <-block },
		WithMaxWorkers(1),
		WithMinWorkers(1),
	)
	defer func() {
		close(block)
		p.Release()
	}()

	if err := p.Invoke("occupy"); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	if err := p.InvokeWithContext(nil, "nil"); !errors.Is(err, ErrInvalidArg) { //nolint:staticcheck // 需要验证 nil context 会被显式拒绝。
		t.Fatalf("InvokeWithContext(nil) error = %v, want ErrInvalidArg", err)
	}
}

func TestInvokeWithContextDoesNotSubmitCanceledTask(t *testing.T) {
	var ran atomic.Int32
	p := NewPoolWithFunc(
		"review-canceled-context",
		func(any) { ran.Add(1) },
		WithMaxWorkers(1),
		WithMinWorkers(1),
	)
	defer p.Release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.InvokeWithContext(ctx, "canceled"); !errors.Is(err, context.Canceled) {
		t.Fatalf("InvokeWithContext(canceled) error = %v, want context.Canceled", err)
	}
	time.Sleep(10 * time.Millisecond)
	if ran.Load() != 0 {
		t.Fatalf("canceled task ran %d time(s), want 0", ran.Load())
	}
}

func TestSubmitRejectsNilTask(t *testing.T) {
	p := New("review-nil-task")
	defer p.Release()

	if err := p.Submit(nil); !errors.Is(err, ErrInvalidArg) {
		t.Fatalf("Submit(nil) error = %v, want ErrInvalidArg", err)
	}
	if _, err := p.SubmitBatch([]func(){nil}); !errors.Is(err, ErrInvalidArg) {
		t.Fatalf("SubmitBatch(nil) error = %v, want ErrInvalidArg", err)
	}
	if p.TrySubmit(nil) {
		t.Fatal("TrySubmit(nil) = true, want false")
	}
}

func TestWithStealBatchSizeOption(t *testing.T) {
	config := DefaultConfig()
	WithStealBatchSize(2)(&config)
	if config.StealBatchSize != 2 {
		t.Fatalf("StealBatchSize = %d, want 2", config.StealBatchSize)
	}
}

func TestWorkerStealsConfiguredBatchSize(t *testing.T) {
	p := New(
		"review-steal-batch",
		WithMaxWorkers(4),
		WithMinWorkers(0),
		WithWorkStealing(true),
		WithStealBatchSize(2),
	)
	defer p.Release()

	generation := p.generation.Load()
	scheduler := NewStealingScheduler()
	thiefQueue := NewWorkStealingDeque[task](8)
	victimQueue := NewWorkStealingDeque[task](8)
	scheduler.Register(1, thiefQueue)
	scheduler.Register(2, victimQueue)
	generation.stealingScheduler = scheduler

	var executed atomic.Int32
	for range 4 {
		victimQueue.PushBottom(&task{fn: func() { executed.Add(1) }})
	}
	thief := &worker{
		pool:       p,
		generation: generation,
		id:         1,
		localQueue: thiefQueue,
	}
	thief.processLocalQueue()

	if got := executed.Load(); got != 2 {
		t.Fatalf("executed tasks = %d, want 2", got)
	}
}

func TestStealBatchSizeStealsConfiguredBatch(t *testing.T) {
	for _, test := range []struct {
		name      string
		batchSize int32
		want      int32
	}{
		{name: "configured", batchSize: 2, want: 2},
		{name: "non-positive", batchSize: 0, want: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			scheduler := NewStealingScheduler()
			victim := NewWorkStealingDeque[task](8)
			thief := NewWorkStealingDeque[task](8)
			scheduler.Register(1, victim)
			scheduler.Register(2, thief)

			var ran atomic.Int32
			for range 3 {
				victim.PushBottom(&task{fn: func() { ran.Add(1) }})
			}
			pool := &Pool{config: Config{StealBatchSize: test.batchSize}}
			worker := &worker{
				id:         2,
				pool:       pool,
				generation: &poolGeneration{stealingScheduler: scheduler, metrics: &Metrics{}},
				localQueue: thief,
			}

			worker.processLocalQueue()

			if got := ran.Load(); got != test.want {
				t.Fatalf("executed %d stolen tasks, want %d", got, test.want)
			}
			if got := worker.generation.metrics.StolenTasks.Load(); got != int64(test.want) {
				t.Fatalf("StolenTasks = %d, want %d", got, test.want)
			}
			if got := int32(victim.Len()); got != 3-test.want {
				t.Fatalf("victim queue length = %d, want %d", got, 3-test.want)
			}
		})
	}
}

func TestWorkerStackRemove(t *testing.T) {
	stack := newWorkerStack(4)
	workers := []*worker{{}, {}, {}, {}}
	for _, worker := range workers {
		if !stack.push(worker) {
			t.Fatal("workerStack.push() = false, want true")
		}
	}
	if !stack.remove(workers[1]) {
		t.Fatal("workerStack.remove() = false, want true")
	}
	if stack.remove(workers[1]) {
		t.Fatal("workerStack.remove() removed the same worker twice")
	}
	if got := stack.size(); got != 3 {
		t.Fatalf("workerStack.size() = %d, want 3", got)
	}
	remaining := make(map[*worker]bool)
	for range 3 {
		worker := stack.pop()
		if worker == nil {
			t.Fatal("workerStack.pop() returned nil before the stack was empty")
		}
		remaining[worker] = true
	}
	if remaining[workers[1]] || len(remaining) != 3 {
		t.Fatalf("workerStack retained the removed worker or lost entries: %#v", remaining)
	}
	if stack.pop() != nil {
		t.Fatal("workerStack.pop() returned a worker from an empty stack")
	}
}

func TestWorkStealingProcessesQueuedSubmissions(t *testing.T) {
	p := New(
		"review-steal-public",
		WithMaxWorkers(2),
		WithMinWorkers(2),
		WithQueueSize(64),
		WithAutoScale(false),
		WithWorkStealing(true),
		WithStealBatchSize(4),
	)
	shortRelease := make(chan struct{})
	longRelease := make(chan struct{})
	var shortOnce, longOnce sync.Once
	releaseShort := func() { shortOnce.Do(func() { close(shortRelease) }) }
	releaseLong := func() { longOnce.Do(func() { close(longRelease) }) }
	defer p.Release()
	defer func() {
		releaseShort()
		releaseLong()
	}()

	started := make(chan struct{}, 2)
	if err := p.Submit(func() {
		started <- struct{}{}
		<-shortRelease
	}); err != nil {
		t.Fatalf("Submit(short blocker) error = %v", err)
	}
	if err := p.Submit(func() {
		started <- struct{}{}
		<-longRelease
	}); err != nil {
		t.Fatalf("Submit(long blocker) error = %v", err)
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("blockers did not start")
		}
	}

	const queued = 20
	var submitted sync.WaitGroup
	submitted.Add(queued)
	for range queued {
		go func() {
			defer submitted.Done()
			if err := p.Submit(func() { time.Sleep(time.Millisecond) }); err != nil {
				t.Errorf("Submit(queued task) error = %v", err)
			}
		}()
	}

	generation := p.generation.Load()
	waitFor := func(condition func() bool, description string) {
		deadline := time.Now().Add(2 * time.Second)
		for !condition() {
			if time.Now().After(deadline) {
				t.Fatalf("%s", description)
			}
			time.Sleep(time.Millisecond)
		}
	}
	waitFor(func() bool {
		return generation.stealingScheduler.TotalTasks() >= queued
	}, "queued submissions did not reach local work-stealing queues")

	releaseShort()
	waitFor(func() bool {
		return p.Metrics().StolenTasks > 0
	}, "an idle worker did not steal from the blocked worker's local queue")

	releaseLong()
	submitted.Wait()
}
