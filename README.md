中文 | [English](README.en.md)

# toolkit

一个生产级 Go 通用工具包，采用领域驱动设计理念。

[![Go Version](https://img.shields.io/badge/Go-%3E%3D%201.22-blue)](https://golang.org/)

## 特性

✅ **领域驱动设计** - 按功能领域组织，清晰的分层架构
✅ **生产级代码** - 经过实战验证的高质量实现
✅ **接口驱动** - 易于扩展和测试
✅ **零拷贝优化** - 高性能字符串/字节操作
✅ **完整监控** - Prometheus 指标支持
✅ **泛型支持** - Go 1.22+ 泛型实现类型安全
✅ **安全优先** - SSRF 防护（IPv6）、HMAC 恒定时间比较、AES-GCM 推荐
✅ **AI 生态** - OpenAI/Claude/Gemini 等 14+ 平台预设客户端与流式响应处理
✅ **多层缓存** - Local → Redis → DB 三层防护（防击穿/穿透/雪崩）
✅ **HTTP 连接池** - 连接复用、重试重放、限流、断路器中间件
✅ **熔断保护** - AI API 专用熔断器预设，多实例管理

## 快速开始

```bash
go get github.com/hexagon-codes/toolkit
```

### 类型转换

```go
import "github.com/hexagon-codes/toolkit/lang/conv"

str := conv.String(123)           // "123"
i := conv.Int("42")               // 42
f := conv.Float64("3.14")         // 3.14

// JSON-Map 互转
m, _ := conv.JsonToMap(`{"key":"value"}`)
json, _ := conv.MapToJson(m)
```

### 字符串工具

```go
import "github.com/hexagon-codes/toolkit/lang/stringx"

// 零拷贝转换
str := stringx.BytesToString([]byte("hello"))
bytes := stringx.String2Bytes("world")

// 大小写转换
stringx.CamelCase("hello_world")     // "helloWorld"
stringx.SnakeCase("HelloWorld")      // "hello_world"
stringx.KebabCase("helloWorld")      // "hello-world"

// 字符串操作
stringx.Truncate("hello world", 5)   // "he..."
stringx.PadLeft("42", 5, "0")        // "00042"
stringx.Reverse("hello")             // "olleh"
```

### Map 操作

```go
import "github.com/hexagon-codes/toolkit/lang/mapx"

m := map[string]int{"a": 1, "b": 2, "c": 3}

keys := mapx.Keys(m)                           // ["a", "b", "c"]
values := mapx.Values(m)                       // [1, 2, 3]
filtered := mapx.Filter(m, func(k string, v int) bool { return v > 1 })
merged := mapx.Merge(m1, m2)
inverted := mapx.Invert(m)                     // map[int]string
```

### 错误处理

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

### 时间工具

```go
import "github.com/hexagon-codes/toolkit/lang/timex"

timex.IsToday(t)                    // 是否今天
timex.IsWeekend(t)                  // 是否周末
timex.StartOfDay(t)                 // 当天 00:00:00
timex.EndOfMonth(t)                 // 月末
timex.DaysBetween(t1, t2)           // 间隔天数
timex.Age(birthday)                 // 计算年龄

// Duration 格式化
timex.FormatDuration(2*time.Hour + 30*time.Minute)  // "2h30m"
d, _ := timex.ParseDuration("1d2h30m")               // 支持天数

// 时区支持
t := timex.NowShanghai()            // 上海时间
t = timex.InShanghai(time.Now())    // 转换为上海时间
```

### 条件工具

```go
import "github.com/hexagon-codes/toolkit/lang/cond"

// If 三元表达式
result := cond.If(age >= 18, "成年", "未成年")

// IfFunc 惰性求值
result := cond.IfFunc(expensive,
    func() string { return compute() },
    func() string { return "default" },
)

// IfZero 零值判断
name := cond.IfZero(user.Name, "Anonymous")

// Coalesce 返回第一个非零值
value := cond.Coalesce(a, b, c, defaultVal)

// Switch 类型安全的 switch
result := cond.Switch[string, string](status).
    Case("pending", "等待中").
    Case("running", "运行中").
    Case("done", "已完成").
    Default("未知")
```

### 元组类型

```go
import "github.com/hexagon-codes/toolkit/lang/tuple"

// 创建元组
t2 := tuple.T2("name", 42)
t3 := tuple.T3("x", "y", "z")

// 解构
a, b := t2.Unpack()

// Swap
swapped := t2.Swap()  // Tuple2[int, string]

// Zip 合并两个切片
names := []string{"Alice", "Bob"}
ages := []int{20, 25}
pairs := tuple.Zip2(names, ages)  // []Tuple2[string, int]

// Unzip 分离
names, ages = tuple.Unzip2(pairs)
```

### Optional 类型

```go
import "github.com/hexagon-codes/toolkit/lang/optional"

// 创建 Option
opt := optional.Some(42)
empty := optional.None[int]()
fromPtr := optional.FromPtr(ptr)  // nil 指针 → None

// 检查和获取
if opt.IsSome() {
    value := opt.Unwrap()
}
value := opt.UnwrapOr(defaultVal)
value := opt.UnwrapOrElse(func() int { return compute() })

// 转换
doubled := optional.Map(opt, func(n int) int { return n * 2 })
result := optional.FlatMap(opt, func(n int) optional.Option[string] {
    return optional.Some(strconv.Itoa(n))
})

// 过滤
positive := opt.Filter(func(n int) bool { return n > 0 })
```

### Stream API

```go
import "github.com/hexagon-codes/toolkit/lang/stream"

// 创建 Stream
s := stream.Of(1, 2, 3, 4, 5)
s := stream.FromSlice(slice)
s := stream.Range(0, 100)
s := stream.Generate(10, func(i int) int { return i * 2 })

// 链式操作
result := stream.Of(1, 2, 3, 4, 5, 6, 7, 8, 9, 10).
    Filter(func(n int) bool { return n%2 == 0 }).  // 偶数
    Map(func(n int) int { return n * n }).          // 平方
    Limit(3).                                        // 取前3个
    Collect()                                        // [4, 16, 36]

// 终端操作
count := s.Count()
sum := s.Reduce(0, func(a, b int) int { return a + b })
first, ok := s.First()
any := s.Any(func(n int) bool { return n > 10 })
all := s.All(func(n int) bool { return n > 0 })

// 类型转换
strings := stream.MapTo(s, func(n int) string {
    return strconv.Itoa(n)
})

// 分组
groups := stream.GroupBy(users, func(u User) string {
    return u.Department
})
```

### 多错误聚合

```go
import "github.com/hexagon-codes/toolkit/lang/errorx"

// MultiError 收集多个错误
me := errorx.NewMultiError()
me.Append(err1).Append(err2)
if err := me.ErrorOrNil(); err != nil {
    return err
}

// 并行执行
me := errorx.Go(
    func() error { return task1() },
    func() error { return task2() },
    func() error { return task3() },
)

// 限制并发数
me := errorx.GoWithLimit(5,
    func() error { return process(item1) },
    func() error { return process(item2) },
    // ... 更多任务
)

// 遍历错误链
errorx.Walk(err, func(e error) bool {
    if myErr, ok := e.(*MyError); ok {
        handle(myErr)
        return false  // 停止遍历
    }
    return true
})
```

### 并发工具

```go
import "github.com/hexagon-codes/toolkit/lang/syncx"

// ConcurrentMap - 泛型并发安全 Map
m := syncx.NewConcurrentMap[string, int]()
m.Store("count", 1)
value, ok := m.Load("count")
m.Update("count", func(v int) int { return v + 1 })  // 原子更新
value := m.GetOrCompute("key", func() int { return expensive() })

// Singleflight - 防止缓存击穿
sf := syncx.NewSingleflight()
result, err := sf.Do("user:123", func() (any, error) {
    return db.GetUser(123)  // 多个并发请求只执行一次
})

// Semaphore - 信号量（支持 context 超时）
sem := syncx.NewSemaphore(10)  // 最多10个并发
sem.Acquire()
defer sem.Release()
sem.TryAcquire()                        // 非阻塞尝试
sem.AcquireContext(ctx)                  // 支持超时取消

// Once - 泛型版 sync.Once（可返回值）
var once syncx.Once[*Config]
cfg := once.Do(func() *Config { return loadConfig() })
val, ok := once.Value()                  // 查询是否已初始化

// OnceErr - 支持错误的 Once
var onceErr syncx.OnceErr[*DB]
db, err := onceErr.Do(func() (*DB, error) { return connectDB() })

// OnceValue / OnceFunc - 函数式包装
getConfig := syncx.OnceValue(func() *Config { return loadConfig() })
cfg1 := getConfig()                      // 首次执行
cfg2 := getConfig()                      // 返回缓存值

initOnce := syncx.OnceFunc(func() { initialize() })
initOnce()                               // 执行
initOnce()                               // 不再执行

// Lazy - 延迟初始化
config := syncx.NewLazy(func() *Config {
    return loadConfigFromFile()
})
cfg := config.Get()                      // 首次调用时初始化
config.IsInitialized()                   // 查询状态

// LazyErr - 支持错误的延迟初始化
db := syncx.NewLazyErr(func() (*DB, error) {
    return connectDB()
})
conn, err := db.Get()                    // 首次调用时初始化
conn = db.MustGet()                      // panic on error
```

### 切片增强

```go
import "github.com/hexagon-codes/toolkit/lang/slicex"

// Partition 分区
even, odd := slicex.Partition(nums, func(n int) bool {
    return n%2 == 0
})

// 聚合操作
min := slicex.Min(nums)
max := slicex.Max(nums)
sum := slicex.Sum(nums)
avg := slicex.Average(nums)

// Range 生成序列
nums := slicex.Range(0, 10, 2)  // [0, 2, 4, 6, 8]

// Shuffle 随机打乱
slicex.Shuffle(slice)
sample := slicex.Sample(slice, 5)  // 随机取5个

// Channel 转换
ch := slicex.ToChannel(slice)
slice := slicex.FromChannel(ch)
```

### Context 工具

```go
import "github.com/hexagon-codes/toolkit/lang/contextx"

// 类型安全的 context key
userKey := contextx.NewKey[User]("user")
ctx = contextx.WithValue(ctx, userKey, user)
user, ok := contextx.Value(ctx, userKey)
user = contextx.ValueOr(ctx, userKey, defaultUser)

// 常用 key 快捷方法
ctx = contextx.WithTraceID(ctx, "trace-123")
ctx = contextx.WithUserID(ctx, 12345)
traceID := contextx.TraceID(ctx)
userID := contextx.UserID(ctx)

// 状态判断
contextx.IsTimeout(ctx)             // 是否超时
contextx.IsCanceled(ctx)            // 是否取消
contextx.IsDone(ctx)                // 是否完成
contextx.Remaining(ctx)             // 剩余时间

// 运行控制
contextx.Run(ctx, func() error { ... })
contextx.RunTimeout(5*time.Second, func() error { ... })

// Detach - 脱离父 context 取消控制，保留值
detached := contextx.Detach(ctx)

// WaitGroup with Context
wg := contextx.NewWaitGroupContext(ctx)
wg.Go(func(ctx context.Context) error { ... })
wg.Wait()

// 协程池
pool := contextx.NewPool(ctx, 10)
pool.Go(func(ctx context.Context) error { ... })
pool.Wait()
```

### AES 加密

```go
import "github.com/hexagon-codes/toolkit/crypto/aes"

key, _ := aes.GenerateKey(32)  // AES-256

// GCM 模式（推荐）
ciphertext, _ := aes.EncryptGCM(plaintext, key)
plaintext, _ := aes.DecryptGCM(ciphertext, key)

// 字符串加解密
encrypted, _ := aes.EncryptGCMString("secret", "32-byte-key-here")
decrypted, _ := aes.DecryptGCMString(encrypted, "32-byte-key-here")
```

### RSA 加密

```go
import "github.com/hexagon-codes/toolkit/crypto/rsa"

kp, _ := rsa.GenerateKeyPair(2048)

// 加解密
ciphertext, _ := kp.Encrypt(plaintext)
plaintext, _ := kp.Decrypt(ciphertext)

// 签名验签
signature, _ := kp.Sign(message)
err := kp.Verify(message, signature)

// PEM 导出
privatePEM := kp.PrivateKeyToPEM()
publicPEM := kp.PublicKeyToPEM()
```

### HMAC 签名

```go
import "github.com/hexagon-codes/toolkit/crypto/sign"

sig := sign.HMACSHA256String("message", "secret-key")
ok := sign.VerifyHMACSHA256String("message", "secret-key", sig)

// API 签名
signer := sign.NewAPISigner("app-key", "app-secret")
sig := signer.Sign(params, timestamp, nonce)
```

### HTTP 客户端

```go
import "github.com/hexagon-codes/toolkit/net/httpx"

// 简单请求
resp, _ := httpx.Get("https://api.example.com/users")
resp, _ := httpx.Post("https://api.example.com/users", body)

// 链式调用
client := httpx.NewClient(
    httpx.WithTimeout(10*time.Second),
    httpx.WithRetry(3, time.Second),
)
resp, _ := client.R().
    SetHeader("Authorization", "Bearer token").
    SetQuery("page", "1").
    Get("/api/users")

// 解析响应
var users []User
resp.JSON(&users)

// SSRF 防护（阻止访问内网地址，支持 IPv6 白名单）
client := httpx.NewClient(
    httpx.WithSSRFProtection("api.trusted.com", "[::1]:8080"),
)
resp, err := client.R().Get(userProvidedURL)
if errors.Is(err, httpx.ErrSSRFBlocked) {
    // 请求被拦截
}
```

### HTTP 连接池

```go
import "github.com/hexagon-codes/toolkit/net/httpx"

// 创建连接池
pool := httpx.NewPool(httpx.PoolConfig{
    MaxIdleConns:    100,
    MaxConnsPerHost: 10,
    IdleConnTimeout: 90 * time.Second,
})
defer pool.Close()

// 执行请求
req, _ := http.NewRequest("GET", "https://api.example.com", nil)
resp, _ := pool.Do(req)

// 查看统计信息
stats := pool.GetStats()
fmt.Printf("总请求: %d, 活跃: %d, 错误: %d\n",
    stats.TotalRequests, stats.ActiveRequests, stats.ErrorCount)

// 全局连接池
httpx.SetGlobalPool(pool)
p := httpx.GlobalPool()

// 主机级连接池（自动按主机分配独立连接池）
hostPool := httpx.NewHostPool()
defer hostPool.Close()
hostPool.SetHostConfig("api.example.com", httpx.PoolConfig{MaxConnsPerHost: 20})
resp, _ = hostPool.Do(req)

// 带重试的连接池（自动缓存 Body 支持重放）
retryPool := httpx.NewRetryPool(pool, httpx.RetryConfig{
    MaxRetries:   3,
    RetryWait:    100 * time.Millisecond,
    MaxRetryWait: 5 * time.Second,
    RetryCondition: func(resp *http.Response, err error) bool {
        return err != nil || resp.StatusCode >= 500
    },
})

// 带限流的连接池
rateLimitedPool := httpx.NewRateLimitedPool(pool, 100)  // 100 QPS
defer rateLimitedPool.Close()

// 带断路器的连接池
cbPool := httpx.NewCircuitBreakerPool(pool, httpx.CircuitBreakerConfig{
    FailureThreshold: 5,
    SuccessThreshold: 2,
    Timeout:          30 * time.Second,
})
```

### AI 客户端预设

```go
import "github.com/hexagon-codes/toolkit/net/httpx"

// 各大 AI 平台预设客户端（自动配置 BaseURL、认证头、超时等）
openai := httpx.OpenAIClient("sk-xxx")
claude := httpx.ClaudeClient("sk-ant-xxx")
gemini := httpx.GeminiClient("AIza-xxx")
deepseek := httpx.DeepSeekClient("sk-xxx")
qwen := httpx.QwenClient("sk-xxx")           // 通义千问
zhipu := httpx.ZhipuClient("xxx.xxx")        // 智谱清言
moonshot := httpx.MoonshotClient("sk-xxx")    // 月之暗面
doubao := httpx.DoubaoClient("xxx")           // 字节豆包

// 自定义 AI 客户端
custom := httpx.CustomAIClient("https://my-api.com", "my-token")

// 流式请求
stream, _ := claude.R().
    SetJSONBody(requestBody).
    PostStream("/v1/messages")
defer stream.Close()

// 读取 SSE 事件
for {
    event, err := stream.ReadSSE()
    if err != nil { break }
    fmt.Println(event.Data)
}

// 读取 OpenAI 格式流式 JSON
var chunk httpx.OpenAIStreamChunk
for {
    err := stream.ReadJSON(&chunk)
    if err != nil { break }
    fmt.Print(chunk.Choices[0].Delta.Content)
}

// 一行收集所有内容
content, _ := stream.CollectOpenAIContent()
```

### SSE 服务端推送

```go
import "github.com/hexagon-codes/toolkit/net/sse"

// 客户端 - 接收 SSE 事件
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

// 服务端 - 发送 SSE 事件
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

// OpenAI 流式响应处理
sse.ReadOpenAIStream(resp.Body, func(chunk ChatCompletion) error {
    fmt.Print(chunk.Choices[0].Delta.Content)
    return nil
})
```

### 熔断器

```go
import "github.com/hexagon-codes/toolkit/util/circuit"

// 基本使用
breaker := circuit.New(
    circuit.WithThreshold(5),           // 5次失败后熔断
    circuit.WithTimeout(30*time.Second), // 熔断持续30秒
    circuit.WithHalfOpenMaxRequests(3), // 半开状态最多3个探测请求
    circuit.WithSuccessThreshold(2),    // 2次成功后恢复
)

result, err := breaker.Execute(func() (any, error) {
    return callAPI()
})

// AI API 专用熔断器（内置预设配置）
openaiBreaker := circuit.NewAIBreaker(circuit.OpenAIConfig)
claudeBreaker := circuit.NewAIBreaker(circuit.ClaudeConfig)
geminiBreaker := circuit.NewAIBreaker(circuit.GeminiConfig)

// 预设风格
aggressiveBreaker := circuit.NewAIBreaker(circuit.AggressiveConfig)       // 快速熔断
conservativeBreaker := circuit.NewAIBreaker(circuit.ConservativeConfig)   // 慢速熔断

// 自定义错误判断
breaker = circuit.New(
    circuit.WithIsFailure(circuit.IsRateLimitOrServerError),  // 仅 429/5xx 触发
)

// 多熔断器管理（按名称隔离）
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

// 状态监听
breaker.OnStateChange(func(from, to circuit.State) {
    log.Printf("熔断器状态: %s -> %s", from, to)
})
```

### 事件总线

```go
import "github.com/hexagon-codes/toolkit/event"

// 创建事件总线
bus := event.New()
defer bus.Close()

// 订阅指定类型事件
unsub := bus.Subscribe("agent.start", func(e event.Event) {
    fmt.Printf("Agent 启动: %v (来源: %s)\n", e.Payload, e.Source)
})
defer unsub()  // 取消订阅

// 订阅所有事件（全局订阅）
unsubAll := bus.SubscribeAll(func(e event.Event) {
    fmt.Printf("[%s] %v\n", e.Type, e.Payload)
})
defer unsubAll()

// 发布事件
bus.Publish(event.Event{
    Type:    "agent.start",
    Payload: "my-agent",
    Source:  "scheduler",
})

// 预定义事件类型常量
bus.Publish(event.Event{Type: event.EventLLMRequest,  Payload: req})
bus.Publish(event.Event{Type: event.EventLLMResponse, Payload: resp})
bus.Publish(event.Event{Type: event.EventToolCall,    Payload: toolName})
bus.Publish(event.Event{Type: event.EventCostUpdate,  Payload: cost})
bus.Publish(event.Event{Type: event.EventAgentError,  Payload: err})

// 配置选项
bus = event.New(
    event.WithMaxGoroutines(512),              // 限制并发 goroutine 数
    event.WithPanicHandler(func(e event.Event, v any) {
        log.Printf("handler panic: %v", v)     // 捕获 handler panic
    }),
)

// 订阅数量统计
count := bus.SubscriberCount("agent.start")
```

### IP 工具

```go
import "github.com/hexagon-codes/toolkit/net/ip"

ip.IsValid("192.168.1.1")           // true
ip.IsPrivate("192.168.1.1")         // true
ip.IsIPv4("192.168.1.1")            // true
ip.IsInCIDR("192.168.1.100", "192.168.1.0/24")  // true

// 从 HTTP 请求获取客户端 IP
clientIP := ip.FromRequest(r)

// 本机 IP
localIP, _ := ip.GetLocalIP()
```

### 日志

```go
import "github.com/hexagon-codes/toolkit/util/logger"

// 快速使用
logger.Info("user login", "userId", 123, "ip", "192.168.1.1")
logger.Error("request failed", "error", err)

// 配置
logger.Init(&logger.Config{
    Level:  "info",
    Format: "json",
    Output: "stdout",
})

// 带字段
log := logger.With("service", "user-api")
log.Info("started", "port", 8080)
```

### 环境变量

```go
import "github.com/hexagon-codes/toolkit/util/env"

port := env.GetIntDefault("PORT", 8080)
debug := env.GetBool("DEBUG")
hosts := env.GetSlice("HOSTS")  // 逗号分隔

if env.IsProd() {
    // 生产环境
}
```

### 编码工具

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

### 反射工具

```go
import "github.com/hexagon-codes/toolkit/util/reflectx"

// Struct ↔ Map 转换
user := User{Name: "Alice", Age: 20}
m := reflectx.StructToMap(user)                    // map[string]any{"Name": "Alice", "Age": 20}
m = reflectx.StructToMapWithTag(user, "json")      // 使用 json tag 作为 key

var user2 User
reflectx.MapToStruct(m, &user2)

// 字段操作
name, _ := reflectx.GetField(user, "Name")
reflectx.SetField(&user, "Name", "Bob")
reflectx.HasField(user, "Name")                    // true
names := reflectx.FieldNames(user)                 // ["Name", "Age"]

// 深拷贝（支持循环引用检测，nil 安全）
copied := reflectx.DeepCopy(original)              // 递归深拷贝
shallow := reflectx.Clone(original)                // 浅拷贝

// 类型检查
reflectx.IsZero(value)
reflectx.IsNil(value)
reflectx.TypeName(value)                           // "User"
reflectx.IsPtr(value)
reflectx.IsStruct(value)
reflectx.IsSlice(value)
```

### 结构体验证

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

// 验证
v := validator.New()
if err := v.Struct(user); err != nil {
    for _, e := range err.(validator.ValidationErrors) {
        fmt.Printf("字段 %s 验证失败: %s\n", e.Field, e.Tag)
    }
}

// 支持的标签
// required  - 必填
// email     - 邮箱格式
// url       - URL 格式
// min=n     - 最小值/最小长度
// max=n     - 最大值/最大长度
// len=n     - 精确长度
// oneof=a b - 枚举值
// regexp=x  - 正则匹配
// omitempty - 空值时跳过

// 自定义验证规则
v.RegisterRule("phone", func(value any) bool {
    return validator.Phone(value.(string))
})
```

### Poolx 协程池

```go
import "github.com/hexagon-codes/toolkit/util/poolx"

// 创建协程池
p := poolx.New("my-pool", poolx.WithMaxWorkers(10))
defer p.Release()

p.Submit(func() {
    // task
})

// Future 模式
future := poolx.SubmitFunc(p, func() (int, error) {
    return compute(), nil
})
result, err := future.Get()

// 并行 Map
results, _ := poolx.Map(ctx, items, 4, func(item T) (R, error) {
    return process(item), nil
})
```

### 配置管理

```go
import "github.com/hexagon-codes/toolkit/util/config"

// 从文件加载（支持 JSON/YAML/TOML/ENV）
cfg, _ := config.Load("config.yaml")

// 获取配置值
name := cfg.GetString("app.name")
port := cfg.GetIntDefault("app.port", 8080)
debug := cfg.GetBool("app.debug")
timeout := cfg.GetDuration("app.timeout")
hosts := cfg.GetStringSlice("app.hosts")

// 从环境变量加载
cfg.LoadEnv("APP")  // APP_NAME -> name, APP_PORT -> port

// 绑定到结构体
var appCfg struct {
    Name    string        `env:"NAME" default:"myapp"`
    Port    int           `env:"PORT" default:"8080"`
    Debug   bool          `env:"DEBUG"`
    Timeout time.Duration `env:"TIMEOUT" default:"30s"`
}
config.BindEnv(&appCfg, "APP")

// 全局配置
config.LoadGlobal("config.yaml")
config.GetString("key")
config.Set("key", "value")
```

### List 双向链表

```go
import "github.com/hexagon-codes/toolkit/collection/list"

// 创建链表
l := list.New(1, 2, 3)
l.PushFront(0)                    // 头部插入
l.PushBack(4)                     // 尾部插入

// 访问元素
front := l.Front()                // 头节点
back := l.Back()                  // 尾节点
next := front.Next()              // 下一个节点
prev := back.Prev()               // 上一个节点

// 移除元素
val, ok := l.PopFront()           // 头部移除
val, ok = l.PopBack()             // 尾部移除
l.Remove(node)                    // 移除指定节点

// 移动节点
l.MoveToFront(node)               // 移到头部
l.MoveToBack(node)                // 移到尾部
l.MoveBefore(node, mark)          // 移到 mark 之前
l.MoveAfter(node, mark)           // 移到 mark 之后

// 查找和遍历
l.Find(func(v int) bool { return v > 2 })
l.ForEach(func(v int) { fmt.Println(v) })
l.ForEachReverse(func(v int) { fmt.Println(v) })

// 其他操作
l.Reverse()                       // 反转链表
l.Clone()                         // 克隆
l.Filter(func(v int) bool { return v%2 == 0 })

// 线程安全版本
sl := list.NewSyncList[int]()
```

### Stack 栈

```go
import "github.com/hexagon-codes/toolkit/collection/stack"

// 创建栈
s := stack.New(1, 2, 3)
s.Push(4, 5)                      // 入栈

// 出栈操作
top, ok := s.Pop()                // 出栈（返回 5）
top, ok = s.Peek()                // 查看栈顶（不移除）

// 批量操作
items := s.PopN(3)                // 出栈 N 个元素
items = s.PeekN(3)                // 查看栈顶 N 个元素

// 遍历
s.ForEach(func(v int) { ... })          // 从栈底到栈顶
s.ForEachReverse(func(v int) { ... })   // 从栈顶到栈底

// 其他操作
s.Reverse()                       // 反转栈
s.Clone()                         // 克隆
s.Contains(func(v int) bool { return v == 3 })

// 线程安全版本
ss := stack.NewSyncStack[int]()
```

### Queue 队列

```go
import "github.com/hexagon-codes/toolkit/collection/queue"

// FIFO 队列
q := queue.New(1, 2, 3)
q.Enqueue(4, 5)
item, ok := q.Dequeue()           // 1, true
front, _ := q.Peek()              // 2

// 双端队列
dq := queue.NewDeque[int]()
dq.PushFront(1)
dq.PushBack(2)
dq.PopFront()                     // 1
dq.PopBack()                      // 2

// 优先级队列（最小堆）
pq := queue.NewMinHeap[int]()
pq.Push(5, 3, 1, 4, 2)
pq.Pop()                          // 1
pq.Pop()                          // 2

// 最大堆
maxPQ := queue.NewMaxHeap[int]()
maxPQ.Push(1, 5, 3)
maxPQ.Pop()                       // 5

// 自定义优先级
type Task struct {
    Name     string
    Priority int
}
taskPQ := queue.NewPriorityQueue[Task](func(a, b Task) bool {
    return a.Priority > b.Priority  // 优先级高的先出
})

// 线程安全版本
sq := queue.NewSyncQueue[int]()
sd := queue.NewSyncDeque[int]()
```

### Set 集合

```go
import "github.com/hexagon-codes/toolkit/collection/set"

// 创建 Set
s := set.New(1, 2, 3)
s.Add(4, 5)
s.Remove(1)

// 基本操作
s.Contains(2)              // true
s.Size()                   // 4
s.IsEmpty()                // false
s.ToSlice()                // [2, 3, 4, 5]

// 集合运算
s1 := set.New(1, 2, 3)
s2 := set.New(2, 3, 4)

union := s1.Union(s2)                    // {1, 2, 3, 4}
intersection := s1.Intersection(s2)      // {2, 3}
difference := s1.Difference(s2)          // {1}
symDiff := s1.SymmetricDifference(s2)    // {1, 4}

// 判断关系
s1.IsSubset(s2)            // false
s1.IsSuperset(s2)          // false
s1.IsDisjoint(s2)          // false
s1.Equal(s2)               // false

// 函数式操作
even := s.Filter(func(n int) bool { return n%2 == 0 })
s.ForEach(func(n int) { fmt.Println(n) })
s.Any(func(n int) bool { return n > 10 })
s.All(func(n int) bool { return n > 0 })
```

## 项目结构

```
toolkit/
├── event/              # 事件总线（发布-订阅，线程安全）
│
├── ai/                 # AI 工具
│   ├── streamx/       # 流式响应处理（OpenAI/Claude/Gemini）
│   ├── tokenizer/     # Token 计数
│   ├── template/      # Prompt 模板
│   └── meter/         # 用量计量
│
├── cache/              # 缓存
│   ├── local/         # 本地缓存（LRU）
│   ├── redis/         # Redis 缓存
│   └── multi/         # 多层缓存（防击穿/穿透/雪崩）
│
├── collection/         # 数据结构（零外部依赖）
│   ├── list/          # 双向链表
│   ├── queue/         # 队列（FIFO/双端/优先级）
│   ├── set/           # 泛型 HashSet
│   └── stack/         # 栈（LIFO）
│
├── crypto/             # 加密工具
│   ├── aes/           # AES 加密（推荐 GCM）
│   ├── rsa/           # RSA 非对称加密
│   └── sign/          # HMAC 签名验签
│
├── infra/              # 基础设施
│   ├── db/            # 数据库
│   │   ├── mysql/
│   │   ├── redis/
│   │   ├── mongodb/
│   │   ├── clickhouse/
│   │   └── elasticsearch/
│   ├── queue/         # 消息队列
│   │   └── asynq/
│   ├── observe/       # 可观测性
│   ├── otel/          # OpenTelemetry
│   └── prometheus/    # Prometheus 指标
│
├── lang/               # 语言增强（零外部依赖）
│   ├── cond/          # 条件工具（If/Switch/Coalesce）
│   ├── contextx/      # Context 工具
│   ├── conv/          # 类型转换
│   ├── errorx/        # 错误处理（MultiError/Walk）
│   ├── mapx/          # Map 工具（泛型）
│   ├── mathx/         # 数学工具（泛型）
│   ├── optional/      # Option 类型
│   ├── slicex/        # 切片工具（泛型）
│   ├── stream/        # Stream API
│   ├── stringx/       # 字符串扩展
│   ├── syncx/         # 并发工具（ConcurrentMap/Semaphore/Once/Lazy/Pool）
│   ├── timex/         # 时间工具
│   └── tuple/         # 元组类型（Tuple2/3/4）
│
├── net/                # 网络工具
│   ├── httpx/         # HTTP 客户端（SSRF 防护/连接池/重试/限流/AI 预设）
│   ├── ip/            # IP 工具
│   └── sse/           # Server-Sent Events
│
├── util/               # 工具组件
│   ├── circuit/       # 熔断器（AI 预设/多实例管理）
│   ├── config/        # 配置管理
│   ├── encoding/      # 编码（Base64/Hex/URL）
│   ├── env/           # 环境变量
│   ├── file/          # 文件操作
│   ├── hash/          # 哈希（MD5/SHA/Bcrypt）
│   ├── idgen/         # ID 生成（Snowflake）
│   ├── json/          # JSON 辅助
│   ├── logger/        # 日志（基于 slog）
│   ├── pagination/    # 分页
│   ├── poolx/         # 高性能协程池
│   ├── rand/          # 随机数
│   ├── rate/          # 限流器
│   ├── reflectx/      # 反射工具（DeepCopy/Clone/StructToMap）
│   ├── retry/         # 重试机制
│   ├── slice/         # 切片工具
│   └── validator/     # 数据验证（含结构体标签）
│
└── examples/           # 使用示例
```

## 测试覆盖率

| 包 | 覆盖率 |
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

## 设计哲学

### 1. 领域驱动组织

代码按功能领域分组，而非技术类型：

```
❌ 不推荐：util/string.go, util/time.go
✅ 推荐：lang/stringx/, lang/timex/
```

### 2. 清晰的分层架构

```
ai (AI工具) → infra (基础设施) → net (网络) → cache (缓存)
     ↓              ↓                ↓            ↓
  外部依赖       外部服务         可能依赖     可独立

     ↓              ↓                ↓            ↓
crypto (加密) → util (工具) → collection (数据结构) → lang (零依赖)
     ↓              ↓                ↓                    ↓
  x/crypto      可能依赖         纯标准库              纯标准库
```

**关键约束**: `lang/` 和 `collection/` 包必须保持零外部依赖。

### 3. 安全优先

- AES-CBC/CTR 标记为 Deprecated，推荐 GCM
- HMAC 验证使用恒定时间比较
- PKCS7 填充验证防止时序攻击
- HTTP 客户端内置 SSRF 防护
- 签名验证支持时间戳过期和 nonce 防重放

### 4. 性能优化

- 零拷贝字符串操作（unsafe）
- 对象池和缓存复用
- 最小化反射使用
- Singleflight 防止缓存击穿

### 5. 泛型优先

所有集合和工具函数优先使用泛型实现类型安全。

## 依赖

核心依赖：
```
github.com/hibiken/asynq           # 任务队列
github.com/redis/go-redis/v9       # Redis 客户端
github.com/prometheus/client_golang # 监控指标
golang.org/x/sync                  # singleflight
golang.org/x/crypto                # 加密扩展
github.com/bytedance/gopkg         # goroutine 池
github.com/google/uuid             # UUID 生成
```

**注意**：`lang/` 和 `collection/` 包零外部依赖，只使用 Go 标准库。

## 开发

```bash
# 运行测试
go test ./...

# 测试覆盖率
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

# 代码检查
go fmt ./...
go vet ./...
```
