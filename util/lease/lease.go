// Package lease 提供带 TTL 的分布式互斥租约与 fencing token。
//
// 租约 = 带过期时间的互斥锁：同一 key 同一时刻至多一个持有者。fencing token 是
// 单调递增序号，每次成功获取自增；下游受保护资源比较 token、拒绝来自过期持有者的
// 写入，解决脑裂问题——「持有者卡顿 → 租约过期 → 新持有者接管 → 旧持有者复活后仍
// 写入」时，旧持有者的 token 更小，会被拒绝。
//
// 本包提供进程内实现 MemoryLease（单进程串行/幂等场景，可注入时钟便于测试）；
// 跨副本场景由 Redis 等后端实现 Lease 接口后注入（SET NX PX 获取 + INCR 发 token），
// 具体实现依赖部署环境，不在本通用包内置以避免拖入存储依赖。
package lease

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// FencingToken 是单调递增的租约序号。值越大表示越新的持有者。
type FencingToken uint64

// Lease 是分布式互斥租约接口。
type Lease interface {
	// Acquire 尝试获取 key 的租约：未被持有（或已过期）时获取成功并返回单调 fencing token；
	// 仍被他人持有时返回 acquired=false。
	Acquire(ctx context.Context, key string, ttl time.Duration) (token FencingToken, acquired bool, err error)
	// Release 释放租约；token 与当前持有者不匹配时不释放（防止误释放他人租约）。
	Release(ctx context.Context, key string, token FencingToken) error
	// Refresh 续租延长 TTL；token 与当前持有者不匹配返回 ErrNotHolder。
	Refresh(ctx context.Context, key string, token FencingToken, ttl time.Duration) error
}

// ErrNotHolder 表示调用方持有的 token 不是该 key 的当前持有者。
var ErrNotHolder = &leaseError{"lease: token 非当前持有者"}

type leaseError struct{ msg string }

func (e *leaseError) Error() string { return e.msg }

// MemoryLease 是进程内租约实现：互斥 + TTL + 全局单调 fencing token。
//
// 适用单进程串行/幂等。线程安全。
type MemoryLease struct {
	mu      sync.Mutex
	holders map[string]holder
	counter atomic.Uint64
	clock   func() time.Time
}

type holder struct {
	token   FencingToken
	expires time.Time
}

// Option 配置 MemoryLease。
type Option func(*MemoryLease)

// WithClock 注入时钟（默认 time.Now），便于测试控制过期。
func WithClock(fn func() time.Time) Option {
	return func(l *MemoryLease) { l.clock = fn }
}

// NewMemoryLease 创建进程内租约。
func NewMemoryLease(opts ...Option) *MemoryLease {
	l := &MemoryLease{
		holders: make(map[string]holder),
		clock:   time.Now,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Acquire 获取租约。
func (l *MemoryLease) Acquire(_ context.Context, key string, ttl time.Duration) (FencingToken, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.clock()
	if h, ok := l.holders[key]; ok && now.Before(h.expires) {
		return 0, false, nil // 仍被持有
	}
	tok := FencingToken(l.counter.Add(1))
	l.holders[key] = holder{token: tok, expires: now.Add(ttl)}
	return tok, true, nil
}

// Release 释放租约（token 不匹配则不释放，返回 nil 表示无副作用）。
func (l *MemoryLease) Release(_ context.Context, key string, token FencingToken) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if h, ok := l.holders[key]; ok && h.token == token {
		delete(l.holders, key)
	}
	return nil
}

// Refresh 续租。
func (l *MemoryLease) Refresh(_ context.Context, key string, token FencingToken, ttl time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	h, ok := l.holders[key]
	if !ok || h.token != token {
		return ErrNotHolder
	}
	h.expires = l.clock().Add(ttl)
	l.holders[key] = h
	return nil
}

// 编译期断言：MemoryLease 满足 Lease 接口。
var _ Lease = (*MemoryLease)(nil)
