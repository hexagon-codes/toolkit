# Retry 重试工具

通用的重试逻辑实现，支持多种退避策略。

## 特性

- ✅ 简单易用 - 一行代码实现重试
- ✅ 灵活配置 - 支持多种配置选项
- ✅ 退避策略 - 固定、线性、指数退避
- ✅ 上下文支持 - 可取消的重试
- ✅ 自定义条件 - 灵活的重试判断
- ✅ 零依赖 - 只使用标准库

## 快速开始

### 基础使用

```go
package main

import (
    "github.com/everyday-items/toolkit/util/retry"
)

func main() {
    // 简单重试（默认 3 次，每次间隔 1 秒）
    err := retry.Do(func() error {
        return apiCall()
    })
}
```

### 自定义配置

```go
// 自定义重试次数和延迟
err := retry.Do(
    func() error {
        return apiCall()
    },
    retry.Attempts(5),              // 最多 5 次
    retry.Delay(2*time.Second),     // 延迟 2 秒
)
```

### 指数退避

```go
err := retry.Do(
    func() error {
        return apiCall()
    },
    retry.Attempts(5),
    retry.Delay(time.Second),
    retry.DelayType(retry.ExponentialBackoff),  // 指数退避
    retry.MaxDelay(30*time.Second),              // 最大延迟 30 秒
)

// 延迟序列: 1s, 2s, 4s, 8s, 16s
```

### 重试回调

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

### 条件重试

```go
// 只在特定错误时重试
err := retry.Do(
    func() error {
        return apiCall()
    },
    retry.RetryIf(func(err error) bool {
        // 只重试网络错误
        return errors.Is(err, ErrNetwork)
    }),
)
```

### 带上下文

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

## 退避策略

### 1. 固定延迟（默认）

每次重试延迟相同。

```go
retry.DelayType(retry.FixedDelay)
// 延迟: 1s, 1s, 1s, 1s
```

### 2. 线性退避

延迟线性增长。

```go
retry.DelayType(retry.LinearBackoff)
// 如果 Delay=1s: 1s, 2s, 3s, 4s
```

### 3. 指数退避

延迟指数增长（推荐）。

```go
retry.DelayType(retry.ExponentialBackoff)
// 如果 Delay=1s, Multiplier=2: 1s, 2s, 4s, 8s, 16s
```

## 配置选项

| 选项 | 说明 | 默认值 |
|------|------|--------|
| `Attempts(n)` | 最大尝试次数 | 3 |
| `Delay(d)` | 重试延迟 | 1s |
| `MaxDelay(d)` | 最大延迟 | 30s |
| `Multiplier(m)` | 延迟倍数（指数退避） | 2.0 |
| `OnRetry(fn)` | 重试回调函数 | nil |
| `RetryIf(fn)` | 重试条件判断 | 任何错误都重试 |
| `DelayType(fn)` | 延迟策略 | 固定延迟 |

## 使用场景

### 1. API 调用重试

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
            // 只重试 5xx 错误
            return strings.Contains(err.Error(), "server error")
        }),
    )
}
```

### 2. 数据库连接重试

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

### 3. 消息队列消费重试

```go
func processMessage(msg *Message) error {
    return retry.Do(
        func() error {
            return process(msg)
        },
        retry.Attempts(3),
        retry.Delay(5*time.Second),
        retry.RetryIf(func(err error) bool {
            // 不重试业务错误
            return !errors.Is(err, ErrBusinessLogic)
        }),
    )
}
```

### 4. 文件上传重试

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

## 最佳实践

### 1. 选择合适的退避策略

```go
// ✅ API 调用：使用指数退避
retry.DelayType(retry.ExponentialBackoff)

// ✅ 轮询检查：使用固定延迟
retry.DelayType(retry.FixedDelay)

// ✅ 有限资源竞争：使用线性退避
retry.DelayType(retry.LinearBackoff)
```

### 2. 设置合理的最大延迟

```go
// ✅ 设置上限，避免过长等待
retry.MaxDelay(30*time.Second)
```

### 3. 使用上下文控制超时

```go
// ✅ 总超时控制
ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
defer cancel()

retry.DoWithContext(ctx, fn)
```

### 4. 记录重试日志

```go
retry.OnRetry(func(n int, err error) {
    log.Printf("[Retry %d/%d] %v", n, maxAttempts, err)
})
```

### 5. 区分可重试和不可重试错误

```go
retry.RetryIf(func(err error) bool {
    // 网络错误、超时：可重试
    if errors.Is(err, ErrNetwork) || errors.Is(err, ErrTimeout) {
        return true
    }

    // 参数错误、认证失败：不重试
    if errors.Is(err, ErrInvalidParam) || errors.Is(err, ErrAuth) {
        return false
    }

    return true
})
```

## 注意事项

1. **幂等性**：重试的操作必须是幂等的
2. **超时控制**：使用 Context 控制总超时时间
3. **错误判断**：区分可重试和不可重试的错误
4. **延迟上限**：设置 MaxDelay 避免等待过长
5. **并发控制**：重试可能导致并发增加，注意控制

## 性能考虑

- 重试会增加延迟，合理设置重试次数
- 指数退避可以有效降低服务器压力
- 使用 RetryIf 避免不必要的重试
