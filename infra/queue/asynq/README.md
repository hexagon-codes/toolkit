中文 | [English](README.en.md)

# Asynq Queue Package

这是基于 `hibiken/asynq` 的生产级任务队列封装。Manager 统一拥有 Asynq client、worker、scheduler、inspector，以及轮询/迁移锁使用的专用 Redis client。

## Redis 连接契约

本包只接受 `infra/redisconn.Config`，不再复制地址、用户名或密码字段，也不会根据地址数量猜测拓扑。

| 模式 | 必填字段 | 说明 |
|---|---|---|
| `redisconn.ModeSingle` | 1 个 `Addrs` | 单机或云 Redis 代理端点 |
| `redisconn.ModeCluster` | 至少 1 个 `Addrs` | 即使只有一个 seed，也仍创建 Cluster client |
| `redisconn.ModeSentinel` | 至少 1 个 `Addrs`、`MasterName` | Sentinel 发现主节点 |

认证策略需要区分 Redis 能力和项目约束：Redis 6+ 协议仍兼容 default user 的单参数 `AUTH password`，但本项目为避免配置歧义，静态凭据只允许“用户名和密码均为空”或“用户名和密码同时非空”。因此 password-only 会在启动前校验失败。

Sentinel 使用两套独立凭据：

- `DataCredentials`：访问 Redis 数据节点。
- `SentinelCredentials`：访问 Sentinel；不要用数据节点凭据代替。

TLS、DB、超时、重试和连接池参数全部由同一份 `redisconn.Config` 传给 Manager 的所有 Redis 客户端。

## 快速开始

```go
startupCtx, cancelStartup := context.WithTimeout(context.Background(), 5*time.Second)
runCtx, cancelRun := context.WithCancel(context.Background())
defer cancelRun()

redisConfig := redisconn.DefaultConfig(
    redisconn.ModeSingle,
    "redis.internal:6379",
)
redisConfig.DataCredentials = redisconn.Credentials{
    Username: "queue-worker",
    Password: os.Getenv("REDIS_PASSWORD"),
}
redisConfig.TLSConfig = &tls.Config{
    MinVersion: tls.VersionTLS12,
    ServerName: "redis.internal",
}

config := asynq.DefaultConfig(redisConfig)
config.Concurrency = 20
config.QueuePrefix = "prod:"

manager, err := asynq.NewManager(startupCtx, config)
if err != nil { // 包括配置、网络和认证失败
    return err
}
cancelStartup()
defer manager.Stop()

manager.RegisterHandler("email:send", handleEmail)
if err := manager.Start(runCtx); err != nil {
    return err
}

_, err = manager.EnqueueTask(
    runCtx,
	"email:send",
	payload,
	hibasynq.Queue(asynq.QueueHigh),
)
```

`NewManager` 在返回前执行 Redis `PING`。错误凭据、不可达地址或非法拓扑会直接返回错误，不会产生“初始化成功但后台不可用”的假象。

## 生命周期与并发

- `Start` 同步启动 worker 和 scheduler；启动失败会回滚本次创建的资源。
- `Stop` 幂等，并关闭 Manager 拥有的所有 Asynq/Redis 客户端，即使从未调用 `Start`。
- `InitManager` 用于进程级单例；重复初始化返回 `ErrManagerAlreadyInitialized`，不会静默沿用第一份配置。
- 队列前缀属于 Manager 实例。`Manager.Enqueue`、`EnqueueTask` 和 `RegisterSchedule` 的 `Queue(...)` 参数，以及 `Config.Queues` key，都必须传未加 namespace 的基名。Manager 统一加前缀并校验结果属于本实例；省略 Queue 时使用 `QueueDefault`，若未配置 default 则返回 `ErrQueueNotFound`。Task 内嵌的 Queue 会被 Manager 的最终路由覆盖。
- `manager.QueueName(base)` 仅用于 Inspector、Asynqmon 等需要实际 Redis 队列名的原生集成。直接使用 `GetClient()` 属于显式 escape hatch，会绕过 Manager 的 namespace 校验。

## 安全租约

轮询和迁移锁共用 token-based Redis lease：

```go
lease, acquired, err := manager.AcquirePollingLease(ctx, taskID)
if err != nil {
    return err // Redis/认证错误默认 fail-closed
}
if !acquired {
    return nil // 另一个 worker 持有 lease
}
defer lease.Release(context.WithoutCancel(ctx))
```

`Refresh` 和 `Release` 都通过 Lua compare-token 执行，过期的旧 owner 无法续租或删除新 owner 的锁。Polling 和 migration 内部固定 key 对非空 `QueuePrefix` 始终执行 `prefix + base`：即使字符串相交也不会逃逸 namespace。不同前缀可独立持锁，相同前缀仍互斥。迁移流程会按 TTL 自动续租；续租失败会取消迁移 context 并返回错误。

## 可选日志适配

配置直接通过 `Config` 注入；不再提供全局 `ConfigProvider` 或第二套 Redis client 全局变量。日志仍可通过 `SetLogger` 或 `manager.SetLogger` 适配。

完整连接、Polling、队列和锁示例见 [USAGE.md](USAGE.md)。
