package poolx

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// Future 扩展测试
// ============================================================================

func TestFutureState_String(t *testing.T) {
	tests := []struct {
		state    FutureState
		expected string
	}{
		{FutureStatePending, "Pending"},
		{FutureStateCompleted, "Completed"},
		{FutureStateFailed, "Failed"},
		{FutureStateCanceled, "Canceled"},
		{FutureState(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.state.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.state.String())
			}
		})
	}
}

func TestFuture_Cancel(t *testing.T) {
	p := New("test-cancel", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	// Create a future that takes time
	started := make(chan struct{})
	blocker := make(chan struct{})
	future := SubmitFunc(p, func() (int, error) {
		close(started)
		<-blocker
		return 42, nil
	})

	// Wait for the task to start
	<-started

	// Cancel the future
	future.Cancel()

	// Should be canceled
	if !future.IsCanceled() {
		t.Error("future should be canceled")
	}

	close(blocker)
}

func TestFuture_IsCompleted(t *testing.T) {
	p := New("test-completed", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	future := SubmitFunc(p, func() (int, error) {
		return 42, nil
	})

	_, _ = future.Get()

	if !future.IsCompleted() {
		t.Error("future should be completed")
	}
}

func TestFuture_IsFailed(t *testing.T) {
	p := New("test-failed", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	future := SubmitFunc(p, func() (int, error) {
		return 0, errors.New("error")
	})

	_, _ = future.Get()

	if !future.IsFailed() {
		t.Error("future should be failed")
	}
}

func TestFuture_State(t *testing.T) {
	p := New("test-state", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	future := SubmitFunc(p, func() (int, error) {
		return 42, nil
	})

	_, _ = future.Get()

	state := future.State()
	if state != FutureStateCompleted {
		t.Errorf("expected FutureStateCompleted, got %v", state)
	}
}

func TestFuture_Done(t *testing.T) {
	p := New("test-done", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	future := SubmitFunc(p, func() (int, error) {
		return 42, nil
	})

	// Wait for done channel
	select {
	case <-future.Done():
		// OK
	case <-time.After(time.Second):
		t.Error("future should be done")
	}
}

func TestSubmitFuncCtx(t *testing.T) {
	p := New("test-ctx", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	ctx := context.Background()
	future := SubmitFuncCtx(p, ctx, func(ctx context.Context) (int, error) {
		return 42, nil
	})

	result, err := future.Get()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

func TestSubmitFuncCtx_Canceled(t *testing.T) {
	p := New("test-ctx-cancel", WithMaxWorkers(1), WithAutoScale(false))
	defer p.Release()

	// Block the worker
	blocker := make(chan struct{})
	_ = p.Submit(func() {
		<-blocker
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	future := SubmitFuncCtx(p, ctx, func(ctx context.Context) (int, error) {
		return 42, nil
	})

	_, err := future.Get()
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	close(blocker)
}

func TestTrySubmitFunc(t *testing.T) {
	p := New("test-try", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	future := TrySubmitFunc(p, func() (int, error) {
		return 42, nil
	})

	if future == nil {
		t.Error("TrySubmitFunc should succeed")
		return
	}

	result, err := future.Get()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

func TestFutureGroup_WaitWithTimeout(t *testing.T) {
	p := New("test-group-timeout", WithMaxWorkers(4), WithAutoScale(false))
	defer p.Release()

	group := NewFutureGroup[int]()

	for i := 0; i < 3; i++ {
		n := i
		f := SubmitFunc(p, func() (int, error) {
			return n, nil
		})
		group.Add(f)
	}

	results, err := group.WaitWithTimeout(time.Second)
	if err != nil {
		t.Errorf("WaitWithTimeout failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestFutureGroup_WaitWithContext(t *testing.T) {
	p := New("test-group-ctx", WithMaxWorkers(4), WithAutoScale(false))
	defer p.Release()

	group := NewFutureGroup[int]()

	for i := 0; i < 3; i++ {
		n := i
		f := SubmitFunc(p, func() (int, error) {
			return n, nil
		})
		group.Add(f)
	}

	ctx := context.Background()
	results, err := group.WaitWithContext(ctx)
	if err != nil {
		t.Errorf("WaitWithContext failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestFutureGroup_AllCompleted(t *testing.T) {
	p := New("test-group-all", WithMaxWorkers(4), WithAutoScale(false))
	defer p.Release()

	group := NewFutureGroup[int]()

	for i := 0; i < 3; i++ {
		n := i
		f := SubmitFunc(p, func() (int, error) {
			return n, nil
		})
		group.Add(f)
	}

	_, _ = group.Wait()

	if !group.AllCompleted() {
		t.Error("all futures should be completed")
	}
}

func TestFutureGroup_AnyFailed(t *testing.T) {
	p := New("test-group-fail", WithMaxWorkers(4), WithAutoScale(false))
	defer p.Release()

	group := NewFutureGroup[int]()

	f1 := SubmitFunc(p, func() (int, error) { return 1, nil })
	f2 := SubmitFunc(p, func() (int, error) { return 0, errors.New("error") })

	group.Add(f1)
	group.Add(f2)

	_, _ = group.Wait()

	if !group.AnyFailed() {
		t.Error("should have failed future")
	}
}

func TestFutureGroup_Count(t *testing.T) {
	group := NewFutureGroup[int]()

	if group.Count() != 0 {
		t.Error("empty group should have count 0")
	}

	p := New("test-group-count", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	f := SubmitFunc(p, func() (int, error) { return 1, nil })
	group.Add(f)

	if group.Count() != 1 {
		t.Errorf("expected count 1, got %d", group.Count())
	}
}

func TestPromise_Fail(t *testing.T) {
	promise, future := NewPromise[int]()

	go func() {
		promise.Fail(errors.New("test error"))
	}()

	_, err := future.Get()
	if err == nil {
		t.Error("expected error")
	}
}

func TestPromise_Future(t *testing.T) {
	promise, future := NewPromise[int]()

	if promise.Future() != future {
		t.Error("Promise.Future() should return the associated future")
	}
}

func TestAsyncCtx(t *testing.T) {
	ctx := context.Background()
	future := AsyncCtx(ctx, func(ctx context.Context) (string, error) {
		return "hello", nil
	})

	result, err := Await(future)
	if err != nil {
		t.Errorf("AsyncCtx failed: %v", err)
	}
	if result != "hello" {
		t.Errorf("expected 'hello', got '%s'", result)
	}
}

func TestAwaitFirst(t *testing.T) {
	p := New("test-first", WithMaxWorkers(4), WithAutoScale(false))
	defer p.Release()

	f1 := SubmitFunc(p, func() (int, error) {
		time.Sleep(100 * time.Millisecond)
		return 1, nil
	})
	f2 := SubmitFunc(p, func() (int, error) {
		return 2, nil
	})

	result, idx, err := AwaitFirst(f1, f2)
	if err != nil {
		t.Errorf("AwaitFirst failed: %v", err)
	}

	// f2 should complete first
	if result != 2 || idx != 1 {
		t.Logf("result=%d, idx=%d (timing dependent)", result, idx)
	}
}

func TestAwaitAny(t *testing.T) {
	p := New("test-any", WithMaxWorkers(4), WithAutoScale(false))
	defer p.Release()

	f1 := SubmitFunc(p, func() (int, error) {
		return 0, errors.New("error1")
	})
	f2 := SubmitFunc(p, func() (int, error) {
		return 42, nil
	})

	result, idx, err := AwaitAny(f1, f2)
	if err != nil {
		t.Errorf("AwaitAny failed: %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
	_ = idx // May vary based on timing
}

// ============================================================================
// Hooks 扩展测试
// ============================================================================

func TestHookType_String(t *testing.T) {
	tests := []struct {
		hook     HookType
		expected string
	}{
		{HookBeforeSubmit, "BeforeSubmit"},
		{HookAfterSubmit, "AfterSubmit"},
		{HookBeforeTask, "BeforeTask"},
		{HookAfterTask, "AfterTask"},
		{HookOnPanic, "OnPanic"},
		{HookOnReject, "OnReject"},
		{HookOnTimeout, "OnTimeout"},
		{HookOnWorkerStart, "OnWorkerStart"},
		{HookOnWorkerStop, "OnWorkerStop"},
		{HookOnScaleUp, "OnScaleUp"},
		{HookOnScaleDown, "OnScaleDown"},
		{HookType(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.hook.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.hook.String())
			}
		})
	}
}

func TestHooks_Register(t *testing.T) {
	hooks := NewHooks()

	var called atomic.Bool
	hooks.Register(HookBeforeTask, func(_ HookType, _ any) {
		called.Store(true)
	})

	hooks.Trigger(HookBeforeTask, nil)

	if !called.Load() {
		t.Error("hook should be called")
	}
}

func TestHooks_Unregister(t *testing.T) {
	hooks := NewHooks()

	hooks.Register(HookBeforeTask, func(_ HookType, _ any) {})
	hooks.Unregister(HookBeforeTask)

	// Trigger should not panic
	hooks.Trigger(HookBeforeTask, nil)
}

func TestHooks_UnregisterAll(t *testing.T) {
	hooks := NewHooks()

	hooks.Register(HookBeforeTask, func(_ HookType, _ any) {})
	hooks.Register(HookAfterTask, func(_ HookType, _ any) {})
	hooks.UnregisterAll()

	// Triggers should not panic
	hooks.Trigger(HookBeforeTask, nil)
	hooks.Trigger(HookAfterTask, nil)
}

func TestHooks_TriggerAsync(t *testing.T) {
	hooks := NewHooks()

	var called atomic.Bool
	hooks.Register(HookBeforeTask, func(_ HookType, _ any) {
		called.Store(true)
	})

	hooks.TriggerAsync(HookBeforeTask, nil)

	time.Sleep(50 * time.Millisecond)

	if !called.Load() {
		t.Error("TriggerAsync should call hook")
	}
}

func TestHookBuilder_BeforeSubmit(t *testing.T) {
	var called atomic.Bool

	hooks := NewHookBuilder().
		BeforeSubmit(func(info *TaskInfo) {
			called.Store(true)
		}).
		Build()

	p := New("test-hook-submit", WithMaxWorkers(2), WithAutoScale(false), WithHooks(hooks))
	defer p.Release()

	_ = p.Submit(func() {})

	time.Sleep(50 * time.Millisecond)

	if !called.Load() {
		t.Error("BeforeSubmit hook not called")
	}
}

func TestHookBuilder_AfterSubmit(t *testing.T) {
	var called atomic.Bool

	hooks := NewHookBuilder().
		AfterSubmit(func(info *TaskInfo) {
			called.Store(true)
		}).
		Build()

	p := New("test-hook-after", WithMaxWorkers(2), WithAutoScale(false), WithHooks(hooks))
	defer p.Release()

	_ = p.Submit(func() {})

	time.Sleep(50 * time.Millisecond)

	if !called.Load() {
		t.Error("AfterSubmit hook not called")
	}
}

func TestHookBuilder_OnWorkerStart(t *testing.T) {
	var called atomic.Bool

	hooks := NewHookBuilder().
		OnWorkerStart(func(info *WorkerInfo) {
			called.Store(true)
		}).
		Build()

	p := New("test-hook-worker", WithMaxWorkers(2), WithMinWorkers(0), WithAutoScale(false), WithHooks(hooks))
	defer p.Release()

	// Submit a task to start a worker
	_ = p.SubmitWait(func() {})

	time.Sleep(50 * time.Millisecond)

	if !called.Load() {
		t.Error("OnWorkerStart hook not called")
	}
}

// ============================================================================
// Pool Options 测试
// ============================================================================

func TestWithQueueSize(t *testing.T) {
	p := New("test-qsize", WithMaxWorkers(2), WithQueueSize(100))
	defer p.Release()

	// Pool should work with custom queue size
	var counter atomic.Int32
	for i := 0; i < 10; i++ {
		_ = p.Submit(func() {
			counter.Add(1)
		})
	}

	time.Sleep(50 * time.Millisecond)

	if counter.Load() != 10 {
		t.Errorf("expected 10, got %d", counter.Load())
	}
}

func TestWithWorkerExpiry(t *testing.T) {
	p := New("test-expiry",
		WithMaxWorkers(4),
		WithMinWorkers(0),
		WithAutoScale(false),
		WithWorkerExpiry(50*time.Millisecond),
	)
	defer p.Release()

	_ = p.SubmitWait(func() {})

	// Wait for workers to expire
	time.Sleep(200 * time.Millisecond)

	// Workers should have expired
	if p.Running() > 0 {
		t.Errorf("expected 0 running workers, got %d", p.Running())
	}
}

func TestWithPreAlloc(t *testing.T) {
	p := New("test-prealloc",
		WithMaxWorkers(4),
		WithMinWorkers(4),
		WithAutoScale(false),
		WithPreAlloc(true),
	)
	defer p.Release()

	// Workers should be pre-allocated
	time.Sleep(50 * time.Millisecond)
}

func TestWithScaleInterval(t *testing.T) {
	p := New("test-interval",
		WithMaxWorkers(10),
		WithMinWorkers(1),
		WithAutoScale(true),
		WithScaleInterval(10*time.Millisecond),
	)
	defer p.Release()

	var counter atomic.Int32
	for i := 0; i < 10; i++ {
		_ = p.Submit(func() {
			counter.Add(1)
		})
	}

	time.Sleep(100 * time.Millisecond)

	if counter.Load() != 10 {
		t.Errorf("expected 10, got %d", counter.Load())
	}
}

func TestWithPriorityQueue(t *testing.T) {
	p := New("test-priority",
		WithMaxWorkers(2),
		WithAutoScale(false),
		WithPriorityQueue(true),
	)
	defer p.Release()

	var counter atomic.Int32
	_ = p.SubmitWithOptions(func() {
		counter.Add(1)
	}, WithTaskPriority(PriorityHigh))

	time.Sleep(50 * time.Millisecond)

	if counter.Load() != 1 {
		t.Errorf("expected 1, got %d", counter.Load())
	}
}

// testLogger implements Logger interface for testing
type testLogger struct {
	called *atomic.Bool
}

func (l *testLogger) Printf(format string, args ...any) {
	l.called.Store(true)
}

func TestWithLogger(t *testing.T) {
	var logCalled atomic.Bool

	logger := &testLogger{called: &logCalled}

	p := New("test-logger",
		WithMaxWorkers(2),
		WithAutoScale(false),
		WithLogger(logger),
	)
	defer p.Release()

	_ = p.SubmitWait(func() {})
}

func TestDefaultPanicHandler(t *testing.T) {
	p := New("test-panic-default", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	_ = p.Submit(func() {
		panic("test panic")
	})

	time.Sleep(100 * time.Millisecond)

	// Pool should still be usable
	var executed atomic.Bool
	_ = p.Submit(func() {
		executed.Store(true)
	})

	time.Sleep(50 * time.Millisecond)

	if !executed.Load() {
		t.Error("pool should still be usable after panic")
	}
}

// ============================================================================
// MetricsSnapshot 扩展测试
// ============================================================================

func TestMetricsSnapshot_AvgWaitTime_Zero(t *testing.T) {
	snapshot := MetricsSnapshot{
		CompletedTasks: 0,
		TotalWaitTime:  time.Second,
	}

	if snapshot.AvgWaitTime() != 0 {
		t.Error("AvgWaitTime should be 0 when no tasks completed")
	}
}

func TestMetricsSnapshot_AvgExecTime_Zero(t *testing.T) {
	snapshot := MetricsSnapshot{
		CompletedTasks: 0,
		TotalExecTime:  time.Second,
	}

	if snapshot.AvgExecTime() != 0 {
		t.Error("AvgExecTime should be 0 when no tasks completed")
	}
}

func TestMetricsSnapshot_Throughput_Zero(t *testing.T) {
	snapshot := MetricsSnapshot{
		CompletedTasks: 100,
	}

	if snapshot.Throughput(0) != 0 {
		t.Error("Throughput should be 0 when duration is 0")
	}
}
