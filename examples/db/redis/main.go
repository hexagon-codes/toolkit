package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/everyday-items/toolkit/infra/db/redis"
	goredis "github.com/redis/go-redis/v9"
)

func main() {
	fmt.Println("=== Redis 使用示例 ===")

	// 1. 单机模式
	fmt.Println("1. 单机模式连接")
	singleClient := initSingleRedis()
	defer singleClient.Close()

	// 2. 集群模式（示例）
	fmt.Println("\n2. 集群模式连接")
	// clusterClient := initClusterRedis()
	// defer clusterClient.Close()

	// 3. 哨兵模式（示例）
	fmt.Println("\n3. 哨兵模式连接")
	// sentinelClient := initSentinelRedis()
	// defer sentinelClient.Close()

	// 4. 基本操作
	fmt.Println("\n4. 基本操作")
	demonstrateBasicOps(singleClient)

	// 5. 数据结构操作
	fmt.Println("\n5. 数据结构操作")
	demonstrateDataStructures(singleClient)

	// 6. 分布式锁使用
	fmt.Println("\n6. 分布式锁")
	demonstrateDistributedLock(singleClient)

	// 7. 高级操作
	fmt.Println("\n7. 高级操作")
	demonstrateAdvancedOps(singleClient)

	// 8. 连接池监控
	fmt.Println("\n8. 连接池监控")
	monitorRedisPool(singleClient)
}

// initSingleRedis 初始化单机模式 Redis
func initSingleRedis() *redis.Client {
	// 方式1: 使用默认配置
	config := redis.DefaultConfig("localhost:6379")

	// 方式2: 自定义配置
	config = &redis.Config{
		Mode:               redis.ModeSingle,
		Addr:               "localhost:6379",
		Password:           "", // 密码
		DB:                 0,  // 数据库编号 (0-15)
		PoolSize:           10, // 连接池大小
		MinIdleConns:       2,  // 最小空闲连接
		MaxRetries:         3,  // 最大重试次数
		PoolTimeout:        4 * time.Second,
		DialTimeout:        5 * time.Second,
		ReadTimeout:        3 * time.Second,
		WriteTimeout:       3 * time.Second,
		IdleTimeout:        5 * time.Minute,
		IdleCheckFrequency: time.Minute,
		Logger:             &redis.StdLogger{},
	}

	client, err := redis.New(config)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	fmt.Printf("✓ Redis 单机模式连接成功\n")

	// 健康检查
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Health(ctx); err != nil {
		log.Fatalf("Health check failed: %v", err)
	}

	fmt.Printf("✓ 健康检查通过\n")

	return client
}

// initClusterRedis 初始化集群模式 Redis
func initClusterRedis() *redis.Client {
	config := redis.DefaultClusterConfig([]string{
		"localhost:7000",
		"localhost:7001",
		"localhost:7002",
	})

	// 自定义集群配置
	config.Password = ""
	config.PoolSize = 20
	config.MinIdleConns = 5

	client, err := redis.New(config)
	if err != nil {
		log.Fatalf("Failed to connect to Redis cluster: %v", err)
	}

	fmt.Printf("✓ Redis 集群模式连接成功\n")
	return client
}

// initSentinelRedis 初始化哨兵模式 Redis
func initSentinelRedis() *redis.Client {
	config := &redis.Config{
		Mode:       redis.ModeSentinel,
		MasterName: "mymaster",
		SentinelAddrs: []string{
			"localhost:26379",
			"localhost:26380",
			"localhost:26381",
		},
		Password:     "",
		DB:           0,
		PoolSize:     10,
		MinIdleConns: 2,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}

	client, err := redis.New(config)
	if err != nil {
		log.Fatalf("Failed to connect to Redis sentinel: %v", err)
	}

	fmt.Printf("✓ Redis 哨兵模式连接成功\n")
	return client
}

// demonstrateBasicOps 演示基本操作
func demonstrateBasicOps(client *redis.Client) {
	ctx := context.Background()

	// SET/GET
	fmt.Println("\n  [SET/GET] 基本读写")
	err := client.Set(ctx, "user:1001:name", "Alice", 0).Err()
	if err != nil {
		log.Printf("SET 失败: %v", err)
	} else {
		fmt.Printf("  ✓ SET user:1001:name = Alice\n")
	}

	val, err := client.Get(ctx, "user:1001:name").Result()
	if err != nil {
		log.Printf("GET 失败: %v", err)
	} else {
		fmt.Printf("  ✓ GET user:1001:name = %s\n", val)
	}

	// SET with expiration
	fmt.Println("\n  [SETEX] 设置过期时间")
	err = client.SetWithExpire(ctx, "session:abc123", "user_data", 10*time.Second)
	if err != nil {
		log.Printf("SETEX 失败: %v", err)
	} else {
		fmt.Printf("  ✓ SET session:abc123 (10秒过期)\n")
	}

	// TTL
	ttl, err := client.GetTTL(ctx, "session:abc123")
	if err != nil {
		log.Printf("TTL 失败: %v", err)
	} else {
		fmt.Printf("  ✓ TTL = %v\n", ttl)
	}

	// DELETE
	fmt.Println("\n  [DEL] 删除键")
	err = client.DeleteKeys(ctx, "user:1001:name")
	if err != nil {
		log.Printf("DEL 失败: %v", err)
	} else {
		fmt.Printf("  ✓ 删除 user:1001:name\n")
	}

	// EXISTS
	count, err := client.ExistsCount(ctx, "user:1001:name", "session:abc123")
	if err != nil {
		log.Printf("EXISTS 失败: %v", err)
	} else {
		fmt.Printf("  ✓ EXISTS count = %d\n", count)
	}
}

// demonstrateDataStructures 演示数据结构操作
func demonstrateDataStructures(client *redis.Client) {
	ctx := context.Background()

	// Hash
	fmt.Println("\n  [HASH] 哈希表操作")
	client.HSet(ctx, "user:1001", "name", "Bob")
	client.HSet(ctx, "user:1001", "age", 25)
	client.HSet(ctx, "user:1001", "email", "bob@example.com")
	fmt.Printf("  ✓ HSET user:1001\n")

	name, _ := client.HGet(ctx, "user:1001", "name").Result()
	fmt.Printf("  ✓ HGET user:1001 name = %s\n", name)

	userMap, _ := client.HGetAll(ctx, "user:1001").Result()
	fmt.Printf("  ✓ HGETALL user:1001 = %v\n", userMap)

	// List
	fmt.Println("\n  [LIST] 列表操作")
	client.RPush(ctx, "queue:tasks", "task1", "task2", "task3")
	fmt.Printf("  ✓ RPUSH queue:tasks\n")

	task, _ := client.LPop(ctx, "queue:tasks").Result()
	fmt.Printf("  ✓ LPOP queue:tasks = %s\n", task)

	length, _ := client.LLen(ctx, "queue:tasks").Result()
	fmt.Printf("  ✓ LLEN queue:tasks = %d\n", length)

	// Set
	fmt.Println("\n  [SET] 集合操作")
	client.SAdd(ctx, "tags:golang", "backend", "microservices", "cloud")
	fmt.Printf("  ✓ SADD tags:golang\n")

	isMember, _ := client.SIsMember(ctx, "tags:golang", "backend").Result()
	fmt.Printf("  ✓ SISMEMBER tags:golang backend = %v\n", isMember)

	members, _ := client.SMembers(ctx, "tags:golang").Result()
	fmt.Printf("  ✓ SMEMBERS tags:golang = %v\n", members)

	// Sorted Set
	fmt.Println("\n  [ZSET] 有序集合操作")
	client.ZAdd(ctx, "leaderboard", goredis.Z{Score: 100, Member: "player1"})
	client.ZAdd(ctx, "leaderboard", goredis.Z{Score: 200, Member: "player2"})
	client.ZAdd(ctx, "leaderboard", goredis.Z{Score: 150, Member: "player3"})
	fmt.Printf("  ✓ ZADD leaderboard\n")

	rank, _ := client.ZRank(ctx, "leaderboard", "player2").Result()
	fmt.Printf("  ✓ ZRANK leaderboard player2 = %d\n", rank)

	topPlayers, _ := client.ZRevRangeWithScores(ctx, "leaderboard", 0, 2).Result()
	fmt.Printf("  ✓ ZREVRANGE leaderboard 0 2:\n")
	for _, z := range topPlayers {
		fmt.Printf("    - %s: %.0f\n", z.Member, z.Score)
	}
}

// demonstrateDistributedLock 演示分布式锁
func demonstrateDistributedLock(client *redis.Client) {
	ctx := context.Background()

	// 方式1: 使用封装的 WithLock
	fmt.Println("\n  [方式1] 使用 WithLock 自动管理")
	err := redis.WithLock(ctx, client.UniversalClient, "resource:lock", 10*time.Second, func() error {
		fmt.Printf("  ✓ 获取锁成功，执行业务逻辑...\n")
		time.Sleep(2 * time.Second)
		fmt.Printf("  ✓ 业务逻辑执行完成\n")
		return nil
	})

	if err != nil {
		fmt.Printf("  ✗ 执行失败: %v\n", err)
	} else {
		fmt.Printf("  ✓ 锁已自动释放\n")
	}

	// 方式2: 手动管理锁
	fmt.Println("\n  [方式2] 手动管理锁")
	lock := redis.NewLock(client.UniversalClient, "order:123:lock", 30*time.Second)

	// 获取锁
	if err := lock.Acquire(ctx); err != nil {
		fmt.Printf("  ✗ 获取锁失败: %v\n", err)
		return
	}
	fmt.Printf("  ✓ 获取锁成功\n")

	// 执行业务逻辑
	fmt.Printf("  ✓ 处理订单...\n")
	time.Sleep(1 * time.Second)

	// 刷新锁（延长过期时间）
	if err := lock.Refresh(ctx); err != nil {
		fmt.Printf("  ✗ 刷新锁失败: %v\n", err)
	} else {
		fmt.Printf("  ✓ 锁已刷新\n")
	}

	// 释放锁
	if err := lock.Release(ctx); err != nil {
		fmt.Printf("  ✗ 释放锁失败: %v\n", err)
	} else {
		fmt.Printf("  ✓ 锁已释放\n")
	}

	// 方式3: 带重试的获取锁
	fmt.Println("\n  [方式3] 带重试的锁获取")
	lock2 := redis.NewLock(client.UniversalClient, "critical:section", 10*time.Second)
	err = lock2.AcquireWithRetry(ctx, 500*time.Millisecond, 5)
	if err != nil {
		fmt.Printf("  ✗ 重试后仍未获取锁: %v\n", err)
	} else {
		fmt.Printf("  ✓ 重试后获取锁成功\n")
		defer lock2.Release(ctx)
	}
}

// demonstrateAdvancedOps 演示高级操作
func demonstrateAdvancedOps(client *redis.Client) {
	ctx := context.Background()

	// 1. Pipeline（批量操作）
	fmt.Println("\n  [Pipeline] 批量操作")
	pipe := client.Pipeline()
	pipe.Set(ctx, "key1", "value1", 0)
	pipe.Set(ctx, "key2", "value2", 0)
	pipe.Set(ctx, "key3", "value3", 0)
	pipe.Incr(ctx, "counter")

	cmds, err := pipe.Exec(ctx)
	if err != nil {
		log.Printf("Pipeline 失败: %v", err)
	} else {
		fmt.Printf("  ✓ Pipeline 执行成功，%d 条命令\n", len(cmds))
	}

	// 2. Transaction（事务）
	fmt.Println("\n  [Transaction] 事务操作")
	err = client.Watch(ctx, func(tx *goredis.Tx) error {
		// 读取当前值
		val, err := tx.Get(ctx, "balance").Int()
		if err != nil && err != goredis.Nil {
			return err
		}

		// 修改值
		newVal := val + 100

		// 在事务中执行
		_, err = tx.TxPipelined(ctx, func(pipe goredis.Pipeliner) error {
			pipe.Set(ctx, "balance", newVal, 0)
			return nil
		})

		return err
	}, "balance")

	if err != nil {
		fmt.Printf("  ✗ 事务失败: %v\n", err)
	} else {
		fmt.Printf("  ✓ 事务执行成功\n")
	}

	// 3. Pub/Sub（发布订阅）
	fmt.Println("\n  [Pub/Sub] 发布订阅")
	pubsub := client.Subscribe(ctx, "notifications")
	defer pubsub.Close()

	// 发布消息
	client.Publish(ctx, "notifications", "New message!")
	fmt.Printf("  ✓ 发布消息到 notifications 频道\n")

	// 接收消息（设置超时）
	go func() {
		msgCh := pubsub.Channel()
		select {
		case msg := <-msgCh:
			fmt.Printf("  ✓ 收到消息: %s\n", msg.Payload)
		case <-time.After(2 * time.Second):
			fmt.Printf("  ✓ 超时未收到消息\n")
		}
	}()

	time.Sleep(3 * time.Second)

	// 4. Scan（遍历键）
	fmt.Println("\n  [Scan] 遍历键")
	var cursor uint64
	var keys []string
	for {
		var scanKeys []string
		var err error
		scanKeys, cursor, err = client.Scan(ctx, cursor, "user:*", 10).Result()
		if err != nil {
			log.Printf("Scan 失败: %v", err)
			break
		}
		keys = append(keys, scanKeys...)
		if cursor == 0 {
			break
		}
	}
	fmt.Printf("  ✓ 找到 %d 个 user:* 键\n", len(keys))
}

// monitorRedisPool 连接池监控
func monitorRedisPool(client *redis.Client) {
	stats := client.Stats()

	fmt.Printf("\n  连接池状态:\n")
	fmt.Printf("  - 总连接数: %d\n", stats.TotalConns)
	fmt.Printf("  - 空闲连接: %d\n", stats.IdleConns)
	fmt.Printf("  - 过期连接: %d\n", stats.StaleConns)
	fmt.Printf("  - 命中次数: %d\n", stats.Hits)
	fmt.Printf("  - 未命中次数: %d\n", stats.Misses)
	fmt.Printf("  - 超时次数: %d\n", stats.Timeouts)

	// 健康检查
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Health(ctx); err != nil {
		fmt.Printf("  ✗ 健康检查失败: %v\n", err)
	} else {
		fmt.Printf("  ✓ 健康检查通过\n")
	}
}
