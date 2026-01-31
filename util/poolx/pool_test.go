package poolx

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// Pool 基础测试
// ============================================================================

func TestPool_Submit(t *testing.T) {
	p := New("test", WithMaxWorkers(4), WithAutoScale(false))
	defer p.Release()

	var counter atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		err := p.Submit(func() {
			counter.Add(1)
			wg.Done()
		})
		if err != nil {
			wg.Done()
			t.Errorf("Submit failed: %v", err)
		}
	}

	wg.Wait()

	if counter.Load() != 100 {
		t.Errorf("expected 100, got %d", counter.Load())
	}
}

func TestPool_TrySubmit(t *testing.T) {
	p := New("test", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	var counter atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		ok := p.TrySubmit(func() {
			time.Sleep(10 * time.Millisecond)
			counter.Add(1)
			wg.Done()
		})
		if !ok {
			wg.Done()
		}
	}

	wg.Wait()

	if counter.Load() == 0 {
		t.Error("expected some tasks to complete")
	}
}

func TestPool_SubmitWait(t *testing.T) {
	p := New("test", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	var executed bool
	err := p.SubmitWait(func() {
		time.Sleep(10 * time.Millisecond)
		executed = true
	})

	if err != nil {
		t.Errorf("SubmitWait failed: %v", err)
	}

	if !executed {
		t.Error("Task should be executed")
	}
}

func TestPool_SubmitWithContext(t *testing.T) {
	p := New("test", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var executed atomic.Bool
	err := p.SubmitWithContext(ctx, func() {
		executed.Store(true)
	})

	if err != nil {
		t.Errorf("SubmitWithContext failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if !executed.Load() {
		t.Error("Task should be executed")
	}
}

func TestPool_SubmitWithContext_Cancel(t *testing.T) {
	// 创建一个小容量池，让任务排队
	p := New("test", WithMaxWorkers(1), WithAutoScale(false), WithNonBlocking(false))
	defer p.Release()

	// 先提交一个阻塞任务
	blocker := make(chan struct{})
	_ = p.Submit(func() {
		<-blocker
	})

	// 创建一个立即取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.SubmitWithContext(ctx, func() {})

	// 应该返回 context 错误
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	close(blocker)
}

func TestPool_Running(t *testing.T) {
	p := New("test", WithMaxWorkers(4), WithAutoScale(false))
	defer p.Release()

	blocker := make(chan struct{})
	for i := 0; i < 4; i++ {
		_ = p.Submit(func() {
			<-blocker
		})
	}

	time.Sleep(50 * time.Millisecond)

	running := p.Running()
	if running < 1 {
		t.Errorf("expected at least 1 running worker, got %d", running)
	}

	close(blocker)
}

func TestPool_Release(t *testing.T) {
	p := New("test", WithMaxWorkers(2), WithAutoScale(false))

	var counter atomic.Int32
	for i := 0; i < 10; i++ {
		_ = p.Submit(func() {
			time.Sleep(10 * time.Millisecond)
			counter.Add(1)
		})
	}

	p.Release()

	if counter.Load() != 10 {
		t.Errorf("expected 10, got %d", counter.Load())
	}
}

func TestPool_ReleaseTimeout(t *testing.T) {
	p := New("test", WithMaxWorkers(2), WithAutoScale(false))

	// 提交一个长时间运行的任务
	_ = p.Submit(func() {
		time.Sleep(100 * time.Millisecond)
	})

	// 使用短超时
	err := p.ReleaseTimeout(10 * time.Millisecond)
	if err != ErrTimeout {
		t.Errorf("expected ErrTimeout, got %v", err)
	}

	// 等待任务完成
	time.Sleep(100 * time.Millisecond)
}

func TestPool_Reboot(t *testing.T) {
	p := New("test", WithMaxWorkers(2), WithAutoScale(false))

	var counter atomic.Int32
	_ = p.Submit(func() {
		counter.Add(1)
	})

	time.Sleep(50 * time.Millisecond)
	p.Release()

	// 重启
	p.Reboot()

	// 应该能继续提交
	err := p.Submit(func() {
		counter.Add(1)
	})

	if err != nil {
		t.Errorf("Submit after Reboot failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	p.Release()

	if counter.Load() != 2 {
		t.Errorf("expected 2, got %d", counter.Load())
	}
}

func TestPool_SubmitAfterClose(t *testing.T) {
	p := New("test", WithMaxWorkers(2), WithAutoScale(false))
	p.Release()

	err := p.Submit(func() {})
	if err != ErrPoolClosed {
		t.Errorf("expected ErrPoolClosed, got %v", err)
	}

	ok := p.TrySubmit(func() {})
	if ok {
		t.Error("TrySubmit should return false after close")
	}

	err = p.SubmitWait(func() {})
	if err != ErrPoolClosed {
		t.Errorf("expected ErrPoolClosed, got %v", err)
	}
}

func TestPool_Tune(t *testing.T) {
	p := New("test", WithMaxWorkers(4), WithAutoScale(false))
	defer p.Release()

	if p.Cap() != 4 {
		t.Errorf("expected cap 4, got %d", p.Cap())
	}

	p.Tune(8)
	if p.Cap() != 8 {
		t.Errorf("expected cap 8, got %d", p.Cap())
	}

	p.Tune(2)
	if p.Cap() != 2 {
		t.Errorf("expected cap 2, got %d", p.Cap())
	}
}

// ============================================================================
// Panic 恢复测试
// ============================================================================

func TestPool_PanicRecovery(t *testing.T) {
	var recovered atomic.Bool

	p := New("test",
		WithMaxWorkers(2),
		WithAutoScale(false),
		WithPanicHandler(func(v any) {
			recovered.Store(true)
		}),
	)
	defer p.Release()

	_ = p.Submit(func() {
		panic("test panic")
	})

	time.Sleep(100 * time.Millisecond)

	if !recovered.Load() {
		t.Error("panic should be recovered")
	}

	// 池应该仍然可用
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
// 非阻塞模式测试
// ============================================================================

func TestPool_NonBlocking(t *testing.T) {
	p := New("test",
		WithMaxWorkers(1),
		WithAutoScale(false),
		WithNonBlocking(true),
	)
	defer p.Release()

	// 先占用唯一的 worker
	blocker := make(chan struct{})
	_ = p.Submit(func() {
		<-blocker
	})

	time.Sleep(50 * time.Millisecond)

	// 尝试提交更多任务应该失败
	err := p.Submit(func() {})
	if err != ErrPoolOverload {
		t.Errorf("expected ErrPoolOverload, got %v", err)
	}

	close(blocker)
}

func TestPool_MaxBlockingTasks(t *testing.T) {
	p := New("test",
		WithMaxWorkers(1),
		WithAutoScale(false),
		WithMaxBlockingTasks(2),
	)
	defer p.Release()

	// 先占用 worker
	blocker := make(chan struct{})
	_ = p.Submit(func() {
		<-blocker
	})

	time.Sleep(50 * time.Millisecond)

	// 启动阻塞任务
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.Submit(func() {})
		}()
	}

	time.Sleep(50 * time.Millisecond)

	// 第三个阻塞任务应该被拒绝
	err := p.Submit(func() {})
	if err != ErrPoolOverload {
		t.Errorf("expected ErrPoolOverload, got %v", err)
	}

	close(blocker)
	wg.Wait()
}

// ============================================================================
// Metrics 测试
// ============================================================================

func TestPool_Metrics(t *testing.T) {
	p := New("test", WithMaxWorkers(4), WithAutoScale(false))
	defer p.Release()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		_ = p.Submit(func() {
			time.Sleep(10 * time.Millisecond)
			wg.Done()
		})
	}

	wg.Wait()
	// Small sleep to allow metrics update (CompletedTasks is updated after wg.Done)
	time.Sleep(20 * time.Millisecond)

	metrics := p.Metrics()

	if metrics.SubmittedTasks != 10 {
		t.Errorf("expected 10 submitted tasks, got %d", metrics.SubmittedTasks)
	}

	if metrics.CompletedTasks != 10 {
		t.Errorf("expected 10 completed tasks, got %d", metrics.CompletedTasks)
	}

	if metrics.FailedTasks != 0 {
		t.Errorf("expected 0 failed tasks, got %d", metrics.FailedTasks)
	}
}

func TestPool_MetricsSnapshot(t *testing.T) {
	snapshot := MetricsSnapshot{
		CompletedTasks: 100,
		TotalWaitTime:  10 * time.Second,
		TotalExecTime:  5 * time.Second,
	}

	avgWait := snapshot.AvgWaitTime()
	if avgWait != 100*time.Millisecond {
		t.Errorf("expected 100ms avg wait, got %v", avgWait)
	}

	avgExec := snapshot.AvgExecTime()
	if avgExec != 50*time.Millisecond {
		t.Errorf("expected 50ms avg exec, got %v", avgExec)
	}

	throughput := snapshot.Throughput(10 * time.Second)
	if throughput != 10 {
		t.Errorf("expected throughput 10, got %f", throughput)
	}
}

func TestPool_MetricsSuccessRate(t *testing.T) {
	tests := []struct {
		completed int64
		failed    int64
		expected  float64
	}{
		{100, 0, 1.0},
		{90, 10, 0.9},
		{50, 50, 0.5},
		{0, 0, 1.0},
	}

	for _, tt := range tests {
		snapshot := MetricsSnapshot{
			CompletedTasks: tt.completed,
			FailedTasks:    tt.failed,
		}
		rate := snapshot.SuccessRate()
		if rate != tt.expected {
			t.Errorf("expected %f, got %f", tt.expected, rate)
		}
	}
}

// ============================================================================
// 全局函数测试
// ============================================================================

func TestGo(t *testing.T) {
	var executed atomic.Bool

	Go(func() {
		executed.Store(true)
	})

	time.Sleep(100 * time.Millisecond)

	if !executed.Load() {
		t.Error("Go should execute the function")
	}
}

func TestGoCtx(t *testing.T) {
	var executed atomic.Bool

	err := GoCtx(context.Background(), func() {
		executed.Store(true)
	})

	if err != nil {
		t.Errorf("GoCtx failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if !executed.Load() {
		t.Error("GoCtx should execute the function")
	}
}

func TestDefaultPool(t *testing.T) {
	p := DefaultPool()
	if p == nil {
		t.Error("DefaultPool should not be nil")
	}

	if p.Name() != "default" {
		t.Errorf("expected name 'default', got '%s'", p.Name())
	}
}

func TestSetCap(t *testing.T) {
	SetCap(100)
	p := DefaultPool()
	if p.Cap() != 100 {
		t.Errorf("expected cap 100, got %d", p.Cap())
	}
}

// ============================================================================
// 命名池测试
// ============================================================================

func TestNamedPool(t *testing.T) {
	p := New("mypool", WithMaxWorkers(4))
	defer p.Release()

	got, ok := GetPool("mypool")
	if !ok {
		t.Fatal("pool should be found")
	}

	if got != p {
		t.Error("should return the same pool")
	}

	// 释放后应该找不到
	p.Release()

	_, ok = GetPool("mypool")
	if ok {
		t.Error("pool should not be found after release")
	}
}

func TestMustGetPool(t *testing.T) {
	p := New("testpool", WithMaxWorkers(2))
	defer p.Release()

	got := MustGetPool("testpool")
	if got != p {
		t.Error("should return the same pool")
	}

	// 测试 panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustGetPool should panic for missing pool")
		}
	}()

	MustGetPool("nonexistent")
}

func TestRangePool(t *testing.T) {
	p1 := New("pool1", WithMaxWorkers(2))
	p2 := New("pool2", WithMaxWorkers(2))
	defer p1.Release()
	defer p2.Release()

	count := 0
	RangePool(func(name string, p *Pool) bool {
		count++
		return true
	})

	if count < 2 {
		t.Errorf("expected at least 2 pools, got %d", count)
	}
}

// ============================================================================
// MultiPool 测试
// ============================================================================

func TestMultiPool_RoundRobin(t *testing.T) {
	mp := NewMultiPool(4, 2, RoundRobin)
	defer mp.Release()

	var counter atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		err := mp.Submit(func() {
			counter.Add(1)
			wg.Done()
		})
		if err != nil {
			wg.Done()
		}
	}

	wg.Wait()

	if counter.Load() != 100 {
		t.Errorf("expected 100, got %d", counter.Load())
	}
}

func TestMultiPool_LeastTasks(t *testing.T) {
	mp := NewMultiPool(4, 2, LeastTasks)
	defer mp.Release()

	var counter atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		err := mp.Submit(func() {
			counter.Add(1)
			wg.Done()
		})
		if err != nil {
			wg.Done()
		}
	}

	wg.Wait()

	if counter.Load() != 50 {
		t.Errorf("expected 50, got %d", counter.Load())
	}
}

func TestMultiPool_TrySubmit(t *testing.T) {
	mp := NewMultiPool(2, 4, RoundRobin)
	defer mp.Release()

	var counter atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		ok := mp.TrySubmit(func() {
			counter.Add(1)
			wg.Done()
		})
		if !ok {
			wg.Done()
		}
	}

	wg.Wait()

	if counter.Load() == 0 {
		t.Error("some tasks should complete")
	}
}

func TestMultiPool_RunningFree(t *testing.T) {
	mp := NewMultiPool(4, 2, RoundRobin, WithAutoScale(false), WithMinWorkers(0))
	defer mp.Release()

	if mp.Free() != 8 {
		t.Errorf("expected free 8, got %d", mp.Free())
	}

	blocker := make(chan struct{})
	for i := 0; i < 4; i++ {
		_ = mp.Submit(func() {
			<-blocker
		})
	}

	time.Sleep(50 * time.Millisecond)

	if mp.Running() < 4 {
		t.Errorf("expected at least 4 running, got %d", mp.Running())
	}

	close(blocker)
}

func TestMultiPool_Reboot(t *testing.T) {
	mp := NewMultiPool(2, 2, RoundRobin, WithAutoScale(false))

	var counter atomic.Int32
	_ = mp.Submit(func() {
		counter.Add(1)
	})

	time.Sleep(50 * time.Millisecond)
	mp.Release()

	mp.Reboot()

	_ = mp.Submit(func() {
		counter.Add(1)
	})

	time.Sleep(50 * time.Millisecond)
	mp.Release()

	if counter.Load() != 2 {
		t.Errorf("expected 2, got %d", counter.Load())
	}
}

// ============================================================================
// 兼容旧 API 测试
// ============================================================================

func TestWorkerPool_Submit(t *testing.T) {
	p := NewWorkerPool(4)
	defer p.Close()

	var counter atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		p.Submit(func() {
			counter.Add(1)
			wg.Done()
		})
	}

	wg.Wait()

	if counter.Load() != 100 {
		t.Errorf("expected 100, got %d", counter.Load())
	}
}

func TestWorkerPool_SubmitWait(t *testing.T) {
	p := NewWorkerPool(2)
	defer p.Close()

	var executed bool
	ok := p.SubmitWait(func() {
		time.Sleep(10 * time.Millisecond)
		executed = true
	})

	if !ok {
		t.Error("SubmitWait should return true")
	}

	if !executed {
		t.Error("Task should be executed")
	}
}

func TestWorkerPool_Running(t *testing.T) {
	p := NewWorkerPool(4)
	defer p.Close()

	blocker := make(chan struct{})
	for i := 0; i < 4; i++ {
		p.Submit(func() {
			<-blocker
		})
	}

	time.Sleep(50 * time.Millisecond)

	running := p.Running()
	if running < 1 {
		t.Errorf("expected at least 1 running worker, got %d", running)
	}

	close(blocker)
}

func TestWorkerPool_Close(t *testing.T) {
	p := NewWorkerPool(2)

	var counter atomic.Int32
	for i := 0; i < 10; i++ {
		p.Submit(func() {
			time.Sleep(10 * time.Millisecond)
			counter.Add(1)
		})
	}

	p.Close()

	if counter.Load() != 10 {
		t.Errorf("expected 10, got %d", counter.Load())
	}
}

func TestWorkerPool_SubmitAfterClose(t *testing.T) {
	p := NewWorkerPool(2)
	p.Close()

	ok := p.TrySubmit(func() {})
	if ok {
		t.Error("TrySubmit should return false after close")
	}

	ok = p.SubmitWait(func() {})
	if ok {
		t.Error("SubmitWait should return false after close")
	}
}

func TestNewWorkerPool_InvalidSize(t *testing.T) {
	p := NewWorkerPool(0)
	defer p.Close()

	var executed bool
	p.SubmitWait(func() {
		executed = true
	})

	if !executed {
		t.Error("Pool with 0 size should still work")
	}
}

// ============================================================================
// ObjectPool 测试
// ============================================================================

func TestObjectPool(t *testing.T) {
	createCount := 0
	resetCount := 0

	pool := NewObjectPool(
		func() *int {
			createCount++
			v := 0
			return &v
		},
		func(v **int) {
			resetCount++
			**v = 0
		},
	)

	obj1 := pool.Get()
	*obj1 = 42
	pool.Put(obj1)

	obj2 := pool.Get()
	if *obj2 != 0 {
		t.Error("Object should be reset")
	}

	if createCount < 1 {
		t.Error("Factory should be called")
	}

	if resetCount < 1 {
		t.Error("Reset should be called")
	}
}

// ============================================================================
// ByteSlicePool 测试
// ============================================================================

func TestByteSlicePool(t *testing.T) {
	pool := NewByteSlicePool(1024)

	b := pool.Get()
	if len(b) != 1024 {
		t.Errorf("expected length 1024, got %d", len(b))
	}

	for i := range b {
		b[i] = byte(i)
	}
	pool.Put(b)

	b2 := pool.Get()
	if cap(b2) < 1024 {
		t.Error("Slice should have enough capacity")
	}
}

func TestByteSlicePool_SmallSlice(t *testing.T) {
	pool := NewByteSlicePool(1024)

	small := make([]byte, 10)
	pool.Put(small)

	b := pool.Get()
	if len(b) != 1024 {
		t.Errorf("expected length 1024, got %d", len(b))
	}
}

// ============================================================================
// BufferPool 测试
// ============================================================================

func TestBufferPool(t *testing.T) {
	pool := NewBufferPool(64)

	buf := pool.Get()
	if len(buf) != 0 {
		t.Error("Buffer should be empty")
	}

	buf = append(buf, []byte("hello")...)
	pool.Put(buf)

	buf2 := pool.Get()
	if len(buf2) != 0 {
		t.Error("Buffer should be empty after reset")
	}
}

// ============================================================================
// ParallelExecutor 测试
// ============================================================================

func TestParallelExecutor_Execute(t *testing.T) {
	exec := NewParallelExecutor(2)
	ctx := context.Background()

	var counter atomic.Int32

	tasks := make([]func() error, 10)
	for i := 0; i < 10; i++ {
		tasks[i] = func() error {
			counter.Add(1)
			return nil
		}
	}

	errs := exec.Execute(ctx, tasks...)

	for i, err := range errs {
		if err != nil {
			t.Errorf("task %d failed: %v", i, err)
		}
	}

	if counter.Load() != 10 {
		t.Errorf("expected 10, got %d", counter.Load())
	}
}

func TestParallelExecutor_ExecuteWithErrors(t *testing.T) {
	exec := NewParallelExecutor(4)
	ctx := context.Background()

	tasks := []func() error{
		func() error { return nil },
		func() error { return errors.New("error1") },
		func() error { return nil },
		func() error { return errors.New("error2") },
	}

	errs := exec.Execute(ctx, tasks...)

	if errs[0] != nil || errs[2] != nil {
		t.Error("Successful tasks should have nil error")
	}

	if errs[1] == nil || errs[3] == nil {
		t.Error("Failed tasks should have error")
	}
}

func TestParallelExecutor_ExecuteAll(t *testing.T) {
	exec := NewParallelExecutor(2)
	ctx := context.Background()

	err := exec.ExecuteAll(ctx,
		func() error { return nil },
		func() error { return nil },
	)
	if err != nil {
		t.Error("ExecuteAll should return nil for all success")
	}

	err = exec.ExecuteAll(ctx,
		func() error { return nil },
		func() error { return errors.New("error") },
	)
	if err == nil {
		t.Error("ExecuteAll should return first error")
	}
}

func TestParallelExecutor_ContextCancellation(t *testing.T) {
	exec := NewParallelExecutor(1)
	ctx, cancel := context.WithCancel(context.Background())

	cancel()

	errs := exec.Execute(ctx,
		func() error { return nil },
		func() error { return nil },
	)

	hasCtxErr := false
	for _, err := range errs {
		if err == context.Canceled {
			hasCtxErr = true
			break
		}
	}

	if !hasCtxErr {
		t.Error("Should have context cancellation error")
	}
}

// ============================================================================
// Map/ForEach 测试
// ============================================================================

func TestMap(t *testing.T) {
	ctx := context.Background()
	items := []int{1, 2, 3, 4, 5}

	results, err := Map(ctx, items, 2, func(n int) (int, error) {
		return n * 2, nil
	})

	if err != nil {
		t.Errorf("Map failed: %v", err)
	}

	expected := []int{2, 4, 6, 8, 10}
	for i, v := range results {
		if v != expected[i] {
			t.Errorf("expected %d, got %d", expected[i], v)
		}
	}
}

func TestMap_WithError(t *testing.T) {
	ctx := context.Background()
	items := []int{1, 2, 3}

	_, err := Map(ctx, items, 2, func(n int) (int, error) {
		if n == 2 {
			return 0, errors.New("error on 2")
		}
		return n, nil
	})

	if err == nil {
		t.Error("Map should return error")
	}
}

func TestMap_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	items := []int{1, 2, 3}
	_, err := Map(ctx, items, 2, func(n int) (int, error) {
		return n, nil
	})

	if err != context.Canceled {
		t.Error("Map should return context.Canceled")
	}
}

func TestForEach(t *testing.T) {
	ctx := context.Background()
	items := []int{1, 2, 3, 4, 5}

	var sum atomic.Int32
	err := ForEach(ctx, items, 2, func(n int) error {
		sum.Add(int32(n))
		return nil
	})

	if err != nil {
		t.Errorf("ForEach failed: %v", err)
	}

	if sum.Load() != 15 {
		t.Errorf("expected sum 15, got %d", sum.Load())
	}
}

func TestForEach_WithError(t *testing.T) {
	ctx := context.Background()
	items := []int{1, 2, 3}

	err := ForEach(ctx, items, 1, func(n int) error {
		if n == 2 {
			return errors.New("error on 2")
		}
		return nil
	})

	if err == nil {
		t.Error("ForEach should return error")
	}
}

func TestNewParallelExecutor_InvalidConcurrency(t *testing.T) {
	exec := NewParallelExecutor(0)
	ctx := context.Background()

	var executed bool
	err := exec.ExecuteAll(ctx, func() error {
		executed = true
		return nil
	})

	if err != nil || !executed {
		t.Error("Executor with 0 concurrency should still work")
	}
}

// ============================================================================
// PoolWithFunc 测试
// ============================================================================

func TestPoolWithFunc_Invoke(t *testing.T) {
	var sum atomic.Int64
	var tasksDone sync.WaitGroup

	p := NewPoolWithFunc("test", func(arg any) {
		sum.Add(int64(arg.(int)))
		tasksDone.Done()
	}, WithMaxWorkers(4), WithAutoScale(false))
	defer p.Release()

	for i := 1; i <= 100; i++ {
		tasksDone.Add(1)
		err := p.Invoke(i)
		if err != nil {
			tasksDone.Done()
			t.Errorf("Invoke failed: %v", err)
		}
	}

	tasksDone.Wait()

	// Sum of 1 to 100 = 5050
	if sum.Load() != 5050 {
		t.Errorf("expected sum 5050, got %d", sum.Load())
	}
}

func TestPoolWithFunc_TryInvoke(t *testing.T) {
	var counter atomic.Int32
	p := NewPoolWithFunc("test", func(arg any) {
		counter.Add(1)
	}, WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	for i := 0; i < 10; i++ {
		p.TryInvoke(i)
	}

	time.Sleep(100 * time.Millisecond)

	if counter.Load() == 0 {
		t.Error("expected some tasks to complete")
	}
}

func TestPoolWithFunc_InvokeWithTimeout(t *testing.T) {
	p := NewPoolWithFunc("test", func(arg any) {
		time.Sleep(10 * time.Millisecond)
	}, WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	err := p.InvokeWithTimeout(1, 100*time.Millisecond)
	if err != nil {
		t.Errorf("InvokeWithTimeout failed: %v", err)
	}
}

func TestPoolWithFunc_InvokeWithContext(t *testing.T) {
	p := NewPoolWithFunc("test", func(arg any) {
		time.Sleep(10 * time.Millisecond)
	}, WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	ctx := context.Background()
	err := p.InvokeWithContext(ctx, 1)
	if err != nil {
		t.Errorf("InvokeWithContext failed: %v", err)
	}
}

func TestPoolWithFunc_Release(t *testing.T) {
	var counter atomic.Int32
	p := NewPoolWithFunc("test", func(arg any) {
		counter.Add(1)
	}, WithMaxWorkers(2), WithAutoScale(false))

	for i := 0; i < 10; i++ {
		_ = p.Invoke(i)
	}

	p.Release()

	if counter.Load() != 10 {
		t.Errorf("expected 10, got %d", counter.Load())
	}
}

func TestPoolWithFunc_Reboot(t *testing.T) {
	var counter atomic.Int32
	p := NewPoolWithFunc("test", func(arg any) {
		counter.Add(1)
	}, WithMaxWorkers(2), WithAutoScale(false))

	_ = p.Invoke(1)
	time.Sleep(50 * time.Millisecond)
	p.Release()

	p.Reboot()

	_ = p.Invoke(2)
	time.Sleep(50 * time.Millisecond)
	p.Release()

	if counter.Load() != 2 {
		t.Errorf("expected 2, got %d", counter.Load())
	}
}

// ============================================================================
// Future 测试
// ============================================================================

func TestFuture_Basic(t *testing.T) {
	p := New("test", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	future := SubmitFunc(p, func() (int, error) {
		return 42, nil
	})

	result, err := future.Get()
	if err != nil {
		t.Errorf("Future.Get failed: %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

func TestFuture_Error(t *testing.T) {
	p := New("test", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	future := SubmitFunc(p, func() (int, error) {
		return 0, errors.New("test error")
	})

	_, err := future.Get()
	if err == nil {
		t.Error("expected error")
	}
}

func TestFuture_GetWithTimeout(t *testing.T) {
	p := New("test", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	future := SubmitFunc(p, func() (int, error) {
		return 42, nil
	})

	result, err := future.GetWithTimeout(time.Second)
	if err != nil {
		t.Errorf("Future.GetWithTimeout failed: %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

func TestFuture_GetWithContext(t *testing.T) {
	p := New("test", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	future := SubmitFunc(p, func() (int, error) {
		return 42, nil
	})

	ctx := context.Background()
	result, err := future.GetWithContext(ctx)
	if err != nil {
		t.Errorf("Future.GetWithContext failed: %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

func TestFuture_IsDone(t *testing.T) {
	p := New("test", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	future := SubmitFunc(p, func() (int, error) {
		time.Sleep(50 * time.Millisecond)
		return 42, nil
	})

	if future.IsDone() {
		t.Error("future should not be done yet")
	}

	time.Sleep(100 * time.Millisecond)

	if !future.IsDone() {
		t.Error("future should be done")
	}
}

func TestFutureGroup(t *testing.T) {
	p := New("test", WithMaxWorkers(4), WithAutoScale(false))
	defer p.Release()

	group := NewFutureGroup[int]()

	for i := 0; i < 5; i++ {
		n := i
		f := SubmitFunc(p, func() (int, error) {
			return n * 2, nil
		})
		group.Add(f)
	}

	results, err := group.Wait()
	if err != nil {
		t.Errorf("FutureGroup.Wait failed: %v", err)
	}

	sum := 0
	for _, r := range results {
		sum += r
	}
	// 0*2 + 1*2 + 2*2 + 3*2 + 4*2 = 20
	if sum != 20 {
		t.Errorf("expected sum 20, got %d", sum)
	}
}

func TestAsync(t *testing.T) {
	future := Async(func() (string, error) {
		return "hello", nil
	})

	result, err := Await(future)
	if err != nil {
		t.Errorf("Async/Await failed: %v", err)
	}
	if result != "hello" {
		t.Errorf("expected 'hello', got '%s'", result)
	}
}

func TestAwaitAll(t *testing.T) {
	p := New("test", WithMaxWorkers(4), WithAutoScale(false))
	defer p.Release()

	f1 := SubmitFunc(p, func() (int, error) { return 1, nil })
	f2 := SubmitFunc(p, func() (int, error) { return 2, nil })
	f3 := SubmitFunc(p, func() (int, error) { return 3, nil })

	results, err := AwaitAll(f1, f2, f3)
	if err != nil {
		t.Errorf("AwaitAll failed: %v", err)
	}

	sum := 0
	for _, r := range results {
		sum += r
	}
	if sum != 6 {
		t.Errorf("expected sum 6, got %d", sum)
	}
}

func TestPromise(t *testing.T) {
	promise, future := NewPromise[int]()

	go func() {
		time.Sleep(10 * time.Millisecond)
		promise.Complete(42)
	}()

	result, err := future.Get()
	if err != nil {
		t.Errorf("Promise failed: %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

// ============================================================================
// Hooks 测试
// ============================================================================

func TestHooks_BeforeAfterTask(t *testing.T) {
	var beforeCalled, afterCalled atomic.Bool

	hooks := NewHookBuilder().
		BeforeTask(func(info *TaskInfo) {
			beforeCalled.Store(true)
		}).
		AfterTask(func(info *TaskInfo) {
			afterCalled.Store(true)
		}).
		Build()

	p := New("test", WithMaxWorkers(2), WithAutoScale(false), WithHooks(hooks))
	defer p.Release()

	_ = p.SubmitWait(func() {
		time.Sleep(10 * time.Millisecond)
	})

	// Small sleep to allow AfterTask hook to complete
	// (SubmitWait returns when task fn completes, but hook fires after)
	time.Sleep(10 * time.Millisecond)

	if !beforeCalled.Load() {
		t.Error("BeforeTask hook not called")
	}
	if !afterCalled.Load() {
		t.Error("AfterTask hook not called")
	}
}

func TestHooks_OnPanic(t *testing.T) {
	var panicCalled atomic.Bool
	var capturedError atomic.Value

	hooks := NewHookBuilder().
		OnPanic(func(info *TaskInfo) {
			panicCalled.Store(true)
			capturedError.Store(info.Error)
		}).
		Build()

	p := New("test",
		WithMaxWorkers(2),
		WithAutoScale(false),
		WithHooks(hooks),
		WithPanicHandler(func(v any) {}),
	)
	defer p.Release()

	_ = p.Submit(func() {
		panic("test panic")
	})

	time.Sleep(100 * time.Millisecond)

	if !panicCalled.Load() {
		t.Error("OnPanic hook not called")
	}
	if capturedError.Load() != "test panic" {
		t.Errorf("expected panic value 'test panic', got %v", capturedError.Load())
	}
}

func TestHooks_OnReject(t *testing.T) {
	var rejectCalled atomic.Bool

	hooks := NewHookBuilder().
		OnReject(func(info *TaskInfo) {
			rejectCalled.Store(true)
		}).
		Build()

	p := New("test",
		WithMaxWorkers(1),
		WithAutoScale(false),
		WithNonBlocking(true),
		WithHooks(hooks),
	)
	defer p.Release()

	// Block the only worker
	blocker := make(chan struct{})
	_ = p.Submit(func() {
		<-blocker
	})

	time.Sleep(50 * time.Millisecond)

	// Try to submit another task
	_ = p.Submit(func() {})

	if !rejectCalled.Load() {
		t.Error("OnReject hook not called")
	}

	close(blocker)
}

// ============================================================================
// Queue 测试
// ============================================================================

func TestLockFreeQueue(t *testing.T) {
	q := NewLockFreeQueue[int](16)

	// Enqueue
	for i := 0; i < 10; i++ {
		if !q.Enqueue(i) {
			t.Errorf("Enqueue %d failed", i)
		}
	}

	if q.Len() != 10 {
		t.Errorf("expected len 10, got %d", q.Len())
	}

	// Dequeue
	for i := 0; i < 10; i++ {
		v, ok := q.Dequeue()
		if !ok {
			t.Errorf("Dequeue failed at %d", i)
		}
		if v != i {
			t.Errorf("expected %d, got %d", i, v)
		}
	}

	if !q.IsEmpty() {
		t.Error("queue should be empty")
	}
}

func TestLockFreeQueue_Concurrent(t *testing.T) {
	q := NewLockFreeQueue[int](1024)
	var wg sync.WaitGroup

	// Concurrent enqueue
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			q.Enqueue(n)
		}(i)
	}

	wg.Wait()

	// Concurrent dequeue
	var dequeued atomic.Int32
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := q.Dequeue(); ok {
				dequeued.Add(1)
			}
		}()
	}

	wg.Wait()

	if dequeued.Load() != 100 {
		t.Errorf("expected 100 dequeued, got %d", dequeued.Load())
	}
}

func TestPriorityQueue(t *testing.T) {
	pq := NewPriorityQueue(0)

	// Push tasks with different priorities
	pq.Push(func() {}, PriorityLow)    // priority 0
	pq.Push(func() {}, PriorityHigh)   // priority 10
	pq.Push(func() {}, PriorityNormal) // priority 5

	// Pop should return in priority order (highest first)
	fn1 := pq.Pop()
	fn2 := pq.Pop()
	fn3 := pq.Pop()

	if fn1 == nil || fn2 == nil || fn3 == nil {
		t.Error("Pop returned nil")
	}

	if pq.Pop() != nil {
		t.Error("Queue should be empty")
	}
}

// ============================================================================
// Spinlock 测试
// ============================================================================

func TestSpinlock(t *testing.T) {
	var lock Spinlock
	var counter int

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lock.Lock()
			counter++
			lock.Unlock()
		}()
	}

	wg.Wait()

	if counter != 100 {
		t.Errorf("expected 100, got %d", counter)
	}
}

func TestSpinlock_TryLock(t *testing.T) {
	var lock Spinlock

	if !lock.TryLock() {
		t.Error("TryLock should succeed")
	}

	if lock.TryLock() {
		t.Error("TryLock should fail when locked")
	}

	lock.Unlock()

	if !lock.TryLock() {
		t.Error("TryLock should succeed after unlock")
	}
	lock.Unlock()
}

// ============================================================================
// WorkStealingDeque 测试
// ============================================================================

func TestWorkStealingDeque(t *testing.T) {
	d := NewWorkStealingDeque[int](16)
	vals := []int{1, 2, 3, 4, 5}

	for _, v := range vals {
		v := v
		d.PushBottom(&v)
	}

	if d.Len() != 5 {
		t.Errorf("expected len 5, got %d", d.Len())
	}

	// Pop from bottom (LIFO)
	v := d.PopBottom()
	if v == nil || *v != 5 {
		t.Errorf("expected 5, got %v", v)
	}

	// Steal from top (FIFO)
	v = d.Steal()
	if v == nil || *v != 1 {
		t.Errorf("expected 1, got %v", v)
	}
}

// ============================================================================
// SubmitWithOptions 测试
// ============================================================================

func TestPool_SubmitWithOptions(t *testing.T) {
	p := New("test", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	var executed atomic.Bool
	err := p.SubmitWithOptions(func() {
		executed.Store(true)
	}, WithTaskPriority(PriorityHigh))

	if err != nil {
		t.Errorf("SubmitWithOptions failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if !executed.Load() {
		t.Error("task should be executed")
	}
}

func TestPool_TaskTimeout(t *testing.T) {
	var timeoutCalled atomic.Bool

	hooks := NewHookBuilder().
		OnTimeout(func(info *TaskInfo) {
			timeoutCalled.Store(true)
		}).
		Build()

	p := New("test", WithMaxWorkers(2), WithAutoScale(false), WithHooks(hooks))
	defer p.Release()

	_ = p.SubmitWithOptions(func() {
		time.Sleep(200 * time.Millisecond)
	}, WithTaskTimeout(50*time.Millisecond))

	time.Sleep(150 * time.Millisecond)

	if !timeoutCalled.Load() {
		t.Error("OnTimeout hook should be called")
	}
}

// ============================================================================
// SubmitBatch 测试
// ============================================================================

func TestPool_SubmitBatch(t *testing.T) {
	p := New("test", WithMaxWorkers(4), WithAutoScale(false))
	defer p.Release()

	var counter atomic.Int32
	var wg sync.WaitGroup

	// Create batch of tasks
	batch := make([]func(), 100)
	for i := range batch {
		wg.Add(1)
		batch[i] = func() {
			counter.Add(1)
			wg.Done()
		}
	}

	// Submit batch
	submitted, err := p.SubmitBatch(batch)
	if err != nil {
		t.Errorf("SubmitBatch failed: %v", err)
	}
	if submitted != 100 {
		t.Errorf("expected 100 submitted, got %d", submitted)
	}

	wg.Wait()

	if counter.Load() != 100 {
		t.Errorf("expected 100, got %d", counter.Load())
	}
}

func TestPool_SubmitBatch_Empty(t *testing.T) {
	p := New("test", WithMaxWorkers(2), WithAutoScale(false))
	defer p.Release()

	submitted, err := p.SubmitBatch(nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if submitted != 0 {
		t.Errorf("expected 0, got %d", submitted)
	}

	submitted, err = p.SubmitBatch([]func(){})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if submitted != 0 {
		t.Errorf("expected 0, got %d", submitted)
	}
}

func TestPool_TrySubmitBatch(t *testing.T) {
	p := New("test", WithMaxWorkers(4), WithAutoScale(false))
	defer p.Release()

	var counter atomic.Int32

	batch := make([]func(), 10)
	for i := range batch {
		batch[i] = func() {
			counter.Add(1)
		}
	}

	submitted := p.TrySubmitBatch(batch)
	if submitted == 0 {
		t.Error("TrySubmitBatch should submit at least some tasks")
	}

	time.Sleep(50 * time.Millisecond)

	if counter.Load() < int32(submitted) {
		t.Errorf("expected at least %d executed, got %d", submitted, counter.Load())
	}
}

func TestPool_SubmitBatch_PoolClosed(t *testing.T) {
	p := New("test", WithMaxWorkers(2), WithAutoScale(false))
	p.Release()

	batch := make([]func(), 10)
	for i := range batch {
		batch[i] = func() {}
	}

	_, err := p.SubmitBatch(batch)
	if err != ErrPoolClosed {
		t.Errorf("expected ErrPoolClosed, got %v", err)
	}
}

// ============================================================================
// ShardedCounter 测试
// ============================================================================

func TestShardedCounter(t *testing.T) {
	c := NewShardedCounter()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				c.Add(1)
			}
		}()
	}

	wg.Wait()

	if c.Load() != 100000 {
		t.Errorf("expected 100000, got %d", c.Load())
	}
}

func TestShardedCounter_IncDec(t *testing.T) {
	c := NewShardedCounter()

	c.Inc()
	c.Inc()
	c.Inc()
	c.Dec()

	if c.Load() != 2 {
		t.Errorf("expected 2, got %d", c.Load())
	}
}

func TestShardedCounter_Reset(t *testing.T) {
	c := NewShardedCounter()

	for i := 0; i < 100; i++ {
		c.Add(1)
	}

	c.Reset()

	if c.Load() != 0 {
		t.Errorf("expected 0, got %d", c.Load())
	}
}

func TestShardedCounter_Store(t *testing.T) {
	c := NewShardedCounter()

	for i := 0; i < 100; i++ {
		c.Add(1)
	}

	c.Store(42)

	if c.Load() != 42 {
		t.Errorf("expected 42, got %d", c.Load())
	}
}

func TestShardedCounter32(t *testing.T) {
	c := NewShardedCounter32()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				c.Add(1)
			}
		}()
	}

	wg.Wait()

	if c.Load() != 100000 {
		t.Errorf("expected 100000, got %d", c.Load())
	}
}

func TestFastCounter(t *testing.T) {
	c := NewFastCounter()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				c.Add(1)
			}
		}()
	}

	wg.Wait()

	if c.Load() != 100000 {
		t.Errorf("expected 100000, got %d", c.Load())
	}
}

// ============================================================================
// Work Stealing 测试
// ============================================================================

func TestPool_WithWorkStealing(t *testing.T) {
	p := New("test",
		WithMaxWorkers(4),
		WithAutoScale(false),
		WithWorkStealing(true),
	)
	defer p.Release()

	var counter atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		err := p.Submit(func() {
			counter.Add(1)
			wg.Done()
		})
		if err != nil {
			wg.Done()
			t.Errorf("Submit failed: %v", err)
		}
	}

	wg.Wait()

	if counter.Load() != 100 {
		t.Errorf("expected 100, got %d", counter.Load())
	}
}

// ============================================================================
// Simple API 测试
// ============================================================================

func TestSimpleAPI_Go(t *testing.T) {
	var executed atomic.Bool
	Go(func() {
		executed.Store(true)
	})
	time.Sleep(50 * time.Millisecond)
	if !executed.Load() {
		t.Error("Go should execute the function")
	}
}

func TestSimpleAPI_TryGo(t *testing.T) {
	var executed atomic.Bool
	if !TryGo(func() {
		executed.Store(true)
	}) {
		t.Error("TryGo should succeed")
	}
	time.Sleep(50 * time.Millisecond)
	if !executed.Load() {
		t.Error("TryGo should execute the function")
	}
}

func TestSimpleAPI_GoWait(t *testing.T) {
	var executed atomic.Bool
	GoWait(func() {
		executed.Store(true)
	})
	if !executed.Load() {
		t.Error("GoWait should execute synchronously")
	}
}

func TestSimpleAPI_GoBatch(t *testing.T) {
	var counter atomic.Int32
	batch := make([]func(), 10)
	for i := range batch {
		batch[i] = func() {
			counter.Add(1)
		}
	}
	n := GoBatch(batch)
	if n != 10 {
		t.Errorf("GoBatch should submit 10, got %d", n)
	}
	time.Sleep(50 * time.Millisecond)
	if counter.Load() != 10 {
		t.Errorf("expected 10 executed, got %d", counter.Load())
	}
}

func TestSimpleAPI_Parallel(t *testing.T) {
	var c1, c2, c3 atomic.Bool
	Parallel(
		func() { c1.Store(true) },
		func() { c2.Store(true) },
		func() { c3.Store(true) },
	)
	if !c1.Load() || !c2.Load() || !c3.Load() {
		t.Error("Parallel should execute all functions")
	}
}

func TestSimpleAPI_NewSimple(t *testing.T) {
	p := NewSimple(4)
	defer p.Release()

	if p.Cap() != 4 {
		t.Errorf("expected cap 4, got %d", p.Cap())
	}

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

func TestSimpleAPI_NewAuto(t *testing.T) {
	p := NewAuto(2, 10)
	defer p.Release()

	if p.Cap() != 10 {
		t.Errorf("expected cap 10, got %d", p.Cap())
	}
}

// Note: Comprehensive benchmarks moved to benchmark_test.go
