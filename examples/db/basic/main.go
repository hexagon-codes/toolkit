package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hexagon-codes/toolkit/infra/db/mysql"
	"github.com/hexagon-codes/toolkit/infra/db/redis"
)

func main() {
	fmt.Println("=== GoPkg DB 示例 ===")

	// MySQL 示例
	mysqlExample()

	// Redis 示例
	redisExample()

	// 分布式锁示例
	lockExample()

	fmt.Println("\n✅ 示例完成!")
}

func mysqlExample() {
	fmt.Println("📦 MySQL 示例:")

	// 初始化 MySQL（实际使用时需要有效的 DSN）
	config := mysql.DefaultConfig("root:password@tcp(localhost:3306)/test?parseTime=true")

	// 注意：这里会连接失败，因为没有真实的 MySQL 服务
	// 实际使用时请提供有效的 DSN
	_, err := mysql.New(config)
	if err != nil {
		fmt.Printf("  ⚠️  MySQL 连接失败（预期行为）: %v\n", err)
		return
	}

	// 示例代码（连接成功后执行）
	fmt.Println("  - 创建用户表")
	fmt.Println("  - 插入用户数据")
	fmt.Println("  - 查询用户列表")
	fmt.Println("  - 事务操作")
	fmt.Println()
}

func redisExample() {
	fmt.Println("📦 Redis 示例:")

	// 默认连接 localhost:6379；可通过环境变量提供实际地址和 ACL/TLS 配置。
	connectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	client, err := newRedisClient(connectCtx)
	cancel()
	if err != nil {
		fmt.Printf("  ⚠️  Redis 连接失败: %v\n", err)
		return
	}
	defer client.Close()
	ctx := context.Background()

	// Set
	fmt.Println("  - Set key: name = Alice")
	client.Set(ctx, "name", "Alice", time.Minute)

	// Get
	val, _ := client.Get(ctx, "name").Result()
	fmt.Printf("  - Get key: name = %s\n", val)

	// Incr
	client.Incr(ctx, "counter")
	fmt.Println("  - Incr counter")

	// Hash
	client.HSet(ctx, "user:1", "name", "Bob", "age", 25)
	fmt.Println("  - HSet user:1")

	// List
	client.LPush(ctx, "queue", "task1", "task2")
	fmt.Println("  - LPush queue")

	fmt.Println()
}

func lockExample() {
	fmt.Println("🔒 分布式锁示例:")

	connectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	client, err := newRedisClient(connectCtx)
	cancel()
	if err != nil {
		fmt.Printf("  ⚠️  Redis 连接失败: %v\n", err)
		return
	}
	defer client.Close()
	ctx := context.Background()

	// 使用 WithLock 自动管理锁（使用 UniversalClient）
	err = redis.WithLock(ctx, client.UniversalClient, "lock:resource", 30*time.Second, func() error {
		fmt.Println("  - 获取锁成功")
		fmt.Println("  - 执行业务逻辑...")
		time.Sleep(100 * time.Millisecond)
		fmt.Println("  - 业务逻辑完成")
		return nil
	})

	if err != nil {
		log.Printf("  ❌ 锁操作失败: %v", err)
		return
	}

	fmt.Println("  - 锁已自动释放")
	fmt.Println()
}

func newRedisClient(ctx context.Context) (*redis.Client, error) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	config := redis.DefaultConfig(redis.ModeSingle, addr)
	config.DataCredentials = redis.Credentials{
		Username: os.Getenv("REDIS_USERNAME"),
		Password: os.Getenv("REDIS_PASSWORD"),
	}
	if serverName := os.Getenv("REDIS_TLS_SERVER_NAME"); serverName != "" {
		config.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName}
	}
	return redis.New(ctx, config)
}
