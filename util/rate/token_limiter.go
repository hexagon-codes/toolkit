package rate

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// LimiterV2 是支持批量令牌与 Context 等待的限流器接口。
type LimiterV2 interface {
	Limiter
	// AllowN 判断是否允许 n 个请求通过。
	AllowN(n int) bool
	// WaitN 等待直到 n 个请求允许通过。
	WaitN(ctx context.Context, n int) error
	// Available 返回当前可用的令牌数。
	Available() int
}

var limiterTransactionSequence atomic.Uint64

// transactionalLimiter 定义多维限流事务所需的包内锁协议。
// 使用私有方法可以确保不支持同一事务协议的外部实现不会被误判为原子实现。
type transactionalLimiter interface {
	LimiterV2
	transactionKey() uint64
	lockTransaction()
	unlockTransaction()
	canAllowNLocked(n int) bool
	consumeNLocked(n int)
	availableLocked() int
	maxTransactionN() int
}

// TokenRateLimiter 同时限制每分钟 Token 数和请求数。
type TokenRateLimiter struct {
	tokensPerMinute   int64
	requestsPerMinute int64
	tokenBucket       *TokenBucketV2
	requestBucket     *TokenBucketV2
	transactionID     uint64
	mu                sync.Mutex
}

// NewTokenRateLimiter 创建 Token 限流器。
func NewTokenRateLimiter(tokensPerMinute, requestsPerMinute int) (*TokenRateLimiter, error) {
	if tokensPerMinute <= 0 {
		return nil, fmt.Errorf("%w: tokens per minute=%d", ErrInvalidCapacity, tokensPerMinute)
	}
	if requestsPerMinute <= 0 {
		return nil, fmt.Errorf("%w: requests per minute=%d", ErrInvalidCapacity, requestsPerMinute)
	}
	return newTokenRateLimiter(tokensPerMinute, requestsPerMinute), nil
}

// newTokenRateLimiter 仅供已通过静态常量或公开构造器校验的调用路径使用。
func newTokenRateLimiter(tokensPerMinute, requestsPerMinute int) *TokenRateLimiter {
	return &TokenRateLimiter{
		tokensPerMinute:   int64(tokensPerMinute),
		requestsPerMinute: int64(requestsPerMinute),
		tokenBucket:       newTokenBucketV2(tokensPerMinute, float64(tokensPerMinute)/60),
		requestBucket:     newTokenBucketV2(requestsPerMinute, float64(requestsPerMinute)/60),
		transactionID:     limiterTransactionSequence.Add(1),
	}
}

// Allow 检查是否允许一个消耗一个 Token 的请求。
func (l *TokenRateLimiter) Allow() bool {
	return l.AllowN(1)
}

// AllowN 原子检查并消费一个请求配额和 n 个 Token 配额。
func (l *TokenRateLimiter) AllowN(tokens int) bool {
	if tokens <= 0 {
		return false
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.validLocked() {
		return false
	}
	if !l.canAllowNLocked(tokens) {
		return false
	}
	l.consumeNLocked(tokens)
	return true
}

// Wait 等待直到允许一个请求，并返回实际等待时间。
func (l *TokenRateLimiter) Wait() time.Duration {
	start := time.Now()
	if err := l.WaitN(context.Background(), 1); err != nil {
		return 0
	}
	return time.Since(start)
}

// WaitN 等待直到两个桶都具备足够配额。
func (l *TokenRateLimiter) WaitN(ctx context.Context, tokens int) error {
	if ctx == nil {
		return ErrNilContext
	}
	if tokens <= 0 {
		return ErrInvalidTokenCount
	}
	l.mu.Lock()
	if !l.validLocked() {
		l.mu.Unlock()
		return ErrUninitializedLimiter
	}
	capacity := l.maxTransactionN()
	l.mu.Unlock()
	if tokens > capacity {
		return fmt.Errorf("%w: requested=%d capacity=%d", ErrInsufficientTokens, tokens, capacity)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if l.AllowN(tokens) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Stats 返回限流器统计快照。
func (l *TokenRateLimiter) Stats() TokenLimiterStats {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.validLocked() {
		return TokenLimiterStats{}
	}
	return TokenLimiterStats{
		TokensAvailable:   l.tokenBucket.availableLocked(),
		RequestsAvailable: l.requestBucket.availableLocked(),
		TokensPerMinute:   l.tokensPerMinute,
		RequestsPerMinute: l.requestsPerMinute,
	}
}

// Available 返回两个维度当前可用数量的较小值。
func (l *TokenRateLimiter) Available() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.validLocked() {
		return 0
	}
	return l.availableLocked()
}

func (l *TokenRateLimiter) transactionKey() uint64 { return l.transactionID }

func (l *TokenRateLimiter) lockTransaction() { l.mu.Lock() }

func (l *TokenRateLimiter) unlockTransaction() { l.mu.Unlock() }

func (l *TokenRateLimiter) canAllowNLocked(n int) bool {
	return l.validLocked() && n > 0 && l.requestBucket.canAllowNLocked(1) && l.tokenBucket.canAllowNLocked(n)
}

func (l *TokenRateLimiter) consumeNLocked(n int) {
	l.requestBucket.consumeNLocked(1)
	l.tokenBucket.consumeNLocked(n)
}

func (l *TokenRateLimiter) availableLocked() int {
	if !l.validLocked() {
		return 0
	}
	tokenAvailable := l.tokenBucket.availableLocked()
	requestAvailable := l.requestBucket.availableLocked()
	if tokenAvailable < requestAvailable {
		return tokenAvailable
	}
	return requestAvailable
}

func (l *TokenRateLimiter) maxTransactionN() int {
	if l.tokenBucket == nil {
		return 0
	}
	return l.tokenBucket.capacity
}

func (l *TokenRateLimiter) validLocked() bool {
	return l.tokenBucket != nil && l.requestBucket != nil &&
		l.tokensPerMinute > 0 && l.requestsPerMinute > 0 &&
		l.tokenBucket.validLocked() && l.requestBucket.validLocked()
}

// TokenLimiterStats 是 Token 限流器的统计快照。
type TokenLimiterStats struct {
	TokensAvailable   int
	RequestsAvailable int
	TokensPerMinute   int64
	RequestsPerMinute int64
}

// TokenBucketV2 是支持批量令牌和 Context 等待的令牌桶。
type TokenBucketV2 struct {
	capacity      int
	tokens        float64
	rate          float64
	lastTime      time.Time
	transactionID uint64
	mu            sync.Mutex
}

// NewTokenBucketV2 创建增强版令牌桶。
func NewTokenBucketV2(capacity int, rate float64) (*TokenBucketV2, error) {
	if capacity <= 0 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidCapacity, capacity)
	}
	if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRate, rate)
	}
	return newTokenBucketV2(capacity, rate), nil
}

func newTokenBucketV2(capacity int, rate float64) *TokenBucketV2 {
	return &TokenBucketV2{
		capacity:      capacity,
		tokens:        float64(capacity),
		rate:          rate,
		lastTime:      time.Now(),
		transactionID: limiterTransactionSequence.Add(1),
	}
}

// Allow 判断是否允许一个请求通过。
func (tb *TokenBucketV2) Allow() bool {
	return tb.AllowN(1)
}

// AllowN 原子检查并消费 n 个令牌。
func (tb *TokenBucketV2) AllowN(n int) bool {
	if n <= 0 {
		return false
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()
	if !tb.validLocked() {
		return false
	}
	if !tb.canAllowNLocked(n) {
		return false
	}
	tb.consumeNLocked(n)
	return true
}

// Wait 等待直到允许一个请求，并返回实际等待时间。
func (tb *TokenBucketV2) Wait() time.Duration {
	start := time.Now()
	if err := tb.WaitN(context.Background(), 1); err != nil {
		return 0
	}
	return time.Since(start)
}

// WaitN 等待直到 n 个令牌可用。
func (tb *TokenBucketV2) WaitN(ctx context.Context, n int) error {
	if ctx == nil {
		return ErrNilContext
	}
	if n <= 0 {
		return ErrInvalidTokenCount
	}
	tb.mu.Lock()
	if !tb.validLocked() {
		tb.mu.Unlock()
		return ErrUninitializedLimiter
	}
	capacity := tb.capacity
	tb.mu.Unlock()
	if n > capacity {
		return fmt.Errorf("%w: requested=%d capacity=%d", ErrInsufficientTokens, n, capacity)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	for {
		tb.mu.Lock()
		if tb.canAllowNLocked(n) {
			tb.consumeNLocked(n)
			tb.mu.Unlock()
			return nil
		}
		waitTime := tokenWaitDuration(float64(n)-tb.tokens, tb.rate)
		tb.mu.Unlock()

		if err := waitForContext(ctx, waitTime); err != nil {
			return err
		}
	}
}

// Available 返回当前可用的完整令牌数。
func (tb *TokenBucketV2) Available() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if !tb.validLocked() {
		return 0
	}
	return tb.availableLocked()
}

func (tb *TokenBucketV2) transactionKey() uint64 { return tb.transactionID }

func (tb *TokenBucketV2) lockTransaction() { tb.mu.Lock() }

func (tb *TokenBucketV2) unlockTransaction() { tb.mu.Unlock() }

func (tb *TokenBucketV2) canAllowNLocked(n int) bool {
	if !tb.validLocked() {
		return false
	}
	tb.refillLocked()
	return n > 0 && tb.tokens >= float64(n)
}

func (tb *TokenBucketV2) consumeNLocked(n int) {
	tb.tokens -= float64(n)
}

func (tb *TokenBucketV2) availableLocked() int {
	if !tb.validLocked() {
		return 0
	}
	tb.refillLocked()
	return int(tb.tokens)
}

func (tb *TokenBucketV2) maxTransactionN() int { return tb.capacity }

// refillLocked 根据经过时间补充令牌，调用方必须持有事务锁。
func (tb *TokenBucketV2) refillLocked() {
	now := time.Now()
	if !now.After(tb.lastTime) {
		return
	}
	elapsed := now.Sub(tb.lastTime).Seconds()
	tb.tokens += elapsed * tb.rate
	if tb.tokens > float64(tb.capacity) {
		tb.tokens = float64(tb.capacity)
	}
	tb.lastTime = now
}

func (tb *TokenBucketV2) validLocked() bool {
	return tb.capacity > 0 && tb.rate > 0 && !math.IsNaN(tb.rate) && !math.IsInf(tb.rate, 0)
}

func tokenWaitDuration(needed, rate float64) time.Duration {
	nanoseconds := needed / rate * float64(time.Second)
	if math.IsInf(nanoseconds, 1) || nanoseconds >= float64(math.MaxInt64) {
		return time.Duration(math.MaxInt64)
	}
	waitTime := time.Duration(math.Ceil(nanoseconds))
	if waitTime < time.Nanosecond {
		return time.Nanosecond
	}
	return waitTime
}

// NewOpenAIGPT4Limiter 创建 OpenAI GPT-4 预设限流器。
func NewOpenAIGPT4Limiter() *TokenRateLimiter { return newTokenRateLimiter(10000, 500) }

// NewOpenAIGPT4oLimiter 创建 OpenAI GPT-4o 预设限流器。
func NewOpenAIGPT4oLimiter() *TokenRateLimiter { return newTokenRateLimiter(30000, 500) }

// NewOpenAIGPT4oMiniLimiter 创建 OpenAI GPT-4o-mini 预设限流器。
func NewOpenAIGPT4oMiniLimiter() *TokenRateLimiter { return newTokenRateLimiter(200000, 500) }

// NewClaudeSonnetLimiter 创建 Claude Sonnet 预设限流器。
func NewClaudeSonnetLimiter() *TokenRateLimiter { return newTokenRateLimiter(40000, 1000) }

// NewClaudeHaikuLimiter 创建 Claude Haiku 预设限流器。
func NewClaudeHaikuLimiter() *TokenRateLimiter { return newTokenRateLimiter(100000, 2000) }

// NewDeepSeekLimiter 创建 DeepSeek 预设限流器。
func NewDeepSeekLimiter() *TokenRateLimiter { return newTokenRateLimiter(60000, 1000) }

// NewQwenLimiter 创建通义千问预设限流器。
func NewQwenLimiter() *TokenRateLimiter { return newTokenRateLimiter(100000, 1000) }

// MultiDimensionLimiter 以一个原子事务同时限制多个维度。
type MultiDimensionLimiter struct {
	limiters []transactionalLimiter
}

// NewMultiDimensionLimiter 创建多维限流器。
func NewMultiDimensionLimiter(limiters ...LimiterV2) (*MultiDimensionLimiter, error) {
	if len(limiters) == 0 {
		return nil, ErrNoLimiters
	}

	transactional := make([]transactionalLimiter, 0, len(limiters))
	seen := make(map[uint64]struct{}, len(limiters))
	for index, limiter := range limiters {
		if isNilLimiter(limiter) {
			return nil, fmt.Errorf("%w: index=%d", ErrNilLimiter, index)
		}
		atomicLimiter, ok := limiter.(transactionalLimiter)
		if !ok {
			return nil, fmt.Errorf("%w: index=%d type=%T", ErrUnsupportedLimiter, index, limiter)
		}
		if atomicLimiter.maxTransactionN() <= 0 {
			return nil, fmt.Errorf("%w: index=%d", ErrUninitializedLimiter, index)
		}
		key := atomicLimiter.transactionKey()
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("%w: index=%d", ErrDuplicateLimiter, index)
		}
		seen[key] = struct{}{}
		transactional = append(transactional, atomicLimiter)
	}

	sort.Slice(transactional, func(i, j int) bool {
		return transactional[i].transactionKey() < transactional[j].transactionKey()
	})
	return &MultiDimensionLimiter{limiters: transactional}, nil
}

func isNilLimiter(limiter LimiterV2) bool {
	if limiter == nil {
		return true
	}
	value := reflect.ValueOf(limiter)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// Allow 判断所有维度是否都允许一个令牌。
func (m *MultiDimensionLimiter) Allow() bool {
	return m.AllowN(1)
}

// AllowN 在同一个临界区内检查并消费所有维度。
func (m *MultiDimensionLimiter) AllowN(n int) bool {
	if n <= 0 || len(m.limiters) == 0 {
		return false
	}
	m.lockAll()
	defer m.unlockAll()
	for _, limiter := range m.limiters {
		if n > limiter.maxTransactionN() || !limiter.canAllowNLocked(n) {
			return false
		}
	}
	for _, limiter := range m.limiters {
		limiter.consumeNLocked(n)
	}
	return true
}

// Wait 等待所有维度允许一个令牌，并返回实际等待时间。
func (m *MultiDimensionLimiter) Wait() time.Duration {
	start := time.Now()
	if err := m.WaitN(context.Background(), 1); err != nil {
		return 0
	}
	return time.Since(start)
}

// WaitN 等待所有维度都允许 n 个令牌。
func (m *MultiDimensionLimiter) WaitN(ctx context.Context, n int) error {
	if ctx == nil {
		return ErrNilContext
	}
	if n <= 0 {
		return ErrInvalidTokenCount
	}
	if len(m.limiters) == 0 {
		return ErrUninitializedLimiter
	}
	for _, limiter := range m.limiters {
		if n > limiter.maxTransactionN() {
			return fmt.Errorf("%w: requested=%d capacity=%d", ErrInsufficientTokens, n, limiter.maxTransactionN())
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if m.AllowN(n) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Available 返回事务快照中最紧维度的可用令牌数。
func (m *MultiDimensionLimiter) Available() int {
	if len(m.limiters) == 0 {
		return 0
	}
	m.lockAll()
	defer m.unlockAll()
	available := math.MaxInt
	for _, limiter := range m.limiters {
		if current := limiter.availableLocked(); current < available {
			available = current
		}
	}
	return available
}

func (m *MultiDimensionLimiter) lockAll() {
	for _, limiter := range m.limiters {
		limiter.lockTransaction()
	}
}

func (m *MultiDimensionLimiter) unlockAll() {
	for index := len(m.limiters) - 1; index >= 0; index-- {
		m.limiters[index].unlockTransaction()
	}
}
