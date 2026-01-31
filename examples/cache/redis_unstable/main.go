package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/everyday-items/toolkit/cache/redis"
	goredis "github.com/redis/go-redis/v9"
)

type Model struct {
	Name    string
	Enabled bool
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

	// 创建不稳定 key 缓存（带版本号）
	cache := redis.NewUnstableCache(rdb, "myapp:models:version",
		redis.WithPrefix("myapp"),
		redis.WithMaxTTL(15*time.Minute),
		redis.WithOnError(func(ctx context.Context, op, key string, err error) {
			log.Printf("缓存错误: op=%s key=%s err=%v", op, key, err)
		}),
	)

	// 模拟数据库
	db := map[string][]Model{
		"chat": {
			{Name: "gpt-4", Enabled: true},
			{Name: "gpt-3.5-turbo", Enabled: true},
		},
		"image": {
			{Name: "dall-e-3", Enabled: true},
			{Name: "dall-e-2", Enabled: false},
		},
	}

	// 示例 1: 获取聚合数据（带版本号）
	var chatModels []Model
	err := cache.GetOrLoad(
		ctx,
		"models:group:chat",
		5*time.Minute,
		&chatModels,
		func(ctx context.Context) (any, error) {
			fmt.Println("从数据库加载 chat 模型列表...")
			if models, ok := db["chat"]; ok {
				return models, nil
			}
			return nil, redis.ErrNotFound
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Chat 模型: %+v\n", chatModels)
	fmt.Printf("当前版本: v%d\n\n", cache.GetVersion())

	// 第二次获取（缓存命中）
	time.Sleep(100 * time.Millisecond) // 确保异步写入完成
	var chatModels2 []Model
	err = cache.GetOrLoad(
		ctx,
		"models:group:chat",
		5*time.Minute,
		&chatModels2,
		func(ctx context.Context) (any, error) {
			fmt.Println("这行不会执行（缓存命中）")
			return nil, nil
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Chat 模型 (缓存命中): %+v\n\n", chatModels2)

	// 示例 2: 数据更新，递增版本号（使所有缓存失效）
	fmt.Println("模拟数据更新...")
	db["chat"] = append(db["chat"], Model{Name: "gpt-4-turbo", Enabled: true})

	// 递增版本号
	err = cache.InvalidateVersion(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("版本号已递增: v%d\n\n", cache.GetVersion())

	// 再次获取（版本号变了，缓存失效）
	var chatModels3 []Model
	err = cache.GetOrLoad(
		ctx,
		"models:group:chat",
		5*time.Minute,
		&chatModels3,
		func(ctx context.Context) (any, error) {
			fmt.Println("从数据库加载更新后的 chat 模型列表...")
			if models, ok := db["chat"]; ok {
				return models, nil
			}
			return nil, redis.ErrNotFound
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Chat 模型 (更新后): %+v\n\n", chatModels3)

	// 示例 3: 批量删除（不使用版本号的场景）
	// 先加载 image 模型
	var imageModels []Model
	err = cache.GetOrLoadWithoutVersion(
		ctx,
		"models:group:image",
		5*time.Minute,
		&imageModels,
		func(ctx context.Context) (any, error) {
			fmt.Println("从数据库加载 image 模型列表...")
			if models, ok := db["image"]; ok {
				return models, nil
			}
			return nil, redis.ErrNotFound
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Image 模型: %+v\n", imageModels)

	// 批量删除所有 models:group:* key
	time.Sleep(100 * time.Millisecond)
	fmt.Println("\n批量删除 models:group:* ...")
	err = cache.InvalidatePattern(ctx, "models:group:*")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("批量删除完成")
}
