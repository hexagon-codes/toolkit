package local

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func TestCache_GetOrLoad_Basic(t *testing.T) {
	cache := NewCache(100)
	defer cache.Stop()

	ctx := context.Background()
	loadCount := 0

	// 第一次加载：缓存未命中
	var user1 User
	err := cache.GetOrLoad(ctx, "user:1", 10*time.Minute, &user1, func(ctx context.Context) (any, error) {
		loadCount++
		return User{ID: 1, Name: "Alice"}, nil
	})
	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}
	if user1.ID != 1 || user1.Name != "Alice" {
		t.Errorf("unexpected user: %+v", user1)
	}
	if loadCount != 1 {
		t.Errorf("expected loadCount=1, got %d", loadCount)
	}

	// 第二次加载：缓存命中
	var user2 User
	err = cache.GetOrLoad(ctx, "user:1", 10*time.Minute, &user2, func(ctx context.Context) (any, error) {
		loadCount++
		return User{ID: 1, Name: "Bob"}, nil // 不应该被调用
	})
	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}
	if user2.ID != 1 || user2.Name != "Alice" {
		t.Errorf("unexpected user from cache: %+v", user2)
	}
	if loadCount != 1 {
		t.Errorf("expected loadCount=1 (cache hit), got %d", loadCount)
	}
}

func TestCache_GetOrLoad_NotFound(t *testing.T) {
	cache := NewCache(100)
	defer cache.Stop()

	ctx := context.Background()

	// 第一次加载：返回 NotFound
	var user User
	err := cache.GetOrLoad(ctx, "user:999", 10*time.Minute, &user, func(ctx context.Context) (any, error) {
		return nil, ErrNotFound
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}

	// 第二次加载：负缓存命中
	err = cache.GetOrLoad(ctx, "user:999", 10*time.Minute, &user, func(ctx context.Context) (any, error) {
		t.Error("loader should not be called (negative cache hit)")
		return nil, errors.New("should not reach here")
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound from negative cache, got: %v", err)
	}
}

func TestCache_GetOrLoad_InvalidParams(t *testing.T) {
	cache := NewCache(100)
	defer cache.Stop()

	ctx := context.Background()

	// 测试 nil dest
	err := cache.GetOrLoad(ctx, "key", time.Minute, nil, func(ctx context.Context) (any, error) {
		return "value", nil
	})
	if !errors.Is(err, ErrInvalidDest) {
		t.Errorf("expected ErrInvalidDest for nil dest, got: %v", err)
	}

	// 测试非指针 dest
	var s string
	err = cache.GetOrLoad(ctx, "key", time.Minute, s, func(ctx context.Context) (any, error) {
		return "value", nil
	})
	if !errors.Is(err, ErrInvalidDest) {
		t.Errorf("expected ErrInvalidDest for non-pointer dest, got: %v", err)
	}

	// 测试空 key
	var dest string
	err = cache.GetOrLoad(ctx, "", time.Minute, &dest, func(ctx context.Context) (any, error) {
		return "value", nil
	})
	if !errors.Is(err, ErrInvalidKey) {
		t.Errorf("expected ErrInvalidKey, got: %v", err)
	}

	// 测试 nil loader
	err = cache.GetOrLoad(ctx, "key", time.Minute, &dest, nil)
	if !errors.Is(err, ErrInvalidLoader) {
		t.Errorf("expected ErrInvalidLoader, got: %v", err)
	}
}

func TestCache_Del(t *testing.T) {
	cache := NewCache(100)
	defer cache.Stop()

	ctx := context.Background()

	// 写入缓存
	var user User
	cache.GetOrLoad(ctx, "user:1", 10*time.Minute, &user, func(ctx context.Context) (any, error) {
		return User{ID: 1, Name: "Alice"}, nil
	})

	// 删除
	err := cache.Del(ctx, "user:1")
	if err != nil {
		t.Fatalf("Del failed: %v", err)
	}

	// 验证删除：应该重新加载
	loadCount := 0
	err = cache.GetOrLoad(ctx, "user:1", 10*time.Minute, &user, func(ctx context.Context) (any, error) {
		loadCount++
		return User{ID: 1, Name: "Bob"}, nil
	})
	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}
	if loadCount != 1 {
		t.Errorf("expected loadCount=1 after Del, got %d", loadCount)
	}
	if user.Name != "Bob" {
		t.Errorf("expected name=Bob, got %s", user.Name)
	}
}

func TestCache_Del_Multiple(t *testing.T) {
	cache := NewCache(100)
	defer cache.Stop()

	ctx := context.Background()

	// 写入多个缓存
	for i := 1; i <= 3; i++ {
		var user User
		cache.GetOrLoad(ctx, "user:"+string(rune('0'+i)), 10*time.Minute, &user, func(ctx context.Context) (any, error) {
			return User{ID: i, Name: "User"}, nil
		})
	}

	// 批量删除
	err := cache.Del(ctx, "user:1", "user:2", "")
	if err != nil {
		t.Fatalf("Del failed: %v", err)
	}

	// Del with no keys
	err = cache.Del(ctx)
	if err != nil {
		t.Errorf("Del with no keys should not error: %v", err)
	}
}

func TestCache_TTL_Expiration(t *testing.T) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mockNow := func() time.Time { return now }

	cache := NewCacheWithCleanup(100, -1, WithNow(mockNow), WithJitter(0)) // 禁用定期清理
	defer cache.Stop()

	ctx := context.Background()

	// 写入缓存，TTL = 1 分钟
	var user User
	cache.GetOrLoad(ctx, "user:1", 1*time.Minute, &user, func(ctx context.Context) (any, error) {
		return User{ID: 1, Name: "Alice"}, nil
	})

	// 时间推进 30 秒（未过期）
	now = now.Add(30 * time.Second)
	var user2 User
	err := cache.GetOrLoad(ctx, "user:1", 1*time.Minute, &user2, func(ctx context.Context) (any, error) {
		t.Error("loader should not be called (not expired)")
		return nil, errors.New("should not reach here")
	})
	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}
	if user2.Name != "Alice" {
		t.Errorf("expected cached value, got: %+v", user2)
	}

	// 时间推进 31 秒（已过期）
	now = now.Add(31 * time.Second)
	var user3 User
	loadCount := 0
	err = cache.GetOrLoad(ctx, "user:1", 1*time.Minute, &user3, func(ctx context.Context) (any, error) {
		loadCount++
		return User{ID: 1, Name: "Bob"}, nil
	})
	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}
	if loadCount != 1 {
		t.Errorf("expected loadCount=1 after expiration, got %d", loadCount)
	}
	if user3.Name != "Bob" {
		t.Errorf("expected new value after expiration, got: %+v", user3)
	}
}

func TestCache_LRU_Eviction(t *testing.T) {
	cache := NewCacheWithCleanup(3, -1) // 最多 3 条，禁用定期清理
	defer cache.Stop()

	ctx := context.Background()

	// 写入 3 条
	for i := 1; i <= 3; i++ {
		var user User
		key := "user:" + string(rune('0'+i))
		cache.GetOrLoad(ctx, key, 10*time.Minute, &user, func(ctx context.Context) (any, error) {
			return User{ID: i, Name: "User"}, nil
		})
	}

	if cache.Len() != 3 {
		t.Errorf("expected len=3, got %d", cache.Len())
	}

	// 访问 user:1（更新访问时间）
	time.Sleep(10 * time.Millisecond)
	var user User
	cache.GetOrLoad(ctx, "user:1", 10*time.Minute, &user, func(ctx context.Context) (any, error) {
		t.Error("should hit cache")
		return nil, errors.New("should not reach")
	})

	// 写入第 4 条，应该驱逐 user:2（最久未访问）
	cache.GetOrLoad(ctx, "user:4", 10*time.Minute, &user, func(ctx context.Context) (any, error) {
		return User{ID: 4, Name: "User4"}, nil
	})

	if cache.Len() != 3 {
		t.Errorf("expected len=3 after eviction, got %d", cache.Len())
	}

	// user:1 应该还在
	loadCount := 0
	cache.GetOrLoad(ctx, "user:1", 10*time.Minute, &user, func(ctx context.Context) (any, error) {
		loadCount++
		return User{ID: 1, Name: "User1"}, nil
	})
	if loadCount != 0 {
		t.Error("user:1 should still be in cache")
	}

	// user:2 应该被驱逐
	cache.GetOrLoad(ctx, "user:2", 10*time.Minute, &user, func(ctx context.Context) (any, error) {
		loadCount++
		return User{ID: 2, Name: "User2"}, nil
	})
	if loadCount != 1 {
		t.Error("user:2 should be evicted and reloaded")
	}
}

func TestCache_PeriodicCleanup(t *testing.T) {
	var mu sync.RWMutex
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mockNow := func() time.Time {
		mu.RLock()
		defer mu.RUnlock()
		return now
	}

	cache := NewCacheWithCleanup(100, 100*time.Millisecond, WithNow(mockNow), WithJitter(0))
	defer cache.Stop()

	ctx := context.Background()

	// 写入缓存，TTL = 1 秒
	var user User
	cache.GetOrLoad(ctx, "user:1", 1*time.Second, &user, func(ctx context.Context) (any, error) {
		return User{ID: 1, Name: "Alice"}, nil
	})

	if cache.Len() != 1 {
		t.Errorf("expected len=1, got %d", cache.Len())
	}

	// 推进时间到过期
	mu.Lock()
	now = now.Add(2 * time.Second)
	mu.Unlock()

	// 等待定期清理运行
	time.Sleep(200 * time.Millisecond)

	if cache.Len() != 0 {
		t.Errorf("expected len=0 after cleanup, got %d", cache.Len())
	}
}

func TestCache_Prefix(t *testing.T) {
	cache := NewCache(100, WithPrefix("test"))
	defer cache.Stop()

	ctx := context.Background()

	var user User
	err := cache.GetOrLoad(ctx, "user:1", 10*time.Minute, &user, func(ctx context.Context) (any, error) {
		return User{ID: 1, Name: "Alice"}, nil
	})
	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}

	// 验证前缀已添加（通过删除测试）
	cache.Del(ctx, "user:1")
	if cache.Len() != 0 {
		t.Error("Del with prefix should work")
	}
}

func TestCache_CustomIsNotFound(t *testing.T) {
	customErr := errors.New("custom not found")

	cache := NewCache(100, WithIsNotFound(func(err error) bool {
		return errors.Is(err, customErr)
	}))
	defer cache.Stop()

	ctx := context.Background()

	// 第一次：返回自定义 NotFound 错误
	var user User
	err := cache.GetOrLoad(ctx, "user:999", 10*time.Minute, &user, func(ctx context.Context) (any, error) {
		return nil, customErr
	})
	if !errors.Is(err, customErr) {
		t.Errorf("expected customErr, got: %v", err)
	}

	// 第二次：负缓存命中
	err = cache.GetOrLoad(ctx, "user:999", 10*time.Minute, &user, func(ctx context.Context) (any, error) {
		t.Error("loader should not be called (negative cache)")
		return nil, errors.New("should not reach")
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound from negative cache, got: %v", err)
	}
}

func TestCache_OnError(t *testing.T) {
	errorCount := 0
	cache := NewCache(100, WithOnError(func(ctx context.Context, op, key string, err error) {
		errorCount++
	}))
	defer cache.Stop()

	ctx := context.Background()

	// 模拟损坏的缓存数据
	cache.mu.Lock()
	cache.items["corrupt"] = localItem{
		packed:     []byte{}, // 空数据，会触发 ErrCorrupt
		expireAt:   time.Now().Add(time.Hour),
		accessedAt: time.Now(),
	}
	cache.mu.Unlock()

	var user User
	cache.GetOrLoad(ctx, "corrupt", 10*time.Minute, &user, func(ctx context.Context) (any, error) {
		return User{ID: 1, Name: "Alice"}, nil
	})

	if errorCount == 0 {
		t.Error("expected OnError to be called for corrupt data")
	}
}

func TestJitterTTL(t *testing.T) {
	// jitter = 0
	ttl := jitterTTL(time.Minute, 0)
	if ttl != time.Minute {
		t.Errorf("jitter=0 should not change ttl")
	}

	// jitter = 0.1
	ttl = jitterTTL(time.Minute, 0.1)
	if ttl < time.Minute || ttl > time.Minute+6*time.Second {
		t.Errorf("jitter=0.1 should add 0-10%%, got: %v", ttl)
	}

	// negative ttl
	ttl = jitterTTL(-time.Minute, 0.1)
	if ttl != -time.Minute {
		t.Errorf("negative ttl should not jitter")
	}
}

func TestPackUnpack(t *testing.T) {
	// Found case
	data := []byte("hello world")
	packed := packFound(data)
	found, unpacked, err := unpack(packed)
	if err != nil {
		t.Fatalf("unpack failed: %v", err)
	}
	if !found {
		t.Error("expected found=true")
	}
	if string(unpacked) != "hello world" {
		t.Errorf("expected 'hello world', got: %s", unpacked)
	}

	// NotFound case
	packed = packNotFound()
	found, unpacked, err = unpack(packed)
	if err != nil {
		t.Fatalf("unpack failed: %v", err)
	}
	if found {
		t.Error("expected found=false")
	}
	if unpacked != nil {
		t.Errorf("expected nil data, got: %v", unpacked)
	}

	// Corrupt case
	_, _, err = unpack([]byte{})
	if !errors.Is(err, ErrCorrupt) {
		t.Errorf("expected ErrCorrupt, got: %v", err)
	}
}

func TestEnsureDestPtr(t *testing.T) {
	// nil
	err := ensureDestPtr(nil)
	if !errors.Is(err, ErrInvalidDest) {
		t.Errorf("expected ErrInvalidDest for nil, got: %v", err)
	}

	// non-pointer
	err = ensureDestPtr("string")
	if !errors.Is(err, ErrInvalidDest) {
		t.Errorf("expected ErrInvalidDest for non-pointer, got: %v", err)
	}

	// nil pointer
	var p *string
	err = ensureDestPtr(p)
	if !errors.Is(err, ErrInvalidDest) {
		t.Errorf("expected ErrInvalidDest for nil pointer, got: %v", err)
	}

	// valid pointer
	s := "test"
	err = ensureDestPtr(&s)
	if err != nil {
		t.Errorf("valid pointer should pass, got: %v", err)
	}
}

func TestJoinPrefix(t *testing.T) {
	if joinPrefix("", "key") != "key" {
		t.Error("empty prefix should not modify key")
	}

	if joinPrefix("prefix", "key") != "prefix:key" {
		t.Error("prefix should be joined with ':'")
	}
}

func TestOptions(t *testing.T) {
	// Test default options
	opts := defaultOptions()
	if opts.Prefix != "" {
		t.Error("default prefix should be empty")
	}
	if opts.Jitter != 0.10 {
		t.Error("default jitter should be 0.10")
	}
	if opts.NegativeTTL != 30*time.Second {
		t.Error("default negative TTL should be 30s")
	}

	// Test option functions
	opts = applyOptions(
		WithPrefix("test"),
		WithJitter(0.5),
		WithNegativeTTL(time.Minute),
		WithCodec(nil), // should use default
	)
	if opts.Prefix != "test" {
		t.Error("WithPrefix not applied")
	}
	if opts.Jitter != 0.5 {
		t.Error("WithJitter not applied")
	}
	if opts.NegativeTTL != time.Minute {
		t.Error("WithNegativeTTL not applied")
	}
	if opts.Codec == nil {
		t.Error("Codec should default to JSONCodec")
	}

	// Test jitter clamping
	opts = applyOptions(WithJitter(-0.5))
	if opts.Jitter != 0 {
		t.Error("negative jitter should be clamped to 0")
	}

	opts = applyOptions(WithJitter(1.5))
	if opts.Jitter != 1 {
		t.Error("jitter > 1 should be clamped to 1")
	}
}
