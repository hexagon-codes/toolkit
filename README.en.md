[中文](README.md) | English

# toolkit

A production-grade Go general-purpose toolkit with domain-driven design principles.

[![Go Version](https://img.shields.io/badge/Go-%3E%3D%201.22-blue)](https://golang.org/)

## Features

✅ **Domain-Driven Design** - Organized by functional domains, clear layered architecture
✅ **Production-Grade Code** - Battle-tested, high-quality implementations
✅ **Interface-Driven** - Easy to extend and test
✅ **Zero-Copy Optimization** - High-performance string/byte operations
✅ **Full Observability** - Prometheus metrics support
✅ **Generics Support** - Type-safe implementations using Go 1.22+ generics
✅ **Security-First** - SSRF protection (IPv6), HMAC constant-time comparison, AES-GCM recommended
✅ **AI Ecosystem** - Preset clients for 14+ platforms (OpenAI/Claude/Gemini) with streaming response handling
✅ **Multi-Layer Cache** - Local → Redis → DB three-layer protection (cache breakdown/penetration/avalanche)
✅ **HTTP Connection Pool** - Connection reuse, retry replay, rate limiting, circuit breaker middleware
✅ **Circuit Breaker** - AI API dedicated circuit breaker presets, multi-instance management

## Quick Start

```bash
go get github.com/hexagon-codes/toolkit
```

### Type Conversion

```go
import "github.com/hexagon-codes/toolkit/lang/conv"

str := conv.String(123)           // "123"
i := conv.Int("42")               // 42
f := conv.Float64("3.14")         // 3.14

// JSON-Map conversion
m, _ := conv.JsonToMap(`{"key":"value"}`)
json, _ := conv.MapToJson(m)
```

### String Utilities

```go
import "github.com/hexagon-codes/toolkit/lang/stringx"

// Zero-copy conversion
str := stringx.BytesToString([]byte("hello"))
bytes := stringx.String2Bytes("world")

// Case conversion
stringx.CamelCase("hello_world")     // "helloWorld"
stringx.SnakeCase("HelloWorld")      // "hello_world"
stringx.KebabCase("helloWorld")      // "hello-world"

// String operations
stringx.Truncate("hello world", 5)   // "he..."
stringx.PadLeft("42", 5, "0")        // "00042"
stringx.Reverse("hello")             // "olleh"
```

### Map Operations

```go
import "github.com/hexagon-codes/toolkit/lang/mapx"

m := map[string]int{"a": 1, "b": 2, "c": 3}

keys := mapx.Keys(m)                           // ["a", "b", "c"]
values := mapx.Values(m)                       // [1, 2, 3]
filtered := mapx.Filter(m, func(k string, v int) bool { return v > 1 })
merged := mapx.Merge(m1, m2)
inverted := mapx.Invert(m)                     // map[int]string
```

### Error Handling

```go
import "github.com/hexagon-codes/toolkit/lang/errorx"

// Must - panic on error
value := errorx.Must(strconv.Atoi("42"))

// Try - catch panic
err := errorx.Try(func() {
    // risky operation
})

// Wrap - add context
err = errorx.Wrap(err, "failed to process")

// Result type
result := errorx.Ok(42)
if result.IsOk() {
    fmt.Println(result.Value())
}
```

### Time Utilities

```go
import "github.com/hexagon-codes/toolkit/lang/timex"

timex.IsToday(t)                    // whether it is today
timex.IsWeekend(t)                  // whether it is a weekend
timex.StartOfDay(t)                 // 00:00:00 of the day
timex.EndOfMonth(t)                 // end of month
timex.DaysBetween(t1, t2)           // number of days between
timex.Age(birthday)                 // calculate age

// Duration formatting
timex.FormatDuration(2*time.Hour + 30*time.Minute)  // "2h30m"
d, _ := timex.ParseDuration("1d2h30m")               // supports day unit

// Timezone support
t := timex.NowShanghai()            // Shanghai time
t = timex.InShanghai(time.Now())    // convert to Shanghai time
```

### Conditional Utilities

```go
import "github.com/hexagon-codes/toolkit/lang/cond"

// If ternary expression
result := cond.If(age >= 18, "adult", "minor")

// IfFunc lazy evaluation
result := cond.IfFunc(expensive,
    func() string { return compute() },
    func() string { return "default" },
)

// IfZero zero-value check
name := cond.IfZero(user.Name, "Anonymous")

// Coalesce returns the first non-zero value
value := cond.Coalesce(a, b, c, defaultVal)

// Switch type-safe switch
result := cond.Switch[string, string](status).
    Case("pending", "Pending").
    Case("running", "Running").
    Case("done", "Done").
    Default("Unknown")
```

### Tuple Types

```go
import "github.com/hexagon-codes/toolkit/lang/tuple"

// Create tuples
t2 := tuple.T2("name", 42)
t3 := tuple.T3("x", "y", "z")

// Destructure
a, b := t2.Unpack()

// Swap
swapped := t2.Swap()  // Tuple2[int, string]

// Zip two slices together
names := []string{"Alice", "Bob"}
ages := []int{20, 25}
pairs := tuple.Zip2(names, ages)  // []Tuple2[string, int]

// Unzip separate
names, ages = tuple.Unzip2(pairs)
```

### Optional Type

```go
import "github.com/hexagon-codes/toolkit/lang/optional"

// Create Option
opt := optional.Some(42)
empty := optional.None[int]()
fromPtr := optional.FromPtr(ptr)  // nil pointer → None

// Check and get
if opt.IsSome() {
    value := opt.Unwrap()
}
value := opt.UnwrapOr(defaultVal)
value := opt.UnwrapOrElse(func() int { return compute() })

// Transform
doubled := optional.Map(opt, func(n int) int { return n * 2 })
result := optional.FlatMap(opt, func(n int) optional.Option[string] {
    return optional.Some(strconv.Itoa(n))
})

// Filter
positive := opt.Filter(func(n int) bool { return n > 0 })
```

### Stream API

```go
import "github.com/hexagon-codes/toolkit/lang/stream"

// Create Stream
s := stream.Of(1, 2, 3, 4, 5)
s := stream.FromSlice(slice)
s := stream.Range(0, 100)
s := stream.Generate(10, func(i int) int { return i * 2 })

// Chained operations
result := stream.Of(1, 2, 3, 4, 5, 6, 7, 8, 9, 10).
    Filter(func(n int) bool { return n%2 == 0 }).  // even numbers
    Map(func(n int) int { return n * n }).          // square
    Limit(3).                                        // take first 3
    Collect()                                        // [4, 16, 36]

// Terminal operations
count := s.Count()
sum := s.Reduce(0, func(a, b int) int { return a + b })
first, ok := s.First()
any := s.Any(func(n int) bool { return n > 10 })
all := s.All(func(n int) bool { return n > 0 })

// Type conversion
strings := stream.MapTo(s, func(n int) string {
    return strconv.Itoa(n)
})

// Grouping
groups := stream.GroupBy(users, func(u User) string {
    return u.Department
})
```

### Multi-Error Aggregation

```go
import "github.com/hexagon-codes/toolkit/lang/errorx"

// MultiError collects multiple errors
me := errorx.NewMultiError()
me.Append(err1).Append(err2)
if err := me.ErrorOrNil(); err != nil {
    return err
}

// Parallel execution
me := errorx.Go(
    func() error { return task1() },
    func() error { return task2() },
    func() error { return task3() },
)

// Limit concurrency
me := errorx.GoWithLimit(5,
    func() error { return process(item1) },
    func() error { return process(item2) },
    // ... more tasks
)

// Walk the error chain
errorx.Walk(err, func(e error) bool {
    if myErr, ok := e.(*MyError); ok {
        handle(myErr)
        return false  // stop walking
    }
    return true
})
```

### Concurrency Utilities

```go
import "github.com/hexagon-codes/toolkit/lang/syncx"

// ConcurrentMap - generic concurrency-safe map
m := syncx.NewConcurrentMap[string, int]()
m.Store("count", 1)
value, ok := m.Load("count")
m.Update("count", func(v int) int { return v + 1 })  // atomic update
value := m.GetOrCompute("key", func() int { return expensive() })

// Singleflight - prevent cache breakdown
sf := syncx.NewSingleflight()
result, err := sf.Do("user:123", func() (any, error) {
    return db.GetUser(123)  // multiple concurrent requests execute only once
})

// Semaphore - semaphore (supports context timeout)
sem := syncx.NewSemaphore(10)  // at most 10 concurrent
sem.Acquire()
defer sem.Release()
sem.TryAcquire()                        // non-blocking attempt
sem.AcquireContext(ctx)                  // supports timeout cancellation

// Once - generic sync.Once (can return value)
var once syncx.Once[*Config]
cfg := once.Do(func() *Config { return loadConfig() })
val, ok := once.Value()                  // query if already initialized

// OnceErr - Once with error support
var onceErr syncx.OnceErr[*DB]
db, err := onceErr.Do(func() (*DB, error) { return connectDB() })

// OnceValue / OnceFunc - functional wrappers
getConfig := syncx.OnceValue(func() *Config { return loadConfig() })
cfg1 := getConfig()                      // first execution
cfg2 := getConfig()                      // returns cached value

initOnce := syncx.OnceFunc(func() { initialize() })
initOnce()                               // executes
initOnce()                               // does not execute again

// Lazy - lazy initialization
config := syncx.NewLazy(func() *Config {
    return loadConfigFromFile()
})
cfg := config.Get()                      // initialized on first call
config.IsInitialized()                   // query state

// LazyErr - lazy initialization with error support
db := syncx.NewLazyErr(func() (*DB, error) {
    return connectDB()
})
conn, err := db.Get()                    // initialized on first call
conn = db.MustGet()                      // panic on error
```

### Slice Enhancements

```go
import "github.com/hexagon-codes/toolkit/lang/slicex"

// Partition
even, odd := slicex.Partition(nums, func(n int) bool {
    return n%2 == 0
})

// Aggregate operations
min := slicex.Min(nums)
max := slicex.Max(nums)
sum := slicex.Sum(nums)
avg := slicex.Average(nums)

// Range generates a sequence
nums := slicex.Range(0, 10, 2)  // [0, 2, 4, 6, 8]

// Shuffle randomly reorders
slicex.Shuffle(slice)
sample := slicex.Sample(slice, 5)  // randomly pick 5

// Channel conversion
ch := slicex.ToChannel(slice)
slice := slicex.FromChannel(ch)
```

### Context Utilities

```go
import "github.com/hexagon-codes/toolkit/lang/contextx"

// Type-safe context key
userKey := contextx.NewKey[User]("user")
ctx = contextx.WithValue(ctx, userKey, user)
user, ok := contextx.Value(ctx, userKey)
user = contextx.ValueOr(ctx, userKey, defaultUser)

// Common key shortcuts
ctx = contextx.WithTraceID(ctx, "trace-123")
ctx = contextx.WithUserID(ctx, 12345)
traceID := contextx.TraceID(ctx)
userID := contextx.UserID(ctx)

// State checks
contextx.IsTimeout(ctx)             // whether timed out
contextx.IsCanceled(ctx)            // whether canceled
contextx.IsDone(ctx)                // whether done
contextx.Remaining(ctx)             // remaining time

// Execution control
contextx.Run(ctx, func() error { ... })
contextx.RunTimeout(5*time.Second, func() error { ... })

// Detach - detach from parent context cancellation, retain values
detached := contextx.Detach(ctx)

// WaitGroup with Context
wg := contextx.NewWaitGroupContext(ctx)
wg.Go(func(ctx context.Context) error { ... })
wg.Wait()

// Goroutine pool
pool := contextx.NewPool(ctx, 10)
pool.Go(func(ctx context.Context) error { ... })
pool.Wait()
```

### AES Encryption

```go
import "github.com/hexagon-codes/toolkit/crypto/aes"

key, _ := aes.GenerateKey(32)  // AES-256

// GCM mode (recommended)
ciphertext, _ := aes.EncryptGCM(plaintext, key)
plaintext, _ := aes.DecryptGCM(ciphertext, key)

// String encrypt/decrypt
encrypted, _ := aes.EncryptGCMString("secret", "32-byte-key-here")
decrypted, _ := aes.DecryptGCMString(encrypted, "32-byte-key-here")
```

### RSA Encryption

```go
import "github.com/hexagon-codes/toolkit/crypto/rsa"

kp, _ := rsa.GenerateKeyPair(2048)

// Encrypt/Decrypt
ciphertext, _ := kp.Encrypt(plaintext)
plaintext, _ := kp.Decrypt(ciphertext)

// Sign/Verify
signature, _ := kp.Sign(message)
err := kp.Verify(message, signature)

// PEM export
privatePEM := kp.PrivateKeyToPEM()
publicPEM := kp.PublicKeyToPEM()
```

### HMAC Signing

```go
import "github.com/hexagon-codes/toolkit/crypto/sign"

sig := sign.HMACSHA256String("message", "secret-key")
ok := sign.VerifyHMACSHA256String("message", "secret-key", sig)

// API signing
signer := sign.NewAPISigner("app-key", "app-secret")
sig := signer.Sign(params, timestamp, nonce)
```

### HTTP Client

```go
import "github.com/hexagon-codes/toolkit/net/httpx"

// Simple requests
resp, _ := httpx.Get("https://api.example.com/users")
resp, _ := httpx.Post("https://api.example.com/users", body)

// Fluent API
client := httpx.NewClient(
    httpx.WithTimeout(10*time.Second),
    httpx.WithRetry(3, time.Second),
)
resp, _ := client.R().
    SetHeader("Authorization", "Bearer token").
    SetQuery("page", "1").
    Get("/api/users")

// Parse response
var users []User
resp.JSON(&users)

// SSRF protection (blocks internal network access, supports IPv6 whitelist)
client := httpx.NewClient(
    httpx.WithSSRFProtection("api.trusted.com", "[::1]:8080"),
)
resp, err := client.R().Get(userProvidedURL)
if errors.Is(err, httpx.ErrSSRFBlocked) {
    // request was blocked
}
```

### HTTP Connection Pool

```go
import "github.com/hexagon-codes/toolkit/net/httpx"

// Create connection pool
pool := httpx.NewPool(httpx.PoolConfig{
    MaxIdleConns:    100,
    MaxConnsPerHost: 10,
    IdleConnTimeout: 90 * time.Second,
})
defer pool.Close()

// Execute request
req, _ := http.NewRequest("GET", "https://api.example.com", nil)
resp, _ := pool.Do(req)

// View statistics
stats := pool.GetStats()
fmt.Printf("Total: %d, Active: %d, Errors: %d\n",
    stats.TotalRequests, stats.ActiveRequests, stats.ErrorCount)

// Global connection pool
httpx.SetGlobalPool(pool)
p := httpx.GlobalPool()

// Host-level connection pool (automatically assigns dedicated pools per host)
hostPool := httpx.NewHostPool()
defer hostPool.Close()
hostPool.SetHostConfig("api.example.com", httpx.PoolConfig{MaxConnsPerHost: 20})
resp, _ = hostPool.Do(req)

// Retry pool (automatically caches body for replay)
retryPool := httpx.NewRetryPool(pool, httpx.RetryConfig{
    MaxRetries:   3,
    RetryWait:    100 * time.Millisecond,
    MaxRetryWait: 5 * time.Second,
    RetryCondition: func(resp *http.Response, err error) bool {
        return err != nil || resp.StatusCode >= 500
    },
})

// Rate-limited connection pool (implements io.Closer, idempotent via sync.Once)
rateLimitedPool := httpx.NewRateLimitedPool(pool, 100)  // 100 QPS
defer rateLimitedPool.Close()

// Circuit breaker connection pool
cbPool := httpx.NewCircuitBreakerPool(pool, httpx.CircuitBreakerConfig{
    FailureThreshold: 5,
    SuccessThreshold: 2,
    Timeout:          30 * time.Second,
})
```

### AI Client Presets

```go
import "github.com/hexagon-codes/toolkit/net/httpx"

// Preset clients for major AI platforms (auto-configures BaseURL, auth headers, timeouts, etc.)
openai := httpx.OpenAIClient("sk-xxx")
claude := httpx.ClaudeClient("sk-ant-xxx")
gemini := httpx.GeminiClient("AIza-xxx")
deepseek := httpx.DeepSeekClient("sk-xxx")
qwen := httpx.QwenClient("sk-xxx")           // Alibaba Qwen
zhipu := httpx.ZhipuClient("xxx.xxx")        // Zhipu ChatGLM
moonshot := httpx.MoonshotClient("sk-xxx")    // Moonshot AI
doubao := httpx.DoubaoClient("xxx")           // ByteDance Doubao

// Custom AI client
custom := httpx.CustomAIClient("https://my-api.com", "my-token")

// Streaming request
stream, _ := claude.R().
    SetJSONBody(requestBody).
    PostStream("/v1/messages")
defer stream.Close()

// Read SSE events
for {
    event, err := stream.ReadSSE()
    if err != nil { break }
    fmt.Println(event.Data)
}

// Read OpenAI-format streaming JSON
var chunk httpx.OpenAIStreamChunk
for {
    err := stream.ReadJSON(&chunk)
    if err != nil { break }
    fmt.Print(chunk.Choices[0].Delta.Content)
}

// Collect all content in one line
content, _ := stream.CollectOpenAIContent()
```

### SSE Server-Sent Events

```go
import "github.com/hexagon-codes/toolkit/net/sse"

// Client - receive SSE events
client := sse.NewClient("https://api.example.com/events",
    sse.WithTimeout(30*time.Second),
    sse.WithLastEventID("last-id"),
)
stream, _ := client.Connect(ctx)
defer stream.Close()

for event := range stream.Events() {
    fmt.Printf("Event: %s, Data: %s\n", event.Event, event.Data)
    var data MyData
    event.JSON(&data)
}

// Server - send SSE events
func handler(w http.ResponseWriter, r *http.Request) {
    writer := sse.NewWriter(w)
    defer writer.Close()

    for {
        writer.Write(&sse.Event{
            ID:    "1",
            Event: "message",
            Data:  "Hello, World!",
        })
        writer.WriteJSON(myData)
        time.Sleep(time.Second)
    }
}

// OpenAI streaming response handling
sse.ReadOpenAIStream(resp.Body, func(chunk ChatCompletion) error {
    fmt.Print(chunk.Choices[0].Delta.Content)
    return nil
})
```

### Circuit Breaker

```go
import "github.com/hexagon-codes/toolkit/util/circuit"

// Basic usage
breaker := circuit.New(
    circuit.WithThreshold(5),           // open circuit after 5 failures
    circuit.WithTimeout(30*time.Second), // circuit stays open for 30 seconds
    circuit.WithHalfOpenMaxRequests(3), // at most 3 probe requests in half-open state
    circuit.WithSuccessThreshold(2),    // recover after 2 successes
)

result, err := breaker.Execute(func() (any, error) {
    return callAPI()
})

// AI API dedicated circuit breakers (with built-in preset configurations)
openaiBreaker := circuit.NewAIBreaker(circuit.OpenAIConfig)
claudeBreaker := circuit.NewAIBreaker(circuit.ClaudeConfig)
geminiBreaker := circuit.NewAIBreaker(circuit.GeminiConfig)

// Preset styles
aggressiveBreaker := circuit.NewAIBreaker(circuit.AggressiveConfig)       // fast trip
conservativeBreaker := circuit.NewAIBreaker(circuit.ConservativeConfig)   // slow trip

// Custom failure predicate
breaker = circuit.New(
    circuit.WithIsFailure(circuit.IsRateLimitOrServerError),  // only 429/5xx triggers
)

// Multi-breaker manager (isolated by name)
manager := circuit.NewBreakerManager(func() *circuit.Breaker {
    return circuit.NewAIBreaker(circuit.OpenAIConfig)
})
result, err = manager.Execute("gpt-4", func() (any, error) {
    return callGPT4()
})
manager.Execute("claude", func() (any, error) {
    return callClaude()
})
states := manager.States()  // map[string]State

// State change listener
breaker.OnStateChange(func(from, to circuit.State) {
    log.Printf("circuit breaker state: %s -> %s", from, to)
})
```

### Event Bus

```go
import "github.com/hexagon-codes/toolkit/event"

// Create event bus
bus := event.New()
defer bus.Close()

// Subscribe to a specific event type
unsub := bus.Subscribe("agent.start", func(e event.Event) {
    fmt.Printf("Agent started: %v (source: %s)\n", e.Payload, e.Source)
})
defer unsub()  // unsubscribe

// Subscribe to all events (global subscription)
unsubAll := bus.SubscribeAll(func(e event.Event) {
    fmt.Printf("[%s] %v\n", e.Type, e.Payload)
})
defer unsubAll()

// Publish event
bus.Publish(event.Event{
    Type:    "agent.start",
    Payload: "my-agent",
    Source:  "scheduler",
})

// Predefined event type constants
bus.Publish(event.Event{Type: event.EventLLMRequest,  Payload: req})
bus.Publish(event.Event{Type: event.EventLLMResponse, Payload: resp})
bus.Publish(event.Event{Type: event.EventToolCall,    Payload: toolName})
bus.Publish(event.Event{Type: event.EventCostUpdate,  Payload: cost})
bus.Publish(event.Event{Type: event.EventAgentError,  Payload: err})

// Configuration options
bus = event.New(
    event.WithMaxGoroutines(512),              // limit concurrent goroutines
    event.WithPanicHandler(func(e event.Event, v any) {
        log.Printf("handler panic: %v", v)     // catch handler panics
    }),
)

// Subscriber count
count := bus.SubscriberCount("agent.start")
```

### IP Utilities

```go
import "github.com/hexagon-codes/toolkit/net/ip"

ip.IsValid("192.168.1.1")           // true
ip.IsPrivate("192.168.1.1")         // true
ip.IsIPv4("192.168.1.1")            // true
ip.IsInCIDR("192.168.1.100", "192.168.1.0/24")  // true

// Get client IP from HTTP request
clientIP := ip.FromRequest(r)

// Local IP
localIP, _ := ip.GetLocalIP()
```

### Logging

```go
import "github.com/hexagon-codes/toolkit/util/logger"

// Quick usage
logger.Info("user login", "userId", 123, "ip", "192.168.1.1")
logger.Error("request failed", "error", err)

// Configuration
logger.Init(&logger.Config{
    Level:  "info",
    Format: "json",
    Output: "stdout",
})

// With fields
log := logger.With("service", "user-api")
log.Info("started", "port", 8080)
```

### Environment Variables

```go
import "github.com/hexagon-codes/toolkit/util/env"

port := env.GetIntDefault("PORT", 8080)
debug := env.GetBool("DEBUG")
hosts := env.GetSlice("HOSTS")  // comma-separated

if env.IsProd() {
    // production environment
}
```

### Encoding Utilities

```go
import "github.com/hexagon-codes/toolkit/util/encoding"

// Base64
encoded := encoding.Base64EncodeString("hello")
decoded, _ := encoding.Base64DecodeString(encoded)

// Hex
hex := encoding.HexEncodeString("hello")

// URL
query := encoding.BuildQuery(map[string]string{"name": "test"})
params, _ := encoding.ParseQuery("name=test&age=18")
```

### Reflection Utilities

```go
import "github.com/hexagon-codes/toolkit/util/reflectx"

// Struct ↔ Map conversion
user := User{Name: "Alice", Age: 20}
m := reflectx.StructToMap(user)                    // map[string]any{"Name": "Alice", "Age": 20}
m = reflectx.StructToMapWithTag(user, "json")      // use json tag as key

var user2 User
reflectx.MapToStruct(m, &user2)

// Field operations
name, _ := reflectx.GetField(user, "Name")
reflectx.SetField(&user, "Name", "Bob")
reflectx.HasField(user, "Name")                    // true
names := reflectx.FieldNames(user)                 // ["Name", "Age"]

// Deep copy (supports cycle detection, nil-safe)
copied := reflectx.DeepCopy(original)              // recursive deep copy
shallow := reflectx.Clone(original)                // shallow copy

// Type checks
reflectx.IsZero(value)
reflectx.IsNil(value)
reflectx.TypeName(value)                           // "User"
reflectx.IsPtr(value)
reflectx.IsStruct(value)
reflectx.IsSlice(value)
```

### Struct Validation

```go
import "github.com/hexagon-codes/toolkit/util/validator"

type User struct {
    Name     string `validate:"required,min=2,max=50"`
    Email    string `validate:"required,email"`
    Age      int    `validate:"min=0,max=150"`
    Password string `validate:"required,min=8"`
    Role     string `validate:"oneof=admin user guest"`
    Website  string `validate:"omitempty,url"`
}

// Validate
v := validator.New()
if err := v.Struct(user); err != nil {
    for _, e := range err.(validator.ValidationErrors) {
        fmt.Printf("Field %s failed validation: %s\n", e.Field, e.Tag)
    }
}

// Supported tags
// required  - required field
// email     - email format
// url       - URL format
// min=n     - minimum value/length
// max=n     - maximum value/length
// len=n     - exact length
// oneof=a b - enum values
// regexp=x  - regex match
// omitempty - skip if empty

// Custom validation rule
v.RegisterRule("phone", func(value any) bool {
    return validator.Phone(value.(string))
})
```

### Poolx Goroutine Pool

```go
import "github.com/hexagon-codes/toolkit/util/poolx"

// Create goroutine pool
p := poolx.New("my-pool", poolx.WithMaxWorkers(10))
defer p.Release()

p.Submit(func() {
    // task
})

// Future pattern
future := poolx.SubmitFunc(p, func() (int, error) {
    return compute(), nil
})
result, err := future.Get()

// Await first completion (uses cancel context to prevent goroutine leaks)
f1 := poolx.SubmitFunc(p, func() (int, error) { return callAPI1() })
f2 := poolx.SubmitFunc(p, func() (int, error) { return callAPI2() })
val, idx, err := poolx.AwaitFirst(f1, f2)

// Parallel Map
results, _ := poolx.Map(ctx, items, 4, func(item T) (R, error) {
    return process(item), nil
})
```

### Configuration Management

```go
import "github.com/hexagon-codes/toolkit/util/config"

// Load from file (supports JSON/YAML/TOML/ENV)
cfg, _ := config.Load("config.yaml")

// Get config values
name := cfg.GetString("app.name")
port := cfg.GetIntDefault("app.port", 8080)
debug := cfg.GetBool("app.debug")
timeout := cfg.GetDuration("app.timeout")
hosts := cfg.GetStringSlice("app.hosts")

// Load from environment variables
cfg.LoadEnv("APP")  // APP_NAME -> name, APP_PORT -> port

// Bind to struct
var appCfg struct {
    Name    string        `env:"NAME" default:"myapp"`
    Port    int           `env:"PORT" default:"8080"`
    Debug   bool          `env:"DEBUG"`
    Timeout time.Duration `env:"TIMEOUT" default:"30s"`
}
config.BindEnv(&appCfg, "APP")

// Global config
config.LoadGlobal("config.yaml")
config.GetString("key")
config.Set("key", "value")
```

### List Doubly Linked List

```go
import "github.com/hexagon-codes/toolkit/collection/list"

// Create list
l := list.New(1, 2, 3)
l.PushFront(0)                    // insert at front
l.PushBack(4)                     // insert at back

// Access elements
front := l.Front()                // front node
back := l.Back()                  // back node
next := front.Next()              // next node
prev := back.Prev()               // previous node

// Remove elements
val, ok := l.PopFront()           // remove from front
val, ok = l.PopBack()             // remove from back
l.Remove(node)                    // remove specific node

// Move nodes
l.MoveToFront(node)               // move to front
l.MoveToBack(node)                // move to back
l.MoveBefore(node, mark)          // move before mark
l.MoveAfter(node, mark)           // move after mark

// Find and traverse
l.Find(func(v int) bool { return v > 2 })
l.ForEach(func(v int) { fmt.Println(v) })
l.ForEachReverse(func(v int) { fmt.Println(v) })

// Other operations
l.Reverse()                       // reverse the list
l.Clone()                         // clone
l.Filter(func(v int) bool { return v%2 == 0 })

// Thread-safe version
sl := list.NewSyncList[int]()
```

### Stack

```go
import "github.com/hexagon-codes/toolkit/collection/stack"

// Create stack
s := stack.New(1, 2, 3)
s.Push(4, 5)                      // push

// Pop operations
top, ok := s.Pop()                // pop (returns 5)
top, ok = s.Peek()                // peek top (without removing)

// Batch operations
items := s.PopN(3)                // pop N elements
items = s.PeekN(3)                // peek top N elements

// Traverse
s.ForEach(func(v int) { ... })          // bottom to top
s.ForEachReverse(func(v int) { ... })   // top to bottom

// Other operations
s.Reverse()                       // reverse the stack
s.Clone()                         // clone
s.Contains(func(v int) bool { return v == 3 })

// Thread-safe version
ss := stack.NewSyncStack[int]()
```

### Queue

```go
import "github.com/hexagon-codes/toolkit/collection/queue"

// FIFO queue
q := queue.New(1, 2, 3)
q.Enqueue(4, 5)
item, ok := q.Dequeue()           // 1, true
front, _ := q.Peek()              // 2

// Deque (double-ended queue)
dq := queue.NewDeque[int]()
dq.PushFront(1)
dq.PushBack(2)
dq.PopFront()                     // 1
dq.PopBack()                      // 2

// Priority queue (min-heap)
pq := queue.NewMinHeap[int]()
pq.Push(5, 3, 1, 4, 2)
pq.Pop()                          // 1
pq.Pop()                          // 2

// Max-heap
maxPQ := queue.NewMaxHeap[int]()
maxPQ.Push(1, 5, 3)
maxPQ.Pop()                       // 5

// Custom priority
type Task struct {
    Name     string
    Priority int
}
taskPQ := queue.NewPriorityQueue[Task](func(a, b Task) bool {
    return a.Priority > b.Priority  // higher priority dequeued first
})

// Thread-safe versions
sq := queue.NewSyncQueue[int]()
sd := queue.NewSyncDeque[int]()
```

### Set

```go
import "github.com/hexagon-codes/toolkit/collection/set"

// Create Set
s := set.New(1, 2, 3)
s.Add(4, 5)
s.Remove(1)

// Basic operations
s.Contains(2)              // true
s.Size()                   // 4
s.IsEmpty()                // false
s.ToSlice()                // [2, 3, 4, 5]

// Set operations
s1 := set.New(1, 2, 3)
s2 := set.New(2, 3, 4)

union := s1.Union(s2)                    // {1, 2, 3, 4}
intersection := s1.Intersection(s2)      // {2, 3}
difference := s1.Difference(s2)          // {1}
symDiff := s1.SymmetricDifference(s2)    // {1, 4}

// Relationship checks
s1.IsSubset(s2)            // false
s1.IsSuperset(s2)          // false
s1.IsDisjoint(s2)          // false
s1.Equal(s2)               // false

// Functional operations
even := s.Filter(func(n int) bool { return n%2 == 0 })
s.ForEach(func(n int) { fmt.Println(n) })
s.Any(func(n int) bool { return n > 10 })
s.All(func(n int) bool { return n > 0 })
```

## Recent Changes

- **net/httpx**: `RateLimitedPool` now implements `io.Closer` with `Close() error`, idempotent via `sync.Once` (safe to call multiple times)
- **util/poolx**: `AwaitFirst` now uses a cancellable context to release waiting goroutines once the first result arrives, preventing goroutine leaks
- **util/poolx**: Fixed `workerStack.retrieveExpiry` ring buffer compaction logic to correctly rebuild the surviving worker queue after expiry cleanup

## Project Structure

```
toolkit/
├── event/              # Event bus (pub/sub, thread-safe)
│
├── cache/              # Caching
│   ├── local/         # Local cache (LRU)
│   ├── redis/         # Redis cache
│   └── multi/         # Multi-layer cache (breakdown/penetration/avalanche protection)
│
├── collection/         # Data structures (zero external dependencies)
│   ├── list/          # Doubly linked list
│   ├── queue/         # Queue (FIFO/Deque/Priority)
│   ├── set/           # Generic HashSet
│   └── stack/         # Stack (LIFO)
│
├── crypto/             # Cryptography utilities
│   ├── aes/           # AES encryption (GCM recommended)
│   ├── rsa/           # RSA asymmetric encryption
│   └── sign/          # HMAC sign/verify
│
├── infra/              # Infrastructure
│   ├── db/            # Databases
│   │   ├── mysql/
│   │   ├── redis/
│   │   ├── mongodb/
│   │   ├── clickhouse/
│   │   └── elasticsearch/
│   ├── queue/         # Message queue
│   │   └── asynq/
│   ├── observe/       # Observability
│   ├── otel/          # OpenTelemetry
│   └── prometheus/    # Prometheus metrics
│
├── lang/               # Language enhancements (zero external dependencies)
│   ├── cond/          # Conditional utilities (If/Switch/Coalesce)
│   ├── contextx/      # Context utilities
│   ├── conv/          # Type conversion
│   ├── errorx/        # Error handling (MultiError/Walk)
│   ├── mapx/          # Map utilities (generics)
│   ├── mathx/         # Math utilities (generics)
│   ├── optional/      # Option type
│   ├── slicex/        # Slice utilities (generics)
│   ├── stream/        # Stream API
│   ├── stringx/       # String extensions
│   ├── syncx/         # Concurrency utilities (ConcurrentMap/Semaphore/Once/Lazy/Pool)
│   ├── timex/         # Time utilities
│   └── tuple/         # Tuple types (Tuple2/3/4)
│
├── net/                # Network utilities
│   ├── httpx/         # HTTP client (SSRF protection/connection pool/retry/rate limiting/AI presets)
│   ├── ip/            # IP utilities
│   └── sse/           # Server-Sent Events
│
├── util/               # Utility components
│   ├── circuit/       # Circuit breaker (AI presets/multi-instance management)
│   ├── config/        # Configuration management
│   ├── encoding/      # Encoding (Base64/Hex/URL)
│   ├── env/           # Environment variables
│   ├── file/          # File operations
│   ├── hash/          # Hashing (MD5/SHA/Bcrypt)
│   ├── idgen/         # ID generation (Snowflake)
│   ├── json/          # JSON helpers
│   ├── logger/        # Logging (based on slog)
│   ├── pagination/    # Pagination
│   ├── poolx/         # High-performance goroutine pool
│   ├── rand/          # Random numbers
│   ├── rate/          # Rate limiter
│   ├── reflectx/      # Reflection utilities (DeepCopy/Clone/StructToMap)
│   ├── retry/         # Retry mechanism
│   ├── slice/         # Slice utilities
│   └── validator/     # Data validation (with struct tags)
│
└── examples/           # Usage examples
```

## Test Coverage

| Package | Coverage |
|---|--------|
| collection/list | 79.4% |
| collection/queue | 90.5% |
| collection/set | 73.8% |
| collection/stack | 100.0% |
| event | 80.6% |
| lang/cond | 94.5% |
| lang/contextx | 82.0% |
| lang/conv | 68.1% |
| lang/errorx | 87.2% |
| lang/mapx | 96.3% |
| lang/mathx | 88.7% |
| lang/optional | 100.0% |
| lang/slicex | 78.4% |
| lang/stream | 94.4% |
| lang/stringx | 95.9% |
| lang/syncx | 84.9% |
| lang/timex | 91.2% |
| lang/tuple | 93.8% |
| crypto/aes | 83.5% |
| crypto/rsa | 81.4% |
| crypto/sign | 80.6% |
| net/httpx | 50.0% |
| net/ip | 64.9% |
| net/sse | 82.5% |
| cache/local | 76.7% |
| cache/multi | 89.5% |
| cache/redis | 75.1% |
| util/circuit | 85.9% |
| util/config | 78.2% |
| util/encoding | 94.0% |
| util/env | 97.4% |
| util/file | 80.0% |
| util/hash | 100.0% |
| util/idgen | 72.8% |
| util/json | 78.7% |
| util/logger | 90.4% |
| util/pagination | 92.6% |
| util/poolx | 70.2% |
| util/rand | 86.8% |
| util/rate | 61.8% |
| util/reflectx | 89.7% |
| util/retry | 63.7% |
| util/slice | 100.0% |
| util/validator | 87.2% |
| infra/db | 75.8% |
| infra/db/mysql | 51.7% |
| infra/db/redis | 79.8% |
| infra/observe | 66.7% |
| infra/otel | 29.7% |
| infra/prometheus | 85.0% |
| infra/queue/asynq | 26.4% |

## Design Philosophy

### 1. Domain-Driven Organization

Code is grouped by functional domain, not technical type:

```
❌ Not recommended: util/string.go, util/time.go
✅ Recommended:     lang/stringx/, lang/timex/
```

### 2. Clear Layered Architecture

```
ai (AI tools) → infra (infrastructure) → net (network) → cache (caching)
     ↓                  ↓                     ↓               ↓
external deps      external services      may depend       standalone

     ↓                  ↓                     ↓               ↓
crypto (crypto) → util (utilities) → collection (data structs) → lang (zero deps)
     ↓                  ↓                     ↓                       ↓
  x/crypto          may depend           stdlib only              stdlib only
```

**Key constraint**: `lang/` and `collection/` packages must maintain zero external dependencies.

### 3. Security-First

- AES-CBC/CTR marked as Deprecated; GCM recommended
- HMAC verification uses constant-time comparison
- PKCS7 padding validation prevents timing attacks
- HTTP client has built-in SSRF protection
- Signature verification supports timestamp expiry and nonce replay prevention

### 4. Performance Optimization

- Zero-copy string operations (unsafe)
- Object pooling and cache reuse
- Minimize reflection usage
- Singleflight prevents cache breakdown

### 5. Generics-First

All collection and utility functions prefer generic implementations for type safety.

## Dependencies

Core dependencies:
```
github.com/hibiken/asynq           # task queue
github.com/redis/go-redis/v9       # Redis client
github.com/prometheus/client_golang # metrics
golang.org/x/sync                  # singleflight
golang.org/x/crypto                # crypto extensions
github.com/bytedance/gopkg         # goroutine pool
github.com/google/uuid             # UUID generation
```

**Note**: `lang/` and `collection/` packages have zero external dependencies and use only the Go standard library.

## Development

```bash
# Run tests
go test ./...

# Test coverage
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

# Code checks
go fmt ./...
go vet ./...
```
