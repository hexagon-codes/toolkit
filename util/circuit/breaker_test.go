package circuit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestBreaker(t *testing.T, options ...Option) *Breaker {
	t.Helper()
	breaker, err := New(options...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return breaker
}

func newTestManager(t *testing.T, options ...Option) *BreakerManager {
	t.Helper()
	manager, err := NewBreakerManager(options...)
	if err != nil {
		t.Fatalf("NewBreakerManager() error = %v", err)
	}
	return manager
}

func TestBreaker_InitialState(t *testing.T) {
	b := newTestBreaker(t)

	if b.State() != StateClosed {
		t.Errorf("expected StateClosed, got %v", b.State())
	}
}

func TestBreaker_OpenOnFailures(t *testing.T) {
	b := newTestBreaker(t, WithThreshold(3))

	// 模拟失败
	for i := 0; i < 3; i++ {
		_, _ = b.Execute(func() (any, error) {
			return nil, errors.New("error")
		})
	}

	if b.State() != StateOpen {
		t.Errorf("expected StateOpen, got %v", b.State())
	}
}

func TestBreaker_RejectWhenOpen(t *testing.T) {
	b := newTestBreaker(t, WithThreshold(1))

	// 触发熔断
	_, _ = b.Execute(func() (any, error) {
		return nil, errors.New("error")
	})

	// 请求应该被拒绝
	_, err := b.Execute(func() (any, error) {
		return "success", nil
	})

	if err != ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestBreaker_TransitionToHalfOpen(t *testing.T) {
	now := time.Now()
	currentTime := now

	b := newTestBreaker(t,
		WithThreshold(1),
		WithTimeout(100*time.Millisecond),
		WithNow(func() time.Time { return currentTime }),
	)

	// 触发熔断
	_, _ = b.Execute(func() (any, error) {
		return nil, errors.New("error")
	})

	if b.State() != StateOpen {
		t.Errorf("expected StateOpen, got %v", b.State())
	}

	// 时间推进
	currentTime = now.Add(200 * time.Millisecond)

	// 应该允许请求（进入半开状态）
	permit, err := b.Acquire()
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if permit == nil {
		t.Fatal("Acquire() permit = nil")
	}

	if b.State() != StateHalfOpen {
		t.Errorf("expected StateHalfOpen, got %v", b.State())
	}
}

func TestBreaker_RecoverFromHalfOpen(t *testing.T) {
	now := time.Now()
	currentTime := now

	b := newTestBreaker(t,
		WithThreshold(1),
		WithTimeout(100*time.Millisecond),
		WithSuccessThreshold(2),
		WithNow(func() time.Time { return currentTime }),
	)

	// 触发熔断
	_, _ = b.Execute(func() (any, error) {
		return nil, errors.New("error")
	})

	// 时间推进
	currentTime = now.Add(200 * time.Millisecond)

	// 执行成功的请求
	for i := 0; i < 2; i++ {
		_, _ = b.Execute(func() (any, error) {
			return "success", nil
		})
	}

	if b.State() != StateClosed {
		t.Errorf("expected StateClosed, got %v", b.State())
	}
}

func TestBreaker_BackToOpenFromHalfOpen(t *testing.T) {
	now := time.Now()
	currentTime := now

	b := newTestBreaker(t,
		WithThreshold(1),
		WithTimeout(100*time.Millisecond),
		WithNow(func() time.Time { return currentTime }),
	)

	// 触发熔断
	_, _ = b.Execute(func() (any, error) {
		return nil, errors.New("error")
	})

	// 时间推进
	currentTime = now.Add(200 * time.Millisecond)

	// 进入半开状态后再次失败
	_, _ = b.Execute(func() (any, error) {
		return nil, errors.New("error")
	})

	if b.State() != StateOpen {
		t.Errorf("expected StateOpen, got %v", b.State())
	}
}

func TestBreaker_SuccessResetFailures(t *testing.T) {
	b := newTestBreaker(t, WithThreshold(3))

	// 2 次失败
	for i := 0; i < 2; i++ {
		_, _ = b.Execute(func() (any, error) {
			return nil, errors.New("error")
		})
	}

	// 1 次成功
	_, _ = b.Execute(func() (any, error) {
		return "success", nil
	})

	// 再 2 次失败不应触发熔断
	for i := 0; i < 2; i++ {
		_, _ = b.Execute(func() (any, error) {
			return nil, errors.New("error")
		})
	}

	if b.State() != StateClosed {
		t.Errorf("expected StateClosed, got %v", b.State())
	}
}

func TestBreaker_ExecuteContext(t *testing.T) {
	b := newTestBreaker(t)

	ctx := context.Background()
	result, err := b.ExecuteContext(ctx, func(ctx context.Context) (any, error) {
		return "hello", nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Errorf("expected 'hello', got %v", result)
	}
}

func TestBreaker_Reset(t *testing.T) {
	b := newTestBreaker(t, WithThreshold(1))

	// 触发熔断
	_, _ = b.Execute(func() (any, error) {
		return nil, errors.New("error")
	})

	if b.State() != StateOpen {
		t.Errorf("expected StateOpen, got %v", b.State())
	}

	// 重置
	if err := b.Reset(); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}

	if b.State() != StateClosed {
		t.Errorf("expected StateClosed after reset, got %v", b.State())
	}
}

func TestBreaker_OnStateChange(t *testing.T) {
	var changes []struct{ from, to State }
	var mu sync.Mutex

	b := newTestBreaker(t,
		WithThreshold(1),
		WithOnStateChange(func(from, to State) {
			mu.Lock()
			changes = append(changes, struct{ from, to State }{from, to})
			mu.Unlock()
		}),
	)

	// 触发熔断
	_, _ = b.Execute(func() (any, error) {
		return nil, errors.New("error")
	})

	// 等待回调
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(changes) != 1 {
		t.Errorf("expected 1 change, got %d", len(changes))
	}
	if changes[0].from != StateClosed || changes[0].to != StateOpen {
		t.Errorf("unexpected change: %+v", changes[0])
	}
}

func TestBreaker_CloseIsIdempotentAndRejectsNewWork(t *testing.T) {
	breaker := newTestBreaker(t,
		WithThreshold(1),
		WithOnStateChange(func(_, _ State) {}),
	)
	permit, err := breaker.Acquire()
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if completeErr := permit.Complete(errors.New("failed")); completeErr != nil {
		t.Fatalf("Complete() error = %v", completeErr)
	}
	breaker.Close()
	breaker.Close()

	_, err = breaker.Execute(func() (any, error) { return nil, nil })
	if !errors.Is(err, ErrBreakerClosed) {
		t.Fatalf("expected ErrBreakerClosed, got %v", err)
	}
}

func TestBreaker_CloseConcurrentWithTransitions(t *testing.T) {
	breaker := newTestBreaker(t,
		WithThreshold(1),
		WithOnStateChange(func(_, _ State) {}),
	)

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = breaker.Execute(func() (any, error) {
				return nil, errors.New("failed")
			})
			_ = breaker.Reset()
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		breaker.Close()
	}()
	wg.Wait()
	breaker.Close()
}

func TestBreaker_Stats(t *testing.T) {
	b := newTestBreaker(t, WithThreshold(5))

	// 3 次失败
	for i := 0; i < 3; i++ {
		_, _ = b.Execute(func() (any, error) {
			return nil, errors.New("error")
		})
	}

	stats := b.Stats()
	if stats.State != StateClosed {
		t.Errorf("expected StateClosed, got %v", stats.State)
	}
	if stats.Failures != 3 {
		t.Errorf("expected 3 failures, got %d", stats.Failures)
	}
}

func TestBreaker_HalfOpenMaxRequests(t *testing.T) {
	now := time.Now()
	currentTime := now

	b := newTestBreaker(t,
		WithThreshold(1),
		WithTimeout(100*time.Millisecond),
		WithHalfOpenMaxRequests(2),
		WithNow(func() time.Time { return currentTime }),
	)

	// 触发熔断
	_, _ = b.Execute(func() (any, error) {
		return nil, errors.New("error")
	})

	// 时间推进
	currentTime = now.Add(200 * time.Millisecond)

	// 前两个请求应该被允许（不提交结果，保持在半开状态）
	first, err1 := b.Acquire()
	if err1 != nil {
		t.Errorf("first request should be allowed, got %v", err1)
	}

	second, err2 := b.Acquire()
	if err2 != nil {
		t.Errorf("second request should be allowed, got %v", err2)
	}
	if first == nil || second == nil {
		t.Fatal("Acquire() returned a nil permit")
	}

	// 第三个应该被拒绝（超过半开状态最大请求数）
	_, err3 := b.Acquire()
	if err3 != ErrTooManyRequests {
		t.Errorf("third request should be rejected, got %v", err3)
	}
}

func TestBreaker_AcquireAndSuccess(t *testing.T) {
	b := newTestBreaker(t)

	permit, err := b.Acquire()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if err := permit.Complete(nil); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	stats := b.Stats()
	if stats.Failures != 0 {
		t.Errorf("expected 0 failures, got %d", stats.Failures)
	}
}

func TestBreaker_AcquireAndFailure(t *testing.T) {
	b := newTestBreaker(t, WithThreshold(1))

	permit, err := b.Acquire()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if err := permit.Complete(errors.New("failed")); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if b.State() != StateOpen {
		t.Errorf("expected StateOpen, got %v", b.State())
	}
}

func TestBreaker_CustomIsFailure(t *testing.T) {
	b := newTestBreaker(t,
		WithThreshold(1),
		WithIsFailure(func(err error) bool {
			// 只有特定错误才认为是失败
			return err != nil && err.Error() == "critical"
		}),
	)

	// 非关键错误不应触发熔断
	_, _ = b.Execute(func() (any, error) {
		return nil, errors.New("normal error")
	})

	if b.State() != StateClosed {
		t.Errorf("expected StateClosed for normal error, got %v", b.State())
	}

	// 关键错误触发熔断
	_, _ = b.Execute(func() (any, error) {
		return nil, errors.New("critical")
	})

	if b.State() != StateOpen {
		t.Errorf("expected StateOpen for critical error, got %v", b.State())
	}
}

func TestState_String(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		if tt.state.String() != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.state.String())
		}
	}
}

func TestNewAIBreaker(t *testing.T) {
	b, err := NewAIBreaker(OpenAIConfig())
	if err != nil {
		t.Fatalf("NewAIBreaker() error = %v", err)
	}

	if b.State() != StateClosed {
		t.Errorf("expected StateClosed, got %v", b.State())
	}

	// 验证配置
	if b.config.Threshold != 5 {
		t.Errorf("expected threshold 5, got %d", b.config.Threshold)
	}
	if b.config.Timeout != 60*time.Second {
		t.Errorf("expected timeout 60s, got %v", b.config.Timeout)
	}
}

func TestNewAIBreaker_WithExtra(t *testing.T) {
	b, err := NewAIBreaker(OpenAIConfig(), WithThreshold(10))
	if err != nil {
		t.Fatalf("NewAIBreaker() error = %v", err)
	}

	// 额外选项应该覆盖预设
	if b.config.Threshold != 10 {
		t.Errorf("expected threshold 10, got %d", b.config.Threshold)
	}
}

func TestPresetConfigsReturnIndependentSlices(t *testing.T) {
	first := OpenAIConfig()
	first[0] = WithThreshold(1)

	b, err := NewAIBreaker(OpenAIConfig())
	if err != nil {
		t.Fatal(err)
	}
	if b.config.Threshold != 5 {
		t.Fatalf("OpenAIConfig() threshold = %d, want 5", b.config.Threshold)
	}
}

type testHTTPError struct {
	code int
}

func (e *testHTTPError) StatusCode() int { return e.code }
func (e *testHTTPError) Error() string   { return "http error" }

func TestIsRateLimitOrServerError(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{nil, false},
		{errors.New("generic error"), true},
		{&testHTTPError{code: 429}, true},
		{&testHTTPError{code: 500}, true},
		{&testHTTPError{code: 503}, true},
		{&testHTTPError{code: 400}, false}, // 4xx 客户端错误不重试
		{&testHTTPError{code: 200}, false}, // 成功不是错误
	}

	for _, tt := range tests {
		result := IsRateLimitOrServerError(tt.err)
		if result != tt.expected {
			t.Errorf("IsRateLimitOrServerError(%v) = %v, want %v", tt.err, result, tt.expected)
		}
	}
}

func TestIsServerError(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{nil, false},
		{errors.New("generic error"), false},
		{&testHTTPError{code: 429}, false},
		{&testHTTPError{code: 500}, true},
		{&testHTTPError{code: 503}, true},
	}

	for _, tt := range tests {
		result := IsServerError(tt.err)
		if result != tt.expected {
			t.Errorf("IsServerError(%v) = %v, want %v", tt.err, result, tt.expected)
		}
	}
}

func TestIsRateLimitError(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{nil, false},
		{errors.New("generic error"), false},
		{&testHTTPError{code: 429}, true},
		{&testHTTPError{code: 500}, false},
	}

	for _, tt := range tests {
		result := IsRateLimitError(tt.err)
		if result != tt.expected {
			t.Errorf("IsRateLimitError(%v) = %v, want %v", tt.err, result, tt.expected)
		}
	}
}

func TestBreakerManager_Get(t *testing.T) {
	manager := newTestManager(t, WithThreshold(3))

	b1, err := manager.Get("service-a")
	if err != nil {
		t.Fatalf("Get(service-a) error = %v", err)
	}
	b2, err := manager.Get("service-a")
	if err != nil {
		t.Fatalf("Get(service-a) second error = %v", err)
	}
	b3, err := manager.Get("service-b")
	if err != nil {
		t.Fatalf("Get(service-b) error = %v", err)
	}

	if b1 != b2 {
		t.Error("expected same breaker for same name")
	}
	if b1 == b3 {
		t.Error("expected different breakers for different names")
	}
}

func TestBreakerManager_Execute(t *testing.T) {
	manager := newTestManager(t, WithThreshold(1))

	// 触发 service-a 熔断
	_, _ = manager.Execute("service-a", func() (any, error) {
		return nil, errors.New("error")
	})

	// service-a 应该被熔断
	_, err := manager.Execute("service-a", func() (any, error) {
		return "success", nil
	})
	if err != ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}

	// service-b 不应被熔断
	result, err := manager.Execute("service-b", func() (any, error) {
		return "success", nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got %v", result)
	}
}

func TestBreakerManager_Reset(t *testing.T) {
	manager := newTestManager(t, WithThreshold(1))

	// 触发熔断
	_, _ = manager.Execute("service", func() (any, error) {
		return nil, errors.New("error")
	})

	// 重置
	if err := manager.Reset("service"); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}

	// 应该可以执行
	result, err := manager.Execute("service", func() (any, error) {
		return "success", nil
	})
	if err != nil {
		t.Errorf("unexpected error after reset: %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got %v", result)
	}
}

func TestBreakerManager_ResetAll(t *testing.T) {
	manager := newTestManager(t, WithThreshold(1))

	// 触发多个熔断
	_, _ = manager.Execute("service-a", func() (any, error) {
		return nil, errors.New("error")
	})
	_, _ = manager.Execute("service-b", func() (any, error) {
		return nil, errors.New("error")
	})

	// 全部重置
	if err := manager.ResetAll(); err != nil {
		t.Fatalf("ResetAll() error = %v", err)
	}

	states := manager.States()
	for name, state := range states {
		if state != StateClosed {
			t.Errorf("%s: expected StateClosed, got %v", name, state)
		}
	}
}

func TestBreakerManager_States(t *testing.T) {
	manager := newTestManager(t, WithThreshold(1))

	// 创建一些熔断器
	if _, err := manager.Get("service-a"); err != nil {
		t.Fatalf("Get(service-a) error = %v", err)
	}
	if _, err := manager.Get("service-b"); err != nil {
		t.Fatalf("Get(service-b) error = %v", err)
	}

	// 触发一个熔断
	_, _ = manager.Execute("service-a", func() (any, error) {
		return nil, errors.New("error")
	})

	states := manager.States()

	if states["service-a"] != StateOpen {
		t.Errorf("service-a: expected StateOpen, got %v", states["service-a"])
	}
	if states["service-b"] != StateClosed {
		t.Errorf("service-b: expected StateClosed, got %v", states["service-b"])
	}
}

func TestBreaker_Concurrent(t *testing.T) {
	b := newTestBreaker(t, WithThreshold(100))

	var wg sync.WaitGroup
	var successCount atomic.Int32
	var errorCount atomic.Int32

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := b.Execute(func() (any, error) {
				if n%2 == 0 {
					return "success", nil
				}
				return nil, errors.New("error")
			})
			if err == nil {
				successCount.Add(1)
			} else {
				errorCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	// 由于成功会重置失败计数，应该没有熔断
	if b.State() != StateClosed {
		t.Logf("State: %v, successes: %d, errors: %d", b.State(), successCount.Load(), errorCount.Load())
	}
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	if _, err := New(WithThreshold(0)); err == nil {
		t.Fatal("New() error = nil, want invalid threshold error")
	}
	if _, err := New(nil); err == nil {
		t.Fatal("New(nil) error = nil, want invalid option error")
	}
}

func TestHalfOpenSuccessThresholdCanExceedConcurrency(t *testing.T) {
	now := time.Now()
	breaker, err := New(
		WithThreshold(1),
		WithTimeout(time.Millisecond),
		WithHalfOpenMaxRequests(1),
		WithSuccessThreshold(2),
		WithNow(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	permit, err := breaker.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if completeErr := permit.Complete(errors.New("failed")); completeErr != nil {
		t.Fatal(completeErr)
	}
	now = now.Add(2 * time.Millisecond)

	for attempt := 0; attempt < 2; attempt++ {
		permit, err = breaker.Acquire()
		if err != nil {
			t.Fatalf("half-open Acquire() attempt %d error = %v", attempt+1, err)
		}
		if err := permit.Complete(nil); err != nil {
			t.Fatalf("half-open Complete() attempt %d error = %v", attempt+1, err)
		}
	}
	if state := breaker.State(); state != StateClosed {
		t.Fatalf("State() = %s, want closed", state)
	}
}

func TestPermitCompletionIsBoundToAdmittedRequest(t *testing.T) {
	now := time.Now()
	breaker, err := New(
		WithThreshold(1),
		WithTimeout(time.Millisecond),
		WithHalfOpenMaxRequests(2),
		WithSuccessThreshold(2),
		WithNow(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	closedPermit, err := breaker.Acquire()
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if completeErr := closedPermit.Complete(errors.New("failed")); completeErr != nil {
		t.Fatalf("Complete() error = %v", completeErr)
	}
	now = now.Add(2 * time.Millisecond)

	first, err := breaker.Acquire()
	if err != nil {
		t.Fatalf("first half-open Acquire() error = %v", err)
	}
	second, err := breaker.Acquire()
	if err != nil {
		t.Fatalf("second half-open Acquire() error = %v", err)
	}
	if err := first.Complete(errors.New("failed again")); err != nil {
		t.Fatalf("first Complete() error = %v", err)
	}
	if err := second.Complete(nil); err != nil {
		t.Fatalf("stale Complete() error = %v", err)
	}
	if breaker.State() != StateOpen {
		t.Fatalf("State() = %s, want open", breaker.State())
	}
}

func TestExecutePanicCompletesPermitAsFailure(t *testing.T) {
	breaker := newTestBreaker(t, WithThreshold(1))
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("Execute() did not propagate panic")
			}
		}()
		_, _ = breaker.Execute(func() (any, error) {
			panic("failed")
		})
	}()
	if breaker.State() != StateOpen {
		t.Fatalf("State() = %s, want open", breaker.State())
	}
}
