[中文](README.md) | English

# Rate Limiting Utility

Provides three rate limiting algorithm implementations: Token Bucket, Leaky Bucket, and Sliding Window.

## Features

- ✅ Token Bucket - supports burst traffic
- ✅ Leaky Bucket - smooth rate limiting
- ✅ Sliding Window - precise time window
- ✅ Unified interface (Limiter)
- ✅ Concurrency-safe
- ✅ Zero external dependencies

## Quick Start

### Token Bucket Rate Limiting

```go
import "github.com/everyday-items/toolkit/util/rate"

// Create token bucket limiter
// Capacity: 10 tokens, rate: 5 tokens per second
limiter := rate.NewTokenBucket(10, 5.0)

// Check if request is allowed
if limiter.Allow() {
    // Allowed, execute business logic
    handleRequest()
} else {
    // Rate limited, return error
    return errors.New("rate limit exceeded")
}

// Wait until allowed
waitTime := limiter.Wait()
fmt.Printf("Waited %v\n", waitTime)
handleRequest()
```

### Leaky Bucket Rate Limiting

```go
// Create leaky bucket limiter
// Capacity: 100 requests, rate: 1 request per 100ms
limiter := rate.NewLeakyBucket(100, 100*time.Millisecond)

if limiter.Allow() {
    handleRequest()
} else {
    return errors.New("rate limit exceeded")
}
```

### Sliding Window Rate Limiting

```go
// Create sliding window limiter
// Capacity: max 1000 requests per minute
limiter := rate.NewSlidingWindow(1000, time.Minute)

if limiter.Allow() {
    handleRequest()
} else {
    return errors.New("rate limit exceeded")
}
```

## API Reference

### Limiter Interface

```go
type Limiter interface {
    // Allow checks if request is allowed
    Allow() bool

    // Wait waits until allowed, returns wait time
    Wait() time.Duration
}
```

### Token Bucket

```go
// NewTokenBucket creates a token bucket limiter
// capacity: bucket capacity (max tokens)
// rate: token generation rate (tokens per second)
NewTokenBucket(capacity int, rate float64) *TokenBucket
```

**Characteristics**:
- Supports burst traffic (when bucket has enough tokens)
- Tokens generated at a steady rate
- Suitable for most scenarios

### Leaky Bucket

```go
// NewLeakyBucket creates a leaky bucket limiter
// capacity: bucket capacity
// rate: leak rate (e.g., 100ms means one request leaks per 100ms)
NewLeakyBucket(capacity int, rate time.Duration) *LeakyBucket
```

**Characteristics**:
- Smooth rate limiting, does not support burst
- Processes requests at a steady rate
- Suitable for scenarios requiring strict rate control

### Sliding Window

```go
// NewSlidingWindow creates a sliding window limiter
// capacity: max requests allowed within the window
// window: window size (e.g., 1 minute)
NewSlidingWindow(capacity int, window time.Duration) *SlidingWindow
```

**Characteristics**:
- Precise time window control
- Memory usage proportional to request count
- Suitable for scenarios requiring precise statistics

## Use Cases

### 1. API Rate Limiting Middleware

```go
func RateLimitMiddleware() gin.HandlerFunc {
    // Max 100 requests per second
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

// Usage
router := gin.Default()
router.Use(RateLimitMiddleware())
```

### 2. Per-User Rate Limiting

```go
type UserRateLimiter struct {
    limiters map[string]rate.Limiter
    mu       sync.RWMutex
}

func (u *UserRateLimiter) Allow(userID string) bool {
    u.mu.Lock()
    defer u.mu.Unlock()

    // Get or create user's limiter
    limiter, ok := u.limiters[userID]
    if !ok {
        // Each user: max 60 requests per minute
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

    // Process request
}
```

### 3. IP Rate Limiting

```go
var ipLimiters = make(map[string]rate.Limiter)
var mu sync.RWMutex

func IPRateLimitMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        ip := c.ClientIP()

        mu.Lock()
        limiter, ok := ipLimiters[ip]
        if !ok {
            // Each IP: max 10 requests per second
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

### 4. External API Call Rate Limiting

```go
type APIClient struct {
    limiter rate.Limiter
}

func NewAPIClient() *APIClient {
    return &APIClient{
        // Third-party API limit: max 5 requests per second
        limiter: rate.NewTokenBucket(5, 5.0),
    }
}

func (c *APIClient) CallAPI(endpoint string) (*Response, error) {
    // Wait until allowed
    waitTime := c.limiter.Wait()
    if waitTime > 0 {
        log.Printf("Rate limited, waited %v", waitTime)
    }

    // Call API
    return http.Get(endpoint)
}
```

### 5. Database Write Rate Limiting

```go
type DBWriter struct {
    limiter rate.Limiter
}

func NewDBWriter() *DBWriter {
    return &DBWriter{
        // Limit DB write rate: one write per 100ms
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

### 6. SMS/Email Send Rate Limiting

```go
// Per-user SMS rate limiting
var smsLimiters = make(map[string]rate.Limiter)

func SendSMS(phone, code string) error {
    mu.Lock()
    limiter, ok := smsLimiters[phone]
    if !ok {
        // Each phone number: max 5 SMS per hour
        limiter = rate.NewSlidingWindow(5, time.Hour)
        smsLimiters[phone] = limiter
    }
    mu.Unlock()

    if !limiter.Allow() {
        return errors.New("SMS rate limit exceeded, please try again later")
    }

    // Send SMS
    return smsService.Send(phone, code)
}
```

### 7. Crawler Rate Limiting

```go
type Crawler struct {
    limiter rate.Limiter
}

func NewCrawler() *Crawler {
    return &Crawler{
        // Crawler rate: one request every 2 seconds
        limiter: rate.NewLeakyBucket(1, 2*time.Second),
    }
}

func (c *Crawler) Crawl(urls []string) {
    for _, url := range urls {
        // Wait until allowed to crawl
        c.limiter.Wait()

        // Crawl page
        resp, err := http.Get(url)
        if err != nil {
            log.Printf("Failed to crawl %s: %v", url, err)
            continue
        }

        // Process response
        processResponse(resp)
    }
}
```

## Algorithm Comparison

| Algorithm | Burst Traffic | Smoothness | Memory Usage | Use Case |
|-----------|--------------|------------|--------------|----------|
| Token Bucket | ✅ Supported | Medium | Low (fixed) | General API rate limiting |
| Leaky Bucket | ❌ Not supported | High | Low (fixed) | Strict rate control |
| Sliding Window | ❌ Not supported | Medium | High (proportional to requests) | Precise time window |

### Selection Guide

**Token Bucket (Recommended)**:
```go
// ✅ First choice for most scenarios
limiter := rate.NewTokenBucket(100, 10.0)
```

**Leaky Bucket**:
```go
// ✅ Need strict rate control (e.g., third-party API calls)
limiter := rate.NewLeakyBucket(10, 100*time.Millisecond)
```

**Sliding Window**:
```go
// ✅ Need precise time window statistics (e.g., "max N requests per minute")
limiter := rate.NewSlidingWindow(1000, time.Minute)
```

## Parameter Configuration Examples

### Token Bucket

```go
// High-concurrency: allow burst, but limit to 100 QPS long-term
limiter := rate.NewTokenBucket(1000, 100.0)

// Low-frequency API: max 5 requests per second
limiter := rate.NewTokenBucket(5, 5.0)

// Supports short burst: capacity 100, rate 10
limiter := rate.NewTokenBucket(100, 10.0)
```

### Leaky Bucket

```go
// Strict control: process 1 request per 100ms (10 QPS)
limiter := rate.NewLeakyBucket(10, 100*time.Millisecond)

// Slow processing: 1 request per second
limiter := rate.NewLeakyBucket(5, time.Second)
```

### Sliding Window

```go
// Per-minute limit
limiter := rate.NewSlidingWindow(1000, time.Minute)

// Per-hour limit
limiter := rate.NewSlidingWindow(10000, time.Hour)

// Per-second limit
limiter := rate.NewSlidingWindow(100, time.Second)
```

## Concurrency Safety

All limiters are concurrency-safe:

```go
limiter := rate.NewTokenBucket(100, 10.0)

// Safe to use from multiple goroutines
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

## Performance

```
TokenBucket.Allow():     500 ns/op
LeakyBucket.Allow():     800 ns/op
SlidingWindow.Allow():   1200 ns/op
```

Token Bucket has the best performance; Sliding Window is the slowest (requires cleaning up expired requests).

## Notes

1. **Memory Management**:
   - Token Bucket and Leaky Bucket: fixed memory usage
   - Sliding Window: memory usage proportional to request count; requires periodic cleanup

2. **Clock Precision**:
   - Depends on system clock (`time.Now()`)
   - Precision subject to OS (typically 1ms)

3. **Distributed Rate Limiting**:
   - This package only supports single-machine rate limiting
   - Distributed scenarios require external storage like Redis

4. **Burst Traffic**:
   - Token Bucket: can handle burst when bucket is full
   - Leaky Bucket/Sliding Window: do not support burst

5. **Reset Mechanism**:
   - Limiters do not support reset after creation
   - Create a new limiter instance if reset is needed

## Dependencies

```bash
# Zero external dependencies, uses only standard library
import (
    "sync"
    "time"
)
```

## Extension Suggestions

For distributed rate limiting, consider:
- `golang.org/x/time/rate` - Official rate limiting library
- Redis + Lua - Distributed token/leaky bucket
- `github.com/juju/ratelimit` - More rate limiting algorithms
