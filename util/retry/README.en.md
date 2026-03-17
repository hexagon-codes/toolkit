[中文](README.md) | English

# Retry Utility

A general-purpose retry logic implementation supporting multiple backoff strategies.

## Features

- ✅ Simple to use - retry in one line of code
- ✅ Flexible configuration - multiple configuration options
- ✅ Backoff strategies - fixed, linear, and exponential backoff
- ✅ Context support - cancellable retries
- ✅ Custom conditions - flexible retry conditions
- ✅ Zero dependencies - uses only the standard library

## Quick Start

### Basic Usage

```go
package main

import (
    "github.com/everyday-items/toolkit/util/retry"
)

func main() {
    // Simple retry (default 3 attempts, 1-second interval)
    err := retry.Do(func() error {
        return apiCall()
    })
}
```

### Custom Configuration

```go
// Custom retry count and delay
err := retry.Do(
    func() error {
        return apiCall()
    },
    retry.Attempts(5),              // up to 5 attempts
    retry.Delay(2*time.Second),     // 2-second delay
)
```

### Exponential Backoff

```go
err := retry.Do(
    func() error {
        return apiCall()
    },
    retry.Attempts(5),
    retry.Delay(time.Second),
    retry.DelayType(retry.ExponentialBackoff),  // exponential backoff
    retry.MaxDelay(30*time.Second),              // max delay 30 seconds
)

// Delay sequence: 1s, 2s, 4s, 8s, 16s
```

### Retry Callback

```go
err := retry.Do(
    func() error {
        return apiCall()
    },
    retry.Attempts(3),
    retry.OnRetry(func(n int, err error) {
        log.Printf("Retry attempt %d: %v", n, err)
    }),
)
```

### Conditional Retry

```go
// Only retry on specific errors
err := retry.Do(
    func() error {
        return apiCall()
    },
    retry.RetryIf(func(err error) bool {
        // Only retry network errors
        return errors.Is(err, ErrNetwork)
    }),
)
```

### With Context

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

err := retry.DoWithContext(ctx,
    func() error {
        return apiCall()
    },
    retry.Attempts(10),
    retry.Delay(time.Second),
)
```

## Backoff Strategies

### 1. Fixed Delay (Default)

Same delay for each retry.

```go
retry.DelayType(retry.FixedDelay)
// Delays: 1s, 1s, 1s, 1s
```

### 2. Linear Backoff

Delay increases linearly.

```go
retry.DelayType(retry.LinearBackoff)
// If Delay=1s: 1s, 2s, 3s, 4s
```

### 3. Exponential Backoff

Delay grows exponentially (recommended).

```go
retry.DelayType(retry.ExponentialBackoff)
// If Delay=1s, Multiplier=2: 1s, 2s, 4s, 8s, 16s
```

## Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `Attempts(n)` | Maximum number of attempts | 3 |
| `Delay(d)` | Retry delay | 1s |
| `MaxDelay(d)` | Maximum delay | 30s |
| `Multiplier(m)` | Delay multiplier (exponential backoff) | 2.0 |
| `OnRetry(fn)` | Retry callback function | nil |
| `RetryIf(fn)` | Retry condition check | Retry on any error |
| `DelayType(fn)` | Delay strategy | Fixed delay |

## Use Cases

### 1. API Call Retry

```go
func callAPI() error {
    return retry.Do(
        func() error {
            resp, err := http.Get("https://api.example.com")
            if err != nil {
                return err
            }
            defer resp.Body.Close()

            if resp.StatusCode >= 500 {
                return fmt.Errorf("server error: %d", resp.StatusCode)
            }

            return nil
        },
        retry.Attempts(3),
        retry.Delay(time.Second),
        retry.RetryIf(func(err error) bool {
            // Only retry 5xx errors
            return strings.Contains(err.Error(), "server error")
        }),
    )
}
```

### 2. Database Connection Retry

```go
func connectDB() (*sql.DB, error) {
    var db *sql.DB

    err := retry.Do(
        func() error {
            var err error
            db, err = sql.Open("mysql", dsn)
            if err != nil {
                return err
            }
            return db.Ping()
        },
        retry.Attempts(5),
        retry.Delay(2*time.Second),
        retry.DelayType(retry.ExponentialBackoff),
        retry.OnRetry(func(n int, err error) {
            log.Printf("DB connection attempt %d failed: %v", n, err)
        }),
    )

    return db, err
}
```

### 3. Message Queue Consumer Retry

```go
func processMessage(msg *Message) error {
    return retry.Do(
        func() error {
            return process(msg)
        },
        retry.Attempts(3),
        retry.Delay(5*time.Second),
        retry.RetryIf(func(err error) bool {
            // Do not retry business logic errors
            return !errors.Is(err, ErrBusinessLogic)
        }),
    )
}
```

### 4. File Upload Retry

```go
func uploadFile(path string) error {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()

    return retry.DoWithContext(ctx,
        func() error {
            return upload(path)
        },
        retry.Attempts(5),
        retry.Delay(time.Second),
        retry.DelayType(retry.ExponentialBackoff),
        retry.MaxDelay(60*time.Second),
    )
}
```

## Best Practices

### 1. Choose the Right Backoff Strategy

```go
// ✅ API calls: use exponential backoff
retry.DelayType(retry.ExponentialBackoff)

// ✅ Polling checks: use fixed delay
retry.DelayType(retry.FixedDelay)

// ✅ Limited resource contention: use linear backoff
retry.DelayType(retry.LinearBackoff)
```

### 2. Set a Reasonable Maximum Delay

```go
// ✅ Set an upper bound to avoid excessive waiting
retry.MaxDelay(30*time.Second)
```

### 3. Use Context to Control Timeouts

```go
// ✅ Total timeout control
ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
defer cancel()

retry.DoWithContext(ctx, fn)
```

### 4. Log Retry Attempts

```go
retry.OnRetry(func(n int, err error) {
    log.Printf("[Retry %d/%d] %v", n, maxAttempts, err)
})
```

### 5. Distinguish Retryable from Non-Retryable Errors

```go
retry.RetryIf(func(err error) bool {
    // Network errors, timeouts: retryable
    if errors.Is(err, ErrNetwork) || errors.Is(err, ErrTimeout) {
        return true
    }

    // Parameter errors, auth failures: do not retry
    if errors.Is(err, ErrInvalidParam) || errors.Is(err, ErrAuth) {
        return false
    }

    return true
})
```

## Notes

1. **Idempotency**: The operation being retried must be idempotent
2. **Timeout Control**: Use Context to control total timeout
3. **Error Classification**: Distinguish retryable from non-retryable errors
4. **Delay Upper Bound**: Set MaxDelay to avoid excessively long waits
5. **Concurrency Control**: Retries may increase concurrency; manage accordingly

## Performance Considerations

- Retries add latency; set retry counts reasonably
- Exponential backoff effectively reduces server load
- Use RetryIf to avoid unnecessary retries
