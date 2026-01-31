package rate

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrRateLimitExceeded 超过速率限制
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
	// ErrInsufficientTokens Token 不足
	ErrInsufficientTokens = errors.New("insufficient tokens")
)

// LimiterV2 增强版限流器接口
type LimiterV2 interface {
	Limiter
	// AllowN 判断是否允许 n 个请求通过
	AllowN(n int) bool
	// WaitN 等待直到 n 个请求允许通过
	WaitN(ctx context.Context, n int) error
	// Available 返回当前可用的令牌数
	Available() int
}

// TokenRateLimiter AI API 专用限流器
// 同时支持 TPM (Tokens Per Minute) 和 RPM (Requests Per Minute) 限制
type TokenRateLimiter struct {
	tokensPerMinute   int64
	requestsPerMinute int64

	tokenBucket   *TokenBucketV2
	requestBucket *TokenBucketV2

	mu sync.Mutex
}

// NewTokenRateLimiter 创建 Token 限流器
// tokensPerMinute: 每分钟允许的最大 token 数 (TPM)
// requestsPerMinute: 每分钟允许的最大请求数 (RPM)
func NewTokenRateLimiter(tokensPerMinute, requestsPerMinute int) *TokenRateLimiter {
	return &TokenRateLimiter{
		tokensPerMinute:   int64(tokensPerMinute),
		requestsPerMinute: int64(requestsPerMinute),
		tokenBucket:       NewTokenBucketV2(tokensPerMinute, float64(tokensPerMinute)/60.0),
		requestBucket:     NewTokenBucketV2(requestsPerMinute, float64(requestsPerMinute)/60.0),
	}
}

// Allow 检查是否允许 1 个请求（消耗 1 个 token）
func (l *TokenRateLimiter) Allow() bool {
	return l.AllowN(1)
}

// AllowN 检查是否允许消耗 n 个 token 的请求
func (l *TokenRateLimiter) AllowN(tokens int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 检查请求数
	if !l.requestBucket.Allow() {
		return false
	}

	// 检查 token 数
	return l.tokenBucket.AllowN(tokens)
}

// Wait 等待直到允许 1 个请求
func (l *TokenRateLimiter) Wait() time.Duration {
	ctx := context.Background()
	start := time.Now()
	_ = l.WaitN(ctx, 1)
	return time.Since(start)
}

// WaitN 等待直到有足够的 token 配额
func (l *TokenRateLimiter) WaitN(ctx context.Context, tokens int) error {
	for {
		if l.AllowN(tokens) {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
			// 短暂等待后重试
		}
	}
}

// Reserve 预留 token（返回需要等待的时间）
func (l *TokenRateLimiter) Reserve(tokens int) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 计算需要等待的时间
	tokenWait := l.tokenBucket.ReserveN(tokens)
	requestWait := l.requestBucket.Reserve()

	if tokenWait > requestWait {
		return tokenWait
	}
	return requestWait
}

// Stats 返回限流器统计信息
func (l *TokenRateLimiter) Stats() TokenLimiterStats {
	l.mu.Lock()
	defer l.mu.Unlock()

	return TokenLimiterStats{
		TokensAvailable:   l.tokenBucket.Available(),
		RequestsAvailable: l.requestBucket.Available(),
		TokensPerMinute:   l.tokensPerMinute,
		RequestsPerMinute: l.requestsPerMinute,
	}
}

// Available 返回当前可用的令牌数（取两个桶的最小值）
func (l *TokenRateLimiter) Available() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	tokenAvail := l.tokenBucket.Available()
	reqAvail := l.requestBucket.Available()
	if tokenAvail < reqAvail {
		return tokenAvail
	}
	return reqAvail
}

// TryAllowN 检查是否有足够令牌但不消费（实现 atomicAllower 接口）
func (l *TokenRateLimiter) TryAllowN(n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 检查请求数桶（1 个请求）
	if !l.requestBucket.tryAllowN(1) {
		return false
	}
	// 检查 token 数桶
	return l.tokenBucket.tryAllowN(n)
}

// ConsumeN 消费 n 个令牌（实现 atomicAllower 接口）
// 注意：调用前应先调用 TryAllowN 确认有足够令牌
func (l *TokenRateLimiter) ConsumeN(n int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.requestBucket.consumeN(1) // 消费 1 个请求配额
	l.tokenBucket.consumeN(n)   // 消费 n 个 token 配额
}

// TokenLimiterStats 限流器统计信息
type TokenLimiterStats struct {
	TokensAvailable   int
	RequestsAvailable int
	TokensPerMinute   int64
	RequestsPerMinute int64
}

// ============== TokenBucketV2 增强版令牌桶 ==============

// TokenBucketV2 增强版令牌桶，支持 AllowN 和 Context
type TokenBucketV2 struct {
	capacity float64
	tokens   float64
	rate     float64
	lastTime time.Time
	mu       sync.Mutex
}

// NewTokenBucketV2 创建增强版令牌桶
func NewTokenBucketV2(capacity int, rate float64) *TokenBucketV2 {
	return &TokenBucketV2{
		capacity: float64(capacity),
		tokens:   float64(capacity),
		rate:     rate,
		lastTime: time.Now(),
	}
}

// Allow 判断是否允许 1 个请求通过
func (tb *TokenBucketV2) Allow() bool {
	return tb.AllowN(1)
}

// AllowN 判断是否允许 n 个请求通过
func (tb *TokenBucketV2) AllowN(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= float64(n) {
		tb.tokens -= float64(n)
		return true
	}

	return false
}

// Wait 等待直到允许 1 个请求通过
func (tb *TokenBucketV2) Wait() time.Duration {
	ctx := context.Background()
	start := time.Now()
	_ = tb.WaitN(ctx, 1)
	return time.Since(start)
}

// WaitN 等待直到允许 n 个请求通过
func (tb *TokenBucketV2) WaitN(ctx context.Context, n int) error {
	for {
		tb.mu.Lock()
		tb.refill()

		if tb.tokens >= float64(n) {
			tb.tokens -= float64(n)
			tb.mu.Unlock()
			return nil
		}

		// 计算需要等待的时间
		needed := float64(n) - tb.tokens
		waitTime := time.Duration(needed/tb.rate*1000) * time.Millisecond
		tb.mu.Unlock()

		// 等待
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
		}
	}
}

// Reserve 预留 1 个 token，返回需要等待的时间
func (tb *TokenBucketV2) Reserve() time.Duration {
	return tb.ReserveN(1)
}

// ReserveN 预留 n 个 token，返回需要等待的时间
func (tb *TokenBucketV2) ReserveN(n int) time.Duration {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= float64(n) {
		tb.tokens -= float64(n)
		return 0
	}

	needed := float64(n) - tb.tokens
	waitTime := time.Duration(needed/tb.rate*1000) * time.Millisecond
	tb.tokens = 0

	return waitTime
}

// Available 返回当前可用的 token 数
func (tb *TokenBucketV2) Available() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()
	return int(tb.tokens)
}

// availableNoLock 返回当前可用的 token 数（不加锁，内部使用）
// 调用者必须持有 tb.mu 锁
func (tb *TokenBucketV2) availableNoLock() int {
	tb.refill()
	return int(tb.tokens)
}

// tryAllowN 检查是否有足够的令牌但不消费（不加锁，内部使用）
// 调用者必须持有 tb.mu 锁
func (tb *TokenBucketV2) tryAllowN(n int) bool {
	tb.refill()
	return tb.tokens >= float64(n)
}

// consumeN 消费 n 个令牌（不加锁，不检查，内部使用）
// 调用者必须持有 tb.mu 锁，并确保已经通过 tryAllowN 检查
func (tb *TokenBucketV2) consumeN(n int) {
	tb.tokens -= float64(n)
}

// TryAllowN 检查是否有足够令牌但不消费（实现 atomicAllower 接口）
func (tb *TokenBucketV2) TryAllowN(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.tryAllowN(n)
}

// ConsumeN 消费 n 个令牌（实现 atomicAllower 接口）
// 注意：调用前应先调用 TryAllowN 确认有足够令牌
func (tb *TokenBucketV2) ConsumeN(n int) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.consumeN(n)
}

// refill 补充令牌
func (tb *TokenBucketV2) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastTime).Seconds()

	newTokens := elapsed * tb.rate
	tb.tokens += newTokens

	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}

	tb.lastTime = now
}

// ============== AI API 预设限流器 ==============

// OpenAI GPT-4 限制
// Tier 1: 10,000 TPM, 500 RPM
func NewOpenAIGPT4Limiter() *TokenRateLimiter {
	return NewTokenRateLimiter(10000, 500)
}

// OpenAI GPT-4o 限制
// Tier 1: 30,000 TPM, 500 RPM
func NewOpenAIGPT4oLimiter() *TokenRateLimiter {
	return NewTokenRateLimiter(30000, 500)
}

// OpenAI GPT-4o-mini 限制
// Tier 1: 200,000 TPM, 500 RPM
func NewOpenAIGPT4oMiniLimiter() *TokenRateLimiter {
	return NewTokenRateLimiter(200000, 500)
}

// Claude 3.5 Sonnet 限制
// 约 40,000 TPM, 1000 RPM
func NewClaudeSonnetLimiter() *TokenRateLimiter {
	return NewTokenRateLimiter(40000, 1000)
}

// Claude 3.5 Haiku 限制
// 约 100,000 TPM, 2000 RPM
func NewClaudeHaikuLimiter() *TokenRateLimiter {
	return NewTokenRateLimiter(100000, 2000)
}

// DeepSeek 限制
func NewDeepSeekLimiter() *TokenRateLimiter {
	return NewTokenRateLimiter(60000, 1000)
}

// 通义千问限制
func NewQwenLimiter() *TokenRateLimiter {
	return NewTokenRateLimiter(100000, 1000)
}

// ============== 多维度限流器 ==============

// atomicAllower 支持原子检查和消费的限流器接口（内部使用）
// 实现此接口的限流器可以在 MultiDimensionLimiter 中获得更好的原子性保证
type atomicAllower interface {
	// TryAllowN 检查是否有足够令牌但不消费，返回 true 表示可以通过
	TryAllowN(n int) bool
	// ConsumeN 消费 n 个令牌（调用者需确保已通过 TryAllowN 检查）
	ConsumeN(n int)
}

// MultiDimensionLimiter 多维度限流器
// 可以同时限制多个维度，例如：用户级 + 全局级
type MultiDimensionLimiter struct {
	limiters []LimiterV2
	mu       sync.Mutex
}

// NewMultiDimensionLimiter 创建多维度限流器
func NewMultiDimensionLimiter(limiters ...LimiterV2) *MultiDimensionLimiter {
	return &MultiDimensionLimiter{
		limiters: limiters,
	}
}

// Allow 所有维度都允许才通过
func (m *MultiDimensionLimiter) Allow() bool {
	return m.AllowN(1)
}

// AllowN 所有维度都允许 n 个才通过
// 使用两阶段检查：先检查所有限流器是否有足够的令牌，再实际消费
// 如果限流器实现了 atomicAllower 接口，则使用原子操作保证一致性
func (m *MultiDimensionLimiter) AllowN(n int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否所有限流器都支持原子操作
	atomicLimiters := make([]atomicAllower, 0, len(m.limiters))
	allAtomic := true
	for _, limiter := range m.limiters {
		if al, ok := limiter.(atomicAllower); ok {
			atomicLimiters = append(atomicLimiters, al)
		} else {
			allAtomic = false
			break
		}
	}

	if allAtomic && len(atomicLimiters) == len(m.limiters) {
		// 使用原子操作：先检查所有，再消费所有
		for _, al := range atomicLimiters {
			if !al.TryAllowN(n) {
				return false
			}
		}
		for _, al := range atomicLimiters {
			al.ConsumeN(n)
		}
		return true
	}

	// 回退到标准两阶段检查
	// 第一阶段：检查所有限流器是否有足够令牌（不消费）
	for _, limiter := range m.limiters {
		if limiter.Available() < n {
			return false
		}
	}

	// 第二阶段：所有检查通过后，实际消费令牌
	for _, limiter := range m.limiters {
		limiter.AllowN(n)
	}

	return true
}

// Wait 等待所有维度都允许
func (m *MultiDimensionLimiter) Wait() time.Duration {
	ctx := context.Background()
	start := time.Now()
	_ = m.WaitN(ctx, 1)
	return time.Since(start)
}

// WaitN 等待所有维度都允许 n 个
func (m *MultiDimensionLimiter) WaitN(ctx context.Context, n int) error {
	for {
		if m.AllowN(n) {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}
