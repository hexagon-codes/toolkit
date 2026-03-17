[中文](README.md) | English

# Local Cache

An in-memory cache based on `sync.Map`, providing LRU eviction, expiration policies, and anti-breakdown capabilities.

## Features

- **LRU Eviction**: Evicts the least-recently-used entry when capacity is full
- **Expiration Policy**: Supports TTL-based expiration and periodic cleanup
- **Anti-breakdown**: Uses Singleflight to prevent cache breakdown
- **Anti-penetration**: Supports negative caching (caching NotFound results)
- **TTL Jitter**: Prevents cache avalanche
- **Flexible Configuration**: Supports custom serialization, key prefixes, error callbacks, etc.
- **Zero External Dependencies**: Only depends on the Go standard library and `golang.org/x/sync`

## Installation

```bash
go get github.com/everyday-items/toolkit/cache/local
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/everyday-items/toolkit/cache/local"
)

type User struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}

func main() {
    // Create local cache (max 1000 entries)
    cache := local.NewCache(1000)
    defer cache.Stop()

    ctx := context.Background()

    // Get or load data
    var user User
    err := cache.GetOrLoad(ctx, "user:123", 10*time.Minute, &user,
        func(ctx context.Context) (any, error) {
            // Only called on cache miss
            return fetchUserFromDB(ctx, 123)
        },
    )
    if err == local.ErrNotFound {
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

## API Reference

### NewCache

Creates a local cache (uses default cleanup interval of 1 minute).

```go
func NewCache(maxEntries int, opts ...Option) *Cache
```

**Parameters**:
- `maxEntries`: Maximum number of entries (triggers LRU eviction when exceeded)
- `opts`: Optional configuration options

**Example**:

```go
cache := local.NewCache(1000)
defer cache.Stop()
```

### NewCacheWithCleanup

Creates a local cache with a custom cleanup interval.

```go
func NewCacheWithCleanup(maxEntries int, cleanupInterval time.Duration, opts ...Option) *Cache
```

**Parameters**:
- `maxEntries`: Maximum number of entries
- `cleanupInterval`: Periodic cleanup interval (pass 0 for default 1 minute, negative value to disable periodic cleanup)
- `opts`: Optional configuration options

**Example**:

```go
// Clean up expired entries every 30 seconds
cache := local.NewCacheWithCleanup(1000, 30*time.Second)
defer cache.Stop()

// Disable periodic cleanup
cache := local.NewCacheWithCleanup(1000, -1)
defer cache.Stop()
```

### GetOrLoad

Gets cached data, loading via loader on cache miss.

```go
func (c *Cache) GetOrLoad(
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
- `local.ErrNotFound`: Data not found (negative cache hit)
- `local.ErrInvalidKey`: Key is empty
- `local.ErrInvalidDest`: Dest is not a valid pointer
- `local.ErrInvalidLoader`: Loader is nil
- `local.ErrCorrupt`: Cache data corrupted
- Other errors: errors returned by loader

**Example**:

```go
var user User
err := cache.GetOrLoad(ctx, "user:123", 10*time.Minute, &user,
    func(ctx context.Context) (any, error) {
        return db.FindUserByID(ctx, 123)
    },
)
if err == local.ErrNotFound {
    // User not found
} else if err != nil {
    // Other error
}
```

### Del

Deletes the cache for the specified key(s).

```go
func (c *Cache) Del(ctx context.Context, keys ...string) error
```

**Parameters**:
- `ctx`: Context
- `keys`: List of keys to delete

**Example**:

```go
// Delete a single key
cache.Del(ctx, "user:123")

// Batch delete
cache.Del(ctx, "user:123", "user:456", "user:789")
```

### Stop

Stops periodic cleanup (call on graceful shutdown).

```go
func (c *Cache) Stop()
```

**Example**:

```go
cache := local.NewCache(1000)
defer cache.Stop()
```

### Len

Returns the current number of cache entries (for monitoring).

```go
func (c *Cache) Len() int
```

**Example**:

```go
fmt.Printf("Current cache entries: %d\n", cache.Len())
```

## Configuration Options

### WithPrefix

Adds a prefix to all keys.

```go
cache := local.NewCache(1000, local.WithPrefix("myapp"))

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

cache := local.NewCache(1000, local.WithCodec(MsgpackCodec{}))
```

### WithJitter

Sets the TTL jitter ratio (prevents cache avalanche).

```go
// 10% jitter: a 10-minute TTL becomes 10~11 minutes
cache := local.NewCache(1000, local.WithJitter(0.1))

// Disable jitter
cache := local.NewCache(1000, local.WithJitter(0))
```

### WithNegativeTTL

Sets the negative cache TTL (prevents cache penetration).

```go
// NotFound results are cached for 60 seconds
cache := local.NewCache(1000, local.WithNegativeTTL(60*time.Second))
```

### WithIsNotFound

Custom NotFound check (e.g., integrating with GORM).

```go
import "gorm.io/gorm"

cache := local.NewCache(1000, local.WithIsNotFound(func(err error) bool {
    return errors.Is(err, gorm.ErrRecordNotFound) ||
           errors.Is(err, local.ErrNotFound)
}))
```

### WithOnError

Error monitoring and logging.

```go
cache := local.NewCache(1000, local.WithOnError(
    func(ctx context.Context, op, key string, err error) {
        log.Printf("Cache error: op=%s key=%s err=%v", op, key, err)

        // Metric tracking
        metrics.Incr("cache.error", map[string]string{
            "op": op,
        })
    },
))
```

### WithNow

Custom time function (for testing).

```go
mockNow := func() time.Time {
    return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
}

cache := local.NewCache(1000, local.WithNow(mockNow))
```

## How It Works

### GetOrLoad Flow

```
1. Query local cache
   ├─ Hit → return
   │  └─ Update access time (LRU)
   └─ Miss ↓

2. Singleflight anti-breakdown
   ├─ Double-check cache
   │  └─ Hit → return
   └─ Call loader ↓

3. Handle loader result
   ├─ Success → write to cache → return
   └─ NotFound → write negative cache → return ErrNotFound
```

### LRU Eviction Policy

```
1. Check capacity on write
   ├─ Under limit → write directly
   └─ Over limit ↓

2. Clean up expired entries
   ├─ Under limit after cleanup → write
   └─ Still over limit ↓

3. LRU eviction
   └─ Evict the least-recently-used entry
```

### Periodic Cleanup

```
Every cleanupInterval (default 1 minute):
├─ Scan all entries
└─ Delete expired entries
```

## Use Cases

### Case 1: Hot Data Caching

```go
// Cache hot user info
cache := local.NewCache(1000,
    local.WithPrefix("hot_users"),
    local.WithJitter(0.1),
)

var user User
err := cache.GetOrLoad(ctx, fmt.Sprintf("user:%d", userID), 10*time.Minute, &user,
    func(ctx context.Context) (any, error) {
        return db.FindUserByID(ctx, userID)
    },
)
```

### Case 2: Anti-Penetration

```go
// Cache NotFound results to prevent invalid queries reaching DB
cache := local.NewCache(1000,
    local.WithNegativeTTL(60*time.Second),
    local.WithIsNotFound(func(err error) bool {
        return errors.Is(err, gorm.ErrRecordNotFound) ||
               errors.Is(err, local.ErrNotFound)
    }),
)
```

### Case 3: Combined with Redis Multi-Level Cache

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

### 1. Set Reasonable Capacity

```go
// Set capacity based on memory and data size
// 1000 entries ≈ 100KB ~ 1MB (depending on object size)
cache := local.NewCache(1000)
```

### 2. Control TTL

```go
// Hot data: 5-10 minutes
cache.GetOrLoad(ctx, key, 10*time.Minute, &user, loader)

// Warm data: 1-3 minutes
cache.GetOrLoad(ctx, key, 3*time.Minute, &config, loader)

// Cold data: not recommended for local cache (use Redis instead)
```

### 3. Delete Cache on Update

```go
// Delete cache after updating user info
func UpdateUser(ctx context.Context, user User) error {
    if err := db.UpdateUser(ctx, user); err != nil {
        return err
    }

    // Delete local cache
    cache.Del(ctx, fmt.Sprintf("user:%d", user.ID))
    return nil
}
```

### 4. Monitor Cache Status

```go
cache := local.NewCache(1000, local.WithOnError(
    func(ctx context.Context, op, key string, err error) {
        // Metric tracking
        metrics.Incr("local_cache.error", map[string]string{
            "op": op,
        })
    },
))

// Periodically report cache entry count
go func() {
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()
    for range ticker.C {
        metrics.Gauge("local_cache.size", cache.Len())
    }
}()
```

### 5. Graceful Shutdown

```go
func main() {
    cache := local.NewCache(1000)
    defer cache.Stop() // Stop periodic cleanup

    // ...
}
```

## Notes

1. **Memory Usage**: Local cache uses process memory; control capacity carefully
2. **Not Cross-Process**: Data is only visible within the current process; use Redis for multi-instance setups
3. **Update Inconsistency**: Manually delete cache after updating data
4. **Capacity Limit**: Exceeding maxEntries triggers LRU eviction, which may reduce cache hit rate
5. **Serialization Overhead**: Every read/write involves serialization/deserialization; be mindful of performance impact

## Comparison with Redis Cache

| Feature | Local Cache | Redis Cache |
|---------|------------|-------------|
| Speed | Extremely fast (in-memory) | Fast (network) |
| Capacity | Limited by process memory | Nearly unlimited |
| Persistence | None | Supported |
| Cross-Process Sharing | Not supported | Supported |
| Operational Cost | None | Requires Redis |
| Use Case | Hot data, in-process sharing | Warm data, cross-process sharing |

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
```

See `cache/multi/README.en.md`.

## Full Example

See the `examples/cache/local/` directory.

## Error Codes

| Error | Description | Recommendation |
|-------|-------------|----------------|
| `ErrNotFound` | Data not found (negative cache hit) | Normal business logic |
| `ErrInvalidKey` | Key is empty | Check key parameter |
| `ErrInvalidDest` | Dest is not a valid pointer | Pass a non-nil pointer |
| `ErrInvalidLoader` | Loader is nil | Check loader parameter |
| `ErrCorrupt` | Cache data corrupted | Delete cache and retry |

## Performance Optimization

### 1. Reduce Serialization Overhead

```go
// Use Msgpack instead of JSON (faster and smaller)
cache := local.NewCache(1000, local.WithCodec(MsgpackCodec{}))
```

### 2. Set Appropriate Cleanup Interval

```go
// High-frequency hot data: shorten cleanup interval
cache := local.NewCacheWithCleanup(1000, 30*time.Second)

// Slow-changing data: extend or disable cleanup
cache := local.NewCacheWithCleanup(1000, 5*time.Minute)
```

### 3. Avoid Caching Large Objects

```go
// Not recommended: caching entire list
var users []User // may be very large
cache.GetOrLoad(ctx, "all_users", ttl, &users, loader)

// Recommended: caching individual objects
var user User
cache.GetOrLoad(ctx, fmt.Sprintf("user:%d", id), ttl, &user, loader)
```

## Testing

```bash
# Run tests
go test ./cache/local

# Check coverage
go test -cover ./cache/local

# Benchmark tests
go test -bench=. ./cache/local
```
