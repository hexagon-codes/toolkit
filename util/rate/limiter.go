package rate

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// Limiter 限流器接口
type Limiter interface {
	// Allow 判断是否允许通过
	Allow() bool
	// Wait 等待直到允许通过
	Wait() time.Duration
}

// TokenBucket 令牌桶限流器
type TokenBucket struct {
	capacity float64   // 桶容量
	tokens   float64   // 当前令牌数
	rate     float64   // 令牌生成速率（每秒）
	lastTime time.Time // 上次更新时间
	mu       sync.Mutex
}

// NewTokenBucket 创建令牌桶限流器
// capacity: 桶容量（最大令牌数）
// rate: 令牌生成速率（每秒生成多少个令牌）
func NewTokenBucket(capacity int, rate float64) (*TokenBucket, error) {
	if capacity <= 0 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidCapacity, capacity)
	}
	if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRate, rate)
	}

	return &TokenBucket{
		capacity: float64(capacity),
		tokens:   float64(capacity),
		rate:     rate,
		lastTime: time.Now(),
	}, nil
}

// MustNewTokenBucket 创建令牌桶；配置无效时 panic。
// 仅适用于参数由常量或已完成校验的配置提供的场景。
func MustNewTokenBucket(capacity int, rate float64) *TokenBucket {
	limiter, err := NewTokenBucket(capacity, rate)
	if err != nil {
		panic(err)
	}
	return limiter
}

// Allow 判断是否允许通过
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if !tb.validLocked() {
		return false
	}

	tb.refillAt(time.Now())

	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}

	return false
}

// Wait 等待直到允许通过，返回等待时间
// 注意：在高并发场景下，实际等待时间可能超过返回值
func (tb *TokenBucket) Wait() time.Duration {
	waited, err := tb.WaitContext(context.Background())
	if err != nil {
		panic(err)
	}
	return waited
}

// WaitContext 等待直到允许通过，并在 context 取消时停止等待。
func (tb *TokenBucket) WaitContext(ctx context.Context) (time.Duration, error) {
	if err := validateWaitContext(ctx); err != nil {
		return 0, err
	}
	started := time.Now()
	waited := false

	for {
		tb.mu.Lock()
		if !tb.validLocked() {
			tb.mu.Unlock()
			return elapsedWait(started, waited), ErrUninitializedLimiter
		}
		tb.refillAt(time.Now())

		if tb.tokens >= 1 {
			tb.tokens--
			tb.mu.Unlock()
			return elapsedWait(started, waited), nil
		}

		waitTime := tokenWaitDuration(1-tb.tokens, tb.rate)
		tb.mu.Unlock()

		waited = true
		if err := waitForContext(ctx, waitTime); err != nil {
			return elapsedWait(started, waited), err
		}
	}
}

// refillAt 补充令牌；时钟未前进时保持原状态，避免回拨产生负令牌。
func (tb *TokenBucket) refillAt(now time.Time) {
	if !now.After(tb.lastTime) {
		return
	}
	elapsed := now.Sub(tb.lastTime).Seconds()

	// 计算新增的令牌数
	newTokens := elapsed * tb.rate
	tb.tokens += newTokens

	// 令牌数不能超过容量
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}

	tb.lastTime = now
}

func (tb *TokenBucket) validLocked() bool {
	return tb.capacity > 0 && tb.rate > 0 && !math.IsNaN(tb.rate) && !math.IsInf(tb.rate, 0)
}

// LeakyBucket 漏桶限流器
type LeakyBucket struct {
	capacity     int           // 桶容量
	rate         time.Duration // 漏水速率（每次漏出的时间间隔）
	water        int           // 当前水量
	lastLeakTime time.Time     // 上次漏水时间
	mu           sync.Mutex
}

// NewLeakyBucket 创建漏桶限流器
// capacity: 桶容量
// rate: 漏水速率（例如：100ms 表示每100ms漏出一滴水）
func NewLeakyBucket(capacity int, rate time.Duration) (*LeakyBucket, error) {
	if capacity <= 0 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidCapacity, capacity)
	}
	if rate <= 0 {
		return nil, fmt.Errorf("%w: %s", ErrInvalidRate, rate)
	}

	return &LeakyBucket{
		capacity:     capacity,
		rate:         rate,
		water:        0,
		lastLeakTime: time.Now(),
	}, nil
}

// MustNewLeakyBucket 创建漏桶；配置无效时 panic。
// 仅适用于参数由常量或已完成校验的配置提供的场景。
func MustNewLeakyBucket(capacity int, rate time.Duration) *LeakyBucket {
	limiter, err := NewLeakyBucket(capacity, rate)
	if err != nil {
		panic(err)
	}
	return limiter
}

// Allow 判断是否允许通过
func (lb *LeakyBucket) Allow() bool {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	if !lb.validLocked() {
		return false
	}

	lb.leakAt(time.Now())

	if lb.water < lb.capacity {
		lb.water++
		return true
	}

	return false
}

// Wait 等待直到允许通过
// 注意：在高并发场景下，实际等待时间可能超过返回值
func (lb *LeakyBucket) Wait() time.Duration {
	waited, err := lb.WaitContext(context.Background())
	if err != nil {
		panic(err)
	}
	return waited
}

// WaitContext 等待直到允许通过，并在 context 取消时停止等待。
func (lb *LeakyBucket) WaitContext(ctx context.Context) (time.Duration, error) {
	if err := validateWaitContext(ctx); err != nil {
		return 0, err
	}
	started := time.Now()
	waited := false

	for {
		lb.mu.Lock()
		if !lb.validLocked() {
			lb.mu.Unlock()
			return elapsedWait(started, waited), ErrUninitializedLimiter
		}
		now := time.Now()
		lb.leakAt(now)

		if lb.water < lb.capacity {
			lb.water++
			lb.mu.Unlock()
			return elapsedWait(started, waited), nil
		}

		waitTime := lb.nextLeakWaitLocked(now)
		lb.mu.Unlock()

		waited = true
		if err := waitForContext(ctx, waitTime); err != nil {
			return elapsedWait(started, waited), err
		}
	}
}

// leakAt 按给定时刻漏水；时钟回拨时保持原状态。
func (lb *LeakyBucket) leakAt(now time.Time) {
	if !now.After(lb.lastLeakTime) {
		return
	}
	elapsed := now.Sub(lb.lastLeakTime)

	// 计算漏出的水量
	leaked := elapsed / lb.rate
	if leaked > 0 {
		if leaked >= time.Duration(lb.water) {
			lb.water = 0
		} else {
			lb.water -= int(leaked)
		}
		// 只推进已消耗的时间，保留余量避免精度丢失
		lb.lastLeakTime = lb.lastLeakTime.Add(leaked * lb.rate)
	}
}

func (lb *LeakyBucket) nextLeakWaitLocked(now time.Time) time.Duration {
	if !now.After(lb.lastLeakTime) {
		return lb.rate
	}
	remaining := lb.rate - now.Sub(lb.lastLeakTime)
	if remaining < time.Nanosecond {
		return time.Nanosecond
	}
	return remaining
}

func (lb *LeakyBucket) validLocked() bool {
	return lb.capacity > 0 && lb.rate > 0
}

// SlidingWindow 滑动窗口限流器
type SlidingWindow struct {
	capacity int           // 窗口容量（允许的最大请求数）
	window   time.Duration // 窗口大小
	requests []time.Time   // 请求时间戳
	mu       sync.Mutex
}

// NewSlidingWindow 创建滑动窗口限流器
// capacity: 窗口内允许的最大请求数
// window: 窗口大小（例如：1分钟）
func NewSlidingWindow(capacity int, window time.Duration) (*SlidingWindow, error) {
	if capacity <= 0 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidCapacity, capacity)
	}
	if window <= 0 {
		return nil, fmt.Errorf("%w: %s", ErrInvalidWindow, window)
	}

	return &SlidingWindow{
		capacity: capacity,
		window:   window,
		requests: make([]time.Time, 0, capacity),
	}, nil
}

// MustNewSlidingWindow 创建滑动窗口；配置无效时 panic。
// 仅适用于参数由常量或已完成校验的配置提供的场景。
func MustNewSlidingWindow(capacity int, window time.Duration) *SlidingWindow {
	limiter, err := NewSlidingWindow(capacity, window)
	if err != nil {
		panic(err)
	}
	return limiter
}

// Allow 判断是否允许通过
func (sw *SlidingWindow) Allow() bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if !sw.validLocked() {
		return false
	}

	now := time.Now()
	sw.cleanup(now)

	if len(sw.requests) < sw.capacity {
		sw.requests = append(sw.requests, now)
		return true
	}

	return false
}

// Wait 等待直到允许通过
// 注意：在高并发场景下，实际等待时间可能超过返回值
func (sw *SlidingWindow) Wait() time.Duration {
	waited, err := sw.WaitContext(context.Background())
	if err != nil {
		panic(err)
	}
	return waited
}

// WaitContext 等待直到允许通过，并在 context 取消时停止等待。
func (sw *SlidingWindow) WaitContext(ctx context.Context) (time.Duration, error) {
	if err := validateWaitContext(ctx); err != nil {
		return 0, err
	}
	started := time.Now()
	waited := false

	for {
		sw.mu.Lock()
		if !sw.validLocked() {
			sw.mu.Unlock()
			return elapsedWait(started, waited), ErrUninitializedLimiter
		}

		now := time.Now()
		sw.cleanup(now)

		if len(sw.requests) < sw.capacity {
			sw.requests = append(sw.requests, now)
			sw.mu.Unlock()
			return elapsedWait(started, waited), nil
		}

		// 计算需要等待的时间（等待最早的请求过期）
		// 安全检查：如果 requests 为空（理论上不应该发生），最小等待 1ms
		var waitTime time.Duration
		if len(sw.requests) > 0 {
			oldestRequest := sw.requests[0]
			elapsed := now.Sub(oldestRequest)
			if elapsed < 0 {
				waitTime = sw.window
			} else if elapsed < sw.window {
				waitTime = sw.window - elapsed
			}
		}
		// 确保 waitTime 为正数且至少 1ms，防止负数或零值导致的问题
		if waitTime < time.Millisecond {
			waitTime = time.Millisecond
		}
		sw.mu.Unlock()

		waited = true
		if err := waitForContext(ctx, waitTime); err != nil {
			return elapsedWait(started, waited), err
		}
	}
}

// cleanup 清理过期的请求
func (sw *SlidingWindow) cleanup(now time.Time) {
	cutoff := now.Add(-sw.window)

	// 找到第一个未过期的请求
	validIdx := len(sw.requests) // 默认全部过期
	for i, reqTime := range sw.requests {
		if reqTime.After(cutoff) {
			validIdx = i
			break
		}
	}

	// 如果有过期请求，原地移动避免分配新切片
	if validIdx > 0 {
		copy(sw.requests, sw.requests[validIdx:])
		// 清零剩余引用帮助 GC
		for i := len(sw.requests) - validIdx; i < len(sw.requests); i++ {
			sw.requests[i] = time.Time{}
		}
		sw.requests = sw.requests[:len(sw.requests)-validIdx]
	}
}

// Count 返回当前窗口内的请求数量
func (sw *SlidingWindow) Count() int {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if !sw.validLocked() {
		return 0
	}
	sw.cleanup(time.Now())
	return len(sw.requests)
}

// Record 记录一次请求，不检查是否超限
// 适用于只需要追踪请求数量而不需要限流的场景
// 为保证 Count 精确，当前窗口内的全部记录都会保留，内存占用与窗口内记录数成正比。
func (sw *SlidingWindow) Record() {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if !sw.validLocked() {
		return
	}
	now := time.Now()
	sw.cleanup(now)

	sw.requests = append(sw.requests, now)
}

// TryAllow 尝试允许请求通过，返回是否成功和当前请求数
// 适用于需要同时获取限流结果和当前状态的场景
func (sw *SlidingWindow) TryAllow() (allowed bool, count int) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if !sw.validLocked() {
		return false, 0
	}
	now := time.Now()
	sw.cleanup(now)

	count = len(sw.requests)
	if count < sw.capacity {
		sw.requests = append(sw.requests, now)
		return true, count + 1
	}

	return false, count
}

// Reset 重置滑动窗口
func (sw *SlidingWindow) Reset() {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.requests = sw.requests[:0]
}

// Capacity 返回窗口容量
func (sw *SlidingWindow) Capacity() int {
	return sw.capacity
}

// Window 返回窗口大小
func (sw *SlidingWindow) Window() time.Duration {
	return sw.window
}

func (sw *SlidingWindow) validLocked() bool {
	return sw.capacity > 0 && sw.window > 0
}

func validateWaitContext(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}
	return ctx.Err()
}

func waitForContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func elapsedWait(started time.Time, waited bool) time.Duration {
	if !waited {
		return 0
	}
	return time.Since(started)
}
