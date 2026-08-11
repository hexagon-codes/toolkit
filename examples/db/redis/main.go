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

	"github.com/hexagon-codes/toolkit/infra/db/redis"
	goredis "github.com/redis/go-redis/v9"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() (resultErr error) {
	addr, redisConfigured := configuredRedisAddress()
	if !redisConfigured {
		fmt.Println("Set REDIS_ADDR to run this example.")
		return nil
	}

	fmt.Println("=== Redis 使用示例 ===")

	// 1. 单机模式
	fmt.Println("1. 单机模式连接")
	singleClient, err := initSingleRedis(addr)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, singleClient.Close())
	}()

	// 2. 集群模式（示例）
	fmt.Println("\n2. 集群模式连接")
	fmt.Println("Set REDIS_CLUSTER_ADDRS to extend this example with cluster mode.")

	// 3. 哨兵模式（示例）
	fmt.Println("\n3. 哨兵模式连接")
	fmt.Println("Set REDIS_SENTINEL_ADDRS and REDIS_SENTINEL_MASTER to extend this example with sentinel mode.")

	// 4. 基本操作
	fmt.Println("\n4. 基本操作")
	if err := demonstrateBasicOps(singleClient); err != nil {
		return err
	}

	// 5. 数据结构操作
	fmt.Println("\n5. 数据结构操作")
	if err := demonstrateDataStructures(singleClient); err != nil {
		return err
	}

	// 6. 分布式锁使用
	fmt.Println("\n6. 分布式锁")
	if err := demonstrateDistributedLock(singleClient); err != nil {
		return err
	}

	// 7. 高级操作
	fmt.Println("\n7. 高级操作")
	if err := demonstrateAdvancedOps(singleClient); err != nil {
		return err
	}

	// 8. 连接池监控
	fmt.Println("\n8. 连接池监控")
	if err := monitorRedisPool(singleClient); err != nil {
		return err
	}
	return nil
}

func configuredRedisAddress() (string, bool) {
	addr := strings.TrimSpace(os.Getenv("REDIS_ADDR"))
	return addr, addr != ""
}

// initSingleRedis 初始化单机模式 Redis
func initSingleRedis(addr string) (*redis.Client, error) {
	config := redis.DefaultConfig(redis.ModeSingle, addr)
	config.DataCredentials = dataCredentials()
	config.TLSConfig = redisTLSConfig()

	connectCtx, cancelConnect := context.WithTimeout(context.Background(), 5*time.Second)
	client, err := redis.New(connectCtx, config)
	cancelConnect()
	if err != nil {
		return nil, fmt.Errorf("connect to Redis: %w", err)
	}

	fmt.Printf("✓ Redis 单机模式连接成功\n")

	// 健康检查
	healthCtx, cancelHealth := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelHealth()
	if err := client.Health(healthCtx); err != nil {
		return nil, errors.Join(fmt.Errorf("check Redis health: %w", err), client.Close())
	}

	fmt.Printf("✓ 健康检查通过\n")

	return client, nil
}

// initClusterRedis 使用调用方显式提供的地址初始化集群模式 Redis。
func initClusterRedis(addresses []string) (*redis.Client, error) {
	if len(addresses) == 0 {
		return nil, fmt.Errorf("Redis cluster addresses are required")
	}
	config := redis.DefaultConfig(redis.ModeCluster, addresses...)

	config.DataCredentials = dataCredentials()
	config.TLSConfig = redisTLSConfig()
	config.PoolSize = 20
	config.MinIdleConns = 5

	connectCtx, cancelConnect := context.WithTimeout(context.Background(), 5*time.Second)
	client, err := redis.New(connectCtx, config)
	cancelConnect()
	if err != nil {
		return nil, fmt.Errorf("connect to Redis cluster: %w", err)
	}

	fmt.Printf("✓ Redis 集群模式连接成功\n")
	return client, nil
}

// initSentinelRedis 使用调用方显式提供的地址和主节点名初始化哨兵模式 Redis。
func initSentinelRedis(addresses []string, masterName string) (*redis.Client, error) {
	if len(addresses) == 0 {
		return nil, fmt.Errorf("Redis sentinel addresses are required")
	}
	if strings.TrimSpace(masterName) == "" {
		return nil, fmt.Errorf("Redis sentinel master name is required")
	}
	config := redis.DefaultConfig(redis.ModeSentinel, addresses...)
	config.MasterName = masterName
	config.DataCredentials = dataCredentials()
	config.SentinelCredentials = redis.Credentials{
		Username: os.Getenv("REDIS_SENTINEL_USERNAME"),
		Password: os.Getenv("REDIS_SENTINEL_PASSWORD"),
	}
	config.TLSConfig = redisTLSConfig()

	connectCtx, cancelConnect := context.WithTimeout(context.Background(), 5*time.Second)
	client, err := redis.New(connectCtx, config)
	cancelConnect()
	if err != nil {
		return nil, fmt.Errorf("connect to Redis sentinel: %w", err)
	}

	fmt.Printf("✓ Redis 哨兵模式连接成功\n")
	return client, nil
}

func dataCredentials() redis.Credentials {
	return redis.Credentials{
		Username: os.Getenv("REDIS_USERNAME"),
		Password: os.Getenv("REDIS_PASSWORD"),
	}
}

func redisTLSConfig() *tls.Config {
	serverName := os.Getenv("REDIS_TLS_SERVER_NAME")
	if serverName == "" {
		return nil
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName}
}

// demonstrateBasicOps 演示基本操作
func demonstrateBasicOps(client *redis.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// SET/GET
	fmt.Println("\n  [SET/GET] 基本读写")
	err := client.Set(ctx, "user:1001:name", "Alice", 0).Err()
	if err != nil {
		return fmt.Errorf("set user name: %w", err)
	}
	fmt.Printf("  ✓ SET user:1001:name = Alice\n")

	val, err := client.Get(ctx, "user:1001:name").Result()
	if err != nil {
		return fmt.Errorf("get user name: %w", err)
	}
	fmt.Printf("  ✓ GET user:1001:name = %s\n", val)

	// SET with expiration
	fmt.Println("\n  [SETEX] 设置过期时间")
	err = client.SetWithExpire(ctx, "session:abc123", "user_data", 10*time.Second)
	if err != nil {
		return fmt.Errorf("set expiring session: %w", err)
	}
	fmt.Printf("  ✓ SET session:abc123 (expires in 10 seconds)\n")

	// TTL
	ttl, err := client.GetTTL(ctx, "session:abc123")
	if err != nil {
		return fmt.Errorf("get session TTL: %w", err)
	}
	fmt.Printf("  ✓ TTL = %v\n", ttl)

	// DELETE
	fmt.Println("\n  [DEL] 删除键")
	err = client.DeleteKeys(ctx, "user:1001:name")
	if err != nil {
		return fmt.Errorf("delete user name: %w", err)
	}
	fmt.Printf("  ✓ Deleted user:1001:name\n")

	// EXISTS
	count, err := client.ExistsCount(ctx, "user:1001:name", "session:abc123")
	if err != nil {
		return fmt.Errorf("count existing keys: %w", err)
	}
	fmt.Printf("  ✓ EXISTS count = %d\n", count)
	return nil
}

// demonstrateDataStructures 演示数据结构操作
func demonstrateDataStructures(client *redis.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Hash
	fmt.Println("\n  [HASH] 哈希表操作")
	if err := client.HSet(ctx, "user:1001", "name", "Bob", "age", 25, "email", "bob@example.com").Err(); err != nil {
		return fmt.Errorf("set user hash: %w", err)
	}
	fmt.Printf("  ✓ HSET user:1001\n")

	name, err := client.HGet(ctx, "user:1001", "name").Result()
	if err != nil {
		return fmt.Errorf("get user hash name: %w", err)
	}
	fmt.Printf("  ✓ HGET user:1001 name = %s\n", name)

	userMap, err := client.HGetAll(ctx, "user:1001").Result()
	if err != nil {
		return fmt.Errorf("get user hash: %w", err)
	}
	fmt.Printf("  ✓ HGETALL user:1001 = %v\n", userMap)

	// List
	fmt.Println("\n  [LIST] 列表操作")
	if err := client.RPush(ctx, "queue:tasks", "task1", "task2", "task3").Err(); err != nil {
		return fmt.Errorf("push task queue: %w", err)
	}
	fmt.Printf("  ✓ RPUSH queue:tasks\n")

	task, err := client.LPop(ctx, "queue:tasks").Result()
	if err != nil {
		return fmt.Errorf("pop task queue: %w", err)
	}
	fmt.Printf("  ✓ LPOP queue:tasks = %s\n", task)

	length, err := client.LLen(ctx, "queue:tasks").Result()
	if err != nil {
		return fmt.Errorf("read task queue length: %w", err)
	}
	fmt.Printf("  ✓ LLEN queue:tasks = %d\n", length)

	// Set
	fmt.Println("\n  [SET] 集合操作")
	if err := client.SAdd(ctx, "tags:golang", "backend", "microservices", "cloud").Err(); err != nil {
		return fmt.Errorf("add Go tags: %w", err)
	}
	fmt.Printf("  ✓ SADD tags:golang\n")

	isMember, err := client.SIsMember(ctx, "tags:golang", "backend").Result()
	if err != nil {
		return fmt.Errorf("check Go tag membership: %w", err)
	}
	fmt.Printf("  ✓ SISMEMBER tags:golang backend = %v\n", isMember)

	members, err := client.SMembers(ctx, "tags:golang").Result()
	if err != nil {
		return fmt.Errorf("list Go tags: %w", err)
	}
	fmt.Printf("  ✓ SMEMBERS tags:golang = %v\n", members)

	// Sorted Set
	fmt.Println("\n  [ZSET] 有序集合操作")
	if err := client.ZAdd(ctx, "leaderboard",
		goredis.Z{Score: 100, Member: "player1"},
		goredis.Z{Score: 200, Member: "player2"},
		goredis.Z{Score: 150, Member: "player3"},
	).Err(); err != nil {
		return fmt.Errorf("add leaderboard entries: %w", err)
	}
	fmt.Printf("  ✓ ZADD leaderboard\n")

	rank, err := client.ZRank(ctx, "leaderboard", "player2").Result()
	if err != nil {
		return fmt.Errorf("read leaderboard rank: %w", err)
	}
	fmt.Printf("  ✓ ZRANK leaderboard player2 = %d\n", rank)

	topPlayers, err := client.ZRevRangeWithScores(ctx, "leaderboard", 0, 2).Result()
	if err != nil {
		return fmt.Errorf("read leaderboard: %w", err)
	}
	fmt.Printf("  ✓ ZREVRANGE leaderboard 0 2:\n")
	for _, z := range topPlayers {
		fmt.Printf("    - %s: %.0f\n", z.Member, z.Score)
	}
	return nil
}

// demonstrateDistributedLock 演示分布式锁
func demonstrateDistributedLock(client *redis.Client) (resultErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 方式1: 使用封装的 WithLock
	fmt.Println("\n  [方式1] 使用 WithLock 自动管理")
	err := redis.WithLock(ctx, client.UniversalClient, "resource:lock", 10*time.Second, func() error {
		fmt.Printf("  ✓ 获取锁成功，执行业务逻辑...\n")
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
		fmt.Printf("  ✓ 业务逻辑执行完成\n")
		return nil
	})

	if err != nil {
		return fmt.Errorf("execute managed Redis lock: %w", err)
	}
	fmt.Printf("  ✓ Managed lock released\n")

	// 方式2: 手动管理锁
	fmt.Println("\n  [方式2] 手动管理锁")
	lock := redis.NewLock(client.UniversalClient, "order:123:lock", 30*time.Second)

	// 获取锁
	if err := lock.Acquire(ctx); err != nil {
		return fmt.Errorf("acquire order lock: %w", err)
	}
	lockHeld := true
	defer func() {
		if lockHeld {
			resultErr = errors.Join(resultErr, releaseRedisLock(lock))
		}
	}()
	fmt.Printf("  ✓ 获取锁成功\n")

	// 执行业务逻辑
	fmt.Printf("  ✓ 处理订单...\n")
	orderTimer := time.NewTimer(time.Second)
	defer orderTimer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("process order under lock: %w", ctx.Err())
	case <-orderTimer.C:
	}

	// 刷新锁（延长过期时间）
	if err := lock.Refresh(ctx); err != nil {
		return fmt.Errorf("refresh order lock: %w", err)
	}
	fmt.Printf("  ✓ Lock refreshed\n")

	// 释放锁
	if err := lock.Release(ctx); err != nil {
		return fmt.Errorf("release order lock: %w", err)
	}
	lockHeld = false
	fmt.Printf("  ✓ Lock released\n")

	// 方式3: 带重试的获取锁
	fmt.Println("\n  [方式3] 带重试的锁获取")
	lock2 := redis.NewLock(client.UniversalClient, "critical:section", 10*time.Second)
	err = lock2.AcquireWithRetry(ctx, 500*time.Millisecond, 5)
	if err != nil {
		return fmt.Errorf("acquire critical-section lock: %w", err)
	}
	fmt.Printf("  ✓ Lock acquired after retry\n")
	defer func() {
		resultErr = errors.Join(resultErr, releaseRedisLock(lock2))
	}()
	return nil
}

func releaseRedisLock(lock *redis.Lock) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := lock.Release(ctx); err != nil {
		return fmt.Errorf("release Redis lock: %w", err)
	}
	return nil
}

// demonstrateAdvancedOps 演示高级操作
func demonstrateAdvancedOps(client *redis.Client) (resultErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Pipeline（批量操作）
	fmt.Println("\n  [Pipeline] 批量操作")
	pipe := client.Pipeline()
	pipe.Set(ctx, "key1", "value1", 0)
	pipe.Set(ctx, "key2", "value2", 0)
	pipe.Set(ctx, "key3", "value3", 0)
	pipe.Incr(ctx, "counter")

	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("execute Redis pipeline: %w", err)
	}
	fmt.Printf("  ✓ Pipeline executed with %d commands\n", len(cmds))

	// 2. Transaction（事务）
	fmt.Println("\n  [Transaction] 事务操作")
	err = client.Watch(ctx, func(tx *goredis.Tx) error {
		// 读取当前值
		val, err := tx.Get(ctx, "balance").Int()
		if err != nil && !errors.Is(err, goredis.Nil) {
			return fmt.Errorf("read watched balance: %w", err)
		}

		// 修改值
		newVal := val + 100

		// 在事务中执行
		_, err = tx.TxPipelined(ctx, func(pipe goredis.Pipeliner) error {
			pipe.Set(ctx, "balance", newVal, 0)
			return nil
		})

		if err != nil {
			return fmt.Errorf("update watched balance: %w", err)
		}
		return nil
	}, "balance")

	if err != nil {
		return fmt.Errorf("execute Redis transaction: %w", err)
	}
	fmt.Printf("  ✓ Transaction committed\n")

	// 3. Pub/Sub（发布订阅）
	fmt.Println("\n  [Pub/Sub] 发布订阅")
	pubsub := client.Subscribe(ctx, "notifications")
	defer func() {
		resultErr = errors.Join(resultErr, pubsub.Close())
	}()
	if _, err := pubsub.Receive(ctx); err != nil {
		return fmt.Errorf("establish Redis subscription: %w", err)
	}

	// 发布消息
	if err := client.Publish(ctx, "notifications", "New message!").Err(); err != nil {
		return fmt.Errorf("publish Redis notification: %w", err)
	}
	fmt.Printf("  ✓ 发布消息到 notifications 频道\n")

	// 接收消息（设置超时）
	receiveTimer := time.NewTimer(2 * time.Second)
	defer receiveTimer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("receive Redis notification: %w", ctx.Err())
	case <-receiveTimer.C:
		return fmt.Errorf("receive Redis notification: timeout")
	case msg, ok := <-pubsub.Channel():
		if !ok {
			return fmt.Errorf("receive Redis notification: subscription closed")
		}
		fmt.Printf("  ✓ Received message: %s\n", msg.Payload)
	}

	// 4. Scan（遍历键）
	fmt.Println("\n  [Scan] 遍历键")
	var cursor uint64
	var keys []string
	for {
		var scanKeys []string
		var err error
		scanKeys, cursor, err = client.Scan(ctx, cursor, "user:*", 10).Result()
		if err != nil {
			return fmt.Errorf("scan Redis user keys: %w", err)
		}
		keys = append(keys, scanKeys...)
		if cursor == 0 {
			break
		}
	}
	fmt.Printf("  ✓ 找到 %d 个 user:* 键\n", len(keys))
	return nil
}

// monitorRedisPool 连接池监控
func monitorRedisPool(client *redis.Client) error {
	stats := client.Stats()
	if stats == nil {
		return fmt.Errorf("read Redis pool stats: client is closed")
	}

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
		return fmt.Errorf("check Redis pool health: %w", err)
	}
	fmt.Printf("  ✓ Health check passed\n")
	return nil
}
