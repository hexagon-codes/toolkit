[中文](README.md) | English

# Cache - General Cache Library

A general-purpose cache library supporting local cache, Redis cache, and multi-level cache, with both stable-key and unstable-key caching strategies.

## Features

- **Local Cache (cache/local)**: In-memory cache based on sync.Map, with LRU eviction and periodic cleanup
- **Redis Cache (cache/redis)**: Distributed cache based on Redis
  - **StableCache**: Stable-key cache, suitable for single-record queries
  - **UnstableCache**: Unstable-key cache, suitable for aggregate queries, JOINs, and list queries
- **Multi-Level Cache (cache/multi)** ⭐ **NEW**: Automatically handles Local + Redis + DB three-layer caching
  - Automatic layer-by-layer querying and backfilling
  - Unified invalidation management
  - Builder pattern, ready to use out of the box
- **Anti-breakdown**: Uses singleflight to prevent cache breakdown
- **Anti-penetration**: Supports negative caching (caching empty results)
- **Anti-avalanche**: TTL jitter mechanism
- **Extensible**: Supports custom serializers (Codec)

## Installation

```bash
go get github.com/hexagon-codes/toolkit/cache
```

## Quick Start

Create and probe the Redis connection once with `infra/redisconn.Factory.Open` during startup, then inject it into the cache layer. Cache packages do not copy or reconstruct authentication settings. Static authentication must be either completely empty or a complete username/password pair; this project intentionally rejects password-only configuration.

### Option 1: Multi-Level Cache (Recommended) ⭐

The simplest way, automatically handles Local + Redis + DB three-layer caching:

```go
package main

import (
    "context"
    "crypto/tls"
    "os"
    "time"

    "github.com/hexagon-codes/toolkit/cache/local"
    "github.com/hexagon-codes/toolkit/cache/multi"
    "github.com/hexagon-codes/toolkit/cache/redis"
    "github.com/hexagon-codes/toolkit/infra/redisconn"
)

type User struct {
    ID int
}

func main() {
    // 1. Create each cache layer
    localCache := local.NewCache(1000)
    defer localCache.Stop()
    ctx := context.Background()
    connection := redisconn.DefaultConfig(redisconn.ModeSingle, os.Getenv("REDIS_ADDR"))
    connection.DataCredentials = redisconn.Credentials{
        Username: os.Getenv("REDIS_USERNAME"),
        Password: os.Getenv("REDIS_PASSWORD"),
    }
    if serverName := os.Getenv("REDIS_TLS_SERVER_NAME"); serverName != "" {
        connection.TLSConfig = &tls.Config{
            MinVersion: tls.VersionTLS12,
            ServerName: serverName,
        }
    }
    factory, err := redisconn.NewFactory(connection)
    if err != nil { panic(err) }
    startupCtx, cancelStartup := context.WithTimeout(ctx, 5*time.Second)
    rdb, err := factory.Open(startupCtx)
    cancelStartup()
    if err != nil { panic(err) }
    defer rdb.Close()
    redisCache := redis.NewStableCache(rdb)

    // 2. Combine into multi-level cache (Builder pattern)
    cache := multi.NewBuilder().
        WithLocal(localCache, 10*time.Minute).
        WithRedis(redisCache, 60*time.Minute).
        Build()

    // 3. Use (automatically handles three layers: local -> redis -> db)
    var user User
    err = cache.GetOrLoad(ctx, "user:123", &user,
        func(ctx context.Context) (any, error) {
            return findUserByID(ctx, 123)  // Only need to care about DB query
        },
    )
    if err != nil { panic(err) }
}

func findUserByID(context.Context, int) (User, error) {
    return User{ID: 123}, nil
}
```

See: [cache/multi/README.en.md](multi/README.en.md)

## Usage Examples

### Local Cache

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/hexagon-codes/toolkit/cache/local"
)

type User struct {
    ID int
}

func main() {
    // Create local cache (max 1000 entries)
    cache := local.NewCache(1000,
        local.WithPrefix("myapp"),
        local.WithNegativeTTL(30*time.Second),
    )
    defer cache.Stop()

    // Get or load data
    var user User
    err := cache.GetOrLoad(
        context.Background(),
        "user:123",
        10*time.Minute,
        &user,
        func(ctx context.Context) (any, error) {
            // Load from database
            return findUserByID(ctx, 123)
        },
    )
    if err == local.ErrNotFound {
        fmt.Println("User not found")
        return
    }
    if err != nil {
        fmt.Println("Error:", err)
        return
    }

    fmt.Printf("User: %+v\n", user)
}

func findUserByID(context.Context, int) (User, error) {
    return User{ID: 123}, nil
}
```

### Redis StableCache (Stable Key)

Use case: Single-record queries with deterministic keys

```go
package example

import (
    "context"
    "time"

    cacheredis "github.com/hexagon-codes/toolkit/cache/redis"
    goredis "github.com/redis/go-redis/v9"
)

type User struct {
    ID int
}

func useStableCache(ctx context.Context, rdb goredis.UniversalClient) error {
    // Create stable-key cache
    cache := cacheredis.NewStableCache(rdb,
        cacheredis.WithPrefix("myapp"),
        cacheredis.WithRedisTimeout(50*time.Millisecond, 50*time.Millisecond),
    )

    // Get or load a single record
    var user User
    if err := cache.GetOrLoad(
        ctx,
        "user:123",
        10*time.Minute,
        &user,
        func(ctx context.Context) (any, error) {
            return User{ID: 123}, nil
        },
    ); err != nil {
        return err
    }

    // Delete cache after update
    return cache.Del(ctx, "user:123")
}
```

### Redis UnstableCache (Unstable Key)

Use case: Aggregate queries, JOINs, list queries

```go
package example

import (
    "context"
    "time"

    cacheredis "github.com/hexagon-codes/toolkit/cache/redis"
    goredis "github.com/redis/go-redis/v9"
)

func useUnstableCache(ctx context.Context, rdb goredis.UniversalClient) error {
    // Create unstable-key cache (with version number)
    cache := cacheredis.NewUnstableCache(rdb, "myapp:version",
        cacheredis.WithPrefix("myapp"),
        cacheredis.WithMaxTTL(15*time.Minute),
    )

    // Get aggregate data (version number added automatically)
    var models []string
    if err := cache.GetOrLoad(
        ctx,
        "models:group:chat",
        5*time.Minute,
        &models,
        func(ctx context.Context) (any, error) {
            return []string{"chat"}, nil
        },
    ); err != nil {
        return err
    }

    // After data update, increment version (invalidates all related caches)
    if err := cache.InvalidateVersion(ctx); err != nil {
        return err
    }

    // Or batch delete matching keys
    return cache.InvalidatePattern(ctx, "models:group:*")
}
```

## Configuration Options

### Common Options (Local + Redis)

```go
// Set key prefix
WithPrefix("myapp")

// Set TTL jitter ratio (0~1) to prevent cache avalanche
WithJitter(0.1) // default 10%

// Set negative cache TTL (prevents cache penetration)
WithNegativeTTL(30*time.Second)

// Custom error handler
WithOnError(func(ctx context.Context, op, key string, err error) {
    log.Printf("Cache error: op=%s key=%s err=%v", op, key, err)
})

// Custom NotFound check (e.g., integrating with GORM)
WithIsNotFound(func(err error) bool {
    return errors.Is(err, gorm.ErrRecordNotFound) ||
           errors.Is(err, redis.ErrNotFound)
})
```

### Redis-Specific Options

```go
// Set Redis read/write timeout
WithRedisTimeout(50*time.Millisecond, 50*time.Millisecond)

// Set maximum TTL (UnstableCache only)
WithMaxTTL(15*time.Minute)

// Custom serializer
WithCodec(myCodec)
```

## Cache Strategy Comparison

| Feature | MultiCache | StableCache | UnstableCache | LocalCache |
|---------|-----------|-------------|---------------|------------|
| Use Case | **General (Recommended)** | Single-record queries | Aggregate/JOIN/List | Hot data |
| Layers | Any (2-3) | Single | Single | Single |
| Ease of Use | ⭐⭐⭐ | ⭐⭐ | ⭐⭐ | ⭐⭐ |
| Flexibility | ⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ |
| Key Type | - | Deterministic | Non-deterministic | Deterministic |
| Invalidation | Unified delete | Exact delete | Version/batch delete | Exact delete |
| TTL | Per-layer config | Long (60min) | Short (5-15min) | Configurable |
| Distributed | Supported | Yes | Yes | No |

## Error Handling

```go
err := cache.GetOrLoad(...)
if err == redis.ErrNotFound {
    // Data not found (negative cache hit)
} else if err != nil {
    // Other errors
}
```

## Best Practices

1. **StableCache for single records**: e.g., `GetUserByID(id)`, `GetChannelByID(id)`
2. **UnstableCache for aggregate queries**: e.g., `GetAllUsers()`, `GetChannelsByGroup(group)`
3. **Use version-based invalidation**: UnstableCache recommends version invalidation over batch deletion
4. **Set reasonable TTLs**: Long TTL (60min) for stable data, short TTL (5-15min) for aggregates
5. **Enable negative caching**: Prevents cache penetration; recommend 30-second negative TTL
6. **Monitor errors**: Use `WithOnError` to monitor and alert on cache errors

## Dependencies

- `github.com/redis/go-redis/v9` - Redis client
- `golang.org/x/sync/singleflight` - Anti-breakdown
