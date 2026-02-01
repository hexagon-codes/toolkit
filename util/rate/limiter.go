package rate

import (
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
func NewTokenBucket(capacity int, rate float64) *TokenBucket {
	return &TokenBucket{
		capacity: float64(capacity),
		tokens:   float64(capacity),
		rate:     rate,
		lastTime: time.Now(),
	}
}

// Allow 判断是否允许通过
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}

	return false
}

// Wait 等待直到允许通过，返回等待时间
// 注意：在高并发场景下，实际等待时间可能超过返回值
func (tb *TokenBucket) Wait() time.Duration {
	var totalWait time.Duration

	for {
		tb.mu.Lock()
		tb.refill()

		if tb.tokens >= 1 {
			tb.tokens--
			tb.mu.Unlock()
			return totalWait
		}

		// 计算需要等待的时间
		waitTime := time.Duration((1-tb.tokens)/tb.rate*1000) * time.Millisecond
		if waitTime < time.Millisecond {
			waitTime = time.Millisecond // 最小等待 1ms
		}
		tb.mu.Unlock()

		// 在锁外等待
		time.Sleep(waitTime)
		totalWait += waitTime
	}
}

// refill 补充令牌
func (tb *TokenBucket) refill() {
	now := time.Now()
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
func NewLeakyBucket(capacity int, rate time.Duration) *LeakyBucket {
	return &LeakyBucket{
		capacity:     capacity,
		rate:         rate,
		water:        0,
		lastLeakTime: time.Now(),
	}
}

// Allow 判断是否允许通过
func (lb *LeakyBucket) Allow() bool {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.leak()

	if lb.water < lb.capacity {
		lb.water++
		return true
	}

	return false
}

// Wait 等待直到允许通过
// 注意：在高并发场景下，实际等待时间可能超过返回值
func (lb *LeakyBucket) Wait() time.Duration {
	var totalWait time.Duration

	for {
		lb.mu.Lock()
		lb.leak()

		if lb.water < lb.capacity {
			lb.water++
			lb.mu.Unlock()
			return totalWait
		}

		// 需要等待
		waitTime := lb.rate
		lb.mu.Unlock()

		// 在锁外等待
		time.Sleep(waitTime)
		totalWait += waitTime
	}
}

// leak 漏水
func (lb *LeakyBucket) leak() {
	now := time.Now()
	elapsed := now.Sub(lb.lastLeakTime)

	// 计算漏出的水量
	leaked := int(elapsed / lb.rate)
	if leaked > 0 {
		lb.water -= leaked
		if lb.water < 0 {
			lb.water = 0
		}
		lb.lastLeakTime = now
	}
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
func NewSlidingWindow(capacity int, window time.Duration) *SlidingWindow {
	return &SlidingWindow{
		capacity: capacity,
		window:   window,
		requests: make([]time.Time, 0, capacity),
	}
}

// Allow 判断是否允许通过
func (sw *SlidingWindow) Allow() bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()

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
	var totalWait time.Duration

	for {
		sw.mu.Lock()

		now := time.Now()
		sw.cleanup(now)

		if len(sw.requests) < sw.capacity {
			sw.requests = append(sw.requests, now)
			sw.mu.Unlock()
			return totalWait
		}

		// 计算需要等待的时间（等待最早的请求过期）
		oldestRequest := sw.requests[0]
		waitTime := sw.window - now.Sub(oldestRequest)
		if waitTime < time.Millisecond {
			waitTime = time.Millisecond // 最小等待 1ms
		}
		sw.mu.Unlock()

		// 在锁外等待
		time.Sleep(waitTime)
		totalWait += waitTime
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

	// 如果有过期请求，重新分配切片避免内存泄漏
	if validIdx > 0 {
		remaining := len(sw.requests) - validIdx
		if remaining == 0 {
			sw.requests = sw.requests[:0]
		} else {
			// 使用 copy 避免底层数组保持对旧数据的引用
			newRequests := make([]time.Time, remaining, sw.capacity)
			copy(newRequests, sw.requests[validIdx:])
			sw.requests = newRequests
		}
	}
}

// Count 返回当前窗口内的请求数量
func (sw *SlidingWindow) Count() int {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	sw.cleanup(time.Now())
	return len(sw.requests)
}

// Record 记录一次请求，不检查是否超限
// 适用于只需要追踪请求数量而不需要限流的场景
func (sw *SlidingWindow) Record() {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()
	sw.cleanup(now)
	sw.requests = append(sw.requests, now)
}

// TryAllow 尝试允许请求通过，返回是否成功和当前请求数
// 适用于需要同时获取限流结果和当前状态的场景
func (sw *SlidingWindow) TryAllow() (allowed bool, count int) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

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
