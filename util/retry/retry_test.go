package retry

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

var (
	errTest = errors.New("test error")
)

func TestDo_Success(t *testing.T) {
	attempts := 0
	err := Do(func() error {
		attempts++
		return nil
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestDo_SuccessAfterRetry(t *testing.T) {
	attempts := 0
	err := Do(func() error {
		attempts++
		if attempts < 3 {
			return errTest
		}
		return nil
	}, Attempts(5))

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestDo_MaxAttemptsReached(t *testing.T) {
	attempts := 0
	err := Do(func() error {
		attempts++
		return errTest
	}, Attempts(3), Delay(10*time.Millisecond))

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrMaxAttemptsReached) {
		t.Errorf("expected ErrMaxAttemptsReached, got %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestDo_FixedDelay(t *testing.T) {
	attempts := 0
	start := time.Now()

	Do(func() error {
		attempts++
		return errTest
	}, Attempts(3), Delay(50*time.Millisecond), DelayType(FixedDelay))

	elapsed := time.Since(start)

	// 2次延迟（3次尝试，最后一次不延迟）
	expectedMin := 100 * time.Millisecond
	expectedMax := 200 * time.Millisecond

	if elapsed < expectedMin || elapsed > expectedMax {
		t.Errorf("expected delay between %v and %v, got %v", expectedMin, expectedMax, elapsed)
	}
}

func TestDo_ExponentialBackoff(t *testing.T) {
	attempts := 0
	delays := []time.Duration{}

	Do(func() error {
		attempts++
		return errTest
	}, Attempts(4), Delay(10*time.Millisecond), DelayType(ExponentialBackoff), Multiplier(2.0))

	// 指数退避：10ms, 20ms, 40ms
	// 实际测试延迟会有误差，这里只验证次数
	if attempts != 4 {
		t.Errorf("expected 4 attempts, got %d", attempts)
	}

	_ = delays
}

func TestDo_LinearBackoff(t *testing.T) {
	attempts := 0

	Do(func() error {
		attempts++
		return errTest
	}, Attempts(3), Delay(10*time.Millisecond), DelayType(LinearBackoff))

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestDo_OnRetry(t *testing.T) {
	attempts := 0
	retryCallbacks := 0

	err := Do(func() error {
		attempts++
		return errTest
	}, Attempts(3), OnRetry(func(n int, err error) {
		retryCallbacks++
		if err != errTest {
			t.Errorf("expected errTest, got %v", err)
		}
	}), Delay(10*time.Millisecond))

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// OnRetry 在每次失败后调用，但最后一次失败不调用
	if retryCallbacks != 2 {
		t.Errorf("expected 2 retry callbacks, got %d", retryCallbacks)
	}
}

func TestDo_IfPredicate(t *testing.T) {
	errSpecial := errors.New("special error")
	attempts := 0

	err := Do(func() error {
		attempts++
		return errSpecial
	}, Attempts(5), If(func(err error) bool {
		// 只重试 errTest，不重试 errSpecial
		return errors.Is(err, errTest)
	}), Delay(10*time.Millisecond))

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// 因为 errSpecial 不满足重试条件，所以只尝试1次
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestDoRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		option Option
	}{
		{name: "attempts", option: Attempts(0)},
		{name: "delay", option: Delay(-time.Second)},
		{name: "max delay", option: MaxDelay(0)},
		{name: "multiplier", option: Multiplier(0)},
		{name: "predicate", option: If(nil)},
		{name: "jitter", option: WithJitter(2)},
		{name: "jitter type", option: WithJitterType(JitterType(99))},
		{name: "nil option", option: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := Do(func() error { return nil }, test.option); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("Do() error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestDoExhaustedErrorIncludesLastFailure(t *testing.T) {
	lastFailure := errors.New("upstream failed")
	err := Do(
		func() error { return lastFailure },
		Attempts(1),
	)
	if !errors.Is(err, ErrMaxAttemptsReached) || !errors.Is(err, lastFailure) {
		t.Fatalf("Do() error chain = %v", err)
	}
}

func TestDoWithContext_Success(t *testing.T) {
	ctx := context.Background()
	attempts := 0

	err := DoWithContext(ctx, func() error {
		attempts++
		return nil
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestDoWithContext_Cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0

	// 在第一次失败后取消上下文
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := DoWithContext(ctx, func() error {
		attempts++
		return errTest
	}, Attempts(10), Delay(30*time.Millisecond))

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	// 应该在取消前尝试1-2次
	if attempts < 1 || attempts > 3 {
		t.Errorf("expected 1-3 attempts, got %d", attempts)
	}
}

func TestDoWithContext_Timeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	attempts := 0

	err := DoWithContext(ctx, func() error {
		attempts++
		time.Sleep(50 * time.Millisecond) // 每次尝试耗时50ms
		return errTest
	}, Attempts(10), Delay(20*time.Millisecond))

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestDo_MaxDelay(t *testing.T) {
	attempts := 0

	Do(func() error {
		attempts++
		return errTest
	}, Attempts(5), Delay(10*time.Millisecond), MaxDelay(50*time.Millisecond), DelayType(ExponentialBackoff), Multiplier(10.0))

	// 指数退避：10ms, 100ms, 1000ms...
	// 但 MaxDelay 限制为 50ms
	if attempts != 5 {
		t.Errorf("expected 5 attempts, got %d", attempts)
	}
}

func TestCalculateDelayCapsRetryAfterAtMaxDelay(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "120")
	config := &Config{
		MaxDelay:        5 * time.Second,
		RetryAfterAware: true,
		LastError:       NewHTTPError(resp),
	}

	if got := calculateDelay(1, config); got != config.MaxDelay {
		t.Fatalf("calculateDelay() = %v, want %v", got, config.MaxDelay)
	}
}

func TestLinearBackoffCapsBeforeDurationMultiplicationOverflow(t *testing.T) {
	config := &Config{Delay: time.Second, MaxDelay: 5 * time.Second}
	maximumInt := int(^uint(0) >> 1)

	if got := LinearBackoff(maximumInt, config); got != config.MaxDelay {
		t.Fatalf("LinearBackoff() = %v, want %v", got, config.MaxDelay)
	}
}

func TestDo_CustomAttempts(t *testing.T) {
	attempts := 0

	err := Do(func() error {
		attempts++
		return errTest
	}, Attempts(5), Delay(10*time.Millisecond))

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// 应该尝试5次
	if attempts != 5 {
		t.Errorf("expected 5 attempts, got %d", attempts)
	}
}

func TestExponentialBackoff_Function(t *testing.T) {
	config := &Config{
		Delay:      time.Second,
		MaxDelay:   30 * time.Second,
		Multiplier: 2.0,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{10, 30 * time.Second}, // 受 MaxDelay 限制
	}

	for _, tt := range tests {
		result := ExponentialBackoff(tt.attempt, config)
		if result != tt.expected {
			t.Errorf("attempt %d: expected %v, got %v", tt.attempt, tt.expected, result)
		}
	}
}

func TestLinearBackoff_Function(t *testing.T) {
	config := &Config{
		Delay:    time.Second,
		MaxDelay: 5 * time.Second,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 3 * time.Second},
		{5, 5 * time.Second},
		{10, 5 * time.Second}, // 受 MaxDelay 限制
	}

	for _, tt := range tests {
		result := LinearBackoff(tt.attempt, config)
		if result != tt.expected {
			t.Errorf("attempt %d: expected %v, got %v", tt.attempt, tt.expected, result)
		}
	}
}

func TestFixedDelay_Function(t *testing.T) {
	config := &Config{
		Delay: time.Second,
	}

	for attempt := 1; attempt <= 10; attempt++ {
		result := FixedDelay(attempt, config)
		if result != time.Second {
			t.Errorf("attempt %d: expected %v, got %v", attempt, time.Second, result)
		}
	}
}

func TestCalculateDelayAppliesJitterAfterConfiguredBackoff(t *testing.T) {
	config := &Config{
		Delay:       time.Nanosecond,
		MaxDelay:    time.Second,
		Multiplier:  1,
		DelayFunc:   FixedDelay,
		JitterType:  FullJitter,
		MaxAttempts: 2,
	}

	// 一纳秒乘以 [0, 1) 的随机数必然向下取整为零，可稳定证明抖动已生效。
	if got := calculateDelay(1, config); got != 0 {
		t.Fatalf("calculateDelay() = %v, want 0", got)
	}
}

func TestExponentialBackoffPreservesZeroDelay(t *testing.T) {
	config := &Config{
		Delay:      0,
		MaxDelay:   time.Second,
		Multiplier: 2,
	}

	if got := ExponentialBackoff(32, config); got != 0 {
		t.Fatalf("ExponentialBackoff() = %v, want 0", got)
	}
}

func TestPresetStrategiesReturnIndependentSlices(t *testing.T) {
	first := OpenAIStrategy()
	first[0] = Attempts(1)

	config, err := newConfig(OpenAIStrategy()...)
	if err != nil {
		t.Fatal(err)
	}
	if config.MaxAttempts != 5 {
		t.Fatalf("OpenAIStrategy() MaxAttempts = %d, want 5", config.MaxAttempts)
	}
}

// Benchmark 测试
func BenchmarkDo_NoRetry(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Do(func() error {
			return nil
		})
	}
}

func BenchmarkDo_WithRetry(b *testing.B) {
	attempts := 0
	for i := 0; i < b.N; i++ {
		attempts = 0
		Do(func() error {
			attempts++
			if attempts < 3 {
				return errTest
			}
			return nil
		}, Attempts(5), Delay(time.Microsecond))
	}
}
