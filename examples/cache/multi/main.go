package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/everyday-items/toolkit/cache/local"
	"github.com/everyday-items/toolkit/cache/multi"
	"github.com/everyday-items/toolkit/cache/redis"
	goredis "github.com/redis/go-redis/v9"
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

func main() {
	fmt.Println("=== Multi-Level Cache Example ===")

	// 1. 创建本地缓存
	localCache := local.NewCache(1000,
		local.WithPrefix("myapp"),
	)
	defer localCache.Stop()

	// 2. 创建 Redis 缓存
	rdb := goredis.NewClient(&goredis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
	defer rdb.Close()

	ctx := context.Background()

	// 测试 Redis 连接
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("警告: Redis 未连接 (%v)，将只使用本地缓存\n\n", err)
		// 只使用本地缓存继续演示
		demonstrateLocalOnly(localCache)
		return
	}

	redisCache := redis.NewStableCache(rdb,
		redis.WithPrefix("myapp"),
	)

	// 3. 创建多层缓存（Builder 模式）
	cache := multi.NewBuilder().
		WithLocal(localCache, 10*time.Minute).
		WithRedis(redisCache, 60*time.Minute).
		WithOnError(func(ctx context.Context, layer, op, key string, err error) {
			log.Printf("[错误] layer=%s op=%s key=%s err=%v", layer, op, key, err)
		}).
		Build()

	fmt.Printf("多层缓存已创建，共 %d 层\n\n", cache.LayerCount())

	// === 示例 1: 首次查询（三层穿透）===
	fmt.Println("--- 示例 1: 首次查询 ---")
	var user1 User
	err := cache.GetOrLoad(ctx, "user:123", &user1, func(ctx context.Context) (any, error) {
		loadCount++
		fmt.Printf("  [DB] 查询数据库 (第 %d 次)\n", loadCount)
		time.Sleep(100 * time.Millisecond) // 模拟 DB 查询延迟
		if u, ok := db[123]; ok {
			return u, nil
		}
		return nil, multi.ErrNotFound
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("结果: %+v\n\n", user1)

	// === 示例 2: 再次查询（Local 命中）===
	fmt.Println("--- 示例 2: 再次查询（应该命中 Local）---")
	time.Sleep(200 * time.Millisecond) // 等待回填完成
	var user2 User
	err = cache.GetOrLoad(ctx, "user:123", &user2, func(ctx context.Context) (any, error) {
		loadCount++
		fmt.Printf("  [DB] 这行不应该执行（Local 缓存命中）\n")
		return nil, errors.New("should not reach here")
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("结果: %+v (命中缓存)\n\n", user2)

	// === 示例 3: 清空 Local，查询 Redis ===
	fmt.Println("--- 示例 3: 清空 Local，应该命中 Redis ---")
	localCache.Del(ctx, "user:123")
	var user3 User
	err = cache.GetOrLoad(ctx, "user:123", &user3, func(ctx context.Context) (any, error) {
		loadCount++
		fmt.Printf("  [DB] 这行不应该执行（Redis 缓存命中）\n")
		return nil, errors.New("should not reach here")
	})
	if err != nil {
		log.Fatal(err)
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
	if err == multi.ErrNotFound {
		fmt.Println("结果: 用户不存在（负缓存）")
	} else if err != nil {
		log.Fatal(err)
	}

	// === 示例 5: 删除缓存（所有层）===
	fmt.Println("--- 示例 5: 删除缓存 ---")
	cache.Del(ctx, "user:123")
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
		log.Fatal(err)
	}
	fmt.Printf("结果: %+v\n\n", user5)

	// 统计
	fmt.Println("=== 统计 ===")
	fmt.Printf("总 DB 查询次数: %d 次\n", loadCount)
	fmt.Printf("本地缓存条目数: %d\n", localCache.Len())
}

// demonstrateLocalOnly 演示只使用本地缓存的情况
func demonstrateLocalOnly(localCache *local.Cache) {
	fmt.Println("=== 演示：只使用本地缓存 ===")

	// 创建单层缓存
	cache := multi.NewBuilder().
		WithLocal(localCache, 10*time.Minute).
		Build()

	ctx := context.Background()
	loadCount := 0

	fmt.Println("--- 首次查询 ---")
	var user User
	err := cache.GetOrLoad(ctx, "user:123", &user, func(ctx context.Context) (any, error) {
		loadCount++
		fmt.Printf("  [DB] 查询数据库 (第 %d 次)\n", loadCount)
		if u, ok := db[123]; ok {
			return u, nil
		}
		return nil, multi.ErrNotFound
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("结果: %+v\n\n", user)

	fmt.Println("--- 再次查询（命中缓存）---")
	var user2 User
	err = cache.GetOrLoad(ctx, "user:123", &user2, func(ctx context.Context) (any, error) {
		loadCount++
		fmt.Println("  [DB] 这行不应该执行")
		return nil, errors.New("should not reach here")
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("结果: %+v\n\n", user2)

	fmt.Printf("总 DB 查询次数: %d 次\n", loadCount)
}
