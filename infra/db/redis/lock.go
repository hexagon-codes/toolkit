package redis

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"strings"
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
	if err := l.validateOperation(ctx, "acquire lock"); err != nil {
		return err
	}
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
	if err := l.validateOperation(ctx, "acquire lock with retry"); err != nil {
		return err
	}
	if maxRetries <= 0 {
		return fmt.Errorf("%w: maximum retry attempts must be positive", ErrInvalidConfig)
	}
	if retryInterval < 0 {
		return fmt.Errorf("%w: retry interval must not be negative", ErrInvalidConfig)
	}

	for i := 0; i < maxRetries; i++ {
		err := l.Acquire(ctx)
		if err == nil {
			return nil
		}

		if !errors.Is(err, ErrLockFailed) {
			return err
		}
		if i == maxRetries-1 {
			return ErrLockFailed
		}

		timer := time.NewTimer(retryInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
			// 继续重试
		}
	}

	return ErrLockFailed
}

// Release 释放锁
func (l *Lock) Release(ctx context.Context) error {
	if err := l.validateOperation(ctx, "release lock"); err != nil {
		return err
	}

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
	if err := l.validateOperation(ctx, "refresh lock"); err != nil {
		return err
	}

	// Lua 脚本确保只刷新自己持有的锁
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("pexpire", KEYS[1], ARGV[2])
		else
			return 0
		end
	`

	expireMs := l.expiration.Milliseconds()
	if expireMs == 0 {
		expireMs = 1
	}
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
	if err := l.validateOperation(ctx, "get lock TTL"); err != nil {
		return 0, err
	}

	ttl, err := l.client.TTL(ctx, l.key).Result()
	if err != nil {
		return 0, fmt.Errorf("get lock TTL: %w", err)
	}

	if ttl < 0 {
		return 0, ErrLockNotHeld
	}

	return ttl, nil
}

func (l *Lock) validateOperation(ctx context.Context, operation string) error {
	if ctx == nil {
		return fmt.Errorf("%w: %s requires a non-nil context", ErrInvalidContext, operation)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if l == nil || isNilUniversalClient(l.client) {
		return fmt.Errorf("%w: Redis client is required", ErrInvalidConfig)
	}
	if strings.TrimSpace(l.key) == "" {
		return fmt.Errorf("%w: lock key is required", ErrInvalidConfig)
	}
	if l.expiration <= 0 {
		return fmt.Errorf("%w: lock expiration must be positive", ErrInvalidConfig)
	}
	return nil
}

func isNilUniversalClient(client redis.UniversalClient) bool {
	if client == nil {
		return true
	}
	value := reflect.ValueOf(client)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
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
func WithLock(ctx context.Context, client redis.UniversalClient, key string, expiration time.Duration, fn func() error) (err error) {
	if fn == nil {
		return fmt.Errorf("%w: lock callback is required", ErrInvalidConfig)
	}
	lock := NewLock(client, key, expiration)

	// 获取锁前先完成所有输入校验，避免无效调用产生外部副作用。
	if acquireErr := lock.Acquire(ctx); acquireErr != nil {
		return acquireErr
	}

	// 确保释放锁
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if releaseErr := lock.Release(releaseCtx); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("release lock: %w", releaseErr))
		}
	}()

	// 执行函数
	return fn()
}
