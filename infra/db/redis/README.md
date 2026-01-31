# Redis 客户端封装

生产级 Redis 客户端封装，支持单机、集群、哨兵模式，以及分布式锁等功能。

## 特性

- ✅ 多种模式 - 支持单机、集群、哨兵
- ✅ 单例模式 - 全局统一实例
- ✅ 连接池管理 - 自动管理连接
- ✅ 健康检查 - Ping 检测连接状态
- ✅ 分布式锁 - 基于 Redis 的分布式锁实现
- ✅ 常用操作 - 封装常用的 Redis 命令
- ✅ Pipeline 支持 - 批量操作优化
- ✅ 日志接口 - 可插拔的日志系统

## 快速开始

### 1. 单机模式

```go
package main

import (
    "context"
    "github.com/everyday-items/toolkit/infra/db/redis"
)

func main() {
    // 使用默认配置
    config := redis.DefaultConfig("localhost:6379")
    config.Password = "your_password" // 可选

    // 初始化全局实例
    client, err := redis.Init(config)
    if err != nil {
        panic(err)
    }
    defer client.Close()

    // 使用客户端
    ctx := context.Background()
    err = client.Set(ctx, "key", "value", 0).Err()
}
```

### 2. 集群模式

```go
config := redis.DefaultClusterConfig([]string{
    "localhost:7001",
    "localhost:7002",
    "localhost:7003",
})
config.Password = "your_password"

client, err := redis.Init(config)
```

### 3. 哨兵模式

```go
config := &redis.Config{
    Mode:          redis.ModeSentinel,
    MasterName:    "mymaster",
    SentinelAddrs: []string{"localhost:26379", "localhost:26380"},
    Password:      "your_password",
    DB:            0,
}

client, err := redis.Init(config)
```

## 使用示例

### 基础操作

```go
ctx := context.Background()
client := redis.GetGlobal()

// Set
err := client.Set(ctx, "name", "Alice", time.Minute).Err()

// Get
val, err := client.Get(ctx, "name").Result()

// Get with default
val := client.GetWithDefault(ctx, "name", "default")

// Delete
err := client.Delete(ctx, "name")

// Exists
exists, err := client.Exists(ctx, "key1", "key2")

// Expire
err := client.Expire(ctx, "key", time.Hour).Err()

// TTL
ttl, err := client.TTL(ctx, "key")
```

### 批量操作

```go
// MGet
values, err := client.MGet(ctx, "key1", "key2", "key3")

// MSet
err := client.MSet(ctx, "key1", "value1", "key2", "value2")

// Pipeline
pipe := client.Pipeline()
pipe.Set(ctx, "key1", "value1", 0)
pipe.Set(ctx, "key2", "value2", 0)
pipe.Incr(ctx, "counter")
_, err := pipe.Exec(ctx)
```

### 计数器

```go
// Incr
newVal, err := client.Incr(ctx, "counter").Result()

// IncrBy
newVal, err := client.IncrBy(ctx, "counter", 10).Result()

// IncrBy with expiration
newVal, err := client.IncrByWithExpire(ctx, "counter", 1, time.Hour)

// Decr
newVal, err := client.Decr(ctx, "counter").Result()
```

### Hash 操作

```go
// HSet
err := client.HSet(ctx, "user:1", "name", "Alice").Err()

// HGet
val, err := client.HGet(ctx, "user:1", "name").Result()

// HGetAll
fields, err := client.HGetAll(ctx, "user:1").Result()

// HMSet
err := client.HMSet(ctx, "user:1", map[string]any{
    "name": "Alice",
    "age":  25,
}).Err()
```

### List 操作

```go
// LPush
err := client.LPush(ctx, "queue", "task1", "task2").Err()

// RPush
err := client.RPush(ctx, "queue", "task3").Err()

// LPop
val, err := client.LPop(ctx, "queue").Result()

// BRPop (阻塞)
vals, err := client.BRPop(ctx, 5*time.Second, "queue").Result()

// LLen
length, err := client.LLen(ctx, "queue").Result()
```

### Set 操作

```go
// SAdd
err := client.SAdd(ctx, "tags", "go", "redis", "database").Err()

// SMembers
members, err := client.SMembers(ctx, "tags").Result()

// SIsMember
isMember, err := client.SIsMember(ctx, "tags", "go").Result()

// SCard
count, err := client.SCard(ctx, "tags").Result()
```

### Sorted Set 操作

```go
// ZAdd
err := client.ZAdd(ctx, "leaderboard",
    redis.Z{Score: 100, Member: "Alice"},
    redis.Z{Score: 90, Member: "Bob"},
).Err()

// ZRange (从低到高)
vals, err := client.ZRange(ctx, "leaderboard", 0, -1).Result()

// ZRevRange (从高到低)
vals, err := client.ZRevRange(ctx, "leaderboard", 0, 10).Result()

// ZScore
score, err := client.ZScore(ctx, "leaderboard", "Alice").Result()
```

## 分布式锁

### 基础用法

```go
ctx := context.Background()
client := redis.GetGlobal()

// 创建锁
lock := redis.NewLock(client, "lock:resource", 30*time.Second)

// 获取锁
err := lock.Acquire(ctx)
if err == redis.ErrLockFailed {
    // 锁已被占用
    return
}

// 执行业务逻辑
// ...

// 释放锁
defer lock.Release(ctx)
```

### 带重试

```go
lock := redis.NewLock(client, "lock:resource", 30*time.Second)

// 每100ms重试一次，最多重试10次
err := lock.AcquireWithRetry(ctx, 100*time.Millisecond, 10)
if err != nil {
    return err
}
defer lock.Release(ctx)

// 执行业务逻辑
```

### 自动获取和释放

```go
err := redis.WithLock(ctx, client, "lock:resource", 30*time.Second, func() error {
    // 在这里执行需要加锁的操作
    // 锁会自动获取和释放
    return nil
})
```

### 刷新锁

```go
lock := redis.NewLock(client, "lock:resource", 30*time.Second)
lock.Acquire(ctx)
defer lock.Release(ctx)

// 长时间操作，需要刷新锁
for {
    // 执行部分工作

    // 刷新锁的过期时间
    if err := lock.Refresh(ctx); err != nil {
        return err
    }
}
```

## 健康检查

```go
ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()

if err := client.Health(ctx); err != nil {
    log.Printf("Redis unhealthy: %v", err)
}
```

## 连接池统计

```go
stats := client.Stats()
if stats != nil {
    fmt.Printf("Hits: %d\n", stats.Hits)
    fmt.Printf("Misses: %d\n", stats.Misses)
    fmt.Printf("Timeouts: %d\n", stats.Timeouts)
    fmt.Printf("TotalConns: %d\n", stats.TotalConns)
    fmt.Printf("IdleConns: %d\n", stats.IdleConns)
    fmt.Printf("StaleConns: %d\n", stats.StaleConns)
}
```

## 配置说明

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `Mode` | Mode | single | 运行模式（single/cluster/sentinel） |
| `Addr` | string | - | 单机地址 (host:port) |
| `Password` | string | "" | 密码 |
| `DB` | int | 0 | 数据库编号 (0-15) |
| `Addrs` | []string | - | 集群节点地址 |
| `PoolSize` | int | 10 | 连接池大小 |
| `MinIdleConns` | int | 2 | 最小空闲连接数 |
| `MaxRetries` | int | 3 | 最大重试次数 |
| `DialTimeout` | Duration | 5s | 连接超时 |
| `ReadTimeout` | Duration | 3s | 读超时 |
| `WriteTimeout` | Duration | 3s | 写超时 |
| `IdleTimeout` | Duration | 5m | 空闲连接超时 |

## 最佳实践

### 1. 使用单例模式

```go
// 初始化一次
func init() {
    config := redis.DefaultConfig(os.Getenv("REDIS_ADDR"))
    if _, err := redis.Init(config); err != nil {
        log.Fatal(err)
    }
}

// 全局使用
func GetUser(id int) (*User, error) {
    client := redis.GetGlobal()
    // ...
}
```

### 2. 合理设置过期时间

```go
// ✅ 始终设置过期时间，避免内存泄漏
client.Set(ctx, "key", "value", time.Hour)

// ❌ 不设置过期时间可能导致内存泄漏
client.Set(ctx, "key", "value", 0)
```

### 3. 使用 Pipeline 批量操作

```go
// ✅ 使用 Pipeline
pipe := client.Pipeline()
for i := 0; i < 1000; i++ {
    pipe.Set(ctx, fmt.Sprintf("key:%d", i), i, 0)
}
pipe.Exec(ctx)

// ❌ 逐个执行（慢）
for i := 0; i < 1000; i++ {
    client.Set(ctx, fmt.Sprintf("key:%d", i), i, 0)
}
```

### 4. 处理 nil 值

```go
val, err := client.Get(ctx, "key").Result()
if err == redis.Nil {
    // key 不存在
    return "default"
} else if err != nil {
    // 其他错误
    return err
}
```

### 5. 分布式锁注意事项

```go
// ✅ 正确：始终释放锁
lock.Acquire(ctx)
defer lock.Release(ctx)

// ✅ 正确：设置合理的过期时间（防止死锁）
lock := redis.NewLock(client, "lock:key", 30*time.Second)

// ❌ 错误：忘记释放锁
lock.Acquire(ctx)
// 没有 defer lock.Release(ctx)
```

## 依赖

```bash
go get -u github.com/redis/go-redis/v9
```

## 注意事项

1. **连接数限制**：PoolSize 不应超过 Redis 的 maxclients
2. **超时设置**：根据业务需求合理设置超时时间
3. **Key 命名**：使用命名空间（如 `user:1:profile`）
4. **大 key 问题**：避免存储过大的值（建议 < 10KB）
5. **热 key 问题**：高并发访问的 key 需要特殊处理
6. **过期时间**：始终设置过期时间，避免内存泄漏
7. **分布式锁**：注意锁的过期时间要大于业务执行时间
