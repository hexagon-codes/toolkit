package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/everyday-items/toolkit/cache/local"
)

type User struct {
	ID   int
	Name string
}

func main() {
	// 创建本地缓存（最多 1000 条）
	cache := local.NewCache(1000,
		local.WithPrefix("myapp"),
		local.WithNegativeTTL(30*time.Second),
		local.WithOnError(func(ctx context.Context, op, key string, err error) {
			log.Printf("缓存错误: op=%s key=%s err=%v", op, key, err)
		}),
	)
	defer cache.Stop()

	// 模拟数据库查询
	db := map[int]User{
		123: {ID: 123, Name: "Alice"},
		456: {ID: 456, Name: "Bob"},
	}

	// 示例 1: 获取存在的用户
	var user User
	err := cache.GetOrLoad(
		context.Background(),
		"user:123",
		10*time.Minute,
		&user,
		func(ctx context.Context) (any, error) {
			fmt.Println("从数据库加载用户 123...")
			if u, ok := db[123]; ok {
				return u, nil
			}
			return nil, local.ErrNotFound
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("用户 1: %+v\n", user)

	// 第二次获取（从缓存）
	var user2 User
	err = cache.GetOrLoad(
		context.Background(),
		"user:123",
		10*time.Minute,
		&user2,
		func(ctx context.Context) (any, error) {
			fmt.Println("这行不会执行，因为命中缓存")
			return nil, nil
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("用户 2 (缓存命中): %+v\n", user2)

	// 示例 2: 获取不存在的用户（负缓存）
	var user3 User
	err = cache.GetOrLoad(
		context.Background(),
		"user:999",
		10*time.Minute,
		&user3,
		func(ctx context.Context) (any, error) {
			fmt.Println("查询用户 999...")
			if u, ok := db[999]; ok {
				return u, nil
			}
			return nil, local.ErrNotFound
		},
	)
	if err == local.ErrNotFound {
		fmt.Println("用户 999 不存在（负缓存）")
	}

	// 删除缓存
	cache.Del(context.Background(), "user:123")
	fmt.Println("已删除用户 123 的缓存")

	// 查看缓存条目数
	fmt.Printf("当前缓存条目数: %d\n", cache.Len())
}
