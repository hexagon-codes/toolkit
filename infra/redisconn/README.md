中文 | [English](README.en.md)

# Redis 统一连接工厂

`redisconn` 基于 `github.com/redis/go-redis/v9`，为 Single、Cluster 和 Sentinel 提供统一、显式且经过校验的连接配置。它只负责配置、客户端构造和启动探活，不维护全局客户端，也不封装 Redis 命令。

## 部署模式

必须显式设置 `Mode`，本包不会根据地址数量猜测拓扑：

| 模式 | 地址要求 | 其他约束 | 返回的 go-redis 客户端 |
| --- | --- | --- | --- |
| `ModeSingle` | 恰好 1 个 Redis 地址 | `DB >= 0` | `*redis.Client` |
| `ModeCluster` | 至少 1 个 Cluster seed | `DB == 0` | `*redis.ClusterClient` |
| `ModeSentinel` | 至少 1 个 Sentinel 地址 | 必须设置 `MasterName`，`DB >= 0` | failover `*redis.Client` |

所有地址都必须非空。Cluster 只配置一个 seed 是合法的，足以启动拓扑发现；生产环境通常配置多个 seed，以降低单个引导节点不可用带来的启动风险。

```go
single := redisconn.DefaultConfig(redisconn.ModeSingle, "redis.internal:6379")

cluster := redisconn.DefaultConfig(
    redisconn.ModeCluster,
    "redis-cluster-1.internal:6379",
    "redis-cluster-2.internal:6379",
)

sentinel := redisconn.DefaultConfig(
    redisconn.ModeSentinel,
    "redis-sentinel-1.internal:26379",
    "redis-sentinel-2.internal:26379",
)
sentinel.MasterName = "primary"
```

`DefaultConfig` 返回已默认化的 `Config`，但不保证配置已经完整有效。例如，Sentinel 的 `MasterName` 仍需调用方设置。`NewFactory` 会再次默认化，复制 `Addrs` 以及 TLS 配置中由本包拥有的可变容器，然后执行完整校验。TLS 中由调用方拥有的函数、接口值和密码学句柄仍需遵守下文的并发约束。

## 现代认证策略

本项目只接受两种静态认证状态：

- 无认证：`Username` 和 `Password` 都为空。
- Redis ACL 认证：`Username` 和 `Password` 同时非空。

仅设置密码或仅设置用户名都会由本项目主动拒绝，并返回 `ErrInvalidCredentials`。这是一项项目级安全与接口决策，并不表示 Redis 6+ 在协议层禁止 password-only。Redis 6+ 引入了 ACL 和具名用户，但 [`AUTH`](https://redis.io/docs/latest/commands/auth/) 仍保留面向 `default` 用户的兼容认证形式。使用本包时，应为生产连接配置具名 ACL 用户；仅有 `requirepass` 的部署需要先迁移认证配置。

```go
config.DataCredentials = redisconn.Credentials{
    Username: os.Getenv("REDIS_USERNAME"),
    Password: os.Getenv("REDIS_PASSWORD"),
}
```

如果 Redis 明确不要求认证，保持 `DataCredentials` 为零值，不要填充占位用户名或密码。

### Sentinel 的两套凭据

Sentinel 模式区分两个独立的认证边界：

- `SentinelCredentials`：连接 Sentinel 节点时使用。
- `DataCredentials`：连接 Sentinel 发现的 Redis 主节点和副本时使用。

两套静态凭据都必须“全空”或“用户名、密码同时非空”。`SentinelCredentials` 只能用于 `ModeSentinel`。

```go
config := redisconn.DefaultConfig(
    redisconn.ModeSentinel,
    os.Getenv("REDIS_SENTINEL_ADDR_1"),
    os.Getenv("REDIS_SENTINEL_ADDR_2"),
)
config.MasterName = os.Getenv("REDIS_MASTER_NAME")
config.SentinelCredentials = redisconn.Credentials{
    Username: os.Getenv("REDIS_SENTINEL_USERNAME"),
    Password: os.Getenv("REDIS_SENTINEL_PASSWORD"),
}
config.DataCredentials = redisconn.Credentials{
    Username: os.Getenv("REDIS_USERNAME"),
    Password: os.Getenv("REDIS_PASSWORD"),
}
```

## 快速开始与生命周期

应用启动时优先使用 `Open`。它会构造客户端并执行 `PING`，只有网络和认证都成功才返回客户端；失败时会关闭刚创建的客户端并返回 `nil`。调用方必须传入带 deadline 的 context（不要直接传 `context.Background()`），为整个启动探活设置总预算，并在应用关闭时调用 `Close`。

```go
package main

import (
    "context"
    "log"
    "os"
    "time"

    "github.com/hexagon-codes/toolkit/infra/redisconn"
)

func main() {
    config := redisconn.DefaultConfig(redisconn.ModeSingle, os.Getenv("REDIS_ADDR"))
    config.DataCredentials = redisconn.Credentials{
        Username: os.Getenv("REDIS_USERNAME"),
        Password: os.Getenv("REDIS_PASSWORD"),
    }

    factory, err := redisconn.NewFactory(config)
    if err != nil {
        log.Fatal(err)
    }

    startupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    client, err := factory.Open(startupCtx)
    if err != nil {
        log.Fatal(err)
    }
    defer func() {
        if err := client.Close(); err != nil {
            log.Printf("close Redis client: %v", err)
        }
    }()

    if err := client.Set(context.Background(), "example:key", "value", time.Minute).Err(); err != nil {
        log.Fatal(err)
    }
}
```

`Factory.NewClient()` 和 `Factory.Open(ctx)` 的生命周期不同：

- `NewClient()` 只创建惰性的 `redis.UniversalClient`，不发起网络请求，也不证明地址、TLS 或认证可用。适合由上层统一安排探活的场景。
- `Open(ctx)` 创建客户端后立即执行 `PING`，适合作为启动期 fail-fast 检查。
- 两者返回的客户端都归调用方所有，成功返回后都必须关闭。
- `Open(nil)` 不会连接，并返回 `ErrInvalidContext`。

## TLS

通过 `TLSConfig` 启用 TLS。以下示例使用系统信任根，并保留证书链与主机名校验：

```go
config := redisconn.DefaultConfig(redisconn.ModeSingle, os.Getenv("REDIS_TLS_ADDR"))
config.TLSConfig = &tls.Config{
    MinVersion: tls.VersionTLS12,
    ServerName: os.Getenv("REDIS_TLS_SERVER_NAME"),
}
```

私有 CA 场景应把经过验证的 CA 池设置到 `RootCAs`；mTLS 还应设置客户端证书。不要通过关闭证书或主机名校验来绕过部署错误。

`Normalize` 和 `NewFactory` 会复制 `tls.Config`、常用切片、证书池和证书字节，避免这些可变容器与调用方共享。但 Go 无法通用深拷贝 TLS 回调、`ClientSessionCache`、`KeyLogWriter`、私钥接口或已解析证书指针等不透明对象；这些对象仍由调用方拥有，创建 Factory 后必须保持不可变或保证并发安全。

## 动态数据节点凭据

`CredentialsProvider` 的签名为：

```go
type CredentialsProvider func(context.Context) (Credentials, error)
```

它会被适配到 go-redis 的 context-aware credentials provider，并在建立新的数据节点连接时读取凭据，因此可以支持轮换。go-redis 可能并发建立连接，因此 Provider 及其访问的状态必须并发安全。返回值仍必须是“全空”或完整的用户名、密码对；不完整凭据会在运行时返回 `ErrInvalidCredentials`。Provider 自身失败会转换为 `ErrCredentialsProvider`，错误消息不会包含凭据值。

```go
config.CredentialsProvider = func(ctx context.Context) (redisconn.Credentials, error) {
    username, password, err := secretStore.RedisACL(ctx)
    if err != nil {
        return redisconn.Credentials{}, err
    }
    return redisconn.Credentials{Username: username, Password: password}, nil
}
```

`CredentialsProvider` 只用于 Redis 数据节点，并与静态 `DataCredentials` 互斥。Sentinel 自身目前只支持静态 `SentinelCredentials`；轮换 Sentinel 凭据时，需要用新配置创建新 Factory 和客户端。

## Cluster 重试语义

Cluster 有两个不同层级的重试配置，不能混用：

- `MaxRedirects`：Cluster 客户端处理网络错误以及 `MOVED` / `ASK` 重定向的命令级重试次数；默认值为 `3`。
- `MaxRetries`：每个底层 Redis 节点客户端的重试次数。Cluster 默认值为 `-1`，即关闭节点级重试，避免与 `MaxRedirects` 形成嵌套重试放大。

Single 和 Sentinel 的 `MaxRetries` 默认值为 `3`。调整重试次数时，应同时评估命令超时、最坏延迟和上游重试，避免多层重试造成延迟与负载放大。

## 默认值与配置字段

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `MaxRetries` | Single/Sentinel `3`；Cluster `-1` | 节点客户端重试 |
| `MaxRedirects` | `3` | 仅 Cluster 使用 |
| `MinRetryBackoff` | `8ms` | 最小重试退避 |
| `MaxRetryBackoff` | `512ms` | 最大重试退避 |
| `DialTimeout` | `5s` | 建连超时 |
| `ReadTimeout` | `3s` | 读超时 |
| `WriteTimeout` | `ReadTimeout`，默认 `3s` | 写超时 |
| `PoolTimeout` | 正数 `ReadTimeout + 1s`，默认 `4s` | 等待连接池超时 |
| `PoolSize` | Single/Sentinel `10 * runtime.GOMAXPROCS(0)`；Cluster `5 * runtime.GOMAXPROCS(0)` | 连接池大小；Cluster 按节点计算 |
| `ConnMaxIdleTime` | `30m` | 连接最大空闲时间 |
| `MinIdleConns`、`MaxIdleConns`、`MaxActiveConns`、`ConnMaxLifetime` | `0` | 保留 go-redis 的零值语义 |

`DB` 默认为 `0`。将受支持字段设置为非零值会覆盖上述默认值；需要禁用 go-redis 支持的重试或超时功能时，必须使用精确的上游哨兵值：`MaxRetries`、`MaxRedirects` 和 retry backoff 使用 `-1`，`ReadTimeout` / `WriteTimeout` 支持 `-1` 和 `-2`，`ConnMaxIdleTime` 使用 `-1`。其他更小的重试或读写超时负值会被拒绝。`DialTimeout`、`PoolTimeout` 和连接池计数字段不得为负，连接池计数还必须落在 go-redis 使用的非负 `int32` 范围内。

## 安全地读取环境变量

不要把凭据写进源码、地址或日志。若环境变量是可选认证来源，应先验证用户名和密码同时存在且同时非空，再放入配置：

```go
func redisCredentialsFromEnv() (redisconn.Credentials, error) {
    username, hasUsername := os.LookupEnv("REDIS_USERNAME")
    password, hasPassword := os.LookupEnv("REDIS_PASSWORD")

    if !hasUsername && !hasPassword {
        return redisconn.Credentials{}, nil
    }
    if !hasUsername || !hasPassword || username == "" || password == "" {
        return redisconn.Credentials{}, errors.New(
            "REDIS_USERNAME and REDIS_PASSWORD must both be non-empty",
        )
    }
    return redisconn.Credentials{Username: username, Password: password}, nil
}
```

生产环境优先使用支持审计与轮换的 Secret Manager，并通过 `CredentialsProvider` 获取数据节点凭据。不要打印完整 `Config`，也不要把用户名、密码或 provider 返回值加入错误消息。

配置和运行时错误可使用 `errors.Is` 分类：`ErrInvalidConfig`、`ErrInvalidCredentials`、`ErrCredentialsProvider` 和 `ErrInvalidContext`。
