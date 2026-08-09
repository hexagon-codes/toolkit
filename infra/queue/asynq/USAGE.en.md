[中文](USAGE.md) | English

# Asynq Usage Guide

## 1. Initialization

A Manager requires a caller-owned `context.Context` and one explicit `Config`:

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

`NewManager` normalizes and validates configuration, creates the canonical Redis factory, and performs `PING` through a dedicated Redis client before returning. Callers must handle initialization errors.

Use the process-wide singleton only when required:

```go
manager, err := asynq.InitManager(ctx, config)
```

A second call returns `ErrManagerAlreadyInitialized`. `ConfigProvider`, `DefaultConfigProvider`, `InitManagerFromConfig`, `SetRedisClient`, and `GetRedisClient` no longer exist.

## 2. Redis topology and authentication

### Standalone or proxy

```go
redisConfig := redisconn.DefaultConfig(
    redisconn.ModeSingle,
    "redis.internal:6379",
)
```

Single mode requires exactly one address.

### Redis Cluster

```go
redisConfig := redisconn.DefaultConfig(
    redisconn.ModeCluster,
    "redis-0.internal:6379", // one seed remains explicit Cluster mode
)
redisConfig.MaxRedirects = 5
```

`ModeCluster`, not address count, selects topology.

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

Data nodes and Sentinel are separate authentication domains; do not conflate their credentials.

### ACL policy

Redis 6+ still supports `AUTH password` for the default user, and older Redis supports only password-only authentication. This project intentionally adopts a stricter modern ACL contract:

- No authentication: both `Username` and `Password` are empty.
- Authentication: both `Username` and `Password` are non-empty.
- Password-only or username-only: validation fails.

Password-protected pre-Redis-6 deployments are therefore outside this version's support contract. Migrate them to a Redis 6+ ACL account; do not supply a fake username to bypass validation.

Dynamic credentials use `CredentialsProvider`, must return a complete pair, and are mutually exclusive with static `DataCredentials`:

```go
redisConfig.CredentialsProvider = func(ctx context.Context) (redisconn.Credentials, error) {
    return loadRedisACLFromSecretManager(ctx)
}
```

### TLS and pools

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

The same factory propagates these settings to the Asynq client, worker, scheduler, inspector, and the Manager's lease client.

## 3. Queues and environment isolation

`QueueCritical`, `QueueHigh`, `QueueDefault`, `QueueScheduled`, `QueueLow`, and `QueueDeadLetter` are immutable base names. `Queue(...)` arguments passed to `Manager.Enqueue`, `EnqueueTask`, and `RegisterSchedule`, plus keys in `Config.Queues`, must be unnamespaced base names. A Manager always concatenates a non-empty `QueuePrefix`, even when its text overlaps the beginning of the base, and verifies that the result belongs to this instance.

```go
config.QueuePrefix = "prod:"

_, err := manager.EnqueueTask(
    ctx,
	"email:send",
	payload,
	hibasynq.Queue(asynq.QueueHigh), // Manager resolves this to prod:high
)
```

Omitting `Queue(...)` selects `QueueDefault`. If a custom `Config.Queues` omits default, enqueue and scheduler registration return `ErrQueueNotFound` instead of sending work to an unconsumed Asynq default queue. The Manager's final canonical Queue overrides Task-embedded Queue options, preventing cross-namespace routing. Do not pass `manager.QueueName(base)` to enqueue APIs; it is treated as a base name and rejected as unconfigured.

The queue arguments to `GetDeadLetterTasks`, `RetryDeadLetterTask`, and `DeleteDeadLetterTask` likewise accept base names only and reject other namespaces. Use `manager.QueueName(base)` only for native integrations such as Inspector or Asynqmon that need the actual Redis queue name. Direct `manager.GetClient()` use is an explicit escape hatch and bypasses the Manager namespace invariant.

Set custom priorities through `Config.Queues`:

```go
config.Queues = map[string]int{
    asynq.QueueCritical: 10,
    asynq.QueueDefault:  4,
    asynq.QueueLow:      1,
}
```

Normalization applies `QueuePrefix` to every key and copies the map, so later caller mutation cannot change the Manager.

## 4. Polling initialization

`PollingConfig` contains only polling policy; it does not duplicate Redis, concurrency, or queue configuration:

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

Manager construction, worker/scheduler startup, or migration-lease failure is returned synchronously and rolls back owned resources.

## 5. Token leases

```go
lease, acquired, err := manager.AcquirePollingLease(ctx, taskID)
if err != nil {
    return err // fail closed; do not execute duplicate work
}
if !acquired {
    return nil
}

defer func() {
    cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
    defer cancel()
    _ = lease.Release(cleanupCtx)
}()

// Long-running work may explicitly refresh its lease.
if err := lease.Refresh(ctx); err != nil {
    return err
}
```

Each lease value is a random token. `Refresh` and `Release` use compare-token Lua scripts, so an expired owner cannot modify a replacement owner's lock.

Polling and migration keys use a dedicated internal-key rule: a non-empty Manager `QueuePrefix` is always concatenated with the fixed base, with no string-prefix guessing. Different tenant prefixes do not contend accidentally, while replicas with the same prefix still share a lock.

Migration uses the same primitive and refreshes at TTL/3. Refresh failure cancels the derived context passed to the migration function and fails closed.

## 6. Lifecycle and observability

- `Start(ctx)` synchronously calls Asynq server/scheduler `Start`.
- Scheduler registration or startup failure never leaves `started` set to true and closes attempt resources.
- `Stop()` is idempotent and closes resources even for a Manager that was never started.
- Logs do not expose usernames or passwords.
- `SetLogger` safely replaces different concrete Logger implementations; configuration itself uses explicit dependency injection.

See `examples/infra/asynq_complete_example.go` for a complete buildable example.
