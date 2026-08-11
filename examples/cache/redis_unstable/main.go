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

	"github.com/hexagon-codes/toolkit/cache/redis"
	"github.com/hexagon-codes/toolkit/infra/redisconn"
)

type Model struct {
	Name    string
	Enabled bool
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() (resultErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	addr, redisConfigured := configuredRedisAddress()
	if !redisConfigured {
		fmt.Println("Set REDIS_ADDR to run this example.")
		return nil
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
	err = cache.GetOrLoad(
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
		return fmt.Errorf("load chat model list: %w", err)
	}
	fmt.Printf("Chat 模型: %+v\n", chatModels)
	fmt.Printf("当前版本: v%d\n\n", cache.GetVersion())

	// 第二次获取（前一次调用已同步完成缓存写入）
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
		return fmt.Errorf("load cached chat model list: %w", err)
	}
	fmt.Printf("Chat 模型 (缓存命中): %+v\n\n", chatModels2)

	// 示例 2: 数据更新，递增版本号（使所有缓存失效）
	fmt.Println("模拟数据更新...")
	db["chat"] = append(db["chat"], Model{Name: "gpt-4-turbo", Enabled: true})

	// 递增版本号
	err = cache.InvalidateVersion(ctx)
	if err != nil {
		return fmt.Errorf("invalidate model cache version: %w", err)
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
		return fmt.Errorf("reload chat model list: %w", err)
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
		return fmt.Errorf("load image model list: %w", err)
	}
	fmt.Printf("Image 模型: %+v\n", imageModels)

	// 批量删除所有 models:group:* key
	fmt.Println("\n批量删除 models:group:* ...")
	err = cache.InvalidatePattern(ctx, "models:group:*")
	if err != nil {
		return fmt.Errorf("invalidate model cache pattern: %w", err)
	}
	fmt.Println("批量删除完成")
	return nil
}

func configuredRedisAddress() (string, bool) {
	addr := strings.TrimSpace(os.Getenv("REDIS_ADDR"))
	return addr, addr != ""
}
