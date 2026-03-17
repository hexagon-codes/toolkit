[中文](README.md) | English

# Redis Cache

Redis-based distributed cache providing two caching strategies: **StableCache** (stable keys) and **UnstableCache** (unstable keys), with version-based invalidation, TTL jitter, and anti-breakdown support.

## Features

- **Dual Cache Strategies**:
  - **StableCache**: Suitable for single-record queries (deterministic keys)
  - **UnstableCache**: Suitable for aggregate queries, JOINs, and lists (non-deterministic keys)
- **Version-Based Invalidation**: Increment version once to invalidate all related caches
- **TTL Jitter**: Prevents cache avalanche
- **Anti-breakdown**: Uses Singleflight to prevent cache breakdown
- **Anti-penetration**: Supports negative caching (caching NotFound results)
- **Async Write**: Cache writes do not block the main flow
- **Timeout Control**: Configurable read/write timeouts
- **Error Fallback**: Automatically falls back to DB on Redis error

## Installation

```bash
go get github.com/everyday-items/toolkit/cache/redis
go get github.com/redis/go-redis/v9
```

## Quick Start

### StableCache - Single-Record Query

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/everyday-items/toolkit/cache/redis"
    goredis "github.com/redis/go-redis/v9"
)

type User struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}

func main() {
    // Create Redis client
    rdb := goredis.NewClient(&goredis.Options{
        Addr: "localhost:6379",
    })

    // Create stable-key cache
    cache := redis.NewStableCache(rdb)

    ctx := context.Background()

    // Get or load data
    var user User
    err := cache.GetOrLoad(ctx, "user:123", 60*time.Minute, &user,
        func(ctx context.Context) (any, error) {
            return fetchUserFromDB(ctx, 123)
        },
    )
    if err == redis.ErrNotFound {
        fmt.Println("User not found")
    } else if err != nil {
        fmt.Println("Failed to get:", err)
    } else {
        fmt.Printf("User: %+v\n", user)
    }

    // Delete cache
    cache.Del(ctx, "user:123")
}

func fetchUserFromDB(ctx context.Context, id int) (User, error) {
    // Simulate DB query
    return User{ID: id, Name: "Alice"}, nil
}
```

### UnstableCache - Aggregate Query

```go
// Create unstable-key cache (with version number)
cache := redis.NewUnstableCache(rdb, "ability:version")

// Get aggregate data (key is automatically appended with version number)
var models []string
err := cache.GetOrLoad(ctx, "ability:group:chat", 5*time.Minute, &models,
    func(ctx context.Context) (any, error) {
        return db.GetGroupEnabledModels(ctx, "chat")
    },
)

// After data update, increment version (invalidates all related caches)
cache.InvalidateVersion(ctx)
```

## API Reference

### StableCache

Suitable for **single-record queries** with deterministic keys, supports precise invalidation.

#### NewStableCache

Creates a stable-key cache.

```go
func NewStableCache(client redis.UniversalClient, opts ...Option) *StableCache
```

**Parameters**:
- `client`: Redis client (supports standalone, sentinel, cluster)
- `opts`: Optional configuration options

**Example**:

```go
rdb := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
cache := redis.NewStableCache(rdb)
```

#### GetOrLoad

Gets cached data, loading via loader on cache miss.

```go
func (c *StableCache) GetOrLoad(
    ctx context.Context,
    key string,
    ttl time.Duration,
    dest any,
    loader func(ctx context.Context) (any, error),
) error
```

**Parameters**:
- `ctx`: Context
- `key`: Cache key
- `ttl`: Expiration duration
- `dest`: Destination object (must be a non-nil pointer)
- `loader`: Function called on cache miss

**Returns**:
- `redis.ErrNotFound`: Data not found (negative cache hit)
- `redis.ErrInvalidKey`: Key is empty
- `redis.ErrInvalidDest`: Dest is not a valid pointer
- `redis.ErrInvalidLoader`: Loader is nil
- `redis.ErrCorrupt`: Cache data corrupted
- Other errors: errors returned by loader

**Example**:

```go
var user User
err := cache.GetOrLoad(ctx, "user:123", 60*time.Minute, &user,
    func(ctx context.Context) (any, error) {
        return db.FindUserByID(ctx, 123)
    },
)
```

#### Del

Deletes the specified key(s) (precise invalidation).

```go
func (c *StableCache) Del(ctx context.Context, keys ...string) error
```

**Example**:

```go
// Delete a single key
cache.Del(ctx, "user:123")

// Batch delete
cache.Del(ctx, "user:123", "user:456")
```

#### Set

Proactively writes to cache (Write-Through mode).

```go
func (c *StableCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error
```

**Example**:

```go
user := User{ID: 123, Name: "Alice"}
cache.Set(ctx, "user:123", user, 60*time.Minute)
```

### UnstableCache

Suitable for **aggregate queries, JOINs, and lists** with non-deterministic keys, supports version-based batch invalidation.

#### NewUnstableCache

Creates an unstable-key cache.

```go
func NewUnstableCache(client redis.UniversalClient, versionKey string, opts ...Option) *UnstableCache
```

**Parameters**:
- `client`: Redis client
- `versionKey`: Redis key for storing the version number (e.g., `"ability:version"`)
- `opts`: Optional configuration options

**Example**:

```go
rdb := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
cache := redis.NewUnstableCache(rdb, "ability:version")
```

#### GetOrLoad

Gets aggregate data (key is automatically appended with version number).

```go
func (c *UnstableCache) GetOrLoad(
    ctx context.Context,
    key string,
    ttl time.Duration,
    dest any,
    loader func(ctx context.Context) (any, error),
) error
```

**Example**:

```go
// key becomes "ability:group:chat:v1"
var models []string
err := cache.GetOrLoad(ctx, "ability:group:chat", 5*time.Minute, &models,
    func(ctx context.Context) (any, error) {
        return db.GetGroupEnabledModels(ctx, "chat")
    },
)
```

#### GetOrLoadWithoutVersion

Loads without using a version number (use short TTL + batch delete).

```go
func (c *UnstableCache) GetOrLoadWithoutVersion(
    ctx context.Context,
    key string,
    ttl time.Duration,
    dest any,
    loader func(ctx context.Context) (any, error),
) error
```

**Example**:

```go
var abilities []Ability
err := cache.GetOrLoadWithoutVersion(ctx, "ability:all:enabled", 2*time.Minute, &abilities, loader)
```

#### InvalidateVersion

Increments the version number (invalidates all keys that use the version number).

```go
func (c *UnstableCache) InvalidateVersion(ctx context.Context) error
```

**Use cases**:
- Call after updating an Ability
- Call after updating a Channel
- Call after updating configuration

**Example**:

```go
// Increment version after updating data
func UpdateAbility(ctx context.Context, ability Ability) error {
    if err := db.UpdateAbility(ctx, ability); err != nil {
        return err
    }

    // Invalidate all ability-related caches
    return cache.InvalidateVersion(ctx)
}
```

#### InvalidatePattern

Batch deletes matching keys (when not using version numbers).

```go
func (c *UnstableCache) InvalidatePattern(ctx context.Context, pattern string) error
```

**Notes**:
- Uses SCAN instead of KEYS in production to avoid blocking
- In cluster mode, iterates over all master nodes

**Example**:

```go
// Delete all "ability:group:*" keys
cache.InvalidatePattern(ctx, "ability:group:*")
```

#### Del

Deletes the specified key(s).

```go
func (c *UnstableCache) Del(ctx context.Context, keys ...string) error
```

**Example**:

```go
cache.Del(ctx, "ability:group:chat:v1", "ability:group:image:v1")
```

#### GetVersion

Gets the current version number.

```go
func (c *UnstableCache) GetVersion() int64
```

**Example**:

```go
version := cache.GetVersion()
fmt.Printf("Current version: %d\n", version)
```

## Configuration Options

### WithPrefix

Adds a prefix to all keys.

```go
cache := redis.NewStableCache(rdb, redis.WithPrefix("myapp"))

// Actual stored key: "myapp:user:123"
cache.GetOrLoad(ctx, "user:123", ttl, &user, loader)
```

### WithCodec

Custom serialization (default is JSON).

```go
type MsgpackCodec struct{}

func (MsgpackCodec) Marshal(v any) ([]byte, error) {
    return msgpack.Marshal(v)
}

func (MsgpackCodec) Unmarshal(b []byte, v any) error {
    return msgpack.Unmarshal(b, v)
}

cache := redis.NewStableCache(rdb, redis.WithCodec(MsgpackCodec{}))
```

### WithJitter

Sets TTL jitter ratio (prevents cache avalanche).

```go
// 10% jitter: a 60-minute TTL becomes 60~66 minutes
cache := redis.NewStableCache(rdb, redis.WithJitter(0.1))

// Disable jitter
cache := redis.NewStableCache(rdb, redis.WithJitter(0))
```

### WithNegativeTTL

Sets negative cache TTL (prevents cache penetration).

```go
// NotFound results are cached for 60 seconds
cache := redis.NewStableCache(rdb, redis.WithNegativeTTL(60*time.Second))
```

### WithMaxTTL

Sets the maximum TTL cap (mainly for UnstableCache).

```go
// Aggregate data cached for at most 15 minutes
cache := redis.NewUnstableCache(rdb, "version", redis.WithMaxTTL(15*time.Minute))
```

### WithRedisTimeout

Sets Redis read/write timeouts.

```go
// Read timeout 100ms, write timeout 200ms
cache := redis.NewStableCache(rdb,
    redis.WithRedisTimeout(100*time.Millisecond, 200*time.Millisecond),
)
```

### WithIsNotFound

Custom NotFound check (e.g., integrating with GORM).

```go
import "gorm.io/gorm"

cache := redis.NewStableCache(rdb, redis.WithIsNotFound(func(err error) bool {
    return errors.Is(err, gorm.ErrRecordNotFound) ||
           errors.Is(err, redis.ErrNotFound)
}))
```

### WithOnError

Error monitoring and logging.

```go
cache := redis.NewStableCache(rdb, redis.WithOnError(
    func(ctx context.Context, op, key string, err error) {
        log.Printf("Redis error: op=%s key=%s err=%v", op, key, err)

        // Metric tracking
        metrics.Incr("redis.error", map[string]string{
            "op": op,
        })
    },
))
```

## How It Works

### StableCache Flow

```
1. Query Redis cache
   ├─ Hit → unpack → return
   └─ Miss ↓

2. Singleflight anti-breakdown
   ├─ Double-check cache
   │  └─ Hit → return
   └─ Call loader ↓

3. Handle loader result
   ├─ Success → async write to Redis → return
   └─ NotFound → async write negative cache → return ErrNotFound

4. Redis error handling
   └─ Fall back to direct loader call
```

### UnstableCache Flow (with Version Number)

```
1. Get version number
   └─ version = 1 (read from Redis)

2. Compose key
   └─ "ability:group:chat" → "ability:group:chat:v1"

3. Query Redis
   ├─ Hit → return
   └─ Miss → load → write

4. After data update
   └─ InvalidateVersion() → version = 2
   └─ Old key "ability:group:chat:v1" auto-expires
```

### TTL Jitter

```
Original TTL: 60 minutes
Jitter ratio: 0.1 (10%)

Calculation:
├─ maxDelta = 60 * 0.1 = 6 minutes
├─ delta = random(0, 6) = 4 minutes
└─ Actual TTL = 60 + 4 = 64 minutes

Effect:
└─ Prevents mass cache expiration at the same time (anti-avalanche)
```

## Use Cases

### Case 1: Single-Record Query (StableCache)

```go
// User info
cache := redis.NewStableCache(rdb, redis.WithPrefix("user"))

var user User
err := cache.GetOrLoad(ctx, fmt.Sprintf("user:%d", userID), 60*time.Minute, &user,
    func(ctx context.Context) (any, error) {
        return db.FindUserByID(ctx, userID)
    },
)

// Delete cache after updating user
cache.Del(ctx, fmt.Sprintf("user:%d", userID))
```

### Case 2: Aggregate Query (UnstableCache + Version Number)

```go
// Ability aggregate query
cache := redis.NewUnstableCache(rdb, "ability:version",
    redis.WithMaxTTL(15*time.Minute),
)

// Get aggregate data
var models []string
err := cache.GetOrLoad(ctx, "ability:group:chat", 5*time.Minute, &models,
    func(ctx context.Context) (any, error) {
        return db.GetGroupEnabledModels(ctx, "chat")
    },
)

// Increment version after updating Ability
cache.InvalidateVersion(ctx)
```

### Case 3: JOIN Query (UnstableCache + Short TTL)

```go
// JOIN queries use short TTL
cache := redis.NewUnstableCache(rdb, "ability:version")

var abilities []Ability
err := cache.GetOrLoad(ctx, "ability:all:with_channels", redis.UnstableCacheTTLShort, &abilities,
    func(ctx context.Context) (any, error) {
        return db.GetAllEnableAbilityWithChannels(ctx)
    },
)
```

### Case 4: List Query (UnstableCache + Pattern Delete)

```go
// User list (without version number)
cache := redis.NewUnstableCache(rdb, "user:list:version")

var users []User
key := fmt.Sprintf("user:list:page:%d:size:%d", page, size)
err := cache.GetOrLoadWithoutVersion(ctx, key, 2*time.Minute, &users,
    func(ctx context.Context) (any, error) {
        return db.GetUserList(ctx, page, size)
    },
)

// Batch delete list caches after updating user
cache.InvalidatePattern(ctx, "user:list:*")
```

### Case 5: Combined with Local Multi-Level Cache

```go
import (
    "github.com/everyday-items/toolkit/cache/local"
    "github.com/everyday-items/toolkit/cache/multi"
    "github.com/everyday-items/toolkit/cache/redis"
)

// Create local cache
localCache := local.NewCache(1000)

// Create Redis cache
rdb := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
redisCache := redis.NewStableCache(rdb)

// Combine into multi-level cache
cache := multi.NewBuilder().
    WithLocal(localCache, 10*time.Minute).
    WithRedis(redisCache, 60*time.Minute).
    Build()

// Use (automatically handles Local → Redis → DB)
var user User
err := cache.GetOrLoad(ctx, "user:123", &user,
    func(ctx context.Context) (any, error) {
        return db.FindUserByID(ctx, 123)
    },
)
```

## Best Practices

### 1. Choose the Right Cache Type

```go
// StableCache: single-record queries
// ✅ GetUserByID(id)
// ✅ GetChannelByID(id)
// ✅ GetAbilityByID(id)

cache := redis.NewStableCache(rdb)

// UnstableCache: aggregate queries, JOINs, lists
// ✅ GetGroupEnabledModels(group)
// ✅ GetAllEnableAbilityWithChannels()
// ✅ GetUserList(page, size)

cache := redis.NewUnstableCache(rdb, "version")
```

### 2. Set Reasonable TTLs

```go
// StableCache: long TTL (data changes rarely)
cache.GetOrLoad(ctx, "user:123", redis.StableCacheTTL, &user, loader) // 60 minutes

// UnstableCache: short TTL (data changes frequently)
cache.GetOrLoad(ctx, "ability:group:chat", redis.UnstableCacheTTLShort, &models, loader) // 5 minutes
cache.GetOrLoad(ctx, "ability:all:join", redis.UnstableCacheTTLMedium, &abilities, loader) // 10 minutes
```

### 3. Invalidate Cache on Update

```go
// StableCache: precise delete
func UpdateUser(ctx context.Context, user User) error {
    if err := db.UpdateUser(ctx, user); err != nil {
        return err
    }
    return cache.Del(ctx, fmt.Sprintf("user:%d", user.ID))
}

// UnstableCache: version-based invalidation
func UpdateAbility(ctx context.Context, ability Ability) error {
    if err := db.UpdateAbility(ctx, ability); err != nil {
        return err
    }
    return cache.InvalidateVersion(ctx)
}
```

### 4. Anti-Penetration

```go
cache := redis.NewStableCache(rdb,
    redis.WithNegativeTTL(60*time.Second),
    redis.WithIsNotFound(func(err error) bool {
        return errors.Is(err, gorm.ErrRecordNotFound) ||
               errors.Is(err, redis.ErrNotFound)
    }),
)
```

### 5. Monitor Redis Errors

```go
cache := redis.NewStableCache(rdb, redis.WithOnError(
    func(ctx context.Context, op, key string, err error) {
        // Metric tracking
        metrics.Incr("redis.error", map[string]string{
            "op": op,
        })

        // Alert on critical errors
        if errors.Is(err, context.DeadlineExceeded) {
            alert.Send("Redis timeout", fmt.Sprintf("op=%s key=%s", op, key))
        }
    },
))
```

### 6. Control Timeouts

```go
// Read timeout 50ms, write timeout 50ms (default)
cache := redis.NewStableCache(rdb,
    redis.WithRedisTimeout(50*time.Millisecond, 50*time.Millisecond),
)

// Automatically falls back to DB on timeout
```

## Notes

1. **Version only increments**: `InvalidateVersion()` only increments the version, never decrements
2. **Version persisted**: Version is stored in Redis, survives restarts
3. **Pattern delete performance**: `InvalidatePattern()` uses SCAN, may be slow with many keys
4. **Cluster mode limitation**: Pattern delete iterates over all master nodes
5. **Async writes**: Cache writes are async, may have brief delay
6. **Timeout fallback**: Redis timeout automatically falls back to DB to ensure availability

## StableCache vs UnstableCache

| Feature | StableCache | UnstableCache |
|---------|------------|--------------|
| Use Case | Single-record queries | Aggregate queries, JOINs, lists |
| Key Type | Deterministic | Non-deterministic |
| Invalidation | Exact delete | Version/Pattern delete |
| TTL | Long (60 minutes) | Short (5-15 minutes) |
| Example | GetUserByID | GetGroupEnabledModels |

## Using with Multi Package

`cache/multi` provides multi-level cache wrapping, recommended to use together:

```go
// multi package automatically handles Local → Redis → DB
cache := multi.NewBuilder().
    WithLocal(localCache, 10*time.Minute).
    WithRedis(redisCache, 60*time.Minute).
    Build()

// Only need to care about DB query
var user User
err := cache.GetOrLoad(ctx, "user:123", &user,
    func(ctx context.Context) (any, error) {
        return db.FindUserByID(ctx, 123)
    },
)

// Delete cache (all layers)
cache.Del(ctx, "user:123")
```

See `cache/multi/README.en.md`.

## Full Example

See the `examples/cache/redis/` directory.

## Error Codes

| Error | Description | Recommendation |
|-------|-------------|----------------|
| `ErrNotFound` | Data not found (negative cache hit) | Normal business logic |
| `ErrInvalidKey` | Key is empty | Check key parameter |
| `ErrInvalidDest` | Dest is not a valid pointer | Pass a non-nil pointer |
| `ErrInvalidLoader` | Loader is nil | Check loader parameter |
| `ErrCorrupt` | Cache data corrupted | Delete cache and retry |

## Performance Optimization

### 1. Use Msgpack Serialization

```go
// Msgpack is faster and smaller than JSON
cache := redis.NewStableCache(rdb, redis.WithCodec(MsgpackCodec{}))
```

### 2. Enable TTL Jitter

```go
// Prevents cache avalanche
cache := redis.NewStableCache(rdb, redis.WithJitter(0.1))
```

### 3. Control MaxTTL

```go
// Limit aggregate data cache duration
cache := redis.NewUnstableCache(rdb, "version", redis.WithMaxTTL(15*time.Minute))
```

### 4. Async Writes

```go
// Cache writes are async by default, do not block main flow
cache.GetOrLoad(ctx, key, ttl, &user, loader)
```

## Testing

```bash
# Run tests (requires Redis)
go test ./cache/redis

# Start Redis with Docker
docker run -d -p 6379:6379 redis:7

# Check coverage
go test -cover ./cache/redis
```

## Dependencies

- `github.com/redis/go-redis/v9`: Redis client
- `golang.org/x/sync/singleflight`: Anti-breakdown
- `github.com/bytedance/gopkg/util/gopool`: Async goroutine pool

## FAQ

### 1. How to choose between StableCache and UnstableCache?

```go
// Look at the query type:
// - Single-record query → StableCache
// - Aggregate/JOIN/list → UnstableCache

// StableCache
cache.GetOrLoad(ctx, "user:123", ttl, &user, loader)

// UnstableCache
cache.GetOrLoad(ctx, "ability:group:chat", ttl, &models, loader)
```

### 2. When should I increment the version?

```go
// Call InvalidateVersion() after updating data
func UpdateAbility(ctx context.Context, ability Ability) error {
    if err := db.UpdateAbility(ctx, ability); err != nil {
        return err
    }
    return cache.InvalidateVersion(ctx) // version increments
}
```

### 3. Will Redis errors affect availability?

```go
// No, Redis errors automatically fall back to DB
cache.GetOrLoad(ctx, key, ttl, &user, func(ctx context.Context) (any, error) {
    // Called directly when Redis errors occur
    return db.FindUserByID(ctx, 123)
})
```

### 4. How to monitor cache hit rate?

```go
cache := redis.NewStableCache(rdb, redis.WithOnError(
    func(ctx context.Context, op, key string, err error) {
        metrics.Incr("redis.error", map[string]string{"op": op})
    },
))

// Track cache miss in loader
cache.GetOrLoad(ctx, key, ttl, &user, func(ctx context.Context) (any, error) {
    metrics.Incr("cache.miss") // cache miss
    return db.FindUserByID(ctx, 123)
})
```

### 5. Will InvalidatePattern be slow in cluster mode?

```go
// It iterates over all master nodes; may be slow with many keys
// Recommendations:
// 1. Prefer version-based invalidation
// 2. Narrow the pattern scope
// 3. Use short TTL

cache.InvalidatePattern(ctx, "ability:group:*") // narrow scope
```
