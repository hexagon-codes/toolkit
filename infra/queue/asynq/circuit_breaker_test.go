package asynq

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		state    CircuitState
		expected string
	}{
		{StateClosed, "CLOSED"},
		{StateOpen, "OPEN"},
		{StateHalfOpen, "HALF_OPEN"},
		{CircuitState(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("state %d: expected %s, got %s", tt.state, tt.expected, got)
		}
	}
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig()

	if config.FailureThreshold != 5 {
		t.Errorf("expected FailureThreshold=5, got %d", config.FailureThreshold)
	}
	if config.SuccessThreshold != 2 {
		t.Errorf("expected SuccessThreshold=2, got %d", config.SuccessThreshold)
	}
	if config.Timeout != 30*time.Second {
		t.Errorf("expected Timeout=30s, got %v", config.Timeout)
	}
	if config.HalfOpenMaxRequests != 3 {
		t.Errorf("expected HalfOpenMaxRequests=3, got %d", config.HalfOpenMaxRequests)
	}
}

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker("test", DefaultCircuitBreakerConfig())

	if cb.State() != StateClosed {
		t.Errorf("expected initial state CLOSED, got %s", cb.State())
	}

	if cb.IsOpen() {
		t.Error("expected IsOpen=false initially")
	}
}

func TestCircuitBreaker_Allow_Closed(t *testing.T) {
	cb := NewCircuitBreaker("test", DefaultCircuitBreakerConfig())

	err := cb.Allow()
	if err != nil {
		t.Errorf("expected no error in CLOSED state, got: %v", err)
	}
}

func TestCircuitBreaker_TransitionToOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold:    3,
		SuccessThreshold:    2,
		Timeout:             time.Second,
		HalfOpenMaxRequests: 2,
	}
	cb := NewCircuitBreaker("test", config)

	// 记录 3 次失败，触发熔断
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.State() != StateOpen {
		t.Errorf("expected state OPEN after %d failures, got %s", config.FailureThreshold, cb.State())
	}

	if !cb.IsOpen() {
		t.Error("expected IsOpen=true after failures")
	}

	// 在 OPEN 状态下，Allow 应该返回错误
	err := cb.Allow()
	if err != ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen, got: %v", err)
	}
}

func TestCircuitBreaker_TransitionToHalfOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    2,
		Timeout:             100 * time.Millisecond,
		HalfOpenMaxRequests: 3,
	}
	cb := NewCircuitBreaker("test", config)

	// 触发熔断
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Fatal("expected state OPEN")
	}

	// 等待超时
	time.Sleep(150 * time.Millisecond)

	// 下次 Allow 应该转为 HALF_OPEN
	err := cb.Allow()
	if err != nil {
		t.Errorf("expected no error when transitioning to HALF_OPEN, got: %v", err)
	}

	if cb.State() != StateHalfOpen {
		t.Errorf("expected state HALF_OPEN, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpen_LimitRequests(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    2,
		Timeout:             100 * time.Millisecond,
		HalfOpenMaxRequests: 2,
	}
	cb := NewCircuitBreaker("test", config)

	// 触发熔断并进入半开状态
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(150 * time.Millisecond)
	cb.Allow() // 进入 HALF_OPEN

	// 第一次允许
	err := cb.Allow()
	if err != nil {
		t.Errorf("expected no error for first request, got: %v", err)
	}

	// 第二次允许
	err = cb.Allow()
	if err != nil {
		t.Errorf("expected no error for second request, got: %v", err)
	}

	// 第三次应该被拒绝（超过限制）
	err = cb.Allow()
	if err != ErrCircuitHalfOpen {
		t.Errorf("expected ErrCircuitHalfOpen, got: %v", err)
	}
}

func TestCircuitBreaker_HalfOpen_SuccessRecovery(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    2,
		Timeout:             100 * time.Millisecond,
		HalfOpenMaxRequests: 3,
	}
	cb := NewCircuitBreaker("test", config)

	// 触发熔断并进入半开状态
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(150 * time.Millisecond)
	cb.Allow() // 进入 HALF_OPEN

	// 记录 2 次成功，应该恢复到 CLOSED
	cb.RecordSuccess()
	if cb.State() == StateClosed {
		t.Error("should not transition to CLOSED after only 1 success")
	}

	cb.RecordSuccess()
	if cb.State() != StateClosed {
		t.Errorf("expected state CLOSED after %d successes, got %s", config.SuccessThreshold, cb.State())
	}
}

func TestCircuitBreaker_HalfOpen_FailureReopen(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    2,
		Timeout:             100 * time.Millisecond,
		HalfOpenMaxRequests: 3,
	}
	cb := NewCircuitBreaker("test", config)

	// 触发熔断并进入半开状态
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(150 * time.Millisecond)
	cb.Allow() // 进入 HALF_OPEN

	// 记录失败，应该立即回到 OPEN
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Errorf("expected state OPEN after failure in HALF_OPEN, got %s", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker("test", DefaultCircuitBreakerConfig())

	// 触发熔断
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}

	if cb.State() != StateOpen {
		t.Fatal("expected state OPEN")
	}

	// 重置
	cb.Reset()

	if cb.State() != StateClosed {
		t.Errorf("expected state CLOSED after reset, got %s", cb.State())
	}

	stats := cb.Stats()
	if stats.FailureCount != 0 {
		t.Errorf("expected FailureCount=0 after reset, got %d", stats.FailureCount)
	}
}

func TestCircuitBreaker_Stats(t *testing.T) {
	cb := NewCircuitBreaker("test-breaker", DefaultCircuitBreakerConfig())

	cb.RecordFailure()
	cb.RecordFailure()

	stats := cb.Stats()

	if stats.Name != "test-breaker" {
		t.Errorf("expected name 'test-breaker', got '%s'", stats.Name)
	}
	if stats.State != "CLOSED" {
		t.Errorf("expected state 'CLOSED', got '%s'", stats.State)
	}
	if stats.FailureCount != 2 {
		t.Errorf("expected FailureCount=2, got %d", stats.FailureCount)
	}
	if stats.ConsecutiveErrors != 2 {
		t.Errorf("expected ConsecutiveErrors=2, got %d", stats.ConsecutiveErrors)
	}
}

func TestCircuitBreaker_OnStateChange(t *testing.T) {
	var changeCount atomic.Int32
	var mu sync.Mutex
	var lastFrom, lastTo CircuitState

	config := CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    2,
		Timeout:             100 * time.Millisecond,
		HalfOpenMaxRequests: 2,
		OnStateChange: func(name string, from, to CircuitState) {
			changeCount.Add(1)
			mu.Lock()
			lastFrom = from
			lastTo = to
			mu.Unlock()
		},
	}

	cb := NewCircuitBreaker("test", config)

	// 触发熔断
	cb.RecordFailure()
	cb.RecordFailure()

	// 等待回调执行
	time.Sleep(50 * time.Millisecond)

	if changeCount.Load() != 1 {
		t.Errorf("expected 1 state change, got %d", changeCount.Load())
	}
	mu.Lock()
	from, to := lastFrom, lastTo
	mu.Unlock()
	if from != StateClosed || to != StateOpen {
		t.Errorf("expected CLOSED -> OPEN, got %s -> %s", from, to)
	}
}

func TestChannelBreakerManager_GetBreaker(t *testing.T) {
	manager := GetChannelBreakerManager()

	breaker1 := manager.GetBreaker(123)
	breaker2 := manager.GetBreaker(123)

	// 应该返回同一个实例
	if breaker1 != breaker2 {
		t.Error("expected same breaker instance for same channel ID")
	}

	breaker3 := manager.GetBreaker(456)
	if breaker1 == breaker3 {
		t.Error("expected different breaker instances for different channel IDs")
	}
}

func TestChannelBreakerManager_Allow(t *testing.T) {
	manager := GetChannelBreakerManager()
	manager.SetConfig(CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    2,
		Timeout:             time.Second,
		HalfOpenMaxRequests: 2,
	})

	channelID := 999

	// 初始应该允许
	err := manager.Allow(channelID)
	if err != nil {
		t.Errorf("expected no error initially, got: %v", err)
	}

	// 触发熔断
	manager.RecordFailure(channelID)
	manager.RecordFailure(channelID)

	// 应该被拒绝
	err = manager.Allow(channelID)
	if err != ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen, got: %v", err)
	}

	// 检查熔断状态
	if !manager.IsOpen(channelID) {
		t.Error("expected IsOpen=true")
	}
}

func TestChannelBreakerManager_Reset(t *testing.T) {
	manager := GetChannelBreakerManager()
	manager.SetConfig(CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    2,
		Timeout:             time.Second,
		HalfOpenMaxRequests: 2,
	})

	channelID := 888

	// 触发熔断
	manager.RecordFailure(channelID)
	manager.RecordFailure(channelID)

	if !manager.IsOpen(channelID) {
		t.Fatal("expected breaker to be open")
	}

	// 重置
	manager.Reset(channelID)

	if manager.IsOpen(channelID) {
		t.Error("expected breaker to be closed after reset")
	}
}

func TestChannelBreakerManager_GetAllStats(t *testing.T) {
	manager := GetChannelBreakerManager()

	// 创建几个熔断器
	manager.GetBreaker(1001)
	manager.GetBreaker(1002)
	manager.GetBreaker(1003)

	stats := manager.GetAllStats()
	if len(stats) < 3 {
		t.Errorf("expected at least 3 stats, got %d", len(stats))
	}
}

func TestChannelBreakerManager_GetOpenBreakers(t *testing.T) {
	manager := GetChannelBreakerManager()
	manager.SetConfig(CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    2,
		Timeout:             time.Second,
		HalfOpenMaxRequests: 2,
	})

	// 触发一个熔断
	manager.RecordFailure(2001)
	manager.RecordFailure(2001)

	openBreakers := manager.GetOpenBreakers()
	found := false
	for _, id := range openBreakers {
		if id == 2001 {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected channel 2001 to be in open breakers list")
	}
}

func TestChannelBreakerManager_ResetAll(t *testing.T) {
	manager := GetChannelBreakerManager()
	manager.SetConfig(CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    2,
		Timeout:             time.Second,
		HalfOpenMaxRequests: 2,
	})

	// 触发多个熔断
	manager.RecordFailure(3001)
	manager.RecordFailure(3001)
	manager.RecordFailure(3002)
	manager.RecordFailure(3002)

	// 重置所有
	manager.ResetAll()

	if manager.IsOpen(3001) || manager.IsOpen(3002) {
		t.Error("expected all breakers to be closed after ResetAll")
	}
}

func TestPlatformBreakerManager_GetBreaker(t *testing.T) {
	manager := GetPlatformBreakerManager()

	breaker1 := manager.GetBreaker("openai")
	breaker2 := manager.GetBreaker("openai")

	// 应该返回同一个实例
	if breaker1 != breaker2 {
		t.Error("expected same breaker instance for same platform")
	}

	breaker3 := manager.GetBreaker("anthropic")
	if breaker1 == breaker3 {
		t.Error("expected different breaker instances for different platforms")
	}
}

func TestPlatformBreakerManager_Basic(t *testing.T) {
	manager := GetPlatformBreakerManager()

	platform := "test-platform"

	// 初始应该允许
	err := manager.Allow(platform)
	if err != nil {
		t.Errorf("expected no error initially, got: %v", err)
	}

	// 记录成功
	manager.RecordSuccess(platform)

	// 仍然应该允许
	err = manager.Allow(platform)
	if err != nil {
		t.Errorf("expected no error after success, got: %v", err)
	}
}

func TestPlatformBreakerManager_GetAllStats(t *testing.T) {
	manager := GetPlatformBreakerManager()

	// 创建几个熔断器
	manager.GetBreaker("platform1")
	manager.GetBreaker("platform2")

	stats := manager.GetAllStats()
	if len(stats) < 2 {
		t.Errorf("expected at least 2 stats, got %d", len(stats))
	}
}

func TestCircuitBreaker_RecordSuccess_InClosedState(t *testing.T) {
	cb := NewCircuitBreaker("test", DefaultCircuitBreakerConfig())

	cb.RecordFailure()
	cb.RecordFailure()

	stats := cb.Stats()
	if stats.FailureCount != 2 {
		t.Fatal("expected 2 failures")
	}

	// 记录成功应该重置失败计数
	cb.RecordSuccess()

	stats = cb.Stats()
	if stats.FailureCount != 0 {
		t.Errorf("expected FailureCount=0 after success in CLOSED, got %d", stats.FailureCount)
	}
	if stats.ConsecutiveErrors != 0 {
		t.Errorf("expected ConsecutiveErrors=0, got %d", stats.ConsecutiveErrors)
	}
}
