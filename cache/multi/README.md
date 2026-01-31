# Multi-Level Cache

多层缓存封装，支持**本地 + Redis + DB** 三层缓存架构，自动处理逐层查询和回填。

## 特性

- **自动逐层查询**：Local → Redis → DB
- **智能回填**：找到数据后自动回填到前面的层
- **灵活配置**：支持任意层数和 TTL
- **统一失效**：一次 Del 删除所有层
- **错误降级**：某层失败自动尝试下一层
- **Builder 模式**：提供友好的构建 API

## 使用示例

### 基本用法

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
    // 1. 创建各层缓存
    localCache := local.NewCache(1000)

    rdb := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
    redisCache := redis.NewStableCache(rdb)

    // 2. 组合为多层缓存
    cache := multi.NewCache([]multi.LayerConfig{
        {Layer: localCache, TTL: 10 * time.Minute, Name: "local"},
        {Layer: redisCache, TTL: 60 * time.Minute, Name: "redis"},
    })

    // 3. 使用（自动处理三层：local -> redis -> db）
    var user User
    err := cache.GetOrLoad(context.Background(), "user:123", &user,
        func(ctx context.Context) (any, error) {
            // 只需关心 DB 查询
            return db.FindUserByID(ctx, 123)
        },
    )
    if err == multi.ErrNotFound {
        // 数据不存在
    } else if err != nil {
        // 其他错误
    }

    // 删除缓存（所有层）
    cache.Del(context.Background(), "user:123")
}
```

### Builder 模式（推荐）

```go
cache := multi.NewBuilder().
    WithLocal(localCache, 10*time.Minute).
    WithRedis(redisCache, 60*time.Minute).
    WithOnError(func(ctx context.Context, layer, op, key string, err error) {
        log.Printf("缓存错误: layer=%s op=%s key=%s err=%v", layer, op, key, err)
    }).
    Build()
```

## 工作原理

### GetOrLoad 流程

```
1. 查询 Local 缓存
   ├─ 命中 → 返回
   └─ 未命中 ↓

2. 查询 Redis 缓存
   ├─ 命中 → 回填到 Local → 返回
   └─ 未命中 ↓

3. 调用 loader（查 DB）
   ├─ 成功 → 回填到 Redis 和 Local → 返回
   └─ 失败 → 返回错误
```

### Del 流程

```
删除所有层的缓存（并发执行）
├─ Local.Del("user:123")
└─ Redis.Del("user:123")
```

## 配置选项

### WithIsNotFound

自定义 NotFound 判断（例如集成 GORM）

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

错误监控和日志

```go
cache := multi.NewBuilder().
    WithLocal(localCache, 10*time.Minute).
    WithRedis(redisCache, 60*time.Minute).
    WithOnError(func(ctx context.Context, layer, op, key string, err error) {
        // 打日志
        log.Printf("缓存错误: layer=%s op=%s key=%s err=%v", layer, op, key, err)

        // 打点监控
        metrics.Incr("cache.error", map[string]string{
            "layer": layer,
            "op":    op,
        })
    }).
    Build()
```

### WithSkipBackfill

跳过回填（减少写入，但降低缓存命中率）

```go
cache := multi.NewBuilder().
    WithLocal(localCache, 10*time.Minute).
    WithRedis(redisCache, 60*time.Minute).
    WithSkipBackfill(true).  // 不回填
    Build()
```

## 高级用法

### 自定义层数

```go
// 四层缓存：Local -> Redis -> Memcached -> DB
cache := multi.NewCache([]multi.LayerConfig{
    {Layer: localCache, TTL: 5 * time.Minute, Name: "local"},
    {Layer: redisCache, TTL: 30 * time.Minute, Name: "redis"},
    {Layer: memcachedCache, TTL: 60 * time.Minute, Name: "memcached"},
})
```

### 只用两层（Local + Redis，无 DB）

```go
// 创建两层缓存
cache := multi.NewBuilder().
    WithLocal(localCache, 10*time.Minute).
    WithRedis(redisCache, 60*time.Minute).
    Build()

// 使用时可以嵌套调用
var user User
err := cache.GetOrLoad(ctx, "user:123", &user,
    func(ctx context.Context) (any, error) {
        // 这里可以再调用另一个 cache 或直接返回数据
        return fetchFromAPI(ctx, 123)
    },
)
```

## 与核心包对比

| 场景 | 核心包（local/redis） | Multi 包 |
|------|---------------------|----------|
| 单层缓存 | ✅ 推荐 | 也可以 |
| 多层缓存 | 手动嵌套 | ✅ 自动处理 |
| 自定义逻辑 | ✅ 灵活 | 受限 |
| 易用性 | 需手写代码 | ✅ 开箱即用 |

## 最佳实践

1. **热数据用 Local**：TTL 设置为 5-10 分钟
2. **温数据用 Redis**：TTL 设置为 30-60 分钟
3. **监控错误**：使用 `WithOnError` 监控各层失败率
4. **合理设置 TTL**：Local TTL < Redis TTL，避免数据不一致
5. **更新时删除**：数据更新后调用 `Del` 删除所有层

## 注意事项

1. **回填是异步的**：不会阻塞主流程，但可能有短暂延迟
2. **TTL 设置**：建议 Local TTL < Redis TTL，保证数据一致性
3. **错误降级**：某层失败会自动尝试下一层，不影响整体可用性
4. **内存占用**：Local 缓存会占用进程内存，注意控制大小

## 完整示例

参见 `examples/cache/multi/` 目录。
