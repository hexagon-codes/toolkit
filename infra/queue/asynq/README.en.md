[中文](README.md) | English

# Asynq Queue Package

## Features

- ✅ Task enqueue and consumption
- ✅ Scheduled task dispatching
- ✅ Task retry and dead letter queue
- ✅ Circuit breaker and backpressure control
- ✅ Task state machine management
- ✅ Prometheus metrics monitoring
- ✅ Task polling lock mechanism
- ✅ Health check
- ✅ Middleware support

## Dependencies

### Required Interfaces

1. **AsynqConfig** - Configuration interface
   - `GetRedisAddrs() []string` - Redis address list
   - `GetRedisPassword() string` - Redis password
   - `GetRedisUsername() string` - Redis username (Redis 6.0+ ACL)
   - `GetConcurrency() int` - Worker concurrency
   - `GetQueuePrefix() string` - Queue prefix (multi-environment isolation)
   - `IsPollingEnabled() bool` - Whether polling is enabled
   - `IsRedisEnabled() bool` - Whether Redis is enabled

2. **AsynqLogger** - Logger interface
   - `Log(msg string)` - Normal log
   - `LogSkip(skip int, msg string)` - Log with call stack skip
   - `Error(msg string)` - Error log
   - `ErrorSkip(skip int, msg string)` - Error log with call stack skip

### Usage

#### Option 1: Use Default Implementation (Quick Start)

```go
import "github.com/everyday-items/toolkit/infra/queue/asynq"

// Use default configuration
config := &asynq.DefaultAsynqConfig{
    RedisAddrs:     []string{"localhost:6379"},
    RedisPassword:  "",
    Concurrency:    10,
    PollingEnabled: true,
    RedisEnabled:   true,
}

// Use stdout logger
logger := &asynq.StdLogger{}
```

#### Option 2: Adapt to Existing Project

If you have an existing configuration and logging system, create adapters:

```go
// Adapter example
type MyConfigAdapter struct {
    // your config struct
}

func (c *MyConfigAdapter) GetRedisAddrs() []string {
    return common.GetRedisAddrs() // adapt to your config
}

func (c *MyConfigAdapter) GetRedisPassword() string {
    return common.GetRedisPassword()
}

// ... implement other methods

type MyLoggerAdapter struct{}

func (l *MyLoggerAdapter) Log(msg string) {
    common.SysLog(msg) // adapt to your logger
}

func (l *MyLoggerAdapter) Error(msg string) {
    common.SysError(msg)
}

// ... implement other methods
```

#### Option 3: Custom Implementation

If you have specific requirements, you can fully customize the configuration and logging implementation.

## File Overview

| File | Description |
|------|-------------|
| `config.go` | Configuration and logger interface definitions |
| `manager.go` | Asynq manager core implementation |
| `task.go` | Task builder and helper functions |
| `task_types.go` | Task type definitions |
| `init.go` | Polling system initialization |
| `queues.go` | Queue configuration management |
| `adapter.go` | Adapters and helper functions |
| `middleware.go` | Middleware implementation |
| `metrics.go` | Prometheus metrics |
| `health.go` | Health check |
| `circuit_breaker.go` | Circuit breaker |
| `backpressure.go` | Backpressure control |
| `dead_letter.go` | Dead letter queue management |
| `state_machine.go` | Task state machine |
| `polling_lock.go` | Polling distributed lock |
| `task_tracer.go` | Task tracing |
| `errors.go` | Error definitions |
| `testing_helpers.go` | Test helper functions |

## Current Status

✅ **Interface refactoring complete**

This package is fully decoupled from external dependencies and can be used directly in any Go project:
- All configuration is provided through the `ConfigProvider` interface
- All logging is output through the `Logger` interface
- Provides `DefaultConfigProvider` and `StdLogger` as ready-to-use default implementations
- Task types and queue names are generic definitions that can be customized as needed

### Next Steps

1. Add complete usage examples to `examples/infra/`
2. Add unit tests
3. Add performance benchmarks
4. Improve API documentation

## Contributing

PRs to help improve this package are welcome!
