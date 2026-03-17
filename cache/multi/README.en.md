[中文](README.md) | English

# Multi-Level Cache

A multi-level cache wrapper supporting **Local + Redis + DB** three-layer cache architecture, automatically handling layer-by-layer querying and backfilling.

## Features

- **Automatic Layer-by-Layer Querying**: Local → Redis → DB
- **Smart Backfilling**: Automatically backfills to preceding layers when data is found
- **Flexible Configuration**: Supports any number of layers and TTLs
- **Unified Invalidation**: A single Del removes from all layers
- **Error Fallback**: Automatically tries the next layer if one fails
- **Builder Pattern**: Provides a friendly construction API

## Usage Examples

### Basic Usage

```go
package main

import (
    "context"
    "time"

    "github.com/everyday-items/toolkit/cache/local"
    "github.com/everyday-items/toolkit/cache/redis"
    "github.com/everyday-items/toolkit/cache/multi"
    goredis "github.com/redis/go-redis/v9"
)

func main() {
    // 1. Create each cache layer
    localCache := local.NewCache(1000)

    rdb := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
    redisCache := redis.NewStableCache(rdb)

    // 2. Combine into multi-level cache
    cache := multi.NewCache([]multi.LayerConfig{
        {Layer: localCache, TTL: 10 * time.Minute, Name: "local"},
        {Layer: redisCache, TTL: 60 * time.Minute, Name: "redis"},
    })

    // 3. Use (automatically handles three layers: local -> redis -> db)
    var user User
    err := cache.GetOrLoad(context.Background(), "user:123", &user,
        func(ctx context.Context) (any, error) {
            // Only need to care about DB query
            return db.FindUserByID(ctx, 123)
        },
    )
    if err == multi.ErrNotFound {
        // Data not found
    } else if err != nil {
        // Other error
    }

    // Delete cache (all layers)
    cache.Del(context.Background(), "user:123")
}
```

### Builder Pattern (Recommended)

```go
cache := multi.NewBuilder().
    WithLocal(localCache, 10*time.Minute).
    WithRedis(redisCache, 60*time.Minute).
    WithOnError(func(ctx context.Context, layer, op, key string, err error) {
        log.Printf("Cache error: layer=%s op=%s key=%s err=%v", layer, op, key, err)
    }).
    Build()
```

## How It Works

### GetOrLoad Flow

```
1. Query Local cache
   ├─ Hit → return
   └─ Miss ↓

2. Query Redis cache
   ├─ Hit → backfill to Local → return
   └─ Miss ↓

3. Call loader (query DB)
   ├─ Success → backfill to Redis and Local → return
   └─ Failure → return error
```

### Del Flow

```
Delete cache from all layers (concurrent execution)
├─ Local.Del("user:123")
└─ Redis.Del("user:123")
```

## Configuration Options

### WithIsNotFound

Custom NotFound check (e.g., integrating with GORM)

```go
import "gorm.io/gorm"

cache := multi.NewBuilder().
    WithLocal(localCache, 10*time.Minute).
    WithRedis(redisCache, 60*time.Minute).
    WithIsNotFound(func(err error) bool {
        return errors.Is(err, gorm.ErrRecordNotFound) ||
               errors.Is(err, multi.ErrNotFound)
    }).
    Build()
```

### WithOnError

Error monitoring and logging

```go
cache := multi.NewBuilder().
    WithLocal(localCache, 10*time.Minute).
    WithRedis(redisCache, 60*time.Minute).
    WithOnError(func(ctx context.Context, layer, op, key string, err error) {
        // Log
        log.Printf("Cache error: layer=%s op=%s key=%s err=%v", layer, op, key, err)

        // Metric tracking
        metrics.Incr("cache.error", map[string]string{
            "layer": layer,
            "op":    op,
        })
    }).
    Build()
```

### WithSkipBackfill

Skip backfilling (reduces writes but lowers cache hit rate)

```go
cache := multi.NewBuilder().
    WithLocal(localCache, 10*time.Minute).
    WithRedis(redisCache, 60*time.Minute).
    WithSkipBackfill(true).  // No backfill
    Build()
```

## Advanced Usage

### Custom Number of Layers

```go
// Four-layer cache: Local -> Redis -> Memcached -> DB
cache := multi.NewCache([]multi.LayerConfig{
    {Layer: localCache, TTL: 5 * time.Minute, Name: "local"},
    {Layer: redisCache, TTL: 30 * time.Minute, Name: "redis"},
    {Layer: memcachedCache, TTL: 60 * time.Minute, Name: "memcached"},
})
```

### Two Layers Only (Local + Redis, No DB)

```go
// Create two-layer cache
cache := multi.NewBuilder().
    WithLocal(localCache, 10*time.Minute).
    WithRedis(redisCache, 60*time.Minute).
    Build()

// Can nest calls when using
var user User
err := cache.GetOrLoad(ctx, "user:123", &user,
    func(ctx context.Context) (any, error) {
        // Can call another cache here or return data directly
        return fetchFromAPI(ctx, 123)
    },
)
```

## Comparison with Core Packages

| Scenario | Core Packages (local/redis) | Multi Package |
|----------|-----------------------------|---------------|
| Single-layer cache | ✅ Recommended | Also works |
| Multi-layer cache | Manual nesting | ✅ Automatic |
| Custom logic | ✅ Flexible | Limited |
| Ease of use | Requires manual code | ✅ Ready to use |

## Best Practices

1. **Use Local for hot data**: Set TTL to 5-10 minutes
2. **Use Redis for warm data**: Set TTL to 30-60 minutes
3. **Monitor errors**: Use `WithOnError` to monitor failure rates per layer
4. **Set reasonable TTLs**: Local TTL < Redis TTL to avoid data inconsistency
5. **Delete on update**: Call `Del` to remove from all layers after data updates

## Notes

1. **Backfill is async**: Does not block the main flow, but may have brief delay
2. **TTL setting**: Recommended Local TTL < Redis TTL to ensure data consistency
3. **Error fallback**: Layer failure automatically tries the next layer, does not affect overall availability
4. **Memory usage**: Local cache uses process memory; control size carefully

## Full Example

See the `examples/cache/multi/` directory.
