package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/everyday-items/toolkit/cache/redis"
	goredis "github.com/redis/go-redis/v9"
)

type Product struct {
	ID    int
	Name  string
	Price float64
}

func main() {
	// 创建 Redis 客户端
	rdb := goredis.NewClient(&goredis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
	defer rdb.Close()

	// 测试连接
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Redis 连接失败: %v\n提示: 请确保 Redis 运行在 localhost:6379", err)
	}

	// 创建稳定 key 缓存
	cache := redis.NewStableCache(rdb,
		redis.WithPrefix("myapp"),
		redis.WithRedisTimeout(100*time.Millisecond, 100*time.Millisecond),
		redis.WithOnError(func(ctx context.Context, op, key string, err error) {
			log.Printf("缓存错误: op=%s key=%s err=%v", op, key, err)
		}),
	)

	// 模拟数据库
	db := map[int]Product{
		100: {ID: 100, Name: "iPhone", Price: 999.99},
		200: {ID: 200, Name: "MacBook", Price: 1999.99},
	}

	// 示例 1: 获取产品（第一次从数据库，第二次从缓存）
	var product Product
	err := cache.GetOrLoad(
		ctx,
		"product:100",
		10*time.Minute,
		&product,
		func(ctx context.Context) (any, error) {
			fmt.Println("从数据库加载产品 100...")
			if p, ok := db[100]; ok {
				return p, nil
			}
			return nil, redis.ErrNotFound
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("产品 1: %+v\n", product)

	// 第二次获取（缓存命中）
	time.Sleep(100 * time.Millisecond) // 确保异步写入完成
	var product2 Product
	err = cache.GetOrLoad(
		ctx,
		"product:100",
		10*time.Minute,
		&product2,
		func(ctx context.Context) (any, error) {
			fmt.Println("这行不会执行（缓存命中）")
			return nil, nil
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("产品 2 (缓存命中): %+v\n", product2)

	// 示例 2: 主动写入缓存
	newProduct := Product{ID: 300, Name: "iPad", Price: 599.99}
	err = cache.Set(ctx, "product:300", newProduct, 5*time.Minute)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("已写入产品 300 到缓存")

	// 示例 3: 删除缓存（模拟更新场景）
	fmt.Println("\n模拟更新产品 100...")
	db[100] = Product{ID: 100, Name: "iPhone Pro", Price: 1099.99}
	cache.Del(ctx, "product:100")
	fmt.Println("已删除产品 100 的缓存")

	// 再次获取（从数据库获取新数据）
	var product3 Product
	err = cache.GetOrLoad(
		ctx,
		"product:100",
		10*time.Minute,
		&product3,
		func(ctx context.Context) (any, error) {
			fmt.Println("从数据库加载更新后的产品 100...")
			if p, ok := db[100]; ok {
				return p, nil
			}
			return nil, redis.ErrNotFound
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("产品 3 (更新后): %+v\n", product3)
}
