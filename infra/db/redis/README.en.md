[中文](README.md) | English

# Redis Client Wrapper

This package builds on `github.com/redis/go-redis/v9` and provides standalone, Cluster, Sentinel, health-check, and distributed-lock support. Connection configuration reuses the canonical model from `infra/redisconn`; this package does not maintain a global client.

## Authentication policy

This project deliberately uses a strict modern authentication contract:

- No authentication: leave both fields in `DataCredentials` empty.
- ACL authentication: provide both username and password.
- Password only: deliberately rejected with `ErrInvalidCredentials`.

Rejecting password-only credentials is a project security/API policy, not a Redis 6+ technical limitation. Redis 6+ still supports [`AUTH [username] password`](https://redis.io/docs/latest/commands/auth/); omitting the username selects the `default` user for backward compatibility. Before using this package with an older Redis server or a `requirepass`-only deployment, migrate the server to a named ACL user.

## Quick start

`New` accepts the caller's `context.Context`, validates the configuration, and runs `PING`. It returns a client only after connectivity and authentication succeed. Each call creates an independent client that the caller must inject and close.

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

When Redis does not require authentication, do not set `DataCredentials`. Validation fails if only one of the username and password environment variables is present.

## Deployment modes

### Single

Standalone mode requires exactly one address:

```go
config := redis.DefaultConfig(redis.ModeSingle, os.Getenv("REDIS_ADDR"))
```

### Cluster

Cluster mode accepts one or more seeds. A single seed is sufficient for topology discovery, although multiple seeds normally improve bootstrap availability in production. Cluster mode requires `DB == 0`.

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

A single seed is also valid:

```go
config := redis.DefaultConfig(redis.ModeCluster, os.Getenv("REDIS_CLUSTER_SEED"))
```

### Sentinel

Sentinel mode has two independent credential pairs:

- `SentinelCredentials` authenticates the client to Sentinel endpoints.
- `DataCredentials` authenticates the client to Redis primaries and replicas discovered through Sentinel.

Each pair must be either completely empty or contain both username and password.

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

Set `TLSConfig` to enable TLS. This configuration uses the system trust store and keeps server-certificate and hostname verification enabled. Do not set `InsecureSkipVerify`.

```go
config := redis.DefaultConfig(redis.ModeSingle, os.Getenv("REDIS_TLS_ADDR"))
config.TLSConfig = &tls.Config{
    MinVersion: tls.VersionTLS12,
    ServerName: os.Getenv("REDIS_TLS_SERVER_NAME"),
}
```

For a private CA or mTLS, add the verified CA pool and client certificates to the same `tls.Config`.

## Dynamic data-node credentials

`CredentialsProvider` resolves data-node credentials whenever a new connection is initialized, making it suitable for credential rotation. It is mutually exclusive with static `DataCredentials` and must return both username and password. It does not provide dynamic credentials for Sentinel itself; Sentinel authentication still uses `SentinelCredentials`.

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

In production, the environment lookup can be replaced with a rotation-aware secret manager. Never log credential values.

## Health, operations, and shutdown

`Client` embeds go-redis's `UniversalClient`, so its commands, pipelines, and transaction APIs remain directly available. The wrapper also provides a small set of convenience methods.

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

`GetWithDefault` returns the fallback only when the key is absent; network, authentication, and context failures are still returned. Call `Close` during application shutdown instead of relying on package-global cleanup.

## Distributed locks

Prefer `WithLock`. It verifies lock ownership and releases the lock automatically; both callback and release failures are returned to the caller.

```go
err := redis.WithLock(ctx, client, "lock:resource", 30*time.Second, func() error {
    return updateResource(ctx)
})
if err != nil {
    return err
}
```

For manual control, use `NewLock`, `Acquire`, `AcquireWithRetry`, `Refresh`, `TTL`, and `Release`. The expiration must be positive and cover the expected work duration; `Acquire` and `WithLock` return `ErrInvalidConfig` for non-positive values before writing to Redis. Refresh long-running work and always inspect release errors. This is an ownership lock on one Redis instance, not a multi-primary Redlock implementation.

## Migrating from the old API

This is a breaking migration with no compatibility layer:

| Old API or field | Replacement |
| --- | --- |
| `DefaultConfig(addr)` | `DefaultConfig(ModeSingle, addr)`, returning a `Config` value |
| `DefaultClusterConfig(addrs)` | `DefaultConfig(ModeCluster, addrs...)` |
| `Addr` / `SentinelAddrs` | Unified as `Addrs` |
| `Password` | `DataCredentials{Username, Password}` |
| `Init(config)` / `GetGlobal()` | Call `New(ctx, config)` at the composition root and inject `*Client` explicitly |
| `IdleTimeout` | `ConnMaxIdleTime` |
| `GetWithDefault(...) string` | `GetWithDefault(...) (string, error)` |

`Logger`, `StdLogger`, and `IdleCheckFrequency` were removed. To share a connection, create one `*Client` at the application's composition root and inject it into consumers. The library no longer uses a first-config-wins global lifecycle.

Configuration failures can be classified with `errors.Is` against `ErrInvalidConfig`, `ErrInvalidCredentials`, `ErrCredentialsProvider`, and `ErrInvalidContext`.
