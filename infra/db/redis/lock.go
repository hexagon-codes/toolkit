package redis

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	// ErrLockFailed 获取锁失败
	ErrLockFailed = errors.New("failed to acquire lock")

	// ErrLockNotHeld 未持有锁
	ErrLockNotHeld = errors.New("lock not held")
)

// Lock 分布式锁
type Lock struct {
	client     redis.UniversalClient
	key        string
	value      string
	expiration time.Duration
}

// NewLock 创建分布式锁
func NewLock(client redis.UniversalClient, key string, expiration time.Duration) *Lock {
	return &Lock{
		client:     client,
		key:        key,
		value:      generateLockValue(),
		expiration: expiration,
	}
}

// Acquire 获取锁
func (l *Lock) Acquire(ctx context.Context) error {
	ok, err := l.client.SetNX(ctx, l.key, l.value, l.expiration).Result()
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}

	if !ok {
		return ErrLockFailed
	}

	return nil
}

// AcquireWithRetry 带重试的获取锁
func (l *Lock) AcquireWithRetry(ctx context.Context, retryInterval time.Duration, maxRetries int) error {
	for i := 0; i < maxRetries; i++ {
		err := l.Acquire(ctx)
		if err == nil {
			return nil
		}

		if err != ErrLockFailed {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryInterval):
			// 继续重试
		}
	}

	return ErrLockFailed
}

// Release 释放锁
func (l *Lock) Release(ctx context.Context) error {
	// Lua 脚本确保只释放自己持有的锁
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`

	result, err := l.client.Eval(ctx, script, []string{l.key}, l.value).Result()
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	if result == int64(0) {
		return ErrLockNotHeld
	}

	return nil
}

// Refresh 刷新锁的过期时间
func (l *Lock) Refresh(ctx context.Context) error {
	// Lua 脚本确保只刷新自己持有的锁
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("pexpire", KEYS[1], ARGV[2])
		else
			return 0
		end
	`

	expireMs := l.expiration.Milliseconds()
	result, err := l.client.Eval(ctx, script, []string{l.key}, l.value, expireMs).Result()
	if err != nil {
		return fmt.Errorf("failed to refresh lock: %w", err)
	}

	if result == int64(0) {
		return ErrLockNotHeld
	}

	return nil
}

// TTL 获取锁的剩余时间
func (l *Lock) TTL(ctx context.Context) (time.Duration, error) {
	ttl, err := l.client.TTL(ctx, l.key).Result()
	if err != nil {
		return 0, err
	}

	if ttl < 0 {
		return 0, ErrLockNotHeld
	}

	return ttl, nil
}

// generateLockValue 生成锁的唯一值
func generateLockValue() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// 如果随机数生成失败，使用时间戳作为后备（极端情况）
		// 这种情况理论上不应该发生，但做好防御
		return fmt.Sprintf("lock-%d", time.Now().UnixNano())
	}
	return base64.URLEncoding.EncodeToString(b)
}

// WithLock 使用锁执行函数（自动获取和释放）
func WithLock(ctx context.Context, client redis.UniversalClient, key string, expiration time.Duration, fn func() error) error {
	lock := NewLock(client, key, expiration)

	// 获取锁
	if err := lock.Acquire(ctx); err != nil {
		return err
	}

	// 确保释放锁
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		lock.Release(releaseCtx)
	}()

	// 执行函数
	return fn()
}
