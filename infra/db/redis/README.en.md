[中文](README.md) | English

# Redis Client Wrapper

Production-grade Redis client wrapper supporting standalone, cluster, and sentinel modes, as well as distributed lock and more.

## Features

- ✅ Multiple modes - supports standalone, cluster, sentinel
- ✅ Singleton pattern - global unified instance
- ✅ Connection pool management - automatic connection management
- ✅ Health check - Ping to detect connection state
- ✅ Distributed lock - Redis-based distributed lock implementation
- ✅ Common operations - encapsulates common Redis commands
- ✅ Pipeline support - batch operation optimization
- ✅ Logger interface - pluggable logging system

## Quick Start

### 1. Standalone Mode

```go
package main

import (
    "context"
    "github.com/everyday-items/toolkit/infra/db/redis"
)

func main() {
    // Use default configuration
    config := redis.DefaultConfig("localhost:6379")
    config.Password = "your_password" // optional

    // Initialize global instance
    client, err := redis.Init(config)
    if err != nil {
        panic(err)
    }
    defer client.Close()

    // Use client
    ctx := context.Background()
    err = client.Set(ctx, "key", "value", 0).Err()
}
```

### 2. Cluster Mode

```go
config := redis.DefaultClusterConfig([]string{
    "localhost:7001",
    "localhost:7002",
    "localhost:7003",
})
config.Password = "your_password"

client, err := redis.Init(config)
```

### 3. Sentinel Mode

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

## Usage Examples

### Basic Operations

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

### Batch Operations

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

### Counter

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

### Hash Operations

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

### List Operations

```go
// LPush
err := client.LPush(ctx, "queue", "task1", "task2").Err()

// RPush
err := client.RPush(ctx, "queue", "task3").Err()

// LPop
val, err := client.LPop(ctx, "queue").Result()

// BRPop (blocking)
vals, err := client.BRPop(ctx, 5*time.Second, "queue").Result()

// LLen
length, err := client.LLen(ctx, "queue").Result()
```

### Set Operations

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

### Sorted Set Operations

```go
// ZAdd
err := client.ZAdd(ctx, "leaderboard",
    redis.Z{Score: 100, Member: "Alice"},
    redis.Z{Score: 90, Member: "Bob"},
).Err()

// ZRange (ascending)
vals, err := client.ZRange(ctx, "leaderboard", 0, -1).Result()

// ZRevRange (descending)
vals, err := client.ZRevRange(ctx, "leaderboard", 0, 10).Result()

// ZScore
score, err := client.ZScore(ctx, "leaderboard", "Alice").Result()
```

## Distributed Lock

### Basic Usage

```go
ctx := context.Background()
client := redis.GetGlobal()

// Create lock
lock := redis.NewLock(client, "lock:resource", 30*time.Second)

// Acquire lock
err := lock.Acquire(ctx)
if err == redis.ErrLockFailed {
    // Lock is already held
    return
}

// Execute business logic
// ...

// Release lock
defer lock.Release(ctx)
```

### With Retry

```go
lock := redis.NewLock(client, "lock:resource", 30*time.Second)

// Retry every 100ms, up to 10 times
err := lock.AcquireWithRetry(ctx, 100*time.Millisecond, 10)
if err != nil {
    return err
}
defer lock.Release(ctx)

// Execute business logic
```

### Auto Acquire and Release

```go
err := redis.WithLock(ctx, client, "lock:resource", 30*time.Second, func() error {
    // Execute locked operations here
    // Lock is acquired and released automatically
    return nil
})
```

### Refresh Lock

```go
lock := redis.NewLock(client, "lock:resource", 30*time.Second)
lock.Acquire(ctx)
defer lock.Release(ctx)

// Long operation, need to refresh lock
for {
    // Do partial work

    // Refresh lock expiration
    if err := lock.Refresh(ctx); err != nil {
        return err
    }
}
```

## Health Check

```go
ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()

if err := client.Health(ctx); err != nil {
    log.Printf("Redis unhealthy: %v", err)
}
```

## Connection Pool Statistics

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

## Configuration Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Mode` | Mode | single | Operation mode (single/cluster/sentinel) |
| `Addr` | string | - | Standalone address (host:port) |
| `Password` | string | "" | Password |
| `DB` | int | 0 | Database number (0-15) |
| `Addrs` | []string | - | Cluster node addresses |
| `PoolSize` | int | 10 | Connection pool size |
| `MinIdleConns` | int | 2 | Minimum idle connections |
| `MaxRetries` | int | 3 | Maximum retry count |
| `DialTimeout` | Duration | 5s | Connection timeout |
| `ReadTimeout` | Duration | 3s | Read timeout |
| `WriteTimeout` | Duration | 3s | Write timeout |
| `IdleTimeout` | Duration | 5m | Idle connection timeout |

## Best Practices

### 1. Use Singleton Pattern

```go
// Initialize once
func init() {
    config := redis.DefaultConfig(os.Getenv("REDIS_ADDR"))
    if _, err := redis.Init(config); err != nil {
        log.Fatal(err)
    }
}

// Use globally
func GetUser(id int) (*User, error) {
    client := redis.GetGlobal()
    // ...
}
```

### 2. Always Set Expiration

```go
// ✅ Always set expiration to avoid memory leaks
client.Set(ctx, "key", "value", time.Hour)

// ❌ No expiration may cause memory leaks
client.Set(ctx, "key", "value", 0)
```

### 3. Use Pipeline for Batch Operations

```go
// ✅ Use Pipeline
pipe := client.Pipeline()
for i := 0; i < 1000; i++ {
    pipe.Set(ctx, fmt.Sprintf("key:%d", i), i, 0)
}
pipe.Exec(ctx)

// ❌ Execute one by one (slow)
for i := 0; i < 1000; i++ {
    client.Set(ctx, fmt.Sprintf("key:%d", i), i, 0)
}
```

### 4. Handle nil Values

```go
val, err := client.Get(ctx, "key").Result()
if err == redis.Nil {
    // key does not exist
    return "default"
} else if err != nil {
    // other error
    return err
}
```

### 5. Distributed Lock Considerations

```go
// ✅ Correct: always release the lock
lock.Acquire(ctx)
defer lock.Release(ctx)

// ✅ Correct: set reasonable expiration (prevent deadlock)
lock := redis.NewLock(client, "lock:key", 30*time.Second)

// ❌ Wrong: forgot to release the lock
lock.Acquire(ctx)
// missing defer lock.Release(ctx)
```

## Dependencies

```bash
go get -u github.com/redis/go-redis/v9
```

## Notes

1. **Connection limit**: PoolSize should not exceed Redis's maxclients
2. **Timeout settings**: Set timeouts appropriately based on business requirements
3. **Key naming**: Use namespaces (e.g., `user:1:profile`)
4. **Large key problem**: Avoid storing overly large values (recommended < 10KB)
5. **Hot key problem**: Keys with high concurrent access need special handling
6. **Expiration time**: Always set expiration to avoid memory leaks
7. **Distributed lock**: Ensure lock expiration is longer than business execution time
