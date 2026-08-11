package circuit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// State 表示熔断器状态。
type State int32

const (
	// StateClosed 表示正常放行请求。
	StateClosed State = iota
	// StateOpen 表示拒绝请求。
	StateOpen
	// StateHalfOpen 表示限量探测恢复状态。
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
	// ErrCircuitOpen 表示熔断器正在拒绝请求。
	ErrCircuitOpen = errors.New("circuit breaker is open")
	// ErrTooManyRequests 表示半开探测请求已达到并发上限。
	ErrTooManyRequests = errors.New("too many requests in half-open state")
	// ErrBreakerClosed 表示熔断器生命周期已经结束。
	ErrBreakerClosed = errors.New("circuit breaker is closed")
	// ErrPermitCompleted 表示请求许可已经提交过结果。
	ErrPermitCompleted = errors.New("circuit permit is already completed")
)

// Config 定义熔断策略。
type Config struct {
	// Threshold 是连续失败达到熔断状态所需的次数。
	Threshold int
	// Timeout 是打开状态持续到允许恢复探测的时间。
	Timeout time.Duration
	// HalfOpenMaxRequests 是半开状态允许的最大并发探测数。
	HalfOpenMaxRequests int
	// SuccessThreshold 是半开状态恢复关闭所需的连续成功次数。
	SuccessThreshold int
	// IsFailure 判断一次执行结果是否应计为失败。
	// 回调在状态锁外执行，可能被并发调用；panic 按失败处理。
	IsFailure func(error) bool
	// OnStateChange 在状态变化后于状态锁外执行。
	// 并发状态变化的回调可能重叠，监听器必须保证自身并发安全；panic 会被隔离。
	OnStateChange func(from, to State)
	// Now 提供当前时间，主要用于确定性测试。
	// 回调在状态锁外执行且可能并发调用；panic 会传播给当前调用方。
	Now func() time.Time
}

// Option 修改熔断策略。
type Option func(*Config)

// WithThreshold 设置连续失败阈值。
func WithThreshold(n int) Option {
	return func(config *Config) { config.Threshold = n }
}

// WithTimeout 设置打开状态持续时间。
func WithTimeout(duration time.Duration) Option {
	return func(config *Config) { config.Timeout = duration }
}

// WithHalfOpenMaxRequests 设置半开探测并发上限。
func WithHalfOpenMaxRequests(n int) Option {
	return func(config *Config) { config.HalfOpenMaxRequests = n }
}

// WithSuccessThreshold 设置恢复所需的连续成功次数。
func WithSuccessThreshold(n int) Option {
	return func(config *Config) { config.SuccessThreshold = n }
}

// WithIsFailure 设置失败分类函数。
func WithIsFailure(classify func(error) bool) Option {
	return func(config *Config) { config.IsFailure = classify }
}

// WithOnStateChange 设置状态变化回调。
func WithOnStateChange(callback func(from, to State)) Option {
	return func(config *Config) { config.OnStateChange = callback }
}

// WithNow 设置时钟函数。
func WithNow(now func() time.Time) Option {
	return func(config *Config) { config.Now = now }
}

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

func buildConfig(options ...Option) (Config, error) {
	config := defaultConfig()
	for index, option := range options {
		if option == nil {
			return Config{}, fmt.Errorf("circuit: option %d must not be nil", index)
		}
		option(&config)
	}
	switch {
	case config.Threshold <= 0:
		return Config{}, errors.New("circuit: threshold must be positive")
	case config.Timeout <= 0:
		return Config{}, errors.New("circuit: timeout must be positive")
	case config.HalfOpenMaxRequests <= 0:
		return Config{}, errors.New("circuit: half-open max requests must be positive")
	case config.SuccessThreshold <= 0:
		return Config{}, errors.New("circuit: success threshold must be positive")
	case config.IsFailure == nil:
		return Config{}, errors.New("circuit: failure classifier must not be nil")
	case config.Now == nil:
		return Config{}, errors.New("circuit: clock must not be nil")
	default:
		return config, nil
	}
}

// Breaker 通过状态代际隔离并发请求的完成结果。
type Breaker struct {
	config Config

	mu             sync.Mutex
	state          State
	generation     uint64
	failures       int
	successes      int
	halfOpenCount  int
	lastFailureAt  time.Time
	openedAt       time.Time
	stateListeners []func(from, to State)
	closed         atomic.Bool
}

// Permit 表示一次已获准执行的请求。
// 每个许可必须且只能调用一次 Complete。
// 值复制不会创建新许可，所有副本共享同一个完成状态。
type Permit struct {
	breaker    *Breaker
	generation uint64
	state      State
	completion *permitCompletion
}

// permitCompletion 在 Permit 被值复制后仍共享同一个完成状态。
type permitCompletion struct {
	completed atomic.Bool
}

type stateChangeEvent struct {
	from, to  State
	listeners []func(from, to State)
}

// New 创建并校验熔断器。
func New(options ...Option) (*Breaker, error) {
	config, err := buildConfig(options...)
	if err != nil {
		return nil, err
	}
	return newBreaker(config), nil
}

func newBreaker(config Config) *Breaker {
	breaker := &Breaker{config: config, state: StateClosed}
	if config.OnStateChange != nil {
		breaker.stateListeners = append(breaker.stateListeners, config.OnStateChange)
	}
	return breaker
}

// State 返回当前状态。
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// Acquire 获取一次请求许可。
func (b *Breaker) Acquire() (*Permit, error) {
	for {
		if b.closed.Load() {
			return nil, ErrBreakerClosed
		}

		b.mu.Lock()
		if b.closed.Load() {
			b.mu.Unlock()
			return nil, ErrBreakerClosed
		}
		if b.state != StateOpen {
			permit, err := b.acquireLocked()
			b.mu.Unlock()
			return permit, err
		}
		generation := b.generation
		openedAt := b.openedAt
		b.mu.Unlock()

		now := b.config.Now()

		var event *stateChangeEvent
		b.mu.Lock()
		if b.closed.Load() {
			b.mu.Unlock()
			return nil, ErrBreakerClosed
		}
		if b.state != StateOpen || b.generation != generation || !b.openedAt.Equal(openedAt) {
			b.mu.Unlock()
			continue
		}
		if now.Sub(b.openedAt) < b.config.Timeout {
			b.mu.Unlock()
			return nil, ErrCircuitOpen
		}
		event = b.transitionLocked(StateHalfOpen, time.Time{})
		permit, err := b.acquireLocked()
		b.mu.Unlock()
		invokeStateChange(event)
		return permit, err
	}
}

func (b *Breaker) acquireLocked() (*Permit, error) {
	if b.state == StateHalfOpen {
		if b.halfOpenCount >= b.config.HalfOpenMaxRequests {
			return nil, ErrTooManyRequests
		}
		b.halfOpenCount++
	}
	permit := &Permit{
		breaker:    b,
		generation: b.generation,
		state:      b.state,
		completion: &permitCompletion{},
	}
	return permit, nil
}

// Complete 提交本次请求结果。
func (p *Permit) Complete(resultErr error) error {
	if p == nil || p.breaker == nil || p.completion == nil {
		return errors.New("circuit: permit must not be nil")
	}
	if !p.completion.completed.CompareAndSwap(false, true) {
		return ErrPermitCompleted
	}
	return p.breaker.complete(p, resultErr)
}

func (b *Breaker) complete(permit *Permit, resultErr error) error {
	if b.closed.Load() {
		return ErrBreakerClosed
	}
	current, err := b.permitCurrent(permit)
	if err != nil {
		return err
	}
	if !current {
		return nil
	}
	isFailure := classifyFailure(b.config.IsFailure, resultErr)
	var failureAt time.Time
	if isFailure {
		current, err = b.permitCurrent(permit)
		if err != nil {
			return err
		}
		if !current {
			return nil
		}
		failureAt = b.config.Now()
	}

	var event *stateChangeEvent
	b.mu.Lock()
	if b.closed.Load() {
		b.mu.Unlock()
		return ErrBreakerClosed
	}
	if permit.generation != b.generation || permit.state != b.state {
		b.mu.Unlock()
		return nil
	}

	switch permit.state {
	case StateClosed:
		if isFailure {
			b.failures++
			b.recordFailureLocked(failureAt)
			if b.failures >= b.config.Threshold {
				event = b.transitionLocked(StateOpen, b.lastFailureAt)
			}
		} else {
			b.failures = 0
		}
	case StateHalfOpen:
		b.halfOpenCount--
		if isFailure {
			b.recordFailureLocked(failureAt)
			event = b.transitionLocked(StateOpen, b.lastFailureAt)
		} else {
			b.successes++
			if b.successes >= b.config.SuccessThreshold {
				event = b.transitionLocked(StateClosed, time.Time{})
			}
		}
	}
	b.mu.Unlock()
	invokeStateChange(event)
	return nil
}

func (b *Breaker) permitCurrent(permit *Permit) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed.Load() {
		return false, ErrBreakerClosed
	}
	return permit.generation == b.generation && permit.state == b.state, nil
}

func (b *Breaker) recordFailureLocked(failureAt time.Time) {
	if b.lastFailureAt.IsZero() || failureAt.After(b.lastFailureAt) {
		b.lastFailureAt = failureAt
	}
}

func classifyFailure(classify func(error) bool, resultErr error) (failure bool) {
	failure = true
	defer func() {
		if recover() != nil {
			failure = true
		}
	}()
	return classify(resultErr)
}

// Execute 执行函数并自动完成请求许可。
func (b *Breaker) Execute(run func() (any, error)) (result any, resultErr error) {
	if run == nil {
		return nil, errors.New("circuit: execute function must not be nil")
	}
	permit, err := b.Acquire()
	if err != nil {
		return nil, err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = permit.Complete(errors.New("circuit: execution panicked")) //nolint:errcheck // 必须优先传播原始 panic。
			panic(recovered)
		}
		resultErr = errors.Join(resultErr, permit.Complete(resultErr))
	}()
	return run()
}

// ExecuteContext 执行带上下文的函数并自动完成请求许可。
func (b *Breaker) ExecuteContext(
	ctx context.Context,
	run func(context.Context) (any, error),
) (result any, resultErr error) {
	if ctx == nil {
		return nil, errors.New("circuit: context must not be nil")
	}
	if run == nil {
		return nil, errors.New("circuit: execute function must not be nil")
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	permit, err := b.Acquire()
	if err != nil {
		return nil, err
	}
	if contextErr := ctx.Err(); contextErr != nil {
		if completeErr := permit.Complete(contextErr); completeErr != nil {
			return nil, errors.Join(contextErr, completeErr)
		}
		return nil, contextErr
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = permit.Complete(errors.New("circuit: execution panicked")) //nolint:errcheck // 必须优先传播原始 panic。
			panic(recovered)
		}
		resultErr = errors.Join(resultErr, permit.Complete(resultErr))
	}()
	return run(ctx)
}

func (b *Breaker) transitionLocked(to State, openedAt time.Time) *stateChangeEvent {
	from := b.state
	if from == to {
		return nil
	}
	b.state = to
	b.generation++
	switch to {
	case StateClosed:
		b.failures = 0
		b.successes = 0
		b.halfOpenCount = 0
		b.openedAt = time.Time{}
	case StateOpen:
		b.successes = 0
		b.halfOpenCount = 0
		b.openedAt = openedAt
	case StateHalfOpen:
		b.successes = 0
		b.halfOpenCount = 0
	}
	listeners := append([]func(from, to State){}, b.stateListeners...)
	return &stateChangeEvent{from: from, to: to, listeners: listeners}
}

func invokeStateChange(event *stateChangeEvent) {
	if event == nil {
		return
	}
	for _, listener := range event.listeners {
		func() {
			defer func() {
				if recover() != nil {
					return
				}
			}()
			listener(event.from, event.to)
		}()
	}
}

// Reset 重置熔断器和全部计数。
func (b *Breaker) Reset() error {
	if b.closed.Load() {
		return ErrBreakerClosed
	}
	b.mu.Lock()
	if b.closed.Load() {
		b.mu.Unlock()
		return ErrBreakerClosed
	}
	event := b.transitionLocked(StateClosed, time.Time{})
	if event == nil {
		b.generation++
		b.failures = 0
		b.successes = 0
		b.halfOpenCount = 0
	}
	b.lastFailureAt = time.Time{}
	b.openedAt = time.Time{}
	b.mu.Unlock()
	invokeStateChange(event)
	return nil
}

// OnStateChange 添加状态变化监听器。
func (b *Breaker) OnStateChange(listener func(from, to State)) error {
	if listener == nil {
		return errors.New("circuit: state listener must not be nil")
	}
	if b.closed.Load() {
		return ErrBreakerClosed
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed.Load() {
		return ErrBreakerClosed
	}
	b.stateListeners = append(b.stateListeners, listener)
	return nil
}

// Close 关闭熔断器；多次调用是安全的。
func (b *Breaker) Close() {
	if !b.closed.CompareAndSwap(false, true) {
		return
	}
	b.mu.Lock()
	b.generation++
	b.stateListeners = nil
	b.mu.Unlock()
}

// Stats 是熔断器的一致性统计快照。
type Stats struct {
	State            State
	Failures         int
	Successes        int
	HalfOpenInFlight int
	LastFailureAt    time.Time
	OpenedAt         time.Time
}

// Stats 返回一致性统计快照。
func (b *Breaker) Stats() Stats {
	b.mu.Lock()
	defer b.mu.Unlock()
	return Stats{
		State:            b.state,
		Failures:         b.failures,
		Successes:        b.successes,
		HalfOpenInFlight: b.halfOpenCount,
		LastFailureAt:    b.lastFailureAt,
		OpenedAt:         b.openedAt,
	}
}

// OpenAIConfig 是 OpenAI 调用的预设策略。
func OpenAIConfig() []Option {
	return []Option{
		WithThreshold(5),
		WithTimeout(60 * time.Second),
		WithHalfOpenMaxRequests(2),
		WithSuccessThreshold(2),
		WithIsFailure(IsRateLimitOrServerError),
	}
}

// ClaudeConfig 是 Claude 调用的预设策略。
func ClaudeConfig() []Option {
	return []Option{
		WithThreshold(3),
		WithTimeout(30 * time.Second),
		WithHalfOpenMaxRequests(1),
		WithSuccessThreshold(1),
		WithIsFailure(IsRateLimitOrServerError),
	}
}

// GeminiConfig 是 Gemini 调用的预设策略。
func GeminiConfig() []Option {
	return []Option{
		WithThreshold(5),
		WithTimeout(30 * time.Second),
		WithHalfOpenMaxRequests(2),
		WithSuccessThreshold(2),
		WithIsFailure(IsRateLimitOrServerError),
	}
}

// AggressiveConfig 是快速熔断预设策略。
func AggressiveConfig() []Option {
	return []Option{
		WithThreshold(3),
		WithTimeout(10 * time.Second),
		WithHalfOpenMaxRequests(1),
		WithSuccessThreshold(1),
	}
}

// ConservativeConfig 是慢速熔断预设策略。
func ConservativeConfig() []Option {
	return []Option{
		WithThreshold(10),
		WithTimeout(120 * time.Second),
		WithHalfOpenMaxRequests(5),
		WithSuccessThreshold(3),
	}
}

// NewAIBreaker 创建并校验 AI API 熔断器。
func NewAIBreaker(preset []Option, extra ...Option) (*Breaker, error) {
	options := make([]Option, 0, len(preset)+len(extra))
	options = append(options, preset...)
	options = append(options, extra...)
	return New(options...)
}

// HTTPError 表示携带 HTTP 状态码的错误。
type HTTPError interface {
	StatusCode() int
}

// IsRateLimitOrServerError 判断错误是否为限流或服务端错误。
func IsRateLimitOrServerError(err error) bool {
	if err == nil {
		return false
	}
	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		code := httpErr.StatusCode()
		return code == 429 || code >= 500 && code < 600
	}
	return true
}

// IsServerError 判断错误是否为服务端错误。
func IsServerError(err error) bool {
	if err == nil {
		return false
	}
	var httpErr HTTPError
	if !errors.As(err, &httpErr) {
		return false
	}
	code := httpErr.StatusCode()
	return code >= 500 && code < 600
}

// IsRateLimitError 判断错误是否为限流错误。
func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	var httpErr HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode() == 429
}

// BreakerManager 管理一组使用相同策略的熔断器。
type BreakerManager struct {
	mu       sync.Mutex
	breakers map[string]*Breaker
	config   Config
	closed   bool
}

// NewBreakerManager 创建并校验熔断器管理器。
func NewBreakerManager(options ...Option) (*BreakerManager, error) {
	config, err := buildConfig(options...)
	if err != nil {
		return nil, err
	}
	return &BreakerManager{breakers: make(map[string]*Breaker), config: config}, nil
}

// Get 返回指定名称的熔断器，不存在时创建。
func (m *BreakerManager) Get(name string) (*Breaker, error) {
	if name == "" {
		return nil, errors.New("circuit: breaker name must not be empty")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrBreakerClosed
	}
	if breaker, ok := m.breakers[name]; ok {
		return breaker, nil
	}
	breaker := newBreaker(m.config)
	m.breakers[name] = breaker
	return breaker, nil
}

// Execute 使用指定名称的熔断器执行函数。
func (m *BreakerManager) Execute(name string, run func() (any, error)) (any, error) {
	breaker, err := m.Get(name)
	if err != nil {
		return nil, err
	}
	return breaker.Execute(run)
}

// Reset 重置指定名称的熔断器。
func (m *BreakerManager) Reset(name string) error {
	breaker, err := m.Get(name)
	if err != nil {
		return err
	}
	return breaker.Reset()
}

// ResetAll 重置所有熔断器。
func (m *BreakerManager) ResetAll() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrBreakerClosed
	}
	breakers := make([]*Breaker, 0, len(m.breakers))
	for _, breaker := range m.breakers {
		breakers = append(breakers, breaker)
	}
	m.mu.Unlock()

	var resultErr error
	for _, breaker := range breakers {
		resultErr = errors.Join(resultErr, breaker.Reset())
	}
	return resultErr
}

// States 返回所有熔断器的状态快照。
func (m *BreakerManager) States() map[string]State {
	m.mu.Lock()
	breakers := make(map[string]*Breaker, len(m.breakers))
	for name, breaker := range m.breakers {
		breakers[name] = breaker
	}
	m.mu.Unlock()

	states := make(map[string]State, len(breakers))
	for name, breaker := range breakers {
		states[name] = breaker.State()
	}
	return states
}

// Close 关闭管理器及其全部熔断器。
func (m *BreakerManager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	breakers := m.breakers
	m.breakers = nil
	m.mu.Unlock()
	for _, breaker := range breakers {
		breaker.Close()
	}
}
