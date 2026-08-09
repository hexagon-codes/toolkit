中文 | [English](USAGE.en.md)

# Asynq 使用指南

## 1. 初始化

Manager 必须接收 caller-owned `context.Context` 和一份显式 `Config`：

```go
redisConfig := redisconn.DefaultConfig(
    redisconn.ModeSingle,
    "redis.internal:6379",
)
redisConfig.DataCredentials = redisconn.Credentials{
    Username: "queue-worker",
    Password: os.Getenv("REDIS_PASSWORD"),
}

config := asynq.DefaultConfig(redisConfig)
config.Concurrency = 16
config.QueuePrefix = "prod:"

startupCtx, cancelStartup := context.WithTimeout(context.Background(), 5*time.Second)
manager, err := asynq.NewManager(startupCtx, config)
cancelStartup()
if err != nil {
    return err
}
defer manager.Stop()

manager.RegisterHandler("email:send", handleEmail)
if err := manager.Start(ctx); err != nil {
    return err
}
```

`NewManager` 会规范化并校验配置、创建 canonical Redis factory，并在返回前通过专用 Redis client 执行 `PING`。因此调用方必须处理初始化错误。

如果确实需要进程级单例，可改用：

```go
manager, err := asynq.InitManager(ctx, config)
```

第二次调用返回 `ErrManagerAlreadyInitialized`。不再提供 `ConfigProvider`、`DefaultConfigProvider`、`InitManagerFromConfig`、`SetRedisClient` 或 `GetRedisClient`。

## 2. Redis 拓扑与认证

### 单机或代理

```go
redisConfig := redisconn.DefaultConfig(
    redisconn.ModeSingle,
    "redis.internal:6379",
)
```

Single 模式必须且只能配置一个地址。

### Redis Cluster

```go
redisConfig := redisconn.DefaultConfig(
    redisconn.ModeCluster,
    "redis-0.internal:6379", // 一个 seed 也明确保持 Cluster 模式
)
redisConfig.MaxRedirects = 5
```

拓扑由 `ModeCluster` 决定，不由地址数量推断。

### Redis Sentinel

```go
redisConfig := redisconn.DefaultConfig(
    redisconn.ModeSentinel,
    "sentinel-0.internal:26379",
    "sentinel-1.internal:26379",
)
redisConfig.MasterName = "mymaster"
redisConfig.DataCredentials = redisconn.Credentials{
    Username: "queue-worker",
    Password: os.Getenv("REDIS_DATA_PASSWORD"),
}
redisConfig.SentinelCredentials = redisconn.Credentials{
    Username: "sentinel-reader",
    Password: os.Getenv("REDIS_SENTINEL_PASSWORD"),
}
```

数据节点和 Sentinel 是两个认证域，两套凭据不能混用。

### ACL 策略

Redis 6+ 仍兼容 default user 的 `AUTH password`，老版本 Redis 也只有 password-only 认证。但本项目采用更严格的现代 ACL 契约：

- 无认证：`Username` 和 `Password` 均为空。
- 有认证：`Username` 和 `Password` 必须同时非空。
- password-only 或 username-only：配置校验失败。

因此，启用了 password-only 的旧 Redis 不在本版本支持范围内；应先迁移到 Redis 6+ ACL 账号，不能通过填入伪用户名绕过校验。

动态凭据通过 `CredentialsProvider` 注入，也必须返回完整账号密码对，并且不能与静态 `DataCredentials` 同时配置：

```go
redisConfig.CredentialsProvider = func(ctx context.Context) (redisconn.Credentials, error) {
    return loadRedisACLFromSecretManager(ctx)
}
```

### TLS 与连接池

```go
redisConfig.TLSConfig = &tls.Config{
    MinVersion: tls.VersionTLS12,
    ServerName: "redis.internal",
}
redisConfig.DialTimeout = 3 * time.Second
redisConfig.ReadTimeout = 2 * time.Second
redisConfig.WriteTimeout = 2 * time.Second
redisConfig.PoolSize = 100
redisConfig.MinIdleConns = 10
```

这些设置由同一个 factory 传递给 Asynq client、worker、scheduler、inspector 和 Manager 的锁 client。

## 3. 队列与环境隔离

`QueueCritical`、`QueueHigh`、`QueueDefault`、`QueueScheduled`、`QueueLow` 和 `QueueDeadLetter` 是不可变基名。`Manager.Enqueue`、`EnqueueTask`、`RegisterSchedule` 的 `Queue(...)` 输入以及 `Config.Queues` key 都必须是未加 namespace 的 base name。Manager 对非空 `QueuePrefix` 始终直接拼接，即使 prefix 与 base 的文本开头相交，并校验最终名称属于本实例。

```go
config.QueuePrefix = "prod:"

_, err := manager.EnqueueTask(
    ctx,
	"email:send",
	payload,
	hibasynq.Queue(asynq.QueueHigh), // Manager 解析为 prod:high
)
```

省略 `Queue(...)` 时，Manager 会选择 `QueueDefault`；自定义 `Config.Queues` 若没有 default，入队和 scheduler 注册会返回 `ErrQueueNotFound`，不会把任务投到无人消费的 Asynq default。Manager 最后追加的规范 Queue 会覆盖 Task 内嵌 Queue，避免跨 namespace。不要提前传 `manager.QueueName(base)`，否则会被视为 base 并因未配置而拒绝。

`GetDeadLetterTasks`、`RetryDeadLetterTask` 和 `DeleteDeadLetterTask` 的 queue 参数同样只接收 base name，并拒绝其他 namespace。`manager.QueueName(base)` 只用于 Inspector、Asynqmon 等需要实际 Redis 队列名的原生集成。直接使用 `manager.GetClient()` 是显式 escape hatch，会绕过 Manager 的 namespace 不变量。

自定义优先级放在 `Config.Queues`：

```go
config.Queues = map[string]int{
    asynq.QueueCritical: 10,
    asynq.QueueDefault:  4,
    asynq.QueueLow:      1,
}
```

规范化时 `QueuePrefix` 会应用到每个 key，并复制 map，调用方后续修改原 map 不会改变 Manager。

## 4. Polling 初始化

`PollingConfig` 只保留轮询策略，不再重复 Redis、并发或队列配置：

```go
cleanup, err := asynq.InitPolling(
    ctx,
    asynq.PollingConfig{
        Enabled:         true,
        MigrateExisting: true,
    },
    config,
    asynq.WorkerDependencies{
        RegisterTaskPollWorker: registerTaskPollWorker,
        RegisterWebhookWorker:  registerWebhookWorker,
        MigrateFunc:            migrateExistingTasks,
    },
)
if err != nil {
    return err
}
defer cleanup()
```

Manager 创建、worker/scheduler 启动或 migration lease 失败都会同步返回错误并回滚资源。

## 5. Token lease

```go
lease, acquired, err := manager.AcquirePollingLease(ctx, taskID)
if err != nil {
    return err // fail-closed，不执行重复任务
}
if !acquired {
    return nil
}

defer func() {
    cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
    defer cancel()
    _ = lease.Release(cleanupCtx)
}()

// 长任务可以主动续租。
if err := lease.Refresh(ctx); err != nil {
    return err
}
```

lease value 是随机 token。`Refresh`/`Release` 使用 Lua compare-token；TTL 过期后，即使其他 worker 获得同一个 key，旧 lease 也不能修改它。

Polling 和 migration key 都通过专用 internal-key 规则自动使用 Manager 的 `QueuePrefix` 作为 namespace：非空 prefix 始终拼接固定 base，不做字符串前缀猜测。不同租户前缀不会错误互斥，同一前缀的副本仍共享锁。

迁移使用相同 primitive，并按 TTL/3 自动续租。续租失败会取消传给迁移函数的派生 context，默认 fail-closed。

## 6. 生命周期与可观测性

- `Start(ctx)` 同步调用 Asynq server/scheduler 的 `Start`。
- scheduler 注册或启动失败时，`started` 不会被错误置为 true，相关 client 会关闭。
- `Stop()` 可重复调用，并关闭未启动 Manager 的资源。
- 日志不会输出用户名或密码。
- `SetLogger` 支持替换不同具体类型的 Logger；配置本身使用显式依赖注入。

完整可编译示例见 `examples/infra/asynq_complete_example.go`。
