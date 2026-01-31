# Rate 限流工具

提供三种限流算法实现：令牌桶、漏桶、滑动窗口。

## 特性

- ✅ 令牌桶（Token Bucket）- 支持突发流量
- ✅ 漏桶（Leaky Bucket）- 平滑限流
- ✅ 滑动窗口（Sliding Window）- 精确时间窗口
- ✅ 统一接口（Limiter）
- ✅ 并发安全
- ✅ 零外部依赖

## 快速开始

### 令牌桶限流

```go
import "github.com/everyday-items/toolkit/util/rate"

// 创建令牌桶限流器
// 容量：10个令牌，速率：每秒生成5个令牌
limiter := rate.NewTokenBucket(10, 5.0)

// 判断是否允许通过
if limiter.Allow() {
    // 允许通过，执行业务逻辑
    handleRequest()
} else {
    // 被限流，返回错误
    return errors.New("rate limit exceeded")
}

// 等待直到允许通过
waitTime := limiter.Wait()
fmt.Printf("Waited %v\n", waitTime)
handleRequest()
```

### 漏桶限流

```go
// 创建漏桶限流器
// 容量：100个请求，速率：每100ms漏出1个请求
limiter := rate.NewLeakyBucket(100, 100*time.Millisecond)

if limiter.Allow() {
    handleRequest()
} else {
    return errors.New("rate limit exceeded")
}
```

### 滑动窗口限流

```go
// 创建滑动窗口限流器
// 容量：每分钟最多1000个请求
limiter := rate.NewSlidingWindow(1000, time.Minute)

if limiter.Allow() {
    handleRequest()
} else {
    return errors.New("rate limit exceeded")
}
```

## API 文档

### 限流器接口

```go
type Limiter interface {
    // Allow 判断是否允许通过
    Allow() bool

    // Wait 等待直到允许通过，返回等待时间
    Wait() time.Duration
}
```

### 令牌桶（Token Bucket）

```go
// NewTokenBucket 创建令牌桶限流器
// capacity: 桶容量（最大令牌数）
// rate: 令牌生成速率（每秒生成多少个令牌）
NewTokenBucket(capacity int, rate float64) *TokenBucket
```

**特点**：
- 支持突发流量（桶内有足够令牌时）
- 令牌匀速生成
- 适合大多数场景

### 漏桶（Leaky Bucket）

```go
// NewLeakyBucket 创建漏桶限流器
// capacity: 桶容量
// rate: 漏水速率（例如：100ms 表示每100ms漏出一滴水）
NewLeakyBucket(capacity int, rate time.Duration) *LeakyBucket
```

**特点**：
- 平滑限流，不支持突发
- 匀速处理请求
- 适合需要严格控制速率的场景

### 滑动窗口（Sliding Window）

```go
// NewSlidingWindow 创建滑动窗口限流器
// capacity: 窗口内允许的最大请求数
// window: 窗口大小（例如：1分钟）
NewSlidingWindow(capacity int, window time.Duration) *SlidingWindow
```

**特点**：
- 精确的时间窗口控制
- 内存占用与请求数成正比
- 适合需要精确统计的场景

## 使用场景

### 1. API 限流中间件

```go
func RateLimitMiddleware() gin.HandlerFunc {
    // 每秒最多100个请求
    limiter := rate.NewTokenBucket(100, 100.0)

    return func(c *gin.Context) {
        if !limiter.Allow() {
            c.JSON(429, gin.H{
                "error": "rate limit exceeded",
            })
            c.Abort()
            return
        }

        c.Next()
    }
}

// 使用
router := gin.Default()
router.Use(RateLimitMiddleware())
```

### 2. 用户级限流

```go
type UserRateLimiter struct {
    limiters map[string]rate.Limiter
    mu       sync.RWMutex
}

func (u *UserRateLimiter) Allow(userID string) bool {
    u.mu.Lock()
    defer u.mu.Unlock()

    // 获取或创建用户的限流器
    limiter, ok := u.limiters[userID]
    if !ok {
        // 每个用户每分钟最多60个请求
        limiter = rate.NewSlidingWindow(60, time.Minute)
        u.limiters[userID] = limiter
    }

    return limiter.Allow()
}

func HandleRequest(c *gin.Context) {
    userID := c.GetString("user_id")

    if !userLimiter.Allow(userID) {
        c.JSON(429, gin.H{"error": "rate limit exceeded"})
        return
    }

    // 处理请求
}
```

### 3. IP 限流

```go
var ipLimiters = make(map[string]rate.Limiter)
var mu sync.RWMutex

func IPRateLimitMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        ip := c.ClientIP()

        mu.Lock()
        limiter, ok := ipLimiters[ip]
        if !ok {
            // 每个 IP 每秒最多10个请求
            limiter = rate.NewTokenBucket(10, 10.0)
            ipLimiters[ip] = limiter
        }
        mu.Unlock()

        if !limiter.Allow() {
            c.JSON(429, gin.H{"error": "too many requests from this IP"})
            c.Abort()
            return
        }

        c.Next()
    }
}
```

### 4. 外部 API 调用限流

```go
type APIClient struct {
    limiter rate.Limiter
}

func NewAPIClient() *APIClient {
    return &APIClient{
        // 第三方 API 限制：每秒最多5个请求
        limiter: rate.NewTokenBucket(5, 5.0),
    }
}

func (c *APIClient) CallAPI(endpoint string) (*Response, error) {
    // 等待直到允许调用
    waitTime := c.limiter.Wait()
    if waitTime > 0 {
        log.Printf("Rate limited, waited %v", waitTime)
    }

    // 调用 API
    return http.Get(endpoint)
}
```

### 5. 数据库写入限流

```go
type DBWriter struct {
    limiter rate.Limiter
}

func NewDBWriter() *DBWriter {
    return &DBWriter{
        // 限制数据库写入速率：每100ms一次
        limiter: rate.NewLeakyBucket(10, 100*time.Millisecond),
    }
}

func (w *DBWriter) Write(data interface{}) error {
    if !w.limiter.Allow() {
        return errors.New("write rate limit exceeded")
    }

    return db.Save(data)
}
```

### 6. 短信/邮件发送限流

```go
// 用户级短信限流
var smsLimiters = make(map[string]rate.Limiter)

func SendSMS(phone, code string) error {
    mu.Lock()
    limiter, ok := smsLimiters[phone]
    if !ok {
        // 每个手机号每小时最多发送5条
        limiter = rate.NewSlidingWindow(5, time.Hour)
        smsLimiters[phone] = limiter
    }
    mu.Unlock()

    if !limiter.Allow() {
        return errors.New("SMS rate limit exceeded, please try again later")
    }

    // 发送短信
    return smsService.Send(phone, code)
}
```

### 7. 爬虫限流

```go
type Crawler struct {
    limiter rate.Limiter
}

func NewCrawler() *Crawler {
    return &Crawler{
        // 爬虫速率：每2秒一个请求
        limiter: rate.NewLeakyBucket(1, 2*time.Second),
    }
}

func (c *Crawler) Crawl(urls []string) {
    for _, url := range urls {
        // 等待直到允许爬取
        c.limiter.Wait()

        // 爬取页面
        resp, err := http.Get(url)
        if err != nil {
            log.Printf("Failed to crawl %s: %v", url, err)
            continue
        }

        // 处理响应
        processResponse(resp)
    }
}
```

## 算法对比

| 算法 | 突发流量 | 平滑性 | 内存占用 | 适用场景 |
|------|---------|--------|----------|----------|
| 令牌桶 | ✅ 支持 | 中等 | 低（固定） | 通用 API 限流 |
| 漏桶 | ❌ 不支持 | 高 | 低（固定） | 严格速率控制 |
| 滑动窗口 | ❌ 不支持 | 中等 | 高（与请求数成正比） | 精确时间窗口 |

### 选择建议

**令牌桶（推荐）**：
```go
// ✅ 大多数场景的首选
limiter := rate.NewTokenBucket(100, 10.0)
```

**漏桶**：
```go
// ✅ 需要严格控制速率（如第三方 API 调用）
limiter := rate.NewLeakyBucket(10, 100*time.Millisecond)
```

**滑动窗口**：
```go
// ✅ 需要精确的时间窗口统计（如"每分钟最多N个请求"）
limiter := rate.NewSlidingWindow(1000, time.Minute)
```

## 参数配置示例

### 令牌桶

```go
// 高并发场景：允许突发，但长期限制在100 QPS
limiter := rate.NewTokenBucket(1000, 100.0)

// 低频 API：每秒最多5个请求
limiter := rate.NewTokenBucket(5, 5.0)

// 支持短期突发：容量100，速率10
limiter := rate.NewTokenBucket(100, 10.0)
```

### 漏桶

```go
// 严格控制：每100ms处理1个请求（10 QPS）
limiter := rate.NewLeakyBucket(10, 100*time.Millisecond)

// 慢速处理：每秒1个请求
limiter := rate.NewLeakyBucket(5, time.Second)
```

### 滑动窗口

```go
// 每分钟限制
limiter := rate.NewSlidingWindow(1000, time.Minute)

// 每小时限制
limiter := rate.NewSlidingWindow(10000, time.Hour)

// 每秒限制
limiter := rate.NewSlidingWindow(100, time.Second)
```

## 并发安全

所有限流器都是并发安全的：

```go
limiter := rate.NewTokenBucket(100, 10.0)

// 可以在多个 goroutine 中安全使用
for i := 0; i < 10; i++ {
    go func() {
        for {
            if limiter.Allow() {
                handleRequest()
            }
            time.Sleep(10 * time.Millisecond)
        }
    }()
}
```

## 性能

```
TokenBucket.Allow():     500 ns/op
LeakyBucket.Allow():     800 ns/op
SlidingWindow.Allow():   1200 ns/op
```

令牌桶性能最好，滑动窗口最慢（需要清理过期请求）。

## 注意事项

1. **内存管理**：
   - 令牌桶和漏桶：内存占用固定
   - 滑动窗口：内存占用与请求数成正比，需定期清理

2. **时钟精度**：
   - 依赖系统时钟（`time.Now()`）
   - 精度受操作系统影响（通常 1ms）

3. **分布式限流**：
   - 本包仅支持单机限流
   - 分布式场景需使用 Redis 等外部存储

4. **突发流量**：
   - 令牌桶：桶满时可处理突发流量
   - 漏桶/滑动窗口：不支持突发

5. **重置机制**：
   - 限流器创建后不支持重置
   - 如需重置，请创建新的限流器实例

## 依赖

```bash
# 零外部依赖，仅使用标准库
import (
    "sync"
    "time"
)
```

## 扩展建议

如需分布式限流，可考虑：
- `golang.org/x/time/rate` - 官方限流库
- Redis + Lua - 分布式令牌桶/漏桶
- `github.com/juju/ratelimit` - 更多限流算法
