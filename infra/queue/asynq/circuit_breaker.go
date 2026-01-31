package asynq

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// =========================================
// 熔断器实现
// 用于保护上游 API，防止故障扩散
// =========================================
// 熔断器状态
type CircuitState int

const (
	// StateClosed 关闭状态（正常）
	StateClosed CircuitState = iota
	// StateOpen 开启状态（熔断中）
	StateOpen
	// StateHalfOpen 半开状态（探测恢复）
	StateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// 熔断器相关错误
var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrCircuitHalfOpen = errors.New("circuit breaker is half-open, limiting requests")
)

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	// FailureThreshold 触发熔断的连续失败次数
	FailureThreshold int
	// SuccessThreshold 恢复正常的连续成功次数
	SuccessThreshold int
	// Timeout 熔断持续时间（开启后多久进入半开状态）
	Timeout time.Duration
	// HalfOpenMaxRequests 半开状态允许的最大请求数
	HalfOpenMaxRequests int
	// OnStateChange 状态变化回调
	OnStateChange func(name string, from, to CircuitState)
}

// DefaultCircuitBreakerConfig 默认配置
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:    5,
		SuccessThreshold:    2,
		Timeout:             30 * time.Second,
		HalfOpenMaxRequests: 3,
	}
}

// CircuitBreaker 熔断器
type CircuitBreaker struct {
	name              string
	config            CircuitBreakerConfig
	mu                sync.RWMutex
	state             CircuitState
	failureCount      int
	successCount      int
	lastFailureTime   time.Time
	halfOpenRequests  int
	consecutiveErrors int
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(name string, config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		name:   name,
		config: config,
		state:  StateClosed,
	}
}

// Allow 检查是否允许请求通过
func (cb *CircuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case StateClosed:
		return nil
	case StateOpen:
		// 检查是否到了恢复探测时间
		if time.Since(cb.lastFailureTime) > cb.config.Timeout {
			cb.toHalfOpen()
			return nil
		}
		return ErrCircuitOpen
	case StateHalfOpen:
		// 半开状态限制请求数
		if cb.halfOpenRequests >= cb.config.HalfOpenMaxRequests {
			return ErrCircuitHalfOpen
		}
		cb.halfOpenRequests++
		return nil
	}
	return nil
}

// RecordSuccess 记录成功
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveErrors = 0
	switch cb.state {
	case StateClosed:
		cb.failureCount = 0
	case StateHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.config.SuccessThreshold {
			cb.toClosed()
		}
	}
}

// RecordFailure 记录失败
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveErrors++
	cb.lastFailureTime = time.Now()
	switch cb.state {
	case StateClosed:
		cb.failureCount++
		if cb.failureCount >= cb.config.FailureThreshold {
			cb.toOpen()
		}
	case StateHalfOpen:
		// 半开状态下任何失败都触发熔断
		cb.toOpen()
	}
}

// State 获取当前状态
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// IsOpen 是否处于熔断状态
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state == StateOpen
}

// Reset 重置熔断器
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.toClosed()
}

// Stats 获取统计信息
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return CircuitBreakerStats{
		Name:              cb.name,
		State:             cb.state.String(),
		FailureCount:      cb.failureCount,
		SuccessCount:      cb.successCount,
		ConsecutiveErrors: cb.consecutiveErrors,
		LastFailureTime:   cb.lastFailureTime,
	}
}

// 状态转换
func (cb *CircuitBreaker) toClosed() {
	if cb.state == StateClosed {
		return
	}
	oldState := cb.state
	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.halfOpenRequests = 0
	cb.consecutiveErrors = 0
	GetLogger().Log(fmt.Sprintf("[CircuitBreaker] %s: %s -> CLOSED", cb.name, oldState))
	if cb.config.OnStateChange != nil {
		go cb.config.OnStateChange(cb.name, oldState, StateClosed)
	}
}
func (cb *CircuitBreaker) toOpen() {
	if cb.state == StateOpen {
		return
	}
	oldState := cb.state
	cb.state = StateOpen
	cb.successCount = 0
	cb.halfOpenRequests = 0
	GetLogger().Log(fmt.Sprintf("[CircuitBreaker] %s: %s -> OPEN (failures=%d)",
		cb.name, oldState, cb.failureCount))
	if cb.config.OnStateChange != nil {
		go cb.config.OnStateChange(cb.name, oldState, StateOpen)
	}
}
func (cb *CircuitBreaker) toHalfOpen() {
	if cb.state == StateHalfOpen {
		return
	}
	oldState := cb.state
	cb.state = StateHalfOpen
	cb.successCount = 0
	cb.halfOpenRequests = 0
	GetLogger().Log(fmt.Sprintf("[CircuitBreaker] %s: %s -> HALF_OPEN", cb.name, oldState))
	if cb.config.OnStateChange != nil {
		go cb.config.OnStateChange(cb.name, oldState, StateHalfOpen)
	}
}

// CircuitBreakerStats 熔断器统计
type CircuitBreakerStats struct {
	Name              string    `json:"name"`
	State             string    `json:"state"`
	FailureCount      int       `json:"failure_count"`
	SuccessCount      int       `json:"success_count"`
	ConsecutiveErrors int       `json:"consecutive_errors"`
	LastFailureTime   time.Time `json:"last_failure_time"`
}

// =========================================
// 渠道熔断器管理
// =========================================
// ChannelCircuitBreakerManager 渠道熔断器管理器
type ChannelCircuitBreakerManager struct {
	breakers map[int]*CircuitBreaker
	mu       sync.RWMutex
	config   CircuitBreakerConfig
}

// 全局渠道熔断器管理器
var (
	channelBreakerManager *ChannelCircuitBreakerManager
	channelBreakerOnce    sync.Once
)

// GetChannelBreakerManager 获取渠道熔断器管理器
func GetChannelBreakerManager() *ChannelCircuitBreakerManager {
	channelBreakerOnce.Do(func() {
		channelBreakerManager = &ChannelCircuitBreakerManager{
			breakers: make(map[int]*CircuitBreaker),
			config:   DefaultCircuitBreakerConfig(),
		}
	})
	return channelBreakerManager
}

// SetConfig 设置默认配置
func (m *ChannelCircuitBreakerManager) SetConfig(config CircuitBreakerConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
}

// GetBreaker 获取渠道熔断器（不存在则创建）
func (m *ChannelCircuitBreakerManager) GetBreaker(channelID int) *CircuitBreaker {
	m.mu.RLock()
	if breaker, ok := m.breakers[channelID]; ok {
		m.mu.RUnlock()
		return breaker
	}
	m.mu.RUnlock()
	// 创建新的熔断器
	m.mu.Lock()
	defer m.mu.Unlock()
	// 双重检查
	if breaker, ok := m.breakers[channelID]; ok {
		return breaker
	}
	name := fmt.Sprintf("channel_%d", channelID)
	breaker := NewCircuitBreaker(name, m.config)
	m.breakers[channelID] = breaker
	return breaker
}

// Allow 检查渠道是否允许请求
func (m *ChannelCircuitBreakerManager) Allow(channelID int) error {
	breaker := m.GetBreaker(channelID)
	return breaker.Allow()
}

// RecordSuccess 记录渠道请求成功
func (m *ChannelCircuitBreakerManager) RecordSuccess(channelID int) {
	breaker := m.GetBreaker(channelID)
	breaker.RecordSuccess()
}

// RecordFailure 记录渠道请求失败
func (m *ChannelCircuitBreakerManager) RecordFailure(channelID int) {
	breaker := m.GetBreaker(channelID)
	breaker.RecordFailure()
}

// IsOpen 检查渠道是否熔断
func (m *ChannelCircuitBreakerManager) IsOpen(channelID int) bool {
	breaker := m.GetBreaker(channelID)
	return breaker.IsOpen()
}

// Reset 重置渠道熔断器
func (m *ChannelCircuitBreakerManager) Reset(channelID int) {
	breaker := m.GetBreaker(channelID)
	breaker.Reset()
}

// ResetAll 重置所有熔断器
func (m *ChannelCircuitBreakerManager) ResetAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, breaker := range m.breakers {
		breaker.Reset()
	}
}

// GetAllStats 获取所有熔断器统计
func (m *ChannelCircuitBreakerManager) GetAllStats() []CircuitBreakerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	stats := make([]CircuitBreakerStats, 0, len(m.breakers))
	for _, breaker := range m.breakers {
		stats = append(stats, breaker.Stats())
	}
	return stats
}

// GetOpenBreakers 获取所有熔断中的渠道
func (m *ChannelCircuitBreakerManager) GetOpenBreakers() []int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var openChannels []int
	for channelID, breaker := range m.breakers {
		if breaker.IsOpen() {
			openChannels = append(openChannels, channelID)
		}
	}
	return openChannels
}

// =========================================
// 平台熔断器管理
// =========================================
// PlatformCircuitBreakerManager 平台熔断器管理器
type PlatformCircuitBreakerManager struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
	config   CircuitBreakerConfig
}

var (
	platformBreakerManager *PlatformCircuitBreakerManager
	platformBreakerOnce    sync.Once
)

// GetPlatformBreakerManager 获取平台熔断器管理器
func GetPlatformBreakerManager() *PlatformCircuitBreakerManager {
	platformBreakerOnce.Do(func() {
		platformBreakerManager = &PlatformCircuitBreakerManager{
			breakers: make(map[string]*CircuitBreaker),
			config:   DefaultCircuitBreakerConfig(),
		}
	})
	return platformBreakerManager
}

// GetBreaker 获取平台熔断器
func (m *PlatformCircuitBreakerManager) GetBreaker(platform string) *CircuitBreaker {
	m.mu.RLock()
	if breaker, ok := m.breakers[platform]; ok {
		m.mu.RUnlock()
		return breaker
	}
	m.mu.RUnlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if breaker, ok := m.breakers[platform]; ok {
		return breaker
	}
	name := fmt.Sprintf("platform_%s", platform)
	breaker := NewCircuitBreaker(name, m.config)
	m.breakers[platform] = breaker
	return breaker
}

// Allow 检查平台是否允许请求
func (m *PlatformCircuitBreakerManager) Allow(platform string) error {
	breaker := m.GetBreaker(platform)
	return breaker.Allow()
}

// RecordSuccess 记录平台请求成功
func (m *PlatformCircuitBreakerManager) RecordSuccess(platform string) {
	breaker := m.GetBreaker(platform)
	breaker.RecordSuccess()
}

// RecordFailure 记录平台请求失败
func (m *PlatformCircuitBreakerManager) RecordFailure(platform string) {
	breaker := m.GetBreaker(platform)
	breaker.RecordFailure()
}

// IsOpen 检查平台是否熔断
func (m *PlatformCircuitBreakerManager) IsOpen(platform string) bool {
	breaker := m.GetBreaker(platform)
	return breaker.IsOpen()
}

// GetAllStats 获取所有平台熔断器统计
func (m *PlatformCircuitBreakerManager) GetAllStats() []CircuitBreakerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	stats := make([]CircuitBreakerStats, 0, len(m.breakers))
	for _, breaker := range m.breakers {
		stats = append(stats, breaker.Stats())
	}
	return stats
}
