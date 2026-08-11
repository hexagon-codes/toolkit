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

	"github.com/hexagon-codes/toolkit/infra/db/mysql"
	"github.com/hexagon-codes/toolkit/infra/db/redis"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	fmt.Println("=== GoPkg Database Example ===")

	mysqlDSN, mysqlConfigured := configuredMySQLDSN()
	if !mysqlConfigured {
		fmt.Println("Set MYSQL_DSN to run the MySQL example.")
	} else if err := mysqlExample(mysqlDSN); err != nil {
		return err
	}

	redisAddress, redisConfigured := configuredRedisAddress()
	if !redisConfigured {
		fmt.Println("Set REDIS_ADDR to run the Redis examples.")
	} else {
		if err := redisExample(redisAddress); err != nil {
			return err
		}
		if err := lockExample(redisAddress); err != nil {
			return err
		}
	}

	fmt.Println("\n✅ 示例完成!")
	return nil
}

func configuredMySQLDSN() (string, bool) {
	dsn := strings.TrimSpace(os.Getenv("MYSQL_DSN"))
	return dsn, dsn != ""
}

func configuredRedisAddress() (string, bool) {
	addr := strings.TrimSpace(os.Getenv("REDIS_ADDR"))
	return addr, addr != ""
}

func mysqlExample(dsn string) (resultErr error) {
	fmt.Println("📦 MySQL Example:")

	config := mysql.DefaultConfig(dsn)
	db, err := mysql.New(config)
	if err != nil {
		return fmt.Errorf("open MySQL client: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, db.Close())
	}()

	// 示例代码（连接成功后执行）
	fmt.Println("  - 创建用户表")
	fmt.Println("  - 插入用户数据")
	fmt.Println("  - 查询用户列表")
	fmt.Println("  - 事务操作")
	fmt.Println()
	return nil
}

func redisExample(addr string) (resultErr error) {
	fmt.Println("📦 Redis 示例:")

	connectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	client, err := newRedisClient(connectCtx, addr)
	cancel()
	if err != nil {
		return fmt.Errorf("open Redis client: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, client.Close())
	}()
	ctx, cancelOperations := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelOperations()

	// Set
	fmt.Println("  - Set key: name = Alice")
	if err := client.Set(ctx, "name", "Alice", time.Minute).Err(); err != nil {
		return fmt.Errorf("set name: %w", err)
	}

	// Get
	val, err := client.Get(ctx, "name").Result()
	if err != nil {
		return fmt.Errorf("get name: %w", err)
	}
	fmt.Printf("  - Get key: name = %s\n", val)

	// Incr
	if err := client.Incr(ctx, "counter").Err(); err != nil {
		return fmt.Errorf("increment counter: %w", err)
	}
	fmt.Println("  - Incr counter")

	// Hash
	if err := client.HSet(ctx, "user:1", "name", "Bob", "age", 25).Err(); err != nil {
		return fmt.Errorf("set user hash: %w", err)
	}
	fmt.Println("  - HSet user:1")

	// List
	if err := client.LPush(ctx, "queue", "task1", "task2").Err(); err != nil {
		return fmt.Errorf("push queue items: %w", err)
	}
	fmt.Println("  - LPush queue")

	fmt.Println()
	return nil
}

func lockExample(addr string) (resultErr error) {
	fmt.Println("🔒 分布式锁示例:")

	connectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	client, err := newRedisClient(connectCtx, addr)
	cancel()
	if err != nil {
		return fmt.Errorf("open Redis lock client: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, client.Close())
	}()
	ctx, cancelOperations := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelOperations()

	// 使用 WithLock 自动管理锁（使用 UniversalClient）
	err = redis.WithLock(ctx, client.UniversalClient, "lock:resource", 30*time.Second, func() error {
		fmt.Println("  - 获取锁成功")
		fmt.Println("  - 执行业务逻辑...")
		time.Sleep(100 * time.Millisecond)
		fmt.Println("  - 业务逻辑完成")
		return nil
	})

	if err != nil {
		return fmt.Errorf("execute Redis lock example: %w", err)
	}

	fmt.Println("  - 锁已自动释放")
	fmt.Println()
	return nil
}

func newRedisClient(ctx context.Context, addr string) (*redis.Client, error) {
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
