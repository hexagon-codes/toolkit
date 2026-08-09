package asynq

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hexagon-codes/toolkit/util/circuit"
)

func testCircuitConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:    1,
		SuccessThreshold:    2,
		Timeout:             5 * time.Millisecond,
		HalfOpenMaxRequests: 2,
	}
}

func newTestCircuitBreaker(t *testing.T, name string, config CircuitBreakerConfig) *CircuitBreaker {
	t.Helper()
	breaker, err := NewCircuitBreaker(name, config)
	if err != nil {
		t.Fatalf("NewCircuitBreaker() error = %v", err)
	}
	t.Cleanup(breaker.Close)
	return breaker
}

func newTestChannelManager(t *testing.T, config CircuitBreakerConfig) *ChannelCircuitBreakerManager {
	t.Helper()
	manager, err := NewChannelCircuitBreakerManager(config)
	if err != nil {
		t.Fatalf("NewChannelCircuitBreakerManager() error = %v", err)
	}
	t.Cleanup(manager.Close)
	return manager
}

func newTestPlatformManager(t *testing.T, config CircuitBreakerConfig) *PlatformCircuitBreakerManager {
	t.Helper()
	manager, err := NewPlatformCircuitBreakerManager(config)
	if err != nil {
		t.Fatalf("NewPlatformCircuitBreakerManager() error = %v", err)
	}
	t.Cleanup(manager.Close)
	return manager
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	if config.FailureThreshold != 5 || config.SuccessThreshold != 2 {
		t.Fatalf("unexpected thresholds: %+v", config)
	}
	if config.Timeout != 30*time.Second || config.HalfOpenMaxRequests != 3 {
		t.Fatalf("unexpected timing config: %+v", config)
	}
}

func TestCircuitBreakerRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CircuitBreakerConfig)
	}{
		{name: "failure threshold", mutate: func(config *CircuitBreakerConfig) { config.FailureThreshold = 0 }},
		{name: "success threshold", mutate: func(config *CircuitBreakerConfig) { config.SuccessThreshold = 0 }},
		{name: "timeout", mutate: func(config *CircuitBreakerConfig) { config.Timeout = 0 }},
		{name: "half-open limit", mutate: func(config *CircuitBreakerConfig) { config.HalfOpenMaxRequests = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := DefaultCircuitBreakerConfig()
			test.mutate(&config)
			if _, err := NewCircuitBreaker("invalid", config); err == nil {
				t.Fatal("NewCircuitBreaker() error = nil")
			}
		})
	}
	if _, err := NewCircuitBreaker("", DefaultCircuitBreakerConfig()); err == nil {
		t.Fatal("NewCircuitBreaker() accepted an empty name")
	}
}

func TestCircuitBreakerAllowsSequentialRecoveryAboveHalfOpenConcurrency(t *testing.T) {
	config := testCircuitConfig()
	config.HalfOpenMaxRequests = 1
	config.SuccessThreshold = 2
	breaker := newTestCircuitBreaker(t, "sequential-recovery", config)

	_, _ = breaker.Execute(func() (any, error) {
		return nil, errors.New("upstream failed")
	})
	time.Sleep(10 * time.Millisecond)

	first, err := breaker.Acquire()
	if err != nil {
		t.Fatalf("first half-open Acquire() error = %v", err)
	}
	if completeErr := first.Complete(nil); completeErr != nil {
		t.Fatalf("first Complete() error = %v", completeErr)
	}
	if breaker.State() != StateHalfOpen {
		t.Fatalf("State() after first success = %s, want half-open", breaker.State())
	}

	second, err := breaker.Acquire()
	if err != nil {
		t.Fatalf("second half-open Acquire() error = %v", err)
	}
	if err := second.Complete(nil); err != nil {
		t.Fatalf("second Complete() error = %v", err)
	}
	if breaker.State() != StateClosed {
		t.Fatalf("State() after sequential recovery = %s, want closed", breaker.State())
	}
}

func TestCircuitBreakerLifecycle(t *testing.T) {
	breaker := newTestCircuitBreaker(t, "lifecycle", testCircuitConfig())
	if breaker.State() != StateClosed {
		t.Fatalf("initial State() = %s", breaker.State())
	}
	_, err := breaker.Execute(func() (any, error) {
		return nil, errors.New("upstream failed")
	})
	if err == nil || breaker.State() != StateOpen {
		t.Fatalf("failure result = (%v, %s)", err, breaker.State())
	}
	if _, acquireErr := breaker.Acquire(); !errors.Is(acquireErr, ErrCircuitOpen) {
		t.Fatalf("Acquire() error = %v, want ErrCircuitOpen", acquireErr)
	}

	time.Sleep(10 * time.Millisecond)
	first, err := breaker.Acquire()
	if err != nil {
		t.Fatalf("first half-open Acquire() error = %v", err)
	}
	second, err := breaker.Acquire()
	if err != nil {
		t.Fatalf("second half-open Acquire() error = %v", err)
	}
	if _, err := breaker.Acquire(); !errors.Is(err, ErrCircuitHalfOpen) {
		t.Fatalf("third half-open Acquire() error = %v", err)
	}
	if err := first.Complete(nil); err != nil {
		t.Fatalf("first Complete() error = %v", err)
	}
	if err := second.Complete(nil); err != nil {
		t.Fatalf("second Complete() error = %v", err)
	}
	if breaker.State() != StateClosed {
		t.Fatalf("recovered State() = %s", breaker.State())
	}
}

func TestCircuitBreakerIgnoresStaleHalfOpenCompletion(t *testing.T) {
	breaker := newTestCircuitBreaker(t, "stale", testCircuitConfig())
	_, _ = breaker.Execute(func() (any, error) { return nil, errors.New("failed") })
	time.Sleep(10 * time.Millisecond)
	first, err := breaker.Acquire()
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	second, err := breaker.Acquire()
	if err != nil {
		t.Fatalf("second Acquire() error = %v", err)
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

func TestCircuitBreakerCallbackIsSynchronousAndPanicSafe(t *testing.T) {
	var calls atomic.Int32
	config := testCircuitConfig()
	config.OnStateChange = func(_ string, from, to CircuitState) {
		if from != StateClosed || to != StateOpen {
			t.Fatalf("state change = %s -> %s", from, to)
		}
		calls.Add(1)
		panic("callback panic")
	}
	breaker := newTestCircuitBreaker(t, "callback", config)
	_, _ = breaker.Execute(func() (any, error) { return nil, errors.New("failed") })
	if calls.Load() != 1 {
		t.Fatalf("callback calls = %d", calls.Load())
	}
}

func TestCircuitBreakerStatsAndReset(t *testing.T) {
	config := testCircuitConfig()
	config.FailureThreshold = 3
	breaker := newTestCircuitBreaker(t, "stats", config)
	_, _ = breaker.Execute(func() (any, error) { return nil, errors.New("failed") })
	stats := breaker.Stats()
	if stats.Name != "stats" || stats.State != "closed" || stats.FailureCount != 1 {
		t.Fatalf("Stats() = %+v", stats)
	}
	if stats.LastFailureTime.IsZero() || stats.LastUsedTime.IsZero() {
		t.Fatalf("Stats() timestamps = %+v", stats)
	}
	if err := breaker.Reset(); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	if reset := breaker.Stats(); reset.FailureCount != 0 || reset.State != "closed" {
		t.Fatalf("reset Stats() = %+v", reset)
	}
}

func TestChannelCircuitBreakerManagerIsIsolatedAndDeterministic(t *testing.T) {
	manager := newTestChannelManager(t, testCircuitConfig())
	first, err := manager.GetBreaker(2)
	if err != nil {
		t.Fatalf("GetBreaker(2) error = %v", err)
	}
	same, err := manager.GetBreaker(2)
	if err != nil || same != first {
		t.Fatalf("second GetBreaker(2) = (%p, %v)", same, err)
	}
	_, _ = manager.Execute(2, func() (any, error) { return nil, errors.New("failed") })
	_, _ = manager.GetBreaker(1)
	if open := manager.GetOpenBreakers(); len(open) != 1 || open[0] != 2 {
		t.Fatalf("GetOpenBreakers() = %v", open)
	}
	stats := manager.GetAllStats()
	if len(stats) != 2 || stats[0].Name != "channel_1" || stats[1].Name != "channel_2" {
		t.Fatalf("GetAllStats() = %+v", stats)
	}
	if resetErr := manager.ResetAll(); resetErr != nil {
		t.Fatalf("ResetAll() error = %v", resetErr)
	}
	open, err := manager.IsOpen(2)
	if err != nil || open {
		t.Fatalf("IsOpen(2) = (%v, %v)", open, err)
	}
}

func TestChannelCircuitBreakerManagerConcurrentGet(t *testing.T) {
	manager := newTestChannelManager(t, testCircuitConfig())
	const workers = 64
	results := make(chan *CircuitBreaker, workers)
	errorsChannel := make(chan error, workers)
	var waitGroup sync.WaitGroup
	for range workers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			breaker, err := manager.GetBreaker(7)
			results <- breaker
			errorsChannel <- err
		}()
	}
	waitGroup.Wait()
	close(results)
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatalf("GetBreaker() error = %v", err)
		}
	}
	var expected *CircuitBreaker
	for breaker := range results {
		if expected == nil {
			expected = breaker
		}
		if breaker != expected {
			t.Fatal("GetBreaker() created duplicate instances")
		}
	}
}

func TestCleanupIdleClosesOnlyExpiredBreakers(t *testing.T) {
	manager := newTestChannelManager(t, testCircuitConfig())
	breaker, err := manager.GetBreaker(8)
	if err != nil {
		t.Fatalf("GetBreaker() error = %v", err)
	}
	removed, err := manager.CleanupIdle(time.Hour)
	if err != nil || removed != 0 {
		t.Fatalf("first CleanupIdle() = (%d, %v)", removed, err)
	}
	breaker.lastUsed.Store(time.Now().Add(-2 * time.Hour).UnixNano())
	removed, err = manager.CleanupIdle(time.Hour)
	if err != nil || removed != 1 {
		t.Fatalf("second CleanupIdle() = (%d, %v)", removed, err)
	}
	if _, err := breaker.Acquire(); !errors.Is(err, circuit.ErrBreakerClosed) {
		t.Fatalf("removed breaker Acquire() error = %v", err)
	}
}

func TestCleanupIdlePreservesBreakerWithActivePermit(t *testing.T) {
	manager := newTestChannelManager(t, testCircuitConfig())
	breaker, err := manager.GetBreaker(9)
	if err != nil {
		t.Fatalf("GetBreaker() error = %v", err)
	}
	permit, err := breaker.Acquire()
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	breaker.lastUsed.Store(time.Now().Add(-2 * time.Hour).UnixNano())

	removed, err := manager.CleanupIdle(time.Hour)
	if err != nil || removed != 0 {
		t.Fatalf("CleanupIdle() with active permit = (%d, %v), want (0, nil)", removed, err)
	}
	if completeErr := permit.Complete(nil); completeErr != nil {
		t.Fatalf("active permit Complete() error = %v", completeErr)
	}

	breaker.lastUsed.Store(time.Now().Add(-2 * time.Hour).UnixNano())
	removed, err = manager.CleanupIdle(time.Hour)
	if err != nil || removed != 1 {
		t.Fatalf("CleanupIdle() after completion = (%d, %v), want (1, nil)", removed, err)
	}
}

func TestPlatformCircuitBreakerManager(t *testing.T) {
	manager := newTestPlatformManager(t, testCircuitConfig())
	if _, err := manager.GetBreaker(""); err == nil {
		t.Fatal("GetBreaker() accepted an empty platform")
	}
	first, err := manager.GetBreaker("openai")
	if err != nil {
		t.Fatalf("GetBreaker(openai) error = %v", err)
	}
	second, err := manager.GetBreaker("openai")
	if err != nil || first != second {
		t.Fatalf("second GetBreaker(openai) = (%p, %v)", second, err)
	}
	_, _ = manager.Execute("openai", func() (any, error) { return nil, errors.New("failed") })
	open, err := manager.IsOpen("openai")
	if err != nil || !open {
		t.Fatalf("IsOpen(openai) = (%v, %v)", open, err)
	}
	if err := manager.Reset("openai"); err != nil {
		t.Fatalf("Reset(openai) error = %v", err)
	}
	if stats := manager.GetAllStats(); len(stats) != 1 || stats[0].Name != "platform_openai" {
		t.Fatalf("GetAllStats() = %+v", stats)
	}
}

func TestCircuitBreakerManagerCloseRejectsNewWork(t *testing.T) {
	manager, err := NewChannelCircuitBreakerManager(testCircuitConfig())
	if err != nil {
		t.Fatalf("NewChannelCircuitBreakerManager() error = %v", err)
	}
	manager.Close()
	manager.Close()
	if _, err := manager.GetBreaker(1); !errors.Is(err, circuit.ErrBreakerClosed) {
		t.Fatalf("GetBreaker() after Close error = %v", err)
	}
}
