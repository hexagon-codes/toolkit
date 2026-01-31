# Redis Cache

Redis 分布式缓存，提供 **StableCache**（稳定 key）和 **UnstableCache**（不稳定 key）两种缓存策略，支持版本号失效、TTL 抖动和防击穿。

## 特性

- **双缓存策略**：
  - **StableCache**：适合单条记录查询（key 确定性强）
  - **UnstableCache**：适合聚合查询、JOIN、列表（key 不确定）
- **版本号失效**：一次递增版本号，批量失效所有相关缓存
- **TTL 抖动**：防止缓存雪崩
- **防击穿**：使用 Singleflight 防止缓存击穿
- **防穿透**：支持负缓存（缓存 NotFound）
- **异步写入**：缓存写入不阻塞主流程
- **超时控制**：支持读写超时配置
- **错误降级**：Redis 错误时自动降级到 DB

## 安装

```bash
go get github.com/everyday-items/toolkit/cache/redis
go get github.com/redis/go-redis/v9
```

## 快速开始

### StableCache - 单条记录查询

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
    // 创建 Redis 客户端
    rdb := goredis.NewClient(&goredis.Options{
        Addr: "localhost:6379",
    })

    // 创建稳定 key 缓存
    cache := redis.NewStableCache(rdb)

    ctx := context.Background()

    // 获取或加载数据
    var user User
    err := cache.GetOrLoad(ctx, "user:123", 60*time.Minute, &user,
        func(ctx context.Context) (any, error) {
            return fetchUserFromDB(ctx, 123)
        },
    )
    if err == redis.ErrNotFound {
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

### UnstableCache - 聚合查询

```go
// 创建不稳定 key 缓存（带版本号）
cache := redis.NewUnstableCache(rdb, "ability:version")

// 获取聚合数据（key 会自动加上版本号）
var models []string
err := cache.GetOrLoad(ctx, "ability:group:chat", 5*time.Minute, &models,
    func(ctx context.Context) (any, error) {
        return db.GetGroupEnabledModels(ctx, "chat")
    },
)

// 更新数据后，递增版本号（所有相关缓存失效）
cache.InvalidateVersion(ctx)
```

## API 文档

### StableCache

适合 **单条记录查询**，key 确定性强，支持精确失效。

#### NewStableCache

创建稳定 key 缓存。

```go
func NewStableCache(client redis.UniversalClient, opts ...Option) *StableCache
```

**参数**：
- `client`：Redis 客户端（支持单点、哨兵、集群）
- `opts`：可选配置项

**示例**：

```go
rdb := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
cache := redis.NewStableCache(rdb)
```

#### GetOrLoad

获取缓存数据，未命中时调用 loader 加载。

```go
func (c *StableCache) GetOrLoad(
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
- `redis.ErrNotFound`：数据不存在（负缓存命中）
- `redis.ErrInvalidKey`：key 为空
- `redis.ErrInvalidDest`：dest 不是有效指针
- `redis.ErrInvalidLoader`：loader 为 nil
- `redis.ErrCorrupt`：缓存数据损坏
- 其他错误：loader 返回的错误

**示例**：

```go
var user User
err := cache.GetOrLoad(ctx, "user:123", 60*time.Minute, &user,
    func(ctx context.Context) (any, error) {
        return db.FindUserByID(ctx, 123)
    },
)
```

#### Del

删除指定 key（精确失效）。

```go
func (c *StableCache) Del(ctx context.Context, keys ...string) error
```

**示例**：

```go
// 删除单个 key
cache.Del(ctx, "user:123")

// 批量删除
cache.Del(ctx, "user:123", "user:456")
```

#### Set

主动写入缓存（Write-Through 模式）。

```go
func (c *StableCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error
```

**示例**：

```go
user := User{ID: 123, Name: "Alice"}
cache.Set(ctx, "user:123", user, 60*time.Minute)
```

### UnstableCache

适合 **聚合查询、JOIN、列表**，key 不确定，支持版本号批量失效。

#### NewUnstableCache

创建不稳定 key 缓存。

```go
func NewUnstableCache(client redis.UniversalClient, versionKey string, opts ...Option) *UnstableCache
```

**参数**：
- `client`：Redis 客户端
- `versionKey`：版本号存储的 Redis key（如 `"ability:version"`）
- `opts`：可选配置项

**示例**：

```go
rdb := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
cache := redis.NewUnstableCache(rdb, "ability:version")
```

#### GetOrLoad

获取聚合数据（key 自动加上版本号）。

```go
func (c *UnstableCache) GetOrLoad(
    ctx context.Context,
    key string,
    ttl time.Duration,
    dest any,
    loader func(ctx context.Context) (any, error),
) error
```

**示例**：

```go
// key 会变成 "ability:group:chat:v1"
var models []string
err := cache.GetOrLoad(ctx, "ability:group:chat", 5*time.Minute, &models,
    func(ctx context.Context) (any, error) {
        return db.GetGroupEnabledModels(ctx, "chat")
    },
)
```

#### GetOrLoadWithoutVersion

不使用版本号的加载（使用短 TTL + 批量删除）。

```go
func (c *UnstableCache) GetOrLoadWithoutVersion(
    ctx context.Context,
    key string,
    ttl time.Duration,
    dest any,
    loader func(ctx context.Context) (any, error),
) error
```

**示例**：

```go
var abilities []Ability
err := cache.GetOrLoadWithoutVersion(ctx, "ability:all:enabled", 2*time.Minute, &abilities, loader)
```

#### InvalidateVersion

递增版本号（使所有使用版本号的 key 失效）。

```go
func (c *UnstableCache) InvalidateVersion(ctx context.Context) error
```

**使用场景**：
- 更新 Ability 后调用
- 更新 Channel 后调用
- 更新配置后调用

**示例**：

```go
// 更新数据后，递增版本号
func UpdateAbility(ctx context.Context, ability Ability) error {
    if err := db.UpdateAbility(ctx, ability); err != nil {
        return err
    }

    // 所有 ability 相关缓存失效
    return cache.InvalidateVersion(ctx)
}
```

#### InvalidatePattern

批量删除匹配的 key（不使用版本号时）。

```go
func (c *UnstableCache) InvalidatePattern(ctx context.Context, pattern string) error
```

**注意**：
- 生产环境使用 SCAN 而不是 KEYS，避免阻塞
- 集群模式会遍历所有 master 节点

**示例**：

```go
// 删除所有 "ability:group:*" key
cache.InvalidatePattern(ctx, "ability:group:*")
```

#### Del

删除指定 key。

```go
func (c *UnstableCache) Del(ctx context.Context, keys ...string) error
```

**示例**：

```go
cache.Del(ctx, "ability:group:chat:v1", "ability:group:image:v1")
```

#### GetVersion

获取当前版本号。

```go
func (c *UnstableCache) GetVersion() int64
```

**示例**：

```go
version := cache.GetVersion()
fmt.Printf("当前版本: %d\n", version)
```

## 配置选项

### WithPrefix

为所有 key 添加前缀。

```go
cache := redis.NewStableCache(rdb, redis.WithPrefix("myapp"))

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

cache := redis.NewStableCache(rdb, redis.WithCodec(MsgpackCodec{}))
```

### WithJitter

设置 TTL 抖动比例（防止缓存雪崩）。

```go
// 抖动 10%：60 分钟 TTL 会变成 60~66 分钟
cache := redis.NewStableCache(rdb, redis.WithJitter(0.1))

// 禁用抖动
cache := redis.NewStableCache(rdb, redis.WithJitter(0))
```

### WithNegativeTTL

设置负缓存 TTL（防穿透）。

```go
// NotFound 也会缓存 60 秒
cache := redis.NewStableCache(rdb, redis.WithNegativeTTL(60*time.Second))
```

### WithMaxTTL

设置最大 TTL 上限（主要用于 UnstableCache）。

```go
// 聚合数据最多缓存 15 分钟
cache := redis.NewUnstableCache(rdb, "version", redis.WithMaxTTL(15*time.Minute))
```

### WithRedisTimeout

设置 Redis 读写超时。

```go
// 读超时 100ms，写超时 200ms
cache := redis.NewStableCache(rdb,
    redis.WithRedisTimeout(100*time.Millisecond, 200*time.Millisecond),
)
```

### WithIsNotFound

自定义 NotFound 判断（例如集成 GORM）。

```go
import "gorm.io/gorm"

cache := redis.NewStableCache(rdb, redis.WithIsNotFound(func(err error) bool {
    return errors.Is(err, gorm.ErrRecordNotFound) ||
           errors.Is(err, redis.ErrNotFound)
}))
```

### WithOnError

错误监控和日志。

```go
cache := redis.NewStableCache(rdb, redis.WithOnError(
    func(ctx context.Context, op, key string, err error) {
        log.Printf("Redis 错误: op=%s key=%s err=%v", op, key, err)

        // 打点监控
        metrics.Incr("redis.error", map[string]string{
            "op": op,
        })
    },
))
```

## 工作原理

### StableCache 流程

```
1. 查询 Redis 缓存
   ├─ 命中 → 解包 → 返回
   └─ 未命中 ↓

2. Singleflight 防击穿
   ├─ 双重检查缓存
   │  └─ 命中 → 返回
   └─ 调用 loader ↓

3. 处理 loader 结果
   ├─ 成功 → 异步写入 Redis → 返回
   └─ NotFound → 异步写入负缓存 → 返回 ErrNotFound

4. Redis 错误处理
   └─ 降级到直接调用 loader
```

### UnstableCache 流程（带版本号）

```
1. 获取版本号
   └─ version = 1（从 Redis 读取）

2. 拼接 key
   └─ "ability:group:chat" → "ability:group:chat:v1"

3. 查询 Redis
   ├─ 命中 → 返回
   └─ 未命中 → 加载 → 写入

4. 更新数据后
   └─ InvalidateVersion() → version = 2
   └─ 旧 key "ability:group:chat:v1" 自动失效
```

### TTL 抖动

```
原始 TTL: 60 分钟
抖动比例: 0.1 (10%)

计算：
├─ maxDelta = 60 * 0.1 = 6 分钟
├─ delta = random(0, 6) = 4 分钟
└─ 实际 TTL = 60 + 4 = 64 分钟

效果：
└─ 避免大量缓存同时过期（防雪崩）
```

## 使用场景

### 场景 1：单条记录查询（StableCache）

```go
// 用户信息
cache := redis.NewStableCache(rdb, redis.WithPrefix("user"))

var user User
err := cache.GetOrLoad(ctx, fmt.Sprintf("user:%d", userID), 60*time.Minute, &user,
    func(ctx context.Context) (any, error) {
        return db.FindUserByID(ctx, userID)
    },
)

// 更新用户后删除缓存
cache.Del(ctx, fmt.Sprintf("user:%d", userID))
```

### 场景 2：聚合查询（UnstableCache + 版本号）

```go
// Ability 聚合查询
cache := redis.NewUnstableCache(rdb, "ability:version",
    redis.WithMaxTTL(15*time.Minute),
)

// 获取聚合数据
var models []string
err := cache.GetOrLoad(ctx, "ability:group:chat", 5*time.Minute, &models,
    func(ctx context.Context) (any, error) {
        return db.GetGroupEnabledModels(ctx, "chat")
    },
)

// 更新 Ability 后递增版本号
cache.InvalidateVersion(ctx)
```

### 场景 3：JOIN 查询（UnstableCache + 短 TTL）

```go
// JOIN 查询使用短 TTL
cache := redis.NewUnstableCache(rdb, "ability:version")

var abilities []Ability
err := cache.GetOrLoad(ctx, "ability:all:with_channels", redis.UnstableCacheTTLShort, &abilities,
    func(ctx context.Context) (any, error) {
        return db.GetAllEnableAbilityWithChannels(ctx)
    },
)
```

### 场景 4：列表查询（UnstableCache + Pattern 删除）

```go
// 用户列表（不使用版本号）
cache := redis.NewUnstableCache(rdb, "user:list:version")

var users []User
key := fmt.Sprintf("user:list:page:%d:size:%d", page, size)
err := cache.GetOrLoadWithoutVersion(ctx, key, 2*time.Minute, &users,
    func(ctx context.Context) (any, error) {
        return db.GetUserList(ctx, page, size)
    },
)

// 更新用户后批量删除列表缓存
cache.InvalidatePattern(ctx, "user:list:*")
```

### 场景 5：配合 Local 多层缓存

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

### 1. 选择合适的缓存类型

```go
// StableCache：单条记录查询
// ✅ GetUserByID(id)
// ✅ GetChannelByID(id)
// ✅ GetAbilityByID(id)

cache := redis.NewStableCache(rdb)

// UnstableCache：聚合查询、JOIN、列表
// ✅ GetGroupEnabledModels(group)
// ✅ GetAllEnableAbilityWithChannels()
// ✅ GetUserList(page, size)

cache := redis.NewUnstableCache(rdb, "version")
```

### 2. 合理设置 TTL

```go
// StableCache：长 TTL（数据变化少）
cache.GetOrLoad(ctx, "user:123", redis.StableCacheTTL, &user, loader) // 60 分钟

// UnstableCache：短 TTL（数据变化多）
cache.GetOrLoad(ctx, "ability:group:chat", redis.UnstableCacheTTLShort, &models, loader) // 5 分钟
cache.GetOrLoad(ctx, "ability:all:join", redis.UnstableCacheTTLMedium, &abilities, loader) // 10 分钟
```

### 3. 更新时失效缓存

```go
// StableCache：精确删除
func UpdateUser(ctx context.Context, user User) error {
    if err := db.UpdateUser(ctx, user); err != nil {
        return err
    }
    return cache.Del(ctx, fmt.Sprintf("user:%d", user.ID))
}

// UnstableCache：版本号失效
func UpdateAbility(ctx context.Context, ability Ability) error {
    if err := db.UpdateAbility(ctx, ability); err != nil {
        return err
    }
    return cache.InvalidateVersion(ctx)
}
```

### 4. 防穿透

```go
cache := redis.NewStableCache(rdb,
    redis.WithNegativeTTL(60*time.Second),
    redis.WithIsNotFound(func(err error) bool {
        return errors.Is(err, gorm.ErrRecordNotFound) ||
               errors.Is(err, redis.ErrNotFound)
    }),
)
```

### 5. 监控 Redis 错误

```go
cache := redis.NewStableCache(rdb, redis.WithOnError(
    func(ctx context.Context, op, key string, err error) {
        // 打点监控
        metrics.Incr("redis.error", map[string]string{
            "op": op,
        })

        // 严重错误告警
        if errors.Is(err, context.DeadlineExceeded) {
            alert.Send("Redis 超时", fmt.Sprintf("op=%s key=%s", op, key))
        }
    },
))
```

### 6. 控制超时

```go
// 读超时 50ms，写超时 50ms（默认）
cache := redis.NewStableCache(rdb,
    redis.WithRedisTimeout(50*time.Millisecond, 50*time.Millisecond),
)

// 超时后自动降级到 DB
```

## 注意事项

1. **版本号只增不减**：`InvalidateVersion()` 只会递增版本号，不会减少
2. **版本号持久化**：版本号存储在 Redis，重启不丢失
3. **Pattern 删除性能**：`InvalidatePattern()` 使用 SCAN，大量 key 时可能较慢
4. **集群模式限制**：Pattern 删除需要遍历所有 master 节点
5. **异步写入**：缓存写入是异步的，可能有短暂延迟
6. **超时降级**：Redis 超时会自动降级到 DB，确保可用性

## StableCache vs UnstableCache

| 特性 | StableCache | UnstableCache |
|------|------------|--------------|
| 适用场景 | 单条记录查询 | 聚合查询、JOIN、列表 |
| Key 特点 | 确定性强 | 不确定 |
| 失效方式 | 精确删除 | 版本号/Pattern 删除 |
| TTL | 长（60 分钟） | 短（5-15 分钟） |
| 示例 | GetUserByID | GetGroupEnabledModels |

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

// 删除缓存（所有层）
cache.Del(ctx, "user:123")
```

详见 `cache/multi/README.md`。

## 完整示例

参见 `examples/cache/redis/` 目录。

## 错误码

| 错误 | 说明 | 处理建议 |
|------|------|---------|
| `ErrNotFound` | 数据不存在（负缓存命中） | 正常业务逻辑 |
| `ErrInvalidKey` | key 为空 | 检查 key 参数 |
| `ErrInvalidDest` | dest 不是有效指针 | 传入非 nil 指针 |
| `ErrInvalidLoader` | loader 为 nil | 检查 loader 参数 |
| `ErrCorrupt` | 缓存数据损坏 | 删除缓存重试 |

## 性能优化

### 1. 使用 Msgpack 序列化

```go
// Msgpack 比 JSON 更快、更小
cache := redis.NewStableCache(rdb, redis.WithCodec(MsgpackCodec{}))
```

### 2. 启用 TTL 抖动

```go
// 防止缓存雪崩
cache := redis.NewStableCache(rdb, redis.WithJitter(0.1))
```

### 3. 控制 MaxTTL

```go
// 限制聚合数据缓存时间
cache := redis.NewUnstableCache(rdb, "version", redis.WithMaxTTL(15*time.Minute))
```

### 4. 异步写入

```go
// 缓存写入是异步的，不阻塞主流程（默认行为）
cache.GetOrLoad(ctx, key, ttl, &user, loader)
```

## 测试

```bash
# 运行测试（需要 Redis）
go test ./cache/redis

# 使用 Docker 启动 Redis
docker run -d -p 6379:6379 redis:7

# 查看覆盖率
go test -cover ./cache/redis
```

## 依赖

- `github.com/redis/go-redis/v9`：Redis 客户端
- `golang.org/x/sync/singleflight`：防击穿
- `github.com/bytedance/gopkg/util/gopool`：异步 goroutine 池

## 常见问题

### 1. 如何选择 StableCache 还是 UnstableCache？

```go
// 看查询类型：
// - 单条记录查询 → StableCache
// - 聚合/JOIN/列表 → UnstableCache

// StableCache
cache.GetOrLoad(ctx, "user:123", ttl, &user, loader)

// UnstableCache
cache.GetOrLoad(ctx, "ability:group:chat", ttl, &models, loader)
```

### 2. 版本号什么时候递增？

```go
// 更新数据后调用 InvalidateVersion()
func UpdateAbility(ctx context.Context, ability Ability) error {
    if err := db.UpdateAbility(ctx, ability); err != nil {
        return err
    }
    return cache.InvalidateVersion(ctx) // 版本号递增
}
```

### 3. Redis 错误会影响可用性吗？

```go
// 不会，Redis 错误会自动降级到 DB
cache.GetOrLoad(ctx, key, ttl, &user, func(ctx context.Context) (any, error) {
    // Redis 错误时直接调用这里
    return db.FindUserByID(ctx, 123)
})
```

### 4. 如何监控缓存命中率？

```go
cache := redis.NewStableCache(rdb, redis.WithOnError(
    func(ctx context.Context, op, key string, err error) {
        metrics.Incr("redis.error", map[string]string{"op": op})
    },
))

// 在 loader 里打点
cache.GetOrLoad(ctx, key, ttl, &user, func(ctx context.Context) (any, error) {
    metrics.Incr("cache.miss") // 缓存未命中
    return db.FindUserByID(ctx, 123)
})
```

### 5. 集群模式下 InvalidatePattern 会很慢吗？

```go
// 会遍历所有 master 节点，大量 key 时可能较慢
// 建议：
// 1. 优先使用版本号失效
// 2. 限制 pattern 范围
// 3. 使用短 TTL

cache.InvalidatePattern(ctx, "ability:group:*") // 限制范围
```
