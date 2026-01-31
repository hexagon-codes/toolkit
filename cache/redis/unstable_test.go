package redis

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUnstableCache_GetOrLoad_WithVersion(t *testing.T) {
	mr, client := setupRedis(t)
	defer mr.Close()
	defer client.Close()

	cache := NewUnstableCache(client, "test:version")
	ctx := context.Background()
	loadCount := 0

	// 第一次加载：缓存未命中
	var models []string
	err := cache.GetOrLoad(ctx, "models:group:chat", 5*time.Minute, &models, func(ctx context.Context) (any, error) {
		loadCount++
		return []string{"gpt-4", "gpt-3.5"}, nil
	})
	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("unexpected models: %+v", models)
	}
	if loadCount != 1 {
		t.Errorf("expected loadCount=1, got %d", loadCount)
	}

	// 等待异步写入
	time.Sleep(50 * time.Millisecond)

	// 第二次加载：缓存命中
	var models2 []string
	err = cache.GetOrLoad(ctx, "models:group:chat", 5*time.Minute, &models2, func(ctx context.Context) (any, error) {
		loadCount++
		return []string{"claude"}, nil
	})
	if err != nil {
		t.Fatalf("GetOrLoad failed: %v", err)
	}
	if len(models2) != 2 {
		t.Errorf("expected cached value, got: %+v", models2)
	}
	if loadCount != 1 {
		t.Errorf("expected loadCount=1 (cache hit), got %d", loadCount)
	}
}

func TestUnstableCache_InvalidateVersion(t *testing.T) {
	mr, client := setupRedis(t)
	defer mr.Close()
	defer client.Close()

	cache := NewUnstableCache(client, "test:version")
	ctx := context.Background()

	// 写入缓存
	var models []string
	cache.GetOrLoad(ctx, "models:group:chat", 5*time.Minute, &models, func(ctx context.Context) (any, error) {
		return []string{"gpt-4"}, nil
	})
	time.Sleep(50 * time.Millisecond)

	oldVersion := cache.getVersion()

	// 失效版本
	err := cache.InvalidateVersion(ctx)
	if err != nil {
		t.Fatalf("InvalidateVersion failed: %v", err)
	}

	newVersion := cache.getVersion()
	if newVersion != oldVersion+1 {
		t.Errorf("expected version to increment, old=%d new=%d", oldVersion, newVersion)
	}

	// 重新加载：应该调用 loader
	loadCount := 0
	cache.GetOrLoad(ctx, "models:group:chat", 5*time.Minute, &models, func(ctx context.Context) (any, error) {
		loadCount++
		return []string{"claude"}, nil
	})
	if loadCount != 1 {
		t.Error("expected loader to be called after version invalidation")
	}
}

func TestUnstableCache_GetOrLoadWithoutVersion(t *testing.T) {
	mr, client := setupRedis(t)
	defer mr.Close()
	defer client.Close()

	cache := NewUnstableCache(client, "test:version")
	ctx := context.Background()
	loadCount := 0

	// 使用无版本加载
	var data string
	err := cache.GetOrLoadWithoutVersion(ctx, "config:key", 5*time.Minute, &data, func(ctx context.Context) (any, error) {
		loadCount++
		return "value1", nil
	})
	if err != nil {
		t.Fatalf("GetOrLoadWithoutVersion failed: %v", err)
	}
	if data != "value1" {
		t.Errorf("unexpected data: %s", data)
	}

	// 等待写入
	time.Sleep(50 * time.Millisecond)

	// 第二次加载：缓存命中
	err = cache.GetOrLoadWithoutVersion(ctx, "config:key", 5*time.Minute, &data, func(ctx context.Context) (any, error) {
		loadCount++
		return "value2", nil
	})
	if err != nil {
		t.Fatalf("GetOrLoadWithoutVersion failed: %v", err)
	}
	if data != "value1" {
		t.Errorf("expected cached value, got: %s", data)
	}
	if loadCount != 1 {
		t.Errorf("expected loadCount=1 (cache hit), got %d", loadCount)
	}
}

func TestUnstableCache_InvalidatePattern(t *testing.T) {
	mr, client := setupRedis(t)
	defer mr.Close()
	defer client.Close()

	cache := NewUnstableCache(client, "test:version", WithPrefix("myapp"))
	ctx := context.Background()

	// 写入多个缓存
	for i := 1; i <= 3; i++ {
		var data string
		key := "config:key" + string(rune('0'+i))
		cache.GetOrLoadWithoutVersion(ctx, key, 5*time.Minute, &data, func(ctx context.Context) (any, error) {
			return "value" + string(rune('0'+i)), nil
		})
	}
	time.Sleep(50 * time.Millisecond)

	// 批量删除
	err := cache.InvalidatePattern(ctx, "config:*")
	if err != nil {
		t.Fatalf("InvalidatePattern failed: %v", err)
	}

	// 验证删除：应该重新加载
	loadCount := 0
	var data string
	cache.GetOrLoadWithoutVersion(ctx, "config:key1", 5*time.Minute, &data, func(ctx context.Context) (any, error) {
		loadCount++
		return "new_value", nil
	})
	if loadCount != 1 {
		t.Error("expected loader to be called after pattern invalidation")
	}
}

func TestUnstableCache_Del(t *testing.T) {
	mr, client := setupRedis(t)
	defer mr.Close()
	defer client.Close()

	cache := NewUnstableCache(client, "test:version")
	ctx := context.Background()

	// 写入缓存
	var data string
	cache.GetOrLoadWithoutVersion(ctx, "config:key", 5*time.Minute, &data, func(ctx context.Context) (any, error) {
		return "value", nil
	})
	time.Sleep(50 * time.Millisecond)

	// 删除
	err := cache.Del(ctx, "config:key")
	if err != nil {
		t.Fatalf("Del failed: %v", err)
	}

	// 验证删除
	loadCount := 0
	cache.GetOrLoadWithoutVersion(ctx, "config:key", 5*time.Minute, &data, func(ctx context.Context) (any, error) {
		loadCount++
		return "new_value", nil
	})
	if loadCount != 1 {
		t.Error("expected loader to be called after Del")
	}
}

func TestUnstableCache_MaxTTL(t *testing.T) {
	mr, client := setupRedis(t)
	defer mr.Close()
	defer client.Close()

	cache := NewUnstableCache(client, "test:version", WithMaxTTL(5*time.Minute), WithJitter(0))
	ctx := context.Background()

	// 尝试设置超过 MaxTTL 的 TTL
	var data string
	cache.GetOrLoadWithoutVersion(ctx, "config:key", 10*time.Minute, &data, func(ctx context.Context) (any, error) {
		return "value", nil
	})
	time.Sleep(50 * time.Millisecond)

	// 检查 TTL 被限制为 MaxTTL
	ttl := mr.TTL("config:key")
	if ttl > 6*time.Minute {
		t.Errorf("TTL should be capped at MaxTTL (5min), got: %v", ttl)
	}
}

func TestUnstableCache_NotFound(t *testing.T) {
	mr, client := setupRedis(t)
	defer mr.Close()
	defer client.Close()

	cache := NewUnstableCache(client, "test:version")
	ctx := context.Background()

	// 返回 NotFound
	var data string
	err := cache.GetOrLoadWithoutVersion(ctx, "missing:key", 5*time.Minute, &data, func(ctx context.Context) (any, error) {
		return nil, ErrNotFound
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}

	// 等待异步写入
	time.Sleep(50 * time.Millisecond)

	// 负缓存命中
	err = cache.GetOrLoadWithoutVersion(ctx, "missing:key", 5*time.Minute, &data, func(ctx context.Context) (any, error) {
		t.Error("loader should not be called (negative cache)")
		return nil, errors.New("should not reach")
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound from negative cache, got: %v", err)
	}
}

func TestUnstableCache_VersionRefresh(t *testing.T) {
	mr, client := setupRedis(t)
	defer mr.Close()
	defer client.Close()

	// 设置初始版本
	client.Set(context.Background(), "test:version", "5", 0)

	cache := NewUnstableCache(client, "test:version")

	// 检查初始版本加载
	version := cache.getVersion()
	if version != 5 {
		t.Errorf("expected version=5, got %d", version)
	}

	// 修改 Redis 中的版本
	client.Incr(context.Background(), "test:version")

	// 等待超过 1 秒，触发版本刷新检查
	time.Sleep(1100 * time.Millisecond)

	// 触发版本刷新（通过调用 GetOrLoad）
	var data string
	cache.GetOrLoad(context.Background(), "key", time.Minute, &data, func(ctx context.Context) (any, error) {
		return "value", nil
	})

	// 等待刷新完成
	time.Sleep(100 * time.Millisecond)

	// 版本应该被刷新
	newVersion := cache.getVersion()
	if newVersion != 6 {
		t.Errorf("expected version=6 after refresh, got %d", newVersion)
	}
}

func TestUnstableCache_OnError(t *testing.T) {
	mr, client := setupRedis(t)
	defer mr.Close()
	defer client.Close()

	errorCount := 0
	cache := NewUnstableCache(client, "test:version", WithOnError(func(ctx context.Context, op, key string, err error) {
		errorCount++
	}))
	ctx := context.Background()

	// 写入缓存
	var data string
	cache.GetOrLoadWithoutVersion(ctx, "key", time.Minute, &data, func(ctx context.Context) (any, error) {
		return "value", nil
	})
	time.Sleep(50 * time.Millisecond)

	// 关闭 Redis，触发错误
	mr.Close()

	// 尝试访问
	cache.GetOrLoadWithoutVersion(ctx, "key", time.Minute, &data, func(ctx context.Context) (any, error) {
		return "value", nil
	})

	if errorCount == 0 {
		t.Error("expected OnError to be called when Redis fails")
	}
}

func TestUnstableCache_GetVersion(t *testing.T) {
	mr, client := setupRedis(t)
	defer mr.Close()
	defer client.Close()

	cache := NewUnstableCache(client, "test:version")

	// 获取版本
	version := cache.GetVersion()
	if version <= 0 {
		t.Errorf("expected version > 0, got %d", version)
	}
}
