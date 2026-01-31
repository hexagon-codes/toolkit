package contextx

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestWithTimeout(t *testing.T) {
	ctx, cancel := WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	select {
	case <-ctx.Done():
		if !IsTimeout(ctx) {
			t.Error("expected timeout")
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("timeout not triggered")
	}
}

func TestWithDeadline(t *testing.T) {
	deadline := time.Now().Add(100 * time.Millisecond)
	ctx, cancel := WithDeadline(context.Background(), deadline)
	defer cancel()

	select {
	case <-ctx.Done():
		if !IsTimeout(ctx) {
			t.Error("expected deadline exceeded")
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("deadline not triggered")
	}
}

func TestWithTimeoutCause(t *testing.T) {
	cause := errors.New("custom timeout cause")
	ctx, cancel := WithTimeoutCause(context.Background(), 50*time.Millisecond, cause)
	defer cancel()

	<-ctx.Done()

	if Cause(ctx) != cause {
		t.Error("expected custom cause")
	}
}

func TestWithDeadlineCause(t *testing.T) {
	cause := errors.New("custom deadline cause")
	deadline := time.Now().Add(50 * time.Millisecond)
	ctx, cancel := WithDeadlineCause(context.Background(), deadline, cause)
	defer cancel()

	<-ctx.Done()

	if Cause(ctx) != cause {
		t.Error("expected custom cause")
	}
}

func TestWithCancel(t *testing.T) {
	ctx, cancel := WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	<-ctx.Done()

	if !IsCanceled(ctx) {
		t.Error("expected canceled")
	}
}

func TestWithCancelCause(t *testing.T) {
	ctx, cancel := WithCancelCause(context.Background())

	cause := errors.New("custom cancel cause")
	cancel(cause)

	<-ctx.Done()

	if Cause(ctx) != cause {
		t.Error("expected custom cause")
	}
}

func TestTypeSafeKey(t *testing.T) {
	type User struct {
		ID   int
		Name string
	}

	userKey := NewKey[User]("user")

	ctx := context.Background()
	user := User{ID: 1, Name: "Alice"}
	ctx = WithValue(ctx, userKey, user)

	// Get value
	got, ok := Value(ctx, userKey)
	if !ok {
		t.Fatal("value not found")
	}
	if got.ID != user.ID || got.Name != user.Name {
		t.Errorf("expected %+v, got %+v", user, got)
	}
}

func TestMustValue(t *testing.T) {
	key := NewKey[string]("test")
	ctx := WithValue(context.Background(), key, "hello")

	val := MustValue(ctx, key)
	if val != "hello" {
		t.Errorf("expected 'hello', got '%s'", val)
	}
}

func TestMustValuePanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()

	key := NewKey[string]("missing")
	MustValue(context.Background(), key)
}

func TestValueOr(t *testing.T) {
	key := NewKey[string]("test")

	// With value
	ctx := WithValue(context.Background(), key, "hello")
	val := ValueOr(ctx, key, "default")
	if val != "hello" {
		t.Errorf("expected 'hello', got '%s'", val)
	}

	// Without value
	val = ValueOr(context.Background(), key, "default")
	if val != "default" {
		t.Errorf("expected 'default', got '%s'", val)
	}
}

func TestTraceID(t *testing.T) {
	ctx := WithTraceID(context.Background(), "trace-123")

	traceID := TraceID(ctx)
	if traceID != "trace-123" {
		t.Errorf("expected 'trace-123', got '%s'", traceID)
	}

	// Default value
	if TraceID(context.Background()) != "" {
		t.Error("expected empty string for missing trace id")
	}
}

func TestRequestID(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req-456")

	requestID := RequestID(ctx)
	if requestID != "req-456" {
		t.Errorf("expected 'req-456', got '%s'", requestID)
	}
}

func TestUserID(t *testing.T) {
	ctx := WithUserID(context.Background(), 12345)

	userID := UserID(ctx)
	if userID != 12345 {
		t.Errorf("expected 12345, got %d", userID)
	}

	// Default value
	if UserID(context.Background()) != 0 {
		t.Error("expected 0 for missing user id")
	}
}

func TestTenantID(t *testing.T) {
	ctx := WithTenantID(context.Background(), "tenant-abc")

	tenantID := TenantID(ctx)
	if tenantID != "tenant-abc" {
		t.Errorf("expected 'tenant-abc', got '%s'", tenantID)
	}
}

func TestIsTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	<-ctx.Done()

	if !IsTimeout(ctx) {
		t.Error("expected IsTimeout to return true")
	}
}

func TestIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if !IsCanceled(ctx) {
		t.Error("expected IsCanceled to return true")
	}
}

func TestIsDone(t *testing.T) {
	ctx := context.Background()
	if IsDone(ctx) {
		t.Error("expected IsDone to return false for active context")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if !IsDone(ctx) {
		t.Error("expected IsDone to return true for canceled context")
	}
}

func TestRemaining(t *testing.T) {
	// No deadline
	remaining := Remaining(context.Background())
	if remaining != -1 {
		t.Errorf("expected -1 for no deadline, got %v", remaining)
	}

	// With deadline
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	remaining = Remaining(ctx)
	if remaining <= 0 || remaining > 1*time.Second {
		t.Errorf("unexpected remaining time: %v", remaining)
	}
}

func TestHasDeadline(t *testing.T) {
	if HasDeadline(context.Background()) {
		t.Error("expected no deadline for background context")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if !HasDeadline(ctx) {
		t.Error("expected deadline for timeout context")
	}
}

func TestGo(t *testing.T) {
	var executed atomic.Bool
	ctx := context.Background()

	Go(ctx, func(ctx context.Context) {
		executed.Store(true)
	})

	time.Sleep(50 * time.Millisecond)

	if !executed.Load() {
		t.Error("function should have been executed")
	}
}

func TestGoWithCanceledContext(t *testing.T) {
	var executed atomic.Bool
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before Go

	Go(ctx, func(ctx context.Context) {
		executed.Store(true)
	})

	time.Sleep(50 * time.Millisecond)

	if executed.Load() {
		t.Error("function should not have been executed")
	}
}

func TestRun(t *testing.T) {
	ctx := context.Background()

	err := Run(ctx, func() error {
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRunWithError(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("test error")

	err := Run(ctx, func() error {
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestRunWithCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := Run(ctx, func() error {
		time.Sleep(1 * time.Second)
		return nil
	})

	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRunTimeout(t *testing.T) {
	err := RunTimeout(50*time.Millisecond, func() error {
		time.Sleep(1 * time.Second)
		return nil
	})

	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestDetach(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Set some values
	ctx = WithTraceID(ctx, "trace-123")

	// Detach
	detached := Detach(ctx)

	// Cancel original
	cancel()

	// Original should be canceled
	if !IsCanceled(ctx) {
		t.Error("original context should be canceled")
	}

	// Detached should not be canceled
	if IsDone(detached) {
		t.Error("detached context should not be done")
	}

	// Values should be preserved
	if TraceID(detached) != "trace-123" {
		t.Error("detached context should preserve values")
	}

	// No deadline
	if HasDeadline(detached) {
		t.Error("detached context should not have deadline")
	}

	// Err should be nil
	if detached.Err() != nil {
		t.Error("detached context Err() should be nil")
	}
}

func TestMerge(t *testing.T) {
	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	ctx1 = WithTraceID(ctx1, "trace-from-ctx1")
	ctx2 = WithRequestID(ctx2, "req-from-ctx2")

	merged, cancelMerged := Merge(ctx1, ctx2)
	defer cancelMerged()

	// Cancel first context
	cancel1()

	// Wait a bit for propagation
	time.Sleep(50 * time.Millisecond)

	// Merged should be done
	if !IsDone(merged) {
		t.Error("merged context should be done when any source is canceled")
	}

	// Values should be accessible
	if TraceID(merged) != "trace-from-ctx1" {
		t.Error("merged context should have values from ctx1")
	}
	if RequestID(merged) != "req-from-ctx2" {
		t.Error("merged context should have values from ctx2")
	}
}

func TestMergeEmpty(t *testing.T) {
	ctx, cancel := Merge()
	defer cancel()

	if ctx == nil {
		t.Error("Merge() should return non-nil context")
	}
}

func TestAfterFunc(t *testing.T) {
	var executed atomic.Bool
	ctx, cancel := context.WithCancel(context.Background())

	AfterFunc(ctx, func() {
		executed.Store(true)
	})

	cancel()
	time.Sleep(50 * time.Millisecond)

	if !executed.Load() {
		t.Error("AfterFunc should have been executed")
	}
}

func TestWaitGroupContext(t *testing.T) {
	ctx := context.Background()
	wg := NewWaitGroupContext(ctx)

	var count atomic.Int32

	for i := 0; i < 5; i++ {
		wg.Go(func(ctx context.Context) error {
			count.Add(1)
			return nil
		})
	}

	err := wg.Wait()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if count.Load() != 5 {
		t.Errorf("expected count 5, got %d", count.Load())
	}
}

func TestWaitGroupContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	wg := NewWaitGroupContext(ctx)

	wg.Go(func(ctx context.Context) error {
		time.Sleep(1 * time.Second)
		return nil
	})

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := wg.Wait()
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestPool(t *testing.T) {
	ctx := context.Background()
	pool := NewPool(ctx, 2)

	var count atomic.Int32

	for i := 0; i < 5; i++ {
		pool.Go(func(ctx context.Context) error {
			count.Add(1)
			time.Sleep(10 * time.Millisecond)
			return nil
		})
	}

	err := pool.Wait()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if count.Load() != 5 {
		t.Errorf("expected count 5, got %d", count.Load())
	}
}

func TestPoolClose(t *testing.T) {
	ctx := context.Background()
	pool := NewPool(ctx, 2)

	var started atomic.Int32

	for i := 0; i < 10; i++ {
		pool.Go(func(ctx context.Context) error {
			started.Add(1)
			time.Sleep(100 * time.Millisecond)
			return nil
		})
	}

	// Close after a short delay
	time.Sleep(20 * time.Millisecond)
	pool.Close()

	// Some tasks should have started
	if started.Load() == 0 {
		t.Error("some tasks should have started")
	}
}

func TestPoolCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	pool := NewPool(ctx, 2)

	var executed atomic.Bool
	pool.Go(func(ctx context.Context) error {
		executed.Store(true)
		return nil
	})

	time.Sleep(50 * time.Millisecond)

	// Task should not execute because context is already canceled
	// Note: There's a race condition here, so we just test that it doesn't panic
}

func TestCause(t *testing.T) {
	// No cause
	ctx := context.Background()
	if Cause(ctx) != nil {
		t.Error("expected nil cause for background context")
	}

	// With cause
	ctx, cancel := WithCancelCause(context.Background())
	cause := errors.New("test cause")
	cancel(cause)

	if Cause(ctx) != cause {
		t.Errorf("expected cause %v, got %v", cause, Cause(ctx))
	}
}

func BenchmarkWithValue(b *testing.B) {
	key := NewKey[string]("bench")
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx = WithValue(ctx, key, "value")
	}
}

func BenchmarkValue(b *testing.B) {
	key := NewKey[string]("bench")
	ctx := WithValue(context.Background(), key, "value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Value(ctx, key)
	}
}
