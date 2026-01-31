package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func setupRedis(t *testing.T) (*miniredis.Miniredis, redis.UniversalClient) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	return mr, client
}

func TestStableCache_GetOrLoad_Basic(t *testing.T) {
	mr, client := setupRedis(t)
	defer mr.Close()
	defer client.Close()

	cache := NewStableCache(client)
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

	// 等待异步写入完成
	time.Sleep(50 * time.Millisecond)

	// 第二次加载：缓存命中
	var user2 User
	err = cache.GetOrLoad(ctx, "user:1", 10*time.Minute, &user2, func(ctx context.Context) (any, error) {
		loadCount++
		return User{ID: 1, Name: "Bob"}, nil
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

func TestStableCache_GetOrLoad_NotFound(t *testing.T) {
	mr, client := setupRedis(t)
	defer mr.Close()
	defer client.Close()

	cache := NewStableCache(client)
	ctx := context.Background()

	// 第一次加载：返回 NotFound
	var user User
	err := cache.GetOrLoad(ctx, "user:999", 10*time.Minute, &user, func(ctx context.Context) (any, error) {
		return nil, ErrNotFound
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}

	// 等待异步写入
	time.Sleep(50 * time.Millisecond)

	// 第二次加载：负缓存命中
	err = cache.GetOrLoad(ctx, "user:999", 10*time.Minute, &user, func(ctx context.Context) (any, error) {
		t.Error("loader should not be called (negative cache hit)")
		return nil, errors.New("should not reach here")
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound from negative cache, got: %v", err)
	}
}

func TestStableCache_GetOrLoad_InvalidParams(t *testing.T) {
	mr, client := setupRedis(t)
	defer mr.Close()
	defer client.Close()

	cache := NewStableCache(client)
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

func TestStableCache_Del(t *testing.T) {
	mr, client := setupRedis(t)
	defer mr.Close()
	defer client.Close()

	cache := NewStableCache(client)
	ctx := context.Background()

	// 写入缓存
	var user User
	cache.GetOrLoad(ctx, "user:1", 10*time.Minute, &user, func(ctx context.Context) (any, error) {
		return User{ID: 1, Name: "Alice"}, nil
	})
	time.Sleep(50 * time.Millisecond)

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

func TestStableCache_Set(t *testing.T) {
	mr, client := setupRedis(t)
	defer mr.Close()
	defer client.Close()

	cache := NewStableCache(client)
	ctx := context.Background()

	// 主动写入
	user := User{ID: 1, Name: "Alice"}
	err := cache.Set(ctx, "user:1", user, 10*time.Minute)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// 读取
	var user2 User
	err = cache.GetOrLoad(ctx, "user:1", 10*time.Minute, &user2, func(ctx context.Context) (any, error) {
		t.Error("loader should not be called")
		return nil, errors.New("should not reach")
	})
	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}
	if user2.Name != "Alice" {
		t.Errorf("expected Alice, got: %s", user2.Name)
	}
}

func TestStableCache_Prefix(t *testing.T) {
	mr, client := setupRedis(t)
	defer mr.Close()
	defer client.Close()

	cache := NewStableCache(client, WithPrefix("test"))
	ctx := context.Background()

	var user User
	err := cache.GetOrLoad(ctx, "user:1", 10*time.Minute, &user, func(ctx context.Context) (any, error) {
		return User{ID: 1, Name: "Alice"}, nil
	})
	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// 验证前缀
	exists := mr.Exists("test:user:1")
	if !exists {
		t.Error("key with prefix should exist")
	}
}

func TestStableCache_Jitter(t *testing.T) {
	mr, client := setupRedis(t)
	defer mr.Close()
	defer client.Close()

	cache := NewStableCache(client, WithJitter(0))
	ctx := context.Background()

	var user User
	cache.GetOrLoad(ctx, "user:1", time.Minute, &user, func(ctx context.Context) (any, error) {
		return User{ID: 1, Name: "Alice"}, nil
	})
	time.Sleep(50 * time.Millisecond)

	// 检查 TTL（jitter=0 时应该精确）
	ttl := mr.TTL("user:1")
	if ttl > 61*time.Second || ttl < 59*time.Second {
		t.Errorf("expected TTL ~60s (jitter=0), got: %v", ttl)
	}
}

func TestStableCache_OnError(t *testing.T) {
	mr, client := setupRedis(t)
	defer mr.Close()
	defer client.Close()

	errorCount := 0
	cache := NewStableCache(client, WithOnError(func(ctx context.Context, op, key string, err error) {
		errorCount++
	}))
	ctx := context.Background()

	// 关闭 Redis，触发错误
	mr.Close()

	var user User
	cache.GetOrLoad(ctx, "user:1", 10*time.Minute, &user, func(ctx context.Context) (any, error) {
		return User{ID: 1, Name: "Alice"}, nil
	})

	if errorCount == 0 {
		t.Error("expected OnError to be called when Redis fails")
	}
}

func TestHelpers(t *testing.T) {
	// Test joinPrefix
	if JoinPrefix("", "key") != "key" {
		t.Error("empty prefix should not modify key")
	}
	if JoinPrefix("prefix", "key") != "prefix:key" {
		t.Error("prefix should be joined with ':'")
	}

	// Test jitterTTL
	ttl := JitterTTL(time.Minute, 0)
	if ttl != time.Minute {
		t.Error("jitter=0 should not change ttl")
	}

	ttl = JitterTTL(time.Minute, 0.1)
	if ttl < time.Minute || ttl > time.Minute+6*time.Second {
		t.Errorf("jitter=0.1 should add 0-10%%, got: %v", ttl)
	}

	// Test WithTimeout
	parent := context.Background()
	ctx, cancel := WithTimeout(parent, time.Minute)
	defer cancel()
	if _, ok := ctx.Deadline(); !ok {
		t.Error("WithTimeout should set deadline")
	}

	// Test WithTimeout with tighter parent deadline
	parent, parentCancel := context.WithTimeout(context.Background(), time.Second)
	defer parentCancel()
	ctx, cancel = WithTimeout(parent, time.Minute)
	defer cancel()
	deadline, _ := ctx.Deadline()
	parentDeadline, _ := parent.Deadline()
	if deadline != parentDeadline {
		t.Error("should keep parent deadline when it's tighter")
	}
}

func TestOptions(t *testing.T) {
	opts := ApplyOptions(
		WithPrefix("test"),
		WithJitter(0.5),
		WithNegativeTTL(time.Minute),
		WithMaxTTL(15*time.Minute),
		WithRedisTimeout(100*time.Millisecond, 100*time.Millisecond),
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
	if opts.MaxTTL != 15*time.Minute {
		t.Error("WithMaxTTL not applied")
	}
	if opts.ReadTimeout != 100*time.Millisecond {
		t.Error("ReadTimeout not applied")
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
