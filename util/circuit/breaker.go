package circuit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// State 熔断器状态
type State int32

const (
	// StateClosed 关闭状态（正常）
	StateClosed State = iota
	// StateOpen 打开状态（熔断）
	StateOpen
	// StateHalfOpen 半开状态（探测）
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

var (
	// ErrCircuitOpen 熔断器打开
	ErrCircuitOpen = errors.New("circuit breaker is open")
	// ErrTooManyRequests 半开状态下请求过多
	ErrTooManyRequests = errors.New("too many requests in half-open state")
)

// Config 熔断器配置
type Config struct {
	// Threshold 失败阈值，达到后触发熔断
	Threshold int
	// Timeout 熔断持续时间
	Timeout time.Duration
	// HalfOpenMaxRequests 半开状态下允许的最大请求数
	HalfOpenMaxRequests int
	// SuccessThreshold 半开状态下恢复所需的连续成功次数
	SuccessThreshold int
	// IsFailure 判断是否为失败（默认任何错误都是失败）
	IsFailure func(error) bool
	// OnStateChange 状态变更回调
	OnStateChange func(from, to State)
	// Now 时间函数（用于测试）
	Now func() time.Time
}

// Option 配置选项
type Option func(*Config)

// WithThreshold 设置失败阈值
func WithThreshold(n int) Option {
	return func(c *Config) { c.Threshold = n }
}

// WithTimeout 设置熔断超时时间
func WithTimeout(d time.Duration) Option {
	return func(c *Config) { c.Timeout = d }
}

// WithHalfOpenMaxRequests 设置半开状态最大请求数
func WithHalfOpenMaxRequests(n int) Option {
	return func(c *Config) { c.HalfOpenMaxRequests = n }
}

// WithSuccessThreshold 设置恢复所需成功次数
func WithSuccessThreshold(n int) Option {
	return func(c *Config) { c.SuccessThreshold = n }
}

// WithIsFailure 设置失败判断函数
func WithIsFailure(fn func(error) bool) Option {
	return func(c *Config) { c.IsFailure = fn }
}

// WithOnStateChange 设置状态变更回调
func WithOnStateChange(fn func(from, to State)) Option {
	return func(c *Config) { c.OnStateChange = fn }
}

// WithNow 设置时间函数
func WithNow(fn func() time.Time) Option {
	return func(c *Config) { c.Now = fn }
}

// defaultConfig 默认配置
func defaultConfig() Config {
	return Config{
		Threshold:           5,
		Timeout:             30 * time.Second,
		HalfOpenMaxRequests: 3,
		SuccessThreshold:    2,
		IsFailure: func(err error) bool {
			return err != nil
		},
		Now: time.Now,
	}
}

// Breaker 熔断器
type Breaker struct {
	config Config

	state         atomic.Int32
	failures      atomic.Int32
	successes     atomic.Int32
	halfOpenCount atomic.Int32
	lastFailureAt atomic.Int64
	openedAt      atomic.Int64

	mu             sync.Mutex
	stateListeners []func(from, to State)
}

// New 创建熔断器
func New(opts ...Option) *Breaker {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	b := &Breaker{
		config: cfg,
	}

	if cfg.OnStateChange != nil {
		b.stateListeners = append(b.stateListeners, cfg.OnStateChange)
	}

	return b
}

// State 返回当前状态
func (b *Breaker) State() State {
	return State(b.state.Load())
}

// Execute 执行函数
func (b *Breaker) Execute(fn func() (any, error)) (any, error) {
	if err := b.beforeExecute(); err != nil {
		return nil, err
	}

	result, err := fn()
	b.afterExecute(err)
	return result, err
}

// ExecuteContext 执行带上下文的函数
func (b *Breaker) ExecuteContext(ctx context.Context, fn func(context.Context) (any, error)) (any, error) {
	if err := b.beforeExecute(); err != nil {
		return nil, err
	}

	result, err := fn(ctx)
	b.afterExecute(err)
	return result, err
}

// Allow 检查是否允许请求通过
func (b *Breaker) Allow() error {
	return b.beforeExecute()
}

// Success 报告成功
func (b *Breaker) Success() {
	b.afterExecute(nil)
}

// Failure 报告失败
func (b *Breaker) Failure() {
	b.afterExecute(errors.New("manual failure"))
}

// beforeExecute 执行前检查
func (b *Breaker) beforeExecute() error {
	now := b.config.Now()

	for {
		state := b.State()

		switch state {
		case StateClosed:
			return nil

		case StateOpen:
			// 检查是否可以进入半开状态
			openedAt := time.Unix(0, b.openedAt.Load())
			if now.Sub(openedAt) >= b.config.Timeout {
				// 使用 CAS 确保只有一个 goroutine 成功转换状态
				if b.state.CompareAndSwap(int32(StateOpen), int32(StateHalfOpen)) {
					// 成功转换，重置计数器
					b.successes.Store(0)
					b.halfOpenCount.Store(0)
					// 通知监听器
					b.notifyStateChange(StateOpen, StateHalfOpen)
				}
				// 转换成功或已被其他 goroutine 转换，重新检查状态
				continue
			}
			return ErrCircuitOpen

		case StateHalfOpen:
			// 限制半开状态下的并发请求（使用 CAS 保证原子性）
			for {
				current := b.halfOpenCount.Load()
				if current >= int32(b.config.HalfOpenMaxRequests) {
					return ErrTooManyRequests
				}
				if b.halfOpenCount.CompareAndSwap(current, current+1) {
					return nil
				}
				// CAS 失败，重试
			}

		default:
			return ErrCircuitOpen
		}
	}
}

// afterExecute 执行后处理
func (b *Breaker) afterExecute(err error) {
	isFailure := b.config.IsFailure(err)
	now := b.config.Now()
	state := b.State()

	switch state {
	case StateClosed:
		if isFailure {
			failures := b.failures.Add(1)
			b.lastFailureAt.Store(now.UnixNano())
			if failures >= int32(b.config.Threshold) {
				b.transitionTo(StateOpen)
			}
		} else {
			// 成功时重置失败计数
			b.failures.Store(0)
		}

	case StateHalfOpen:
		b.halfOpenCount.Add(-1)
		if isFailure {
			// 失败，回到打开状态
			b.transitionTo(StateOpen)
		} else {
			successes := b.successes.Add(1)
			if successes >= int32(b.config.SuccessThreshold) {
				// 足够多的成功，恢复到关闭状态
				b.transitionTo(StateClosed)
			}
		}
	}
}

// transitionTo 状态转换（使用 CAS 保证原子性）
func (b *Breaker) transitionTo(to State) {
	for {
		from := State(b.state.Load())
		if from == to {
			return
		}

		// 使用 CAS 确保状态转换的原子性
		if !b.state.CompareAndSwap(int32(from), int32(to)) {
			// CAS 失败，状态已被其他 goroutine 改变，重试
			continue
		}

		// CAS 成功，更新相关状态
		switch to {
		case StateClosed:
			b.failures.Store(0)
			b.successes.Store(0)
			b.halfOpenCount.Store(0)
		case StateOpen:
			b.openedAt.Store(b.config.Now().UnixNano())
			b.successes.Store(0)
			b.halfOpenCount.Store(0)
		case StateHalfOpen:
			b.successes.Store(0)
			b.halfOpenCount.Store(0)
		}

		// 通知监听器
		b.notifyStateChange(from, to)
		return
	}
}

// notifyStateChange 通知状态变更监听器（异步执行，带 panic 保护）
func (b *Breaker) notifyStateChange(from, to State) {
	b.mu.Lock()
	listeners := make([]func(from, to State), len(b.stateListeners))
	copy(listeners, b.stateListeners)
	b.mu.Unlock()

	for _, listener := range listeners {
		listener := listener // 避免闭包问题
		go func() {
			defer func() {
				if r := recover(); r != nil {
					// 监听器 panic 不应影响熔断器正常工作
				}
			}()
			listener(from, to)
		}()
	}
}

// Reset 重置熔断器
func (b *Breaker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.state.Store(int32(StateClosed))
	b.failures.Store(0)
	b.successes.Store(0)
	b.halfOpenCount.Store(0)
	b.lastFailureAt.Store(0)
	b.openedAt.Store(0)
}

// OnStateChange 添加状态变更监听器
func (b *Breaker) OnStateChange(fn func(from, to State)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stateListeners = append(b.stateListeners, fn)
}

// Stats 统计信息
type Stats struct {
	State         State
	Failures      int
	Successes     int
	LastFailureAt time.Time
	OpenedAt      time.Time
}

// Stats 返回统计信息
func (b *Breaker) Stats() Stats {
	stats := Stats{
		State:     b.State(),
		Failures:  int(b.failures.Load()),
		Successes: int(b.successes.Load()),
	}
	// 只有在有实际时间值时才设置（避免返回 1970-01-01）
	if lastFailure := b.lastFailureAt.Load(); lastFailure > 0 {
		stats.LastFailureAt = time.Unix(0, lastFailure)
	}
	if opened := b.openedAt.Load(); opened > 0 {
		stats.OpenedAt = time.Unix(0, opened)
	}
	return stats
}

// ============== AI API 预设配置 ==============

// OpenAIConfig OpenAI 推荐配置
var OpenAIConfig = []Option{
	WithThreshold(5),
	WithTimeout(60 * time.Second),
	WithHalfOpenMaxRequests(2),
	WithSuccessThreshold(2),
	WithIsFailure(IsRateLimitOrServerError),
}

// ClaudeConfig Claude 推荐配置
var ClaudeConfig = []Option{
	WithThreshold(3),
	WithTimeout(30 * time.Second),
	WithHalfOpenMaxRequests(1),
	WithSuccessThreshold(2),
	WithIsFailure(IsRateLimitOrServerError),
}

// GeminiConfig Gemini 推荐配置
var GeminiConfig = []Option{
	WithThreshold(5),
	WithTimeout(30 * time.Second),
	WithHalfOpenMaxRequests(2),
	WithSuccessThreshold(2),
	WithIsFailure(IsRateLimitOrServerError),
}

// AggressiveConfig 激进配置（快速熔断）
var AggressiveConfig = []Option{
	WithThreshold(3),
	WithTimeout(10 * time.Second),
	WithHalfOpenMaxRequests(1),
	WithSuccessThreshold(1),
}

// ConservativeConfig 保守配置（慢速熔断）
var ConservativeConfig = []Option{
	WithThreshold(10),
	WithTimeout(120 * time.Second),
	WithHalfOpenMaxRequests(5),
	WithSuccessThreshold(3),
}

// NewAIBreaker 创建 AI API 专用熔断器
func NewAIBreaker(preset []Option, extra ...Option) *Breaker {
	opts := make([]Option, 0, len(preset)+len(extra))
	opts = append(opts, preset...)
	opts = append(opts, extra...)
	return New(opts...)
}

// ============== 错误判断函数 ==============

// HTTPError 带状态码的 HTTP 错误
type HTTPError interface {
	StatusCode() int
}

// IsRateLimitOrServerError 判断是否为限流或服务器错误
func IsRateLimitOrServerError(err error) bool {
	if err == nil {
		return false
	}

	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		code := httpErr.StatusCode()
		// 429 (Rate Limit) 或 5xx (Server Error)
		return code == 429 || code >= 500
	}

	// 非 HTTP 错误认为是失败
	return true
}

// IsServerError 判断是否为服务器错误（5xx）
func IsServerError(err error) bool {
	if err == nil {
		return false
	}

	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode() >= 500
	}

	return false
}

// IsRateLimitError 判断是否为限流错误（429）
func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode() == 429
	}

	return false
}

// ============== 多熔断器管理 ==============

// BreakerManager 熔断器管理器
type BreakerManager struct {
	breakers sync.Map
	factory  func() *Breaker
}

// NewBreakerManager 创建熔断器管理器
func NewBreakerManager(factory func() *Breaker) *BreakerManager {
	return &BreakerManager{
		factory: factory,
	}
}

// Get 获取指定名称的熔断器
func (m *BreakerManager) Get(name string) *Breaker {
	if b, ok := m.breakers.Load(name); ok {
		return b.(*Breaker)
	}

	newBreaker := m.factory()
	actual, _ := m.breakers.LoadOrStore(name, newBreaker)
	return actual.(*Breaker)
}

// Execute 使用指定名称的熔断器执行函数
func (m *BreakerManager) Execute(name string, fn func() (any, error)) (any, error) {
	return m.Get(name).Execute(fn)
}

// Reset 重置指定名称的熔断器
func (m *BreakerManager) Reset(name string) {
	if b, ok := m.breakers.Load(name); ok {
		b.(*Breaker).Reset()
	}
}

// ResetAll 重置所有熔断器
func (m *BreakerManager) ResetAll() {
	m.breakers.Range(func(key, value any) bool {
		value.(*Breaker).Reset()
		return true
	})
}

// States 返回所有熔断器状态
func (m *BreakerManager) States() map[string]State {
	states := make(map[string]State)
	m.breakers.Range(func(key, value any) bool {
		states[key.(string)] = value.(*Breaker).State()
		return true
	})
	return states
}
