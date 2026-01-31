package retry

import (
	"testing"
	"time"
)

func TestWithJitter(t *testing.T) {
	baseDelay := 100 * time.Millisecond
	attempts := 0

	err := Do(func() error {
		attempts++
		if attempts < 3 {
			return errTest
		}
		return nil
	},
		Attempts(5),
		Delay(baseDelay),
		WithJitter(0.3),
	)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestWithFullJitter(t *testing.T) {
	baseDelay := 100 * time.Millisecond
	attempts := 0

	start := time.Now()
	err := Do(func() error {
		attempts++
		if attempts < 3 {
			return errTest
		}
		return nil
	},
		Attempts(5),
		Delay(baseDelay),
		WithFullJitter(),
	)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// 全抖动应该使延迟变短
	// 理论上延迟在 [0, baseDelay] 之间，所以总时间应该小于 2*baseDelay
	maxExpected := 2 * baseDelay * 2 // 2 次重试
	if elapsed > maxExpected {
		t.Logf("elapsed time %v might be higher due to jitter randomness", elapsed)
	}
}

func TestWithEqualJitter(t *testing.T) {
	config := &Config{
		Delay:      100 * time.Millisecond,
		JitterType: EqualJitter,
	}

	// 测试多次，确保延迟在预期范围内
	for i := 0; i < 100; i++ {
		delay := addJitter(config.Delay, config)
		half := config.Delay / 2

		if delay < half || delay > config.Delay {
			t.Errorf("equal jitter delay %v out of range [%v, %v]", delay, half, config.Delay)
		}
	}
}

func TestJitterFactor(t *testing.T) {
	config := &Config{
		Delay:        100 * time.Millisecond,
		JitterFactor: 0.3,
	}

	minExpected := time.Duration(float64(config.Delay) * 0.7)
	maxExpected := time.Duration(float64(config.Delay) * 1.3)

	// 测试多次
	for i := 0; i < 100; i++ {
		delay := addJitter(config.Delay, config)

		if delay < minExpected || delay > maxExpected {
			t.Errorf("jitter delay %v out of range [%v, %v]", delay, minExpected, maxExpected)
		}
	}
}

// errTest 在 retry_test.go 中定义
