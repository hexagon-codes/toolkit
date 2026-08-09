package poolx

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestSubmitWithContextKeepsWorkerUntilTaskReturns(t *testing.T) {
	taskStarted := make(chan struct{})
	cancellationObserved := make(chan struct{})
	releaseTask := make(chan struct{})
	taskFinished := make(chan struct{})

	hooks := NewHookBuilder().
		AfterTask(func(*TaskInfo) {
			close(taskFinished)
		}).
		Build()
	p := New("context-task-contract",
		WithMaxWorkers(1),
		WithAutoScale(false),
		WithHooks(hooks),
	)
	defer p.Release()
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			close(releaseTask)
		})
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := p.SubmitWithContext(ctx, func(taskCtx context.Context) {
		close(taskStarted)
		<-taskCtx.Done()
		close(cancellationObserved)
		<-releaseTask
	})
	if err != nil {
		t.Fatalf("SubmitWithContext() error = %v", err)
	}

	select {
	case <-taskStarted:
	case <-time.After(time.Second):
		t.Fatal("context-aware task did not start")
	}

	cancel()
	select {
	case <-cancellationObserved:
	case <-time.After(time.Second):
		t.Fatal("task did not observe context cancellation")
	}

	if got := p.Metrics().CompletedTasks; got != 0 {
		t.Fatalf("CompletedTasks before task return = %d, want 0", got)
	}
	if p.TrySubmit(func() {}) {
		t.Fatal("worker became available before the canceled task returned")
	}

	release()
	select {
	case <-taskFinished:
	case <-time.After(time.Second):
		t.Fatal("task did not finish after release")
	}
	if got := p.Metrics().CompletedTasks; got != 1 {
		t.Fatalf("CompletedTasks after task return = %d, want 1", got)
	}
}

func TestSubmitFuncCtxCancelReachesRunningTask(t *testing.T) {
	p := New("future-context-cancel", WithMaxWorkers(1), WithAutoScale(false))
	defer p.Release()
	parentCtx, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	taskStarted := make(chan struct{})
	cancellationObserved := make(chan struct{})

	future := SubmitFuncCtx(parentCtx, p, func(ctx context.Context) (int, error) {
		close(taskStarted)
		<-ctx.Done()
		close(cancellationObserved)
		return 0, ctx.Err()
	})
	select {
	case <-taskStarted:
	case <-time.After(time.Second):
		t.Fatal("context-aware future task did not start")
	}

	future.Cancel()
	select {
	case <-cancellationObserved:
	case <-time.After(time.Second):
		t.Fatal("running future task did not observe Future.Cancel()")
	}
}

func TestAwaitAnyDoesNotLeaveWaiterGoroutines(t *testing.T) {
	const pendingCount = 128

	futures := make([]*Future[int], 0, pendingCount+1)
	for range pendingCount {
		future := NewFuture[int]()
		futures = append(futures, future)
		t.Cleanup(future.Cancel)
	}
	winner := NewFuture[int]()
	winner.Complete(42)
	futures = append(futures, winner)

	before := runtime.NumGoroutine()
	value, index, err := AwaitAny(futures...)
	if err != nil {
		t.Fatalf("AwaitAny() error = %v", err)
	}
	if value != 42 || index != pendingCount {
		t.Fatalf("AwaitAny() = (%d, %d), want (42, %d)", value, index, pendingCount)
	}

	if delta := runtime.NumGoroutine() - before; delta > 4 {
		t.Fatalf("AwaitAny() left %d additional goroutines", delta)
	}
}

func TestAutoScalerStopJoinsBeforeRestart(t *testing.T) {
	previousProcs := runtime.GOMAXPROCS(1)
	t.Cleanup(func() {
		runtime.GOMAXPROCS(previousProcs)
	})

	p := New("scaler-lifecycle", WithMaxWorkers(1), WithAutoScale(false))
	defer p.Release()
	scaler := NewAutoScaler(p, ScalerConfig{ScaleInterval: time.Hour})
	baseline := autoScalerLoopCount()

	scaler.Start()
	scaler.Stop()
	scaler.Start()
	runtime.Gosched()
	loopsAfterRestart := autoScalerLoopCount() - baseline

	scaler.Stop()
	loopsAfterStop := autoScalerLoopCount() - baseline
	cleanupDeadline := time.Now().Add(time.Second)
	for autoScalerLoopCount() > baseline && time.Now().Before(cleanupDeadline) {
		runtime.Gosched()
	}

	if loopsAfterRestart != 1 {
		t.Errorf("active scaling loops after restart = %d, want 1", loopsAfterRestart)
	}
	if loopsAfterStop != 0 {
		t.Errorf("active scaling loops after Stop() = %d, want 0", loopsAfterStop)
	}
}

func TestAutoScalerConcurrentStartStop(t *testing.T) {
	p := New("scaler-concurrent-lifecycle", WithMaxWorkers(1), WithAutoScale(false))
	defer p.Release()
	scaler := NewAutoScaler(p, ScalerConfig{ScaleInterval: time.Hour})
	defer scaler.Stop()

	start := make(chan struct{})
	var callers sync.WaitGroup
	for caller := 0; caller < 16; caller++ {
		callers.Add(1)
		go func(startWithStop bool) {
			defer callers.Done()
			<-start
			for iteration := 0; iteration < 32; iteration++ {
				if (iteration%2 == 0) == startWithStop {
					scaler.Stop()
					continue
				}
				scaler.Start()
			}
		}(caller%2 == 0)
	}
	close(start)
	callers.Wait()
	scaler.Stop()

	if scaler.IsRunning() {
		t.Fatal("scaler is still running after Stop()")
	}
}

func autoScalerLoopCount() int {
	bufferSize := 64 << 10
	for {
		stacks := make([]byte, bufferSize)
		length := runtime.Stack(stacks, true)
		if length < len(stacks) {
			return bytes.Count(stacks[:length], []byte("(*AutoScaler).scalingLoop"))
		}
		bufferSize *= 2
	}
}

func TestNewMultiPoolRejectsNonPositiveSize(t *testing.T) {
	tests := []struct {
		name string
		size int
	}{
		{name: "zero", size: 0},
		{name: "negative", size: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pool, err := NewMultiPool(test.size, 1, RoundRobin, WithAutoScale(false))
			if pool != nil {
				pool.Release()
				t.Fatal("NewMultiPool() returned a pool for a non-positive size")
			}
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("NewMultiPool() error = %v, want ErrInvalidConfig", err)
			}
			const want = "invalid configuration: multi-pool size must be greater than zero"
			if err.Error() != want {
				t.Fatalf("NewMultiPool() error = %q, want %q", err, want)
			}
		})
	}
}

func TestNewObjectPoolRejectsNilFactory(t *testing.T) {
	t.Run("untyped nil", func(t *testing.T) {
		pool, err := NewObjectPool[*int](nil, nil)
		if pool != nil {
			t.Fatal("NewObjectPool() returned a pool for a nil factory")
		}
		if !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("NewObjectPool() error = %v, want ErrInvalidConfig", err)
		}
		const want = "invalid configuration: object pool factory must not be nil"
		if err.Error() != want {
			t.Fatalf("NewObjectPool() error = %q, want %q", err, want)
		}
	})

	t.Run("typed nil", func(t *testing.T) {
		var factory func() *int
		pool, err := NewObjectPool(factory, nil)
		if pool != nil {
			t.Fatal("NewObjectPool() returned a pool for a typed nil factory")
		}
		if !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("NewObjectPool() error = %v, want ErrInvalidConfig", err)
		}
	})
}

func TestNewObjectPoolRejectsFactoryReturningTypedNil(t *testing.T) {
	pool, err := NewObjectPool(
		func() *int { return nil },
		func(**int) {},
	)
	if pool != nil {
		t.Fatal("NewObjectPool() returned a pool for a factory returning typed nil")
	}
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("NewObjectPool() error = %v, want ErrInvalidConfig", err)
	}
	const want = "invalid configuration: object pool factory must return a non-nil value"
	if err.Error() != want {
		t.Fatalf("NewObjectPool() error = %q, want %q", err, want)
	}
}
