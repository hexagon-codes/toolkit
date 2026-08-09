[中文](README.md) | English

# Asynq Queue Package

This is a production-oriented wrapper around `hibiken/asynq`. A Manager owns its Asynq client, worker, scheduler, inspector, and the dedicated Redis client used by polling and migration leases.

## Redis connection contract

The package accepts only `infra/redisconn.Config`. It no longer duplicates address or credential fields, and it never guesses topology from the number of addresses.

| Mode | Required fields | Meaning |
|---|---|---|
| `redisconn.ModeSingle` | exactly 1 `Addrs` entry | Standalone Redis or a cloud proxy endpoint |
| `redisconn.ModeCluster` | at least 1 `Addrs` entry | One seed still creates a Cluster client |
| `redisconn.ModeSentinel` | at least 1 `Addrs` entry and `MasterName` | Primary discovery through Sentinel |

Redis capability and project policy are intentionally distinct. Redis 6+ still supports single-argument `AUTH password` for the default user. This project rejects password-only static configuration to eliminate ambiguous deployments: username and password must either both be empty or both be non-empty. Invalid credentials fail validation before startup.

Sentinel has two independent credential domains:

- `DataCredentials` authenticate to Redis data nodes.
- `SentinelCredentials` authenticate to Sentinel; they must not be replaced by data-node credentials.

TLS, DB, timeouts, retries, and pool settings come from the same `redisconn.Config` for every client owned by the Manager.

## Quick start

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
if err != nil { // includes validation, network, and authentication failures
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

`NewManager` performs a Redis `PING` before it returns. Wrong credentials, unreachable endpoints, and invalid topology are reported synchronously instead of producing a false-success background startup.

## Lifecycle and concurrency

- `Start` synchronously starts the worker and scheduler and rolls back resources created by a failed attempt.
- `Stop` is idempotent and closes every Asynq/Redis client owned by the Manager, even if `Start` was never called.
- `InitManager` installs a process-wide singleton. Repeated initialization returns `ErrManagerAlreadyInitialized`; it never silently keeps the first configuration.
- Queue prefixes are Manager-scoped. `Queue(...)` arguments passed to `Manager.Enqueue`, `EnqueueTask`, and `RegisterSchedule`, along with `Config.Queues` keys, must be unnamespaced base names. The Manager applies its prefix and verifies that the resolved queue belongs to its configuration. Omitting Queue selects `QueueDefault`; if default is not configured, the operation returns `ErrQueueNotFound`. The Manager's final routing option overrides Queue options embedded in a Task.
- Use `manager.QueueName(base)` only for native integrations such as Inspector or Asynqmon that require the actual Redis queue name. Direct `GetClient()` use is an explicit escape hatch and bypasses Manager namespace validation.

## Safe leases

Polling and migration locks use the same token-based Redis lease primitive:

```go
lease, acquired, err := manager.AcquirePollingLease(ctx, taskID)
if err != nil {
    return err // Redis/authentication errors fail closed
}
if !acquired {
    return nil // another worker owns the lease
}
defer lease.Release(context.WithoutCancel(ctx))
```

`Refresh` and `Release` execute compare-token Lua scripts, so an expired owner cannot extend or delete a replacement owner's lock. Polling and migration internal base keys always use `prefix + base` for a non-empty `QueuePrefix`, even when the strings overlap. Different prefixes can own leases independently, while identical prefixes still contend. Migration automatically refreshes its lease; refresh failure cancels the migration context and returns an error.

## Optional logging adapter

Configuration is injected directly through `Config`; there is no global `ConfigProvider` or second global Redis-client chain. Logging can still be adapted through `SetLogger` or `manager.SetLogger`.

See [USAGE.en.md](USAGE.en.md) for connection modes, Polling, queues, and lease examples.
