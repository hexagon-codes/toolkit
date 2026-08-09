中文 | [English](README.en.md)

# Redis 客户端封装

本包基于 `github.com/redis/go-redis/v9`，提供单机、Cluster、Sentinel、健康检查和分布式锁能力。连接配置复用 `infra/redisconn` 的统一模型；本包不维护全局客户端。

## 认证策略

本项目采用明确的现代认证契约：

- 无认证：`DataCredentials` 的用户名和密码都留空。
- ACL 认证：用户名和密码必须同时提供。
- 仅密码：本项目主动拒绝，并返回 `ErrInvalidCredentials`。

“仅密码被拒绝”是本项目的安全与接口策略，不是 Redis 6+ 的技术限制。Redis 6+ 的 [`AUTH [username] password`](https://redis.io/docs/latest/commands/auth/) 仍允许省略用户名，并将其解释为 `default` 用户，以兼容旧客户端。使用本包连接旧版 Redis 或 `requirepass`-only 部署前，应先迁移为具名 ACL 用户。

## 快速开始

`New` 接收调用方的 `context.Context`，校验配置并执行 `PING`；只有连接和认证成功才返回客户端。每次调用都会创建独立客户端，调用方负责注入和关闭它。

```go
package main

import (
    "context"
    "log"
    "os"
    "time"

    "github.com/hexagon-codes/toolkit/infra/db/redis"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    config := redis.DefaultConfig(redis.ModeSingle, os.Getenv("REDIS_ADDR"))
    config.DataCredentials = redis.Credentials{
        Username: os.Getenv("REDIS_USERNAME"),
        Password: os.Getenv("REDIS_PASSWORD"),
    }

    client, err := redis.New(ctx, config)
    if err != nil {
        log.Fatal(err)
    }
    defer func() {
        if err := client.Close(); err != nil {
            log.Printf("close Redis: %v", err)
        }
    }()

    if err := client.Set(context.Background(), "example:key", "value", time.Minute).Err(); err != nil {
        log.Fatal(err)
    }
}
```

若 Redis 不要求认证，不要设置 `DataCredentials`。如果环境变量中只存在用户名或密码，配置校验会失败。

## 部署模式

### Single

单机模式必须且只能提供一个地址：

```go
config := redis.DefaultConfig(redis.ModeSingle, os.Getenv("REDIS_ADDR"))
```

### Cluster

Cluster 接受一个或多个 seed；一个 seed 即可启动拓扑发现，生产环境通常配置多个 seed 以提高引导可用性。Cluster 只支持 `DB == 0`。

```go
config := redis.DefaultConfig(
    redis.ModeCluster,
    os.Getenv("REDIS_CLUSTER_SEED_1"),
    os.Getenv("REDIS_CLUSTER_SEED_2"),
)
config.DataCredentials = redis.Credentials{
    Username: os.Getenv("REDIS_USERNAME"),
    Password: os.Getenv("REDIS_PASSWORD"),
}
```

只配置单个 seed 也是合法的：

```go
config := redis.DefaultConfig(redis.ModeCluster, os.Getenv("REDIS_CLUSTER_SEED"))
```

### Sentinel

Sentinel 模式有两套独立凭据：

- `SentinelCredentials` 用于客户端连接 Sentinel。
- `DataCredentials` 用于客户端连接 Sentinel 发现的 Redis 主节点和副本。

两套凭据都遵循“全空或用户名、密码同时非空”的规则。

```go
config := redis.DefaultConfig(
    redis.ModeSentinel,
    os.Getenv("REDIS_SENTINEL_ADDR_1"),
    os.Getenv("REDIS_SENTINEL_ADDR_2"),
)
config.MasterName = os.Getenv("REDIS_MASTER_NAME")
config.SentinelCredentials = redis.Credentials{
    Username: os.Getenv("REDIS_SENTINEL_USERNAME"),
    Password: os.Getenv("REDIS_SENTINEL_PASSWORD"),
}
config.DataCredentials = redis.Credentials{
    Username: os.Getenv("REDIS_USERNAME"),
    Password: os.Getenv("REDIS_PASSWORD"),
}
```

### TLS

通过 `TLSConfig` 启用 TLS。以下配置使用系统根证书并保持服务端证书和主机名校验；不要设置 `InsecureSkipVerify`。

```go
config := redis.DefaultConfig(redis.ModeSingle, os.Getenv("REDIS_TLS_ADDR"))
config.TLSConfig = &tls.Config{
    MinVersion: tls.VersionTLS12,
    ServerName: os.Getenv("REDIS_TLS_SERVER_NAME"),
}
```

需要私有 CA 或 mTLS 时，将经过验证的 CA 池和客户端证书设置到同一个 `tls.Config`。

## 动态数据节点凭据

`CredentialsProvider` 在新连接初始化时解析数据节点凭据，适合凭据轮换。它与静态 `DataCredentials` 互斥，且必须返回完整的用户名和密码。它不提供 Sentinel 自身的动态凭据；Sentinel 认证仍使用 `SentinelCredentials`。

```go
config := redis.DefaultConfig(redis.ModeSingle, os.Getenv("REDIS_ADDR"))
config.CredentialsProvider = func(ctx context.Context) (redis.Credentials, error) {
    if err := ctx.Err(); err != nil {
        return redis.Credentials{}, err
    }
    return redis.Credentials{
        Username: os.Getenv("REDIS_USERNAME"),
        Password: os.Getenv("REDIS_PASSWORD"),
    }, nil
}
```

生产环境可将环境变量读取替换为支持轮换的 Secret Manager，但不要记录凭据值。

## 健康检查、操作与关闭

`Client` 嵌入了 go-redis 的 `UniversalClient`，可直接调用其命令、Pipeline 和事务 API。本包还提供少量便捷方法。

```go
healthCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()
if err := client.Health(healthCtx); err != nil {
    return err
}

value, err := client.GetWithDefault(healthCtx, "cache:key", "fallback")
if err != nil {
    return err
}

if err := client.DeleteKeys(healthCtx, "cache:key"); err != nil {
    return err
}
```

`GetWithDefault` 只在 key 不存在时返回默认值；网络、认证和 context 错误仍会返回。应用关闭时应调用 `Close`，不要依赖进程级全局清理。

## 分布式锁

优先使用 `WithLock`，它会校验锁所有权并自动释放；业务函数错误与释放错误都会返回给调用方。

```go
err := redis.WithLock(ctx, client, "lock:resource", 30*time.Second, func() error {
    return updateResource(ctx)
})
if err != nil {
    return err
}
```

需要手动控制时使用 `NewLock`、`Acquire`、`AcquireWithRetry`、`Refresh`、`TTL` 和 `Release`。过期时间必须大于零并覆盖预期业务时长；`Acquire` 和 `WithLock` 会在写 Redis 前对非正值返回 `ErrInvalidConfig`。长任务应定期 `Refresh`，并始终检查释放错误。该实现是单 Redis 实例上的所有权锁，不是多主 Redlock。

## 从旧 API 迁移

这是 breaking migration，不保留旧 API 兼容层：

| 旧接口或字段 | 新接口或字段 |
| --- | --- |
| `DefaultConfig(addr)` | `DefaultConfig(ModeSingle, addr)`，返回值类型 `Config` |
| `DefaultClusterConfig(addrs)` | `DefaultConfig(ModeCluster, addrs...)` |
| `Addr` / `SentinelAddrs` | 统一为 `Addrs` |
| `Password` | `DataCredentials{Username, Password}` |
| `Init(config)` / `GetGlobal()` | 在组合根调用 `New(ctx, config)`，显式注入 `*Client` |
| `IdleTimeout` | `ConnMaxIdleTime` |
| `GetWithDefault(...) string` | `GetWithDefault(...) (string, error)` |

`Logger`、`StdLogger` 和 `IdleCheckFrequency` 已删除。需要共享连接时，由应用组合根创建一个 `*Client` 并注入消费者；库内部不再采用 first-config-wins 的全局生命周期。

配置错误可通过 `errors.Is` 判断 `ErrInvalidConfig`、`ErrInvalidCredentials`、`ErrCredentialsProvider` 和 `ErrInvalidContext`。
