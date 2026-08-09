[中文](README.md) | English

# Unified Redis Connection Factory

`redisconn` builds on `github.com/redis/go-redis/v9` and provides one explicit, validated connection model for Single, Cluster, and Sentinel deployments. It owns configuration, client construction, and startup probing only. It neither maintains a global client nor wraps Redis commands.

## Deployment modes

`Mode` is mandatory. The package never guesses topology from the number of addresses:

| Mode | Address requirement | Additional constraints | Returned go-redis client |
| --- | --- | --- | --- |
| `ModeSingle` | Exactly one Redis address | `DB >= 0` | `*redis.Client` |
| `ModeCluster` | At least one Cluster seed | `DB == 0` | `*redis.ClusterClient` |
| `ModeSentinel` | At least one Sentinel address | `MasterName` is required and `DB >= 0` | failover `*redis.Client` |

Every address must be non-blank. A single Cluster seed is valid and sufficient to bootstrap topology discovery. Production deployments normally provide multiple seeds to reduce startup dependence on one node.

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

`DefaultConfig` returns a normalized `Config`, but it does not guarantee that the configuration is complete and valid. For example, the caller must still set Sentinel's `MasterName`. `NewFactory` normalizes again, copies `Addrs` and the mutable TLS containers owned by this package, and then performs full validation. Caller-owned functions, interface values, and cryptographic handles referenced by TLS must still obey the concurrency contract below.

## Modern authentication policy

This project accepts exactly two static authentication states:

- No authentication: both `Username` and `Password` are empty.
- Redis ACL authentication: both `Username` and `Password` are non-empty.

The project deliberately rejects username-only and password-only configurations with `ErrInvalidCredentials`. This is a project security and API decision; it does not mean that Redis 6+ prohibits password-only authentication at the protocol level. Redis 6+ introduced ACLs and named users while [`AUTH`](https://redis.io/docs/latest/commands/auth/) retained a compatibility form for the `default` user. Production users of this package should configure a named ACL user. A `requirepass`-only deployment must migrate its authentication configuration first.

```go
config.DataCredentials = redisconn.Credentials{
    Username: os.Getenv("REDIS_USERNAME"),
    Password: os.Getenv("REDIS_PASSWORD"),
}
```

When Redis explicitly requires no authentication, leave `DataCredentials` at its zero value. Do not supply placeholder usernames or passwords.

### Two credential pairs in Sentinel mode

Sentinel mode has two independent authentication boundaries:

- `SentinelCredentials` authenticates connections to Sentinel nodes.
- `DataCredentials` authenticates connections to Redis primaries and replicas discovered through Sentinel.

Each static pair must be either completely empty or contain both username and password. `SentinelCredentials` is valid only in `ModeSentinel`.

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

## Quick start and lifecycle

Prefer `Open` during application startup. It constructs a client and executes `PING`, returning a client only after networking and authentication succeed. On failure, it closes the newly constructed client and returns `nil`. The caller must pass a context with a deadline—not `context.Background()` directly—to bound the whole startup probe, and must close the client during application shutdown.

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

`Factory.NewClient()` and `Factory.Open(ctx)` have different lifecycle guarantees:

- `NewClient()` only creates a lazy `redis.UniversalClient`. It performs no network I/O and proves nothing about addresses, TLS, or authentication. Use it when a higher layer owns readiness checks.
- `Open(ctx)` creates a client and immediately runs `PING`, making it suitable for fail-fast startup checks.
- In both cases, a successfully returned client belongs to the caller and must be closed.
- `Open(nil)` does not connect and returns `ErrInvalidContext`.

## TLS

Set `TLSConfig` to enable TLS. This example uses the system trust store and keeps certificate-chain and hostname verification enabled:

```go
config := redisconn.DefaultConfig(redisconn.ModeSingle, os.Getenv("REDIS_TLS_ADDR"))
config.TLSConfig = &tls.Config{
    MinVersion: tls.VersionTLS12,
    ServerName: os.Getenv("REDIS_TLS_SERVER_NAME"),
}
```

For a private CA, assign a verified CA pool to `RootCAs`. For mTLS, also provide client certificates. Do not bypass certificate or hostname verification to conceal a deployment error.

`Normalize` and `NewFactory` copy the `tls.Config`, common slices, certificate pools, and certificate bytes so those mutable containers are not shared with the caller. Go cannot generically deep-copy opaque objects such as TLS callbacks, `ClientSessionCache`, `KeyLogWriter`, private-key interfaces, or parsed-certificate pointers. Those objects remain caller-owned and must be immutable or safe for concurrent use after constructing a Factory.

## Dynamic data-node credentials

`CredentialsProvider` has this signature:

```go
type CredentialsProvider func(context.Context) (Credentials, error)
```

It is adapted to go-redis's context-aware credentials provider and resolves credentials when a new data-node connection is established, enabling credential rotation. go-redis may establish connections concurrently, so the provider and any state it accesses must be concurrency-safe. Its result must still be either completely empty or a complete username/password pair. Incomplete runtime credentials return `ErrInvalidCredentials`. Provider failures become `ErrCredentialsProvider`, and error messages do not include credential values.

```go
config.CredentialsProvider = func(ctx context.Context) (redisconn.Credentials, error) {
    username, password, err := secretStore.RedisACL(ctx)
    if err != nil {
        return redisconn.Credentials{}, err
    }
    return redisconn.Credentials{Username: username, Password: password}, nil
}
```

`CredentialsProvider` applies only to Redis data nodes and is mutually exclusive with static `DataCredentials`. Sentinel itself currently supports static `SentinelCredentials` only. To rotate Sentinel credentials, construct a new Factory and client from the new configuration.

## Cluster retry semantics

Cluster exposes two distinct retry layers that must not be conflated:

- `MaxRedirects` controls the Cluster client's command-level retries for network failures and `MOVED` / `ASK` redirects. It defaults to `3`.
- `MaxRetries` controls retries inside each underlying Redis node client. It defaults to `-1` in Cluster mode, disabling node-level retries and avoiding retry multiplication with `MaxRedirects`.

`MaxRetries` defaults to `3` in Single and Sentinel modes. When changing retry counts, evaluate command deadlines, worst-case latency, and upstream retries together to prevent multi-layer retry amplification.

## Defaults and configuration fields

| Field | Default | Meaning |
| --- | --- | --- |
| `MaxRetries` | Single/Sentinel `3`; Cluster `-1` | Node-client retries |
| `MaxRedirects` | `3` | Used by Cluster only |
| `MinRetryBackoff` | `8ms` | Minimum retry backoff |
| `MaxRetryBackoff` | `512ms` | Maximum retry backoff |
| `DialTimeout` | `5s` | Connection timeout |
| `ReadTimeout` | `3s` | Read timeout |
| `WriteTimeout` | `ReadTimeout`, initially `3s` | Write timeout |
| `PoolTimeout` | Positive `ReadTimeout + 1s`, initially `4s` | Pool wait timeout |
| `PoolSize` | Single/Sentinel `10 * runtime.GOMAXPROCS(0)`; Cluster `5 * runtime.GOMAXPROCS(0)` | Pool size; calculated per node in Cluster mode |
| `ConnMaxIdleTime` | `30m` | Maximum connection idle time |
| `MinIdleConns`, `MaxIdleConns`, `MaxActiveConns`, `ConnMaxLifetime` | `0` | Retain go-redis zero-value semantics |

`DB` defaults to `0`. A supported non-zero field value overrides the default above. To disable retry or timeout behavior supported by go-redis, use the exact upstream sentinel: `-1` for `MaxRetries`, `MaxRedirects`, and retry backoffs; `-1` or `-2` for `ReadTimeout` / `WriteTimeout`; and `-1` for `ConnMaxIdleTime`. Smaller retry or read/write-timeout negatives are rejected. `DialTimeout`, `PoolTimeout`, and pool-count fields cannot be negative, and pool counts must fit go-redis's non-negative `int32` range.

## Reading environment variables safely

Never put credentials in source code, endpoint strings, or logs. If environment variables are an optional authentication source, verify that username and password are both present and non-empty before assigning them to the configuration:

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

Prefer an auditable, rotation-aware Secret Manager in production and use `CredentialsProvider` for data-node credentials. Do not print a complete `Config`, and never include usernames, passwords, or provider results in error messages.

Classify configuration and runtime failures with `errors.Is` against `ErrInvalidConfig`, `ErrInvalidCredentials`, `ErrCredentialsProvider`, and `ErrInvalidContext`.
