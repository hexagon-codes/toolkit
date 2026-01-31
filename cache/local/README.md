# Local Cache

本地内存缓存，基于 `sync.Map` 实现，提供 LRU 驱逐、过期策略和防击穿能力。

## 特性

- **LRU 驱逐**：容量满时驱逐最久未访问的条目
- **过期策略**：支持 TTL 过期和定期清理
- **防击穿**：使用 Singleflight 防止缓存击穿
- **防穿透**：支持负缓存（缓存 NotFound）
- **TTL 抖动**：防止缓存雪崩
- **灵活配置**：支持自定义序列化、前缀、错误回调等
- **零外部依赖**：仅依赖 Go 标准库和 `golang.org/x/sync`

## 安装

```bash
go get github.com/everyday-items/toolkit/cache/local
```

## 快速开始

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
    // 创建本地缓存（最多 1000 条）
    cache := local.NewCache(1000)
    defer cache.Stop()

    ctx := context.Background()

    // 获取或加载数据
    var user User
    err := cache.GetOrLoad(ctx, "user:123", 10*time.Minute, &user,
        func(ctx context.Context) (any, error) {
            // 只在缓存未命中时调用
            return fetchUserFromDB(ctx, 123)
        },
    )
    if err == local.ErrNotFound {
        fmt.Println("用户不存在")
    } else if err != nil {
        fmt.Println("获取失败:", err)
    } else {
        fmt.Printf("用户: %+v\n", user)
    }

    // 删除缓存
    cache.Del(ctx, "user:123")
}

func fetchUserFromDB(ctx context.Context, id int) (User, error) {
    // 模拟 DB 查询
    return User{ID: id, Name: "Alice"}, nil
}
```

## API 文档

### NewCache

创建本地缓存（使用默认清理间隔 1 分钟）。

```go
func NewCache(maxEntries int, opts ...Option) *Cache
```

**参数**：
- `maxEntries`：最大条目数（超过时触发 LRU 驱逐）
- `opts`：可选配置项

**示例**：

```go
cache := local.NewCache(1000)
defer cache.Stop()
```

### NewCacheWithCleanup

创建本地缓存（可自定义清理间隔）。

```go
func NewCacheWithCleanup(maxEntries int, cleanupInterval time.Duration, opts ...Option) *Cache
```

**参数**：
- `maxEntries`：最大条目数
- `cleanupInterval`：定期清理间隔（传入 0 使用默认值 1 分钟，负值则禁用定期清理）
- `opts`：可选配置项

**示例**：

```go
// 每 30 秒清理一次过期条目
cache := local.NewCacheWithCleanup(1000, 30*time.Second)
defer cache.Stop()

// 禁用定期清理
cache := local.NewCacheWithCleanup(1000, -1)
defer cache.Stop()
```

### GetOrLoad

获取缓存数据，未命中时调用 loader 加载。

```go
func (c *Cache) GetOrLoad(
    ctx context.Context,
    key string,
    ttl time.Duration,
    dest any,
    loader func(ctx context.Context) (any, error),
) error
```

**参数**：
- `ctx`：上下文
- `key`：缓存 key
- `ttl`：过期时间
- `dest`：目标对象（必须是非 nil 指针）
- `loader`：缓存未命中时的加载函数

**返回**：
- `local.ErrNotFound`：数据不存在（负缓存命中）
- `local.ErrInvalidKey`：key 为空
- `local.ErrInvalidDest`：dest 不是有效指针
- `local.ErrInvalidLoader`：loader 为 nil
- `local.ErrCorrupt`：缓存数据损坏
- 其他错误：loader 返回的错误

**示例**：

```go
var user User
err := cache.GetOrLoad(ctx, "user:123", 10*time.Minute, &user,
    func(ctx context.Context) (any, error) {
        return db.FindUserByID(ctx, 123)
    },
)
if err == local.ErrNotFound {
    // 用户不存在
} else if err != nil {
    // 其他错误
}
```

### Del

删除指定 key 的缓存。

```go
func (c *Cache) Del(ctx context.Context, keys ...string) error
```

**参数**：
- `ctx`：上下文
- `keys`：要删除的 key 列表

**示例**：

```go
// 删除单个 key
cache.Del(ctx, "user:123")

// 批量删除
cache.Del(ctx, "user:123", "user:456", "user:789")
```

### Stop

停止定期清理（优雅关闭时调用）。

```go
func (c *Cache) Stop()
```

**示例**：

```go
cache := local.NewCache(1000)
defer cache.Stop()
```

### Len

返回当前缓存条目数（用于监控）。

```go
func (c *Cache) Len() int
```

**示例**：

```go
fmt.Printf("当前缓存条目数: %d\n", cache.Len())
```

## 配置选项

### WithPrefix

为所有 key 添加前缀。

```go
cache := local.NewCache(1000, local.WithPrefix("myapp"))

// 实际存储的 key: "myapp:user:123"
cache.GetOrLoad(ctx, "user:123", ttl, &user, loader)
```

### WithCodec

自定义序列化方式（默认 JSON）。

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

设置 TTL 抖动比例（防止缓存雪崩）。

```go
// 抖动 10%：10 分钟 TTL 会变成 10~11 分钟
cache := local.NewCache(1000, local.WithJitter(0.1))

// 禁用抖动
cache := local.NewCache(1000, local.WithJitter(0))
```

### WithNegativeTTL

设置负缓存 TTL（防穿透）。

```go
// NotFound 也会缓存 60 秒
cache := local.NewCache(1000, local.WithNegativeTTL(60*time.Second))
```

### WithIsNotFound

自定义 NotFound 判断（例如集成 GORM）。

```go
import "gorm.io/gorm"

cache := local.NewCache(1000, local.WithIsNotFound(func(err error) bool {
    return errors.Is(err, gorm.ErrRecordNotFound) ||
           errors.Is(err, local.ErrNotFound)
}))
```

### WithOnError

错误监控和日志。

```go
cache := local.NewCache(1000, local.WithOnError(
    func(ctx context.Context, op, key string, err error) {
        log.Printf("缓存错误: op=%s key=%s err=%v", op, key, err)

        // 打点监控
        metrics.Incr("cache.error", map[string]string{
            "op": op,
        })
    },
))
```

### WithNow

自定义时间函数（用于测试）。

```go
mockNow := func() time.Time {
    return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
}

cache := local.NewCache(1000, local.WithNow(mockNow))
```

## 工作原理

### GetOrLoad 流程

```
1. 查询本地缓存
   ├─ 命中 → 返回
   │  └─ 更新访问时间（LRU）
   └─ 未命中 ↓

2. Singleflight 防击穿
   ├─ 双重检查缓存
   │  └─ 命中 → 返回
   └─ 调用 loader ↓

3. 处理 loader 结果
   ├─ 成功 → 写入缓存 → 返回
   └─ NotFound → 写入负缓存 → 返回 ErrNotFound
```

### LRU 驱逐策略

```
1. 写入缓存时检查容量
   ├─ 未超限 → 直接写入
   └─ 超限 ↓

2. 清理过期条目
   ├─ 清理后未超限 → 写入
   └─ 仍超限 ↓

3. LRU 驱逐
   └─ 驱逐最久未访问的条目
```

### 定期清理

```
每隔 cleanupInterval（默认 1 分钟）：
├─ 扫描所有条目
└─ 删除已过期的条目
```

## 使用场景

### 场景 1：热数据缓存

```go
// 缓存热门用户信息
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

### 场景 2：防穿透

```go
// 缓存 NotFound 结果，防止无效查询穿透到 DB
cache := local.NewCache(1000,
    local.WithNegativeTTL(60*time.Second),
    local.WithIsNotFound(func(err error) bool {
        return errors.Is(err, gorm.ErrRecordNotFound) ||
               errors.Is(err, local.ErrNotFound)
    }),
)
```

### 场景 3：配合 Redis 多层缓存

```go
import (
    "github.com/everyday-items/toolkit/cache/local"
    "github.com/everyday-items/toolkit/cache/multi"
    "github.com/everyday-items/toolkit/cache/redis"
)

// 创建本地缓存
localCache := local.NewCache(1000)

// 创建 Redis 缓存
rdb := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
redisCache := redis.NewStableCache(rdb)

// 组合为多层缓存
cache := multi.NewBuilder().
    WithLocal(localCache, 10*time.Minute).
    WithRedis(redisCache, 60*time.Minute).
    Build()

// 使用（自动处理 Local → Redis → DB）
var user User
err := cache.GetOrLoad(ctx, "user:123", &user,
    func(ctx context.Context) (any, error) {
        return db.FindUserByID(ctx, 123)
    },
)
```

## 最佳实践

### 1. 合理设置容量

```go
// 根据内存和数据大小设置容量
// 1000 条 = 约 100KB ~ 1MB（取决于对象大小）
cache := local.NewCache(1000)
```

### 2. 控制 TTL

```go
// 热数据：5-10 分钟
cache.GetOrLoad(ctx, key, 10*time.Minute, &user, loader)

// 温数据：1-3 分钟
cache.GetOrLoad(ctx, key, 3*time.Minute, &config, loader)

// 冷数据：不建议用本地缓存（用 Redis）
```

### 3. 更新时删除缓存

```go
// 更新用户信息后删除缓存
func UpdateUser(ctx context.Context, user User) error {
    if err := db.UpdateUser(ctx, user); err != nil {
        return err
    }

    // 删除本地缓存
    cache.Del(ctx, fmt.Sprintf("user:%d", user.ID))
    return nil
}
```

### 4. 监控缓存状态

```go
cache := local.NewCache(1000, local.WithOnError(
    func(ctx context.Context, op, key string, err error) {
        // 打点监控
        metrics.Incr("local_cache.error", map[string]string{
            "op": op,
        })
    },
))

// 定期打点缓存条目数
go func() {
    ticker := time.NewTicker(time.Minute)
    defer ticker.Stop()
    for range ticker.C {
        metrics.Gauge("local_cache.size", cache.Len())
    }
}()
```

### 5. 优雅关闭

```go
func main() {
    cache := local.NewCache(1000)
    defer cache.Stop() // 停止定期清理

    // ...
}
```

## 注意事项

1. **内存占用**：本地缓存占用进程内存，注意控制容量
2. **不跨进程**：数据仅在当前进程可见，多实例需配合 Redis
3. **更新不一致**：更新数据后需手动删除缓存
4. **容量限制**：超过 maxEntries 会触发 LRU 驱逐，可能导致缓存命中率下降
5. **序列化开销**：每次读写都会序列化/反序列化，注意性能影响

## 与 Redis 缓存对比

| 特性 | Local Cache | Redis Cache |
|------|------------|-------------|
| 速度 | 极快（内存） | 快（网络） |
| 容量 | 受进程内存限制 | 几乎无限 |
| 持久化 | 无 | 支持 |
| 跨进程共享 | 不支持 | 支持 |
| 运维成本 | 无 | 需要 Redis |
| 适用场景 | 热数据、进程内共享 | 温数据、跨进程共享 |

## 与 Multi 包配合

`cache/multi` 提供了多层缓存封装，推荐配合使用：

```go
// multi 包自动处理 Local → Redis → DB
cache := multi.NewBuilder().
    WithLocal(localCache, 10*time.Minute).
    WithRedis(redisCache, 60*time.Minute).
    Build()

// 只需关心 DB 查询
var user User
err := cache.GetOrLoad(ctx, "user:123", &user,
    func(ctx context.Context) (any, error) {
        return db.FindUserByID(ctx, 123)
    },
)
```

详见 `cache/multi/README.md`。

## 完整示例

参见 `examples/cache/local/` 目录。

## 错误码

| 错误 | 说明 | 处理建议 |
|------|------|---------|
| `ErrNotFound` | 数据不存在（负缓存命中） | 正常业务逻辑 |
| `ErrInvalidKey` | key 为空 | 检查 key 参数 |
| `ErrInvalidDest` | dest 不是有效指针 | 传入非 nil 指针 |
| `ErrInvalidLoader` | loader 为 nil | 检查 loader 参数 |
| `ErrCorrupt` | 缓存数据损坏 | 删除缓存重试 |

## 性能优化

### 1. 减少序列化开销

```go
// 使用 Msgpack 代替 JSON（更快、更小）
cache := local.NewCache(1000, local.WithCodec(MsgpackCodec{}))
```

### 2. 合理设置清理间隔

```go
// 热点数据多：缩短清理间隔
cache := local.NewCacheWithCleanup(1000, 30*time.Second)

// 数据更新慢：延长清理间隔或禁用
cache := local.NewCacheWithCleanup(1000, 5*time.Minute)
```

### 3. 避免缓存大对象

```go
// 不推荐：缓存整个列表
var users []User // 可能很大
cache.GetOrLoad(ctx, "all_users", ttl, &users, loader)

// 推荐：缓存单个对象
var user User
cache.GetOrLoad(ctx, fmt.Sprintf("user:%d", id), ttl, &user, loader)
```

## 测试

```bash
# 运行测试
go test ./cache/local

# 查看覆盖率
go test -cover ./cache/local

# 基准测试
go test -bench=. ./cache/local
```
