package redis

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func setupLockTest(t *testing.T) (*miniredis.Miniredis, *Client) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	cfg := DefaultConfig(mr.Addr())
	cfg.DialTimeout = 1 * time.Second

	client, err := New(cfg)
	if err != nil {
		mr.Close()
		t.Fatalf("failed to create redis client: %v", err)
	}

	return mr, client
}

func TestNewLock(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	key := "test-lock"
	expiration := 10 * time.Second

	lock := NewLock(client.UniversalClient, key, expiration)

	if lock == nil {
		t.Fatal("expected lock to be created")
	}

	if lock.key != key {
		t.Errorf("expected key %s, got %s", key, lock.key)
	}

	if lock.expiration != expiration {
		t.Errorf("expected expiration %v, got %v", expiration, lock.expiration)
	}

	if lock.value == "" {
		t.Error("expected lock value to be generated")
	}

	if lock.client == nil {
		t.Error("expected client to be set")
	}
}

func TestGenerateLockValue(t *testing.T) {
	value1 := generateLockValue()
	value2 := generateLockValue()

	if value1 == "" {
		t.Error("expected non-empty lock value")
	}

	if value1 == value2 {
		t.Error("expected different lock values for consecutive calls")
	}

	// 验证长度（base64 编码的 16 字节应该是 24 个字符左右）
	if len(value1) < 20 {
		t.Errorf("expected lock value length >= 20, got %d", len(value1))
	}
}

func TestLockAcquire(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	lock := NewLock(client.UniversalClient, "acquire-lock", 10*time.Second)

	// 第一次获取应该成功
	err := lock.Acquire(ctx)
	if err != nil {
		t.Fatalf("expected acquire to succeed, got error: %v", err)
	}

	// 验证锁存在
	val, err := client.Get(ctx, lock.key).Result()
	if err != nil {
		t.Fatalf("failed to get lock key: %v", err)
	}

	if val != lock.value {
		t.Errorf("expected lock value %s, got %s", lock.value, val)
	}
}

func TestLockAcquireFailure(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	key := "contested-lock"

	// 第一个锁获取成功
	lock1 := NewLock(client.UniversalClient, key, 10*time.Second)
	err := lock1.Acquire(ctx)
	if err != nil {
		t.Fatalf("expected first acquire to succeed, got error: %v", err)
	}

	// 第二个锁获取应该失败
	lock2 := NewLock(client.UniversalClient, key, 10*time.Second)
	err = lock2.Acquire(ctx)
	if err == nil {
		t.Error("expected second acquire to fail")
	}

	if err != ErrLockFailed {
		t.Errorf("expected ErrLockFailed, got %v", err)
	}
}

func TestLockAcquireWithRetry(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	key := "retry-lock"

	// 第一个锁获取成功，设置较短的过期时间
	lock1 := NewLock(client.UniversalClient, key, 50*time.Millisecond)
	err := lock1.Acquire(ctx)
	if err != nil {
		t.Fatalf("expected first acquire to succeed, got error: %v", err)
	}

	// 使用 miniredis 的 FastForward 来模拟时间流逝
	mr.FastForward(100 * time.Millisecond)

	// 第二个锁现在应该能获取
	lock2 := NewLock(client.UniversalClient, key, 10*time.Second)
	err = lock2.AcquireWithRetry(ctx, 10*time.Millisecond, 3)
	if err != nil {
		t.Errorf("expected acquire with retry to succeed, got error: %v", err)
	}
}

func TestLockAcquireWithRetryFailure(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	key := "retry-fail-lock"

	// 第一个锁获取成功，过期时间较长
	lock1 := NewLock(client.UniversalClient, key, 10*time.Second)
	err := lock1.Acquire(ctx)
	if err != nil {
		t.Fatalf("expected first acquire to succeed, got error: %v", err)
	}

	// 第二个锁重试应该失败
	lock2 := NewLock(client.UniversalClient, key, 10*time.Second)
	err = lock2.AcquireWithRetry(ctx, 10*time.Millisecond, 3)
	if err == nil {
		t.Error("expected acquire with retry to fail")
	}

	if err != ErrLockFailed {
		t.Errorf("expected ErrLockFailed, got %v", err)
	}
}

func TestLockAcquireWithRetryContextCanceled(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	key := "context-lock"

	// 第一个锁获取成功
	lock1 := NewLock(client.UniversalClient, key, 10*time.Second)
	err := lock1.Acquire(context.Background())
	if err != nil {
		t.Fatalf("expected first acquire to succeed, got error: %v", err)
	}

	// 创建可取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	// 第二个锁应该因为 context 取消而失败
	lock2 := NewLock(client.UniversalClient, key, 10*time.Second)
	err = lock2.AcquireWithRetry(ctx, 10*time.Millisecond, 10)
	if err == nil {
		t.Error("expected acquire to fail due to context cancellation")
	}

	// 错误可能是包装后的 context.Canceled
	if err != context.Canceled && err.Error() != "failed to acquire lock: context canceled" {
		t.Errorf("expected context cancellation error, got %v", err)
	}
}

func TestLockRelease(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	lock := NewLock(client.UniversalClient, "release-lock", 10*time.Second)

	// 获取锁
	err := lock.Acquire(ctx)
	if err != nil {
		t.Fatalf("expected acquire to succeed, got error: %v", err)
	}

	// 释放锁
	err = lock.Release(ctx)
	if err != nil {
		t.Fatalf("expected release to succeed, got error: %v", err)
	}

	// 验证锁已释放
	_, err = client.Get(ctx, lock.key).Result()
	if err == nil {
		t.Error("expected lock key to be deleted")
	}
}

func TestLockReleaseNotHeld(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	lock := NewLock(client.UniversalClient, "not-held-lock", 10*time.Second)

	// 未获取锁就释放
	err := lock.Release(ctx)
	if err == nil {
		t.Error("expected release to fail for unheld lock")
	}

	if err != ErrLockNotHeld {
		t.Errorf("expected ErrLockNotHeld, got %v", err)
	}
}

func TestLockReleaseWrongOwner(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	key := "owner-lock"

	// 第一个锁获取
	lock1 := NewLock(client.UniversalClient, key, 10*time.Second)
	err := lock1.Acquire(ctx)
	if err != nil {
		t.Fatalf("expected first acquire to succeed, got error: %v", err)
	}

	// 第二个锁尝试释放（不是拥有者）
	lock2 := NewLock(client.UniversalClient, key, 10*time.Second)
	err = lock2.Release(ctx)
	if err == nil {
		t.Error("expected release to fail for wrong owner")
	}

	if err != ErrLockNotHeld {
		t.Errorf("expected ErrLockNotHeld, got %v", err)
	}

	// 验证锁仍然存在
	val, err := client.Get(ctx, lock1.key).Result()
	if err != nil {
		t.Fatalf("failed to get lock key: %v", err)
	}

	if val != lock1.value {
		t.Errorf("expected lock value %s, got %s", lock1.value, val)
	}
}

func TestLockRefresh(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	lock := NewLock(client.UniversalClient, "refresh-lock", 10*time.Second)

	// 获取锁
	err := lock.Acquire(ctx)
	if err != nil {
		t.Fatalf("expected acquire to succeed, got error: %v", err)
	}

	// 使用 FastForward 模拟时间流逝
	mr.FastForward(2 * time.Second)

	// 获取初始 TTL（应该减少了）
	ttl1, err := client.TTL(ctx, lock.key).Result()
	if err != nil {
		t.Fatalf("failed to get TTL: %v", err)
	}

	// 刷新锁
	err = lock.Refresh(ctx)
	if err != nil {
		t.Fatalf("expected refresh to succeed, got error: %v", err)
	}

	// 获取刷新后的 TTL
	ttl2, err := client.TTL(ctx, lock.key).Result()
	if err != nil {
		t.Fatalf("failed to get TTL: %v", err)
	}

	// TTL 应该接近 10 秒（刷新后的完整过期时间）
	if ttl2 <= ttl1 {
		t.Errorf("expected TTL to increase after refresh, got %v -> %v", ttl1, ttl2)
	}
}

func TestLockRefreshNotHeld(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	lock := NewLock(client.UniversalClient, "refresh-not-held-lock", 10*time.Second)

	// 未获取锁就刷新
	err := lock.Refresh(ctx)
	if err == nil {
		t.Error("expected refresh to fail for unheld lock")
	}

	if err != ErrLockNotHeld {
		t.Errorf("expected ErrLockNotHeld, got %v", err)
	}
}

func TestLockRefreshWrongOwner(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	key := "refresh-owner-lock"

	// 第一个锁获取
	lock1 := NewLock(client.UniversalClient, key, 10*time.Second)
	err := lock1.Acquire(ctx)
	if err != nil {
		t.Fatalf("expected first acquire to succeed, got error: %v", err)
	}

	// 第二个锁尝试刷新（不是拥有者）
	lock2 := NewLock(client.UniversalClient, key, 10*time.Second)
	err = lock2.Refresh(ctx)
	if err == nil {
		t.Error("expected refresh to fail for wrong owner")
	}

	if err != ErrLockNotHeld {
		t.Errorf("expected ErrLockNotHeld, got %v", err)
	}
}

func TestLockTTL(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	expiration := 10 * time.Second
	lock := NewLock(client.UniversalClient, "ttl-lock", expiration)

	// 获取锁
	err := lock.Acquire(ctx)
	if err != nil {
		t.Fatalf("expected acquire to succeed, got error: %v", err)
	}

	// 获取 TTL
	ttl, err := lock.TTL(ctx)
	if err != nil {
		t.Fatalf("expected TTL to succeed, got error: %v", err)
	}

	if ttl <= 0 || ttl > expiration {
		t.Errorf("expected TTL between 0 and %v, got %v", expiration, ttl)
	}
}

func TestLockTTLNotHeld(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	lock := NewLock(client.UniversalClient, "ttl-not-held-lock", 10*time.Second)

	// 未获取锁就查询 TTL
	ttl, err := lock.TTL(ctx)
	if err == nil {
		t.Error("expected TTL to fail for unheld lock")
	}

	if err != ErrLockNotHeld {
		t.Errorf("expected ErrLockNotHeld, got %v", err)
	}

	if ttl != 0 {
		t.Errorf("expected TTL 0, got %v", ttl)
	}
}

func TestWithLock(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	key := "with-lock"
	expiration := 10 * time.Second

	executed := false

	// 使用 WithLock
	err := WithLock(ctx, client.UniversalClient, key, expiration, func() error {
		executed = true
		return nil
	})

	if err != nil {
		t.Fatalf("expected WithLock to succeed, got error: %v", err)
	}

	if !executed {
		t.Error("expected function to be executed")
	}

	// 验证锁已释放
	_, err = client.Get(ctx, key).Result()
	if err == nil {
		t.Error("expected lock to be released after WithLock")
	}
}

func TestWithLockFunctionError(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	key := "with-lock-error"
	expiration := 10 * time.Second

	expectedErr := ErrLockFailed

	// 函数返回错误
	err := WithLock(ctx, client.UniversalClient, key, expiration, func() error {
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}

	// 验证锁已释放
	_, err = client.Get(ctx, key).Result()
	if err == nil {
		t.Error("expected lock to be released after WithLock error")
	}
}

func TestWithLockAcquireFailure(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	key := "with-lock-acquire-fail"
	expiration := 10 * time.Second

	// 先获取锁
	lock1 := NewLock(client.UniversalClient, key, expiration)
	err := lock1.Acquire(ctx)
	if err != nil {
		t.Fatalf("expected first acquire to succeed, got error: %v", err)
	}

	executed := false

	// WithLock 应该失败
	err = WithLock(ctx, client.UniversalClient, key, expiration, func() error {
		executed = true
		return nil
	})

	if err == nil {
		t.Error("expected WithLock to fail due to lock acquisition failure")
	}

	if executed {
		t.Error("expected function not to be executed")
	}
}

func TestLockConcurrency(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	key := "concurrent-lock"
	expiration := 100 * time.Millisecond

	var counter int
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 启动多个 goroutine 竞争锁
	goroutines := 10
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			lock := NewLock(client.UniversalClient, key, expiration)
			err := lock.AcquireWithRetry(ctx, 10*time.Millisecond, 20)
			if err != nil {
				return
			}

			// 临界区
			mu.Lock()
			counter++
			mu.Unlock()

			lock.Release(ctx)
		}()
	}

	wg.Wait()

	// 验证所有 goroutine 都成功执行
	if counter != goroutines {
		t.Errorf("expected counter %d, got %d", goroutines, counter)
	}
}

func TestLockExpiration(t *testing.T) {
	mr, client := setupLockTest(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	key := "expiration-lock"
	expiration := 2 * time.Second

	// 获取锁
	lock := NewLock(client.UniversalClient, key, expiration)
	err := lock.Acquire(ctx)
	if err != nil {
		t.Fatalf("expected acquire to succeed, got error: %v", err)
	}

	// 使用 FastForward 模拟时间流逝
	mr.FastForward(3 * time.Second)

	// 验证锁已过期
	_, err = client.Get(ctx, lock.key).Result()
	if err == nil {
		t.Error("expected lock to be expired")
	}

	// 另一个锁应该能获取
	lock2 := NewLock(client.UniversalClient, key, expiration)
	err = lock2.Acquire(ctx)
	if err != nil {
		t.Fatalf("expected second acquire to succeed after expiration, got error: %v", err)
	}
}
