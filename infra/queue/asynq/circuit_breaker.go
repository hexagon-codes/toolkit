package asynq

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexagon-codes/toolkit/util/circuit"
)

// CircuitState 复用仓库统一的熔断状态定义。
type CircuitState = circuit.State

const (
	// StateClosed 表示正常放行请求。
	StateClosed = circuit.StateClosed
	// StateOpen 表示拒绝请求。
	StateOpen = circuit.StateOpen
	// StateHalfOpen 表示限量探测恢复状态。
	StateHalfOpen = circuit.StateHalfOpen
)

var (
	// ErrCircuitOpen 表示熔断器正在拒绝请求。
	ErrCircuitOpen = circuit.ErrCircuitOpen
	// ErrCircuitHalfOpen 表示半开探测请求已达到并发上限。
	ErrCircuitHalfOpen = circuit.ErrTooManyRequests
)

// CircuitBreakerConfig 定义 Asynq 上游调用的熔断策略。
type CircuitBreakerConfig struct {
	FailureThreshold    int
	SuccessThreshold    int
	Timeout             time.Duration
	HalfOpenMaxRequests int
	OnStateChange       func(name string, from, to CircuitState)
}

// DefaultCircuitBreakerConfig 返回默认熔断策略。
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:    5,
		SuccessThreshold:    2,
		Timeout:             30 * time.Second,
		HalfOpenMaxRequests: 3,
	}
}

// CircuitBreaker 为统一熔断器补充名称和空闲时间语义。
type CircuitBreaker struct {
	name        string
	breaker     *circuit.Breaker
	lastUsed    atomic.Int64
	lifecycleMu sync.Mutex
	inFlight    int
	retired     bool
}

// CircuitPermit 表示一次由 Asynq 熔断器跟踪的请求许可。
type CircuitPermit struct {
	permit    *circuit.Permit
	owner     *CircuitBreaker
	completed atomic.Bool
}

// NewCircuitBreaker 创建并校验熔断器。
func NewCircuitBreaker(name string, config CircuitBreakerConfig) (*CircuitBreaker, error) {
	if name == "" {
		return nil, errors.New("asynq: circuit breaker name must not be empty")
	}
	options := []circuit.Option{
		circuit.WithThreshold(config.FailureThreshold),
		circuit.WithSuccessThreshold(config.SuccessThreshold),
		circuit.WithTimeout(config.Timeout),
		circuit.WithHalfOpenMaxRequests(config.HalfOpenMaxRequests),
	}
	if config.OnStateChange != nil {
		options = append(options, circuit.WithOnStateChange(func(from, to circuit.State) {
			config.OnStateChange(name, from, to)
		}))
	}
	breaker, err := circuit.New(options...)
	if err != nil {
		return nil, fmt.Errorf("asynq: invalid circuit breaker config: %w", err)
	}
	result := &CircuitBreaker{name: name, breaker: breaker}
	result.touch()
	return result, nil
}

func (cb *CircuitBreaker) touch() {
	cb.lastUsed.Store(time.Now().UnixNano())
}

func (cb *CircuitBreaker) lastUsedAt() time.Time {
	return time.Unix(0, cb.lastUsed.Load())
}

// Acquire 获取一次请求许可。
func (cb *CircuitBreaker) Acquire() (*CircuitPermit, error) {
	if !cb.beginUse() {
		return nil, circuit.ErrBreakerClosed
	}
	permit, err := cb.breaker.Acquire()
	if err != nil {
		cb.endUse()
		return nil, err
	}
	return &CircuitPermit{permit: permit, owner: cb}, nil
}

// Execute 执行函数并自动完成请求许可。
func (cb *CircuitBreaker) Execute(run func() (any, error)) (result any, resultErr error) {
	if run == nil {
		return nil, errors.New("asynq: circuit breaker execute function must not be nil")
	}
	permit, err := cb.Acquire()
	if err != nil {
		return nil, err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = permit.Complete(errors.New("asynq: circuit breaker execution panicked")) //nolint:errcheck // 必须优先传播原始 panic。
			panic(recovered)
		}
		resultErr = errors.Join(resultErr, permit.Complete(resultErr))
	}()
	return run()
}

// Complete 提交许可结果并释放活动引用。
func (permit *CircuitPermit) Complete(resultErr error) error {
	if permit == nil || permit.permit == nil || permit.owner == nil {
		return errors.New("asynq: circuit permit must not be nil")
	}
	if !permit.completed.CompareAndSwap(false, true) {
		return circuit.ErrPermitCompleted
	}
	defer permit.owner.endUse()
	return permit.permit.Complete(resultErr)
}

func (cb *CircuitBreaker) beginUse() bool {
	cb.lifecycleMu.Lock()
	defer cb.lifecycleMu.Unlock()
	if cb.retired {
		return false
	}
	cb.inFlight++
	cb.touch()
	return true
}

func (cb *CircuitBreaker) endUse() {
	cb.lifecycleMu.Lock()
	cb.inFlight--
	cb.touch()
	cb.lifecycleMu.Unlock()
}

// State 返回当前熔断状态。
func (cb *CircuitBreaker) State() CircuitState {
	return cb.breaker.State()
}

// IsOpen 判断当前是否处于打开状态。
func (cb *CircuitBreaker) IsOpen() bool {
	return cb.State() == StateOpen
}

// Reset 重置熔断器。
func (cb *CircuitBreaker) Reset() error {
	if !cb.beginUse() {
		return circuit.ErrBreakerClosed
	}
	defer cb.endUse()
	return cb.breaker.Reset()
}

// Close 关闭熔断器；多次调用是安全的。
func (cb *CircuitBreaker) Close() {
	cb.lifecycleMu.Lock()
	if cb.retired {
		cb.lifecycleMu.Unlock()
		return
	}
	cb.retired = true
	cb.breaker.Close()
	cb.lifecycleMu.Unlock()
}

// retireIfIdle 仅在没有活动许可且超过空闲期限时关闭熔断器。
func (cb *CircuitBreaker) retireIfIdle(cutoff time.Time) bool {
	cb.lifecycleMu.Lock()
	defer cb.lifecycleMu.Unlock()
	if cb.retired || cb.inFlight != 0 || !cb.lastUsedAt().Before(cutoff) {
		return false
	}
	cb.retired = true
	cb.breaker.Close()
	return true
}

// CircuitBreakerStats 是熔断器的一致性统计快照。
type CircuitBreakerStats struct {
	Name             string    `json:"name"`
	State            string    `json:"state"`
	FailureCount     int       `json:"failure_count"`
	SuccessCount     int       `json:"success_count"`
	HalfOpenInFlight int       `json:"half_open_in_flight"`
	LastFailureTime  time.Time `json:"last_failure_time"`
	LastUsedTime     time.Time `json:"last_used_time"`
}

// Stats 返回一致性统计快照。
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	stats := cb.breaker.Stats()
	return CircuitBreakerStats{
		Name:             cb.name,
		State:            stats.State.String(),
		FailureCount:     stats.Failures,
		SuccessCount:     stats.Successes,
		HalfOpenInFlight: stats.HalfOpenInFlight,
		LastFailureTime:  stats.LastFailureAt,
		LastUsedTime:     cb.lastUsedAt(),
	}
}

type breakerStore[K comparable] struct {
	mu       sync.Mutex
	breakers map[K]*CircuitBreaker
	config   CircuitBreakerConfig
	name     func(K) string
	closed   bool
}

func newBreakerStore[K comparable](config CircuitBreakerConfig, name func(K) string) (*breakerStore[K], error) {
	if name == nil {
		return nil, errors.New("asynq: circuit breaker name function must not be nil")
	}
	probe, err := NewCircuitBreaker("config_validation", config)
	if err != nil {
		return nil, err
	}
	probe.Close()
	return &breakerStore[K]{
		breakers: make(map[K]*CircuitBreaker),
		config:   config,
		name:     name,
	}, nil
}

func (store *breakerStore[K]) get(key K) (*CircuitBreaker, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed {
		return nil, circuit.ErrBreakerClosed
	}
	if breaker, ok := store.breakers[key]; ok {
		return breaker, nil
	}
	breaker, err := NewCircuitBreaker(store.name(key), store.config)
	if err != nil {
		return nil, err
	}
	store.breakers[key] = breaker
	return breaker, nil
}

func (store *breakerStore[K]) snapshot() map[K]*CircuitBreaker {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make(map[K]*CircuitBreaker, len(store.breakers))
	for key, breaker := range store.breakers {
		result[key] = breaker
	}
	return result
}

func (store *breakerStore[K]) cleanupIdle(maxAge time.Duration) (int, error) {
	if maxAge <= 0 {
		return 0, errors.New("asynq: circuit breaker max idle age must be positive")
	}
	cutoff := time.Now().Add(-maxAge)
	store.mu.Lock()
	if store.closed {
		store.mu.Unlock()
		return 0, circuit.ErrBreakerClosed
	}
	removed := 0
	for key, breaker := range store.breakers {
		if breaker.retireIfIdle(cutoff) {
			delete(store.breakers, key)
			removed++
		}
	}
	store.mu.Unlock()
	return removed, nil
}

func (store *breakerStore[K]) resetAll() error {
	var resultErr error
	for _, breaker := range store.snapshot() {
		resultErr = errors.Join(resultErr, breaker.Reset())
	}
	return resultErr
}

func (store *breakerStore[K]) close() {
	store.mu.Lock()
	if store.closed {
		store.mu.Unlock()
		return
	}
	store.closed = true
	breakers := store.breakers
	store.breakers = nil
	store.mu.Unlock()
	for _, breaker := range breakers {
		breaker.Close()
	}
}

// ChannelCircuitBreakerManager 管理渠道级熔断器。
type ChannelCircuitBreakerManager struct {
	store *breakerStore[int]
}

// NewChannelCircuitBreakerManager 创建渠道熔断器管理器。
func NewChannelCircuitBreakerManager(config CircuitBreakerConfig) (*ChannelCircuitBreakerManager, error) {
	store, err := newBreakerStore(config, func(channelID int) string {
		return fmt.Sprintf("channel_%d", channelID)
	})
	if err != nil {
		return nil, err
	}
	return &ChannelCircuitBreakerManager{store: store}, nil
}

// GetBreaker 返回渠道熔断器，不存在时创建。
func (m *ChannelCircuitBreakerManager) GetBreaker(channelID int) (*CircuitBreaker, error) {
	return m.store.get(channelID)
}

// Acquire 获取渠道请求许可。
func (m *ChannelCircuitBreakerManager) Acquire(channelID int) (*CircuitPermit, error) {
	breaker, err := m.GetBreaker(channelID)
	if err != nil {
		return nil, err
	}
	return breaker.Acquire()
}

// Execute 执行渠道请求并自动完成许可。
func (m *ChannelCircuitBreakerManager) Execute(channelID int, run func() (any, error)) (any, error) {
	breaker, err := m.GetBreaker(channelID)
	if err != nil {
		return nil, err
	}
	return breaker.Execute(run)
}

// IsOpen 判断渠道是否处于打开状态。
func (m *ChannelCircuitBreakerManager) IsOpen(channelID int) (bool, error) {
	breaker, err := m.GetBreaker(channelID)
	if err != nil {
		return false, err
	}
	return breaker.IsOpen(), nil
}

// Reset 重置渠道熔断器。
func (m *ChannelCircuitBreakerManager) Reset(channelID int) error {
	breaker, err := m.GetBreaker(channelID)
	if err != nil {
		return err
	}
	return breaker.Reset()
}

// ResetAll 重置全部渠道熔断器。
func (m *ChannelCircuitBreakerManager) ResetAll() error {
	return m.store.resetAll()
}

// CleanupIdle 关闭并移除超时未使用的渠道熔断器。
func (m *ChannelCircuitBreakerManager) CleanupIdle(maxAge time.Duration) (int, error) {
	return m.store.cleanupIdle(maxAge)
}

// GetAllStats 返回按名称排序的渠道熔断统计。
func (m *ChannelCircuitBreakerManager) GetAllStats() []CircuitBreakerStats {
	breakers := m.store.snapshot()
	stats := make([]CircuitBreakerStats, 0, len(breakers))
	for _, breaker := range breakers {
		stats = append(stats, breaker.Stats())
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Name < stats[j].Name })
	return stats
}

// GetOpenBreakers 返回按编号排序的打开渠道。
func (m *ChannelCircuitBreakerManager) GetOpenBreakers() []int {
	openChannels := make([]int, 0)
	for channelID, breaker := range m.store.snapshot() {
		if breaker.IsOpen() {
			openChannels = append(openChannels, channelID)
		}
	}
	sort.Ints(openChannels)
	return openChannels
}

// Close 关闭管理器及全部渠道熔断器。
func (m *ChannelCircuitBreakerManager) Close() {
	m.store.close()
}

// PlatformCircuitBreakerManager 管理平台级熔断器。
type PlatformCircuitBreakerManager struct {
	store *breakerStore[string]
}

// NewPlatformCircuitBreakerManager 创建平台熔断器管理器。
func NewPlatformCircuitBreakerManager(config CircuitBreakerConfig) (*PlatformCircuitBreakerManager, error) {
	store, err := newBreakerStore(config, func(platform string) string {
		return "platform_" + platform
	})
	if err != nil {
		return nil, err
	}
	return &PlatformCircuitBreakerManager{store: store}, nil
}

// GetBreaker 返回平台熔断器，不存在时创建。
func (m *PlatformCircuitBreakerManager) GetBreaker(platform string) (*CircuitBreaker, error) {
	if platform == "" {
		return nil, errors.New("asynq: platform must not be empty")
	}
	return m.store.get(platform)
}

// Acquire 获取平台请求许可。
func (m *PlatformCircuitBreakerManager) Acquire(platform string) (*CircuitPermit, error) {
	breaker, err := m.GetBreaker(platform)
	if err != nil {
		return nil, err
	}
	return breaker.Acquire()
}

// Execute 执行平台请求并自动完成许可。
func (m *PlatformCircuitBreakerManager) Execute(platform string, run func() (any, error)) (any, error) {
	breaker, err := m.GetBreaker(platform)
	if err != nil {
		return nil, err
	}
	return breaker.Execute(run)
}

// IsOpen 判断平台是否处于打开状态。
func (m *PlatformCircuitBreakerManager) IsOpen(platform string) (bool, error) {
	breaker, err := m.GetBreaker(platform)
	if err != nil {
		return false, err
	}
	return breaker.IsOpen(), nil
}

// Reset 重置平台熔断器。
func (m *PlatformCircuitBreakerManager) Reset(platform string) error {
	breaker, err := m.GetBreaker(platform)
	if err != nil {
		return err
	}
	return breaker.Reset()
}

// ResetAll 重置全部平台熔断器。
func (m *PlatformCircuitBreakerManager) ResetAll() error {
	return m.store.resetAll()
}

// CleanupIdle 关闭并移除超时未使用的平台熔断器。
func (m *PlatformCircuitBreakerManager) CleanupIdle(maxAge time.Duration) (int, error) {
	return m.store.cleanupIdle(maxAge)
}

// GetAllStats 返回按名称排序的平台熔断统计。
func (m *PlatformCircuitBreakerManager) GetAllStats() []CircuitBreakerStats {
	breakers := m.store.snapshot()
	stats := make([]CircuitBreakerStats, 0, len(breakers))
	for _, breaker := range breakers {
		stats = append(stats, breaker.Stats())
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Name < stats[j].Name })
	return stats
}

// Close 关闭管理器及全部平台熔断器。
func (m *PlatformCircuitBreakerManager) Close() {
	m.store.close()
}
