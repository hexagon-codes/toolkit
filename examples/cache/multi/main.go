package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/hexagon-codes/toolkit/cache/local"
	"github.com/hexagon-codes/toolkit/cache/multi"
	"github.com/hexagon-codes/toolkit/cache/redis"
	"github.com/hexagon-codes/toolkit/infra/redisconn"
)

type User struct {
	ID   int
	Name string
}

// 模拟数据库
var db = map[int]User{
	123: {ID: 123, Name: "Alice"},
	456: {ID: 456, Name: "Bob"},
}

var loadCount = 0 // 统计 DB 查询次数

var errCacheBackfillPending = errors.New("cache backfill is pending")

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() (resultErr error) {
	fmt.Println("=== Multi-Level Cache Example ===")

	// 1. 创建本地缓存
	localCache := local.NewCache(1000,
		local.WithPrefix("myapp"),
	)
	defer func() {
		resultErr = errors.Join(resultErr, localCache.Close())
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	addr, redisConfigured := configuredRedisAddress()
	if !redisConfigured {
		fmt.Println("Set REDIS_ADDR to enable the Redis layer; running the local-only example.")
		return demonstrateLocalOnly(ctx, localCache)
	}
	connection := redisconn.DefaultConfig(redisconn.ModeSingle, addr)
	connection.DataCredentials = redisconn.Credentials{
		Username: os.Getenv("REDIS_USERNAME"),
		Password: os.Getenv("REDIS_PASSWORD"),
	}
	if serverName := os.Getenv("REDIS_TLS_SERVER_NAME"); serverName != "" {
		connection.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName}
	}
	factory, err := redisconn.NewFactory(connection)
	if err != nil {
		return fmt.Errorf("create Redis factory: %w", err)
	}
	startupCtx, cancelStartup := context.WithTimeout(ctx, 5*time.Second)
	rdb, err := factory.Open(startupCtx)
	cancelStartup()
	if err != nil {
		return fmt.Errorf("open Redis client: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, rdb.Close())
	}()

	redisCache := redis.NewStableCache(rdb,
		redis.WithPrefix("myapp"),
	)

	// 3. 创建多层缓存（Builder 模式）
	cache, err := multi.NewBuilder().
		WithLocal(localCache, 10*time.Minute).
		WithRedis(redisCache, 60*time.Minute).
		WithOnError(func(ctx context.Context, layer, op, key string, err error) {
			log.Printf("[错误] layer=%s op=%s key=%s err=%v", layer, op, key, err)
		}).
		Build()
	if err != nil {
		return fmt.Errorf("create multi-layer cache: %w", err)
	}

	fmt.Printf("多层缓存已创建，共 %d 层\n\n", cache.LayerCount())

	// === 示例 1: 首次查询（三层穿透）===
	fmt.Println("--- 示例 1: 首次查询 ---")
	var user1 User
	err = cache.GetOrLoad(ctx, "user:123", &user1, func(ctx context.Context) (any, error) {
		loadCount++
		fmt.Printf("  [DB] 查询数据库 (第 %d 次)\n", loadCount)
		time.Sleep(100 * time.Millisecond) // 模拟 DB 查询延迟
		if u, ok := db[123]; ok {
			return u, nil
		}
		return nil, multi.ErrNotFound
	})
	if err != nil {
		return fmt.Errorf("load user 123: %w", err)
	}
	fmt.Printf("结果: %+v\n\n", user1)
	var localBackfill, redisBackfill User
	if err := waitForCachedUser(ctx, localCache, 10*time.Minute, "user:123", &localBackfill); err != nil {
		return fmt.Errorf("wait for local user 123 backfill: %w", err)
	}
	if err := waitForCachedUser(ctx, redisCache, 60*time.Minute, "user:123", &redisBackfill); err != nil {
		return fmt.Errorf("wait for Redis user 123 backfill: %w", err)
	}

	// === 示例 2: 再次查询（Local 命中）===
	fmt.Println("--- 示例 2: 再次查询（应该命中 Local）---")
	var user2 User
	if err := cache.GetOrLoad(ctx, "user:123", &user2, func(context.Context) (any, error) {
		return nil, errCacheBackfillPending
	}); err != nil {
		return fmt.Errorf("load cached user 123 from local layer: %w", err)
	}
	fmt.Printf("结果: %+v (命中缓存)\n\n", user2)

	// === 示例 3: 清空 Local，查询 Redis ===
	fmt.Println("--- 示例 3: 清空 Local，应该命中 Redis ---")
	if err := localCache.Del(ctx, "user:123"); err != nil {
		return fmt.Errorf("delete user 123 from local layer: %w", err)
	}
	var user3 User
	if err := cache.GetOrLoad(ctx, "user:123", &user3, func(context.Context) (any, error) {
		return nil, errCacheBackfillPending
	}); err != nil {
		return fmt.Errorf("load cached user 123 from Redis layer: %w", err)
	}
	if err := waitForCachedUser(ctx, localCache, 10*time.Minute, "user:123", &localBackfill); err != nil {
		return fmt.Errorf("wait for Redis-to-local user 123 backfill: %w", err)
	}
	fmt.Printf("结果: %+v (命中 Redis)\n\n", user3)

	// === 示例 4: 查询不存在的数据（负缓存）===
	fmt.Println("--- 示例 4: 查询不存在的数据 ---")
	var user4 User
	err = cache.GetOrLoad(ctx, "user:999", &user4, func(ctx context.Context) (any, error) {
		loadCount++
		fmt.Printf("  [DB] 查询数据库 (第 %d 次)\n", loadCount)
		if u, ok := db[999]; ok {
			return u, nil
		}
		return nil, multi.ErrNotFound
	})
	if errors.Is(err, multi.ErrNotFound) {
		fmt.Println("结果: 用户不存在（负缓存）")
	} else if err != nil {
		return fmt.Errorf("load missing user 999: %w", err)
	}

	// === 示例 5: 删除缓存（所有层）===
	fmt.Println("--- 示例 5: 删除缓存 ---")
	if err := cache.Del(ctx, "user:123"); err != nil {
		return fmt.Errorf("delete user 123 from all cache layers: %w", err)
	}
	fmt.Println("已删除 user:123 的所有层缓存")

	// 再次查询（应该查 DB）
	var user5 User
	err = cache.GetOrLoad(ctx, "user:123", &user5, func(ctx context.Context) (any, error) {
		loadCount++
		fmt.Printf("  [DB] 查询数据库 (第 %d 次)\n", loadCount)
		if u, ok := db[123]; ok {
			return u, nil
		}
		return nil, multi.ErrNotFound
	})
	if err != nil {
		return fmt.Errorf("reload user 123 after deletion: %w", err)
	}
	fmt.Printf("结果: %+v\n\n", user5)
	if err := waitForCachedUser(ctx, localCache, 10*time.Minute, "user:123", &localBackfill); err != nil {
		return fmt.Errorf("wait for final local user 123 backfill: %w", err)
	}
	if err := waitForCachedUser(ctx, redisCache, 60*time.Minute, "user:123", &redisBackfill); err != nil {
		return fmt.Errorf("wait for final Redis user 123 backfill: %w", err)
	}

	// 统计
	fmt.Println("=== 统计 ===")
	fmt.Printf("总 DB 查询次数: %d 次\n", loadCount)
	fmt.Printf("本地缓存条目数: %d\n", localCache.Len())
	return nil
}

func configuredRedisAddress() (string, bool) {
	addr := strings.TrimSpace(os.Getenv("REDIS_ADDR"))
	return addr, addr != ""
}

// demonstrateLocalOnly 演示只使用本地缓存的情况
func demonstrateLocalOnly(ctx context.Context, localCache *local.Cache) error {
	fmt.Println("=== 演示：只使用本地缓存 ===")

	// 使用直接构造函数创建单层缓存
	cache, err := multi.NewCache([]multi.LayerConfig{
		{Layer: localCache, TTL: 10 * time.Minute, Name: "local"},
	})
	if err != nil {
		return fmt.Errorf("create local-only cache: %w", err)
	}

	loadCount := 0

	fmt.Println("--- 首次查询 ---")
	var user User
	err = cache.GetOrLoad(ctx, "user:123", &user, func(ctx context.Context) (any, error) {
		loadCount++
		fmt.Printf("  [DB] 查询数据库 (第 %d 次)\n", loadCount)
		if u, ok := db[123]; ok {
			return u, nil
		}
		return nil, multi.ErrNotFound
	})
	if err != nil {
		return fmt.Errorf("load local-only user 123: %w", err)
	}
	fmt.Printf("结果: %+v\n\n", user)
	var localBackfill User
	if err := waitForCachedUser(ctx, localCache, 10*time.Minute, "user:123", &localBackfill); err != nil {
		return fmt.Errorf("wait for local-only user 123 backfill: %w", err)
	}

	fmt.Println("--- 再次查询（命中缓存）---")
	var user2 User
	if err := cache.GetOrLoad(ctx, "user:123", &user2, func(context.Context) (any, error) {
		return nil, errCacheBackfillPending
	}); err != nil {
		return fmt.Errorf("load cached local-only user 123: %w", err)
	}
	fmt.Printf("结果: %+v\n\n", user2)

	fmt.Printf("总 DB 查询次数: %d 次\n", loadCount)
	return nil
}

func waitForCachedUser(ctx context.Context, layer multi.Layer, ttl time.Duration, key string, dest *User) error {
	waitCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		err := layer.GetOrLoad(waitCtx, key, ttl, dest, func(context.Context) (any, error) {
			return nil, errCacheBackfillPending
		})
		if err == nil {
			return nil
		}
		if !errors.Is(err, errCacheBackfillPending) {
			return err
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("wait for cache backfill: %w", waitCtx.Err())
		case <-ticker.C:
		}
	}
}
