# Cache - 通用缓存库

通用缓存库，支持本地缓存、Redis 缓存和多层缓存，提供稳定 key 和不稳定 key 两种缓存策略。

## 特性

- **本地缓存（cache/local）**：基于 sync.Map 的内存缓存，支持 LRU 驱逐和定期清理
- **Redis 缓存（cache/redis）**：基于 Redis 的分布式缓存
  - **StableCache**：稳定 key 缓存，适合单条记录查询
  - **UnstableCache**：不稳定 key 缓存，适合聚合查询、JOIN、列表等
- **多层缓存（cache/multi）** ⭐ **NEW**：自动处理 Local + Redis + DB 三层缓存
  - 自动逐层查询和回填
  - 统一失效管理
  - Builder 模式，开箱即用
- **防击穿**：使用 singleflight 防止缓存击穿
- **防穿透**：支持负缓存（缓存空值）
- **防雪崩**：TTL 抖动机制
- **可扩展**：支持自定义序列化器（Codec）

## 安装

```bash
go get github.com/everyday-items/toolkit/cache
```

## 快速开始

### 方案 1: 多层缓存（推荐）⭐

最简单的方式，自动处理 Local + Redis + DB 三层缓存：

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

    // 2. 组合为多层缓存（Builder 模式）
    cache := multi.NewBuilder().
        WithLocal(localCache, 10*time.Minute).
        WithRedis(redisCache, 60*time.Minute).
        Build()

    // 3. 使用（自动处理三层：local -> redis -> db）
    var user User
    err := cache.GetOrLoad(context.Background(), "user:123", &user,
        func(ctx context.Context) (any, error) {
            return db.FindUserByID(ctx, 123)  // 只需关心 DB 查询
        },
    )
}
```

详见：[cache/multi/README.md](multi/README.md)

## 使用示例

### 本地缓存

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/everyday-items/toolkit/cache/local"
)

func main() {
    // 创建本地缓存（最多 1000 条）
    cache := local.NewCache(1000,
        local.WithPrefix("myapp"),
        local.WithNegativeTTL(30*time.Second),
    )
    defer cache.Stop()

    // 获取或加载数据
    var user User
    err := cache.GetOrLoad(
        context.Background(),
        "user:123",
        10*time.Minute,
        &user,
        func(ctx context.Context) (any, error) {
            // 从数据库加载
            return db.FindUserByID(ctx, 123)
        },
    )
    if err == local.ErrNotFound {
        fmt.Println("用户不存在")
        return
    }
    if err != nil {
        fmt.Println("错误:", err)
        return
    }

    fmt.Printf("用户: %+v\n", user)
}
```

### Redis StableCache（稳定 key）

适用场景：单条记录查询，key 确定性强

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/everyday-items/toolkit/cache/redis"
    goredis "github.com/redis/go-redis/v9"
)

func main() {
    // 创建 Redis 客户端
    rdb := goredis.NewClient(&goredis.Options{
        Addr: "localhost:6379",
    })

    // 创建稳定 key 缓存
    cache := redis.NewStableCache(rdb,
        redis.WithPrefix("myapp"),
        redis.WithRedisTimeout(50*time.Millisecond, 50*time.Millisecond),
    )

    // 获取或加载单条记录
    var user User
    err := cache.GetOrLoad(
        context.Background(),
        "user:123",
        10*time.Minute,
        &user,
        func(ctx context.Context) (any, error) {
            return db.FindUserByID(ctx, 123)
        },
    )

    // 更新后删除缓存
    cache.Del(context.Background(), "user:123")
}
```

### Redis UnstableCache（不稳定 key）

适用场景：聚合查询、JOIN、列表查询

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/everyday-items/toolkit/cache/redis"
    goredis "github.com/redis/go-redis/v9"
)

func main() {
    rdb := goredis.NewClient(&goredis.Options{
        Addr: "localhost:6379",
    })

    // 创建不稳定 key 缓存（带版本号）
    cache := redis.NewUnstableCache(rdb, "myapp:version",
        redis.WithPrefix("myapp"),
        redis.WithMaxTTL(15*time.Minute),
    )

    // 获取聚合数据（自动加版本号）
    var models []string
    err := cache.GetOrLoad(
        context.Background(),
        "models:group:chat",
        5*time.Minute,
        &models,
        func(ctx context.Context) (any, error) {
            return db.GetGroupEnabledModels(ctx, "chat")
        },
    )

    // 数据更新后，递增版本号（使所有相关缓存失效）
    cache.InvalidateVersion(context.Background())

    // 或者批量删除匹配的 key
    cache.InvalidatePattern(context.Background(), "models:group:*")
}
```

## 配置选项

### 通用选项（Local + Redis）

```go
// 设置 key 前缀
WithPrefix("myapp")

// 设置 TTL 抖动比例（0~1），防止缓存雪崩
WithJitter(0.1) // 默认 10%

// 设置负缓存 TTL（防止缓存穿透）
WithNegativeTTL(30*time.Second)

// 自定义错误处理
WithOnError(func(ctx context.Context, op, key string, err error) {
    log.Printf("缓存错误: op=%s key=%s err=%v", op, key, err)
})

// 自定义 NotFound 判断（例如集成 GORM）
WithIsNotFound(func(err error) bool {
    return errors.Is(err, gorm.ErrRecordNotFound) ||
           errors.Is(err, redis.ErrNotFound)
})
```

### Redis 专用选项

```go
// 设置 Redis 读写超时
WithRedisTimeout(50*time.Millisecond, 50*time.Millisecond)

// 设置最大 TTL（UnstableCache 专用）
WithMaxTTL(15*time.Minute)

// 自定义序列化器
WithCodec(myCodec)
```

## 缓存策略对比

| 特性 | MultiCache | StableCache | UnstableCache | LocalCache |
|------|-----------|-------------|---------------|------------|
| 适用场景 | **通用（推荐）** | 单条记录查询 | 聚合/JOIN/列表 | 热点数据 |
| 层数 | 任意（2-3层） | 单层 | 单层 | 单层 |
| 易用性 | ⭐⭐⭐ | ⭐⭐ | ⭐⭐ | ⭐⭐ |
| 灵活性 | ⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ |
| Key 特点 | - | 确定性强 | 不确定 | 确定性强 |
| 失效方式 | 统一删除 | 精确删除 | 版本号/批量删除 | 精确删除 |
| TTL | 分层配置 | 长（60分钟） | 短（5-15分钟） | 可配置 |
| 分布式 | 支持 | 是 | 是 | 否 |

## 错误处理

```go
err := cache.GetOrLoad(...)
if err == redis.ErrNotFound {
    // 数据不存在（负缓存命中）
} else if err != nil {
    // 其他错误
}
```

## 最佳实践

1. **StableCache 用于单条记录**：如 `GetUserByID(id)`、`GetChannelByID(id)`
2. **UnstableCache 用于聚合查询**：如 `GetAllUsers()`、`GetChannelsByGroup(group)`
3. **使用版本号失效**：UnstableCache 推荐使用版本号而非批量删除
4. **设置合理的 TTL**：稳定数据用长 TTL（60分钟），聚合数据用短 TTL（5-15分钟）
5. **启用负缓存**：防止缓存穿透，建议设置 30 秒负缓存 TTL
6. **监控错误**：使用 `WithOnError` 监控缓存错误并打点

## 依赖

- `github.com/redis/go-redis/v9` - Redis 客户端
- `golang.org/x/sync/singleflight` - 防击穿

