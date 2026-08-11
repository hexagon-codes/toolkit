中文 | [English](README.en.md)

# toolkit

一个生产级 Go 通用工具包，采用领域驱动设计理念。

[![Go Version](https://img.shields.io/badge/Go-%3E%3D%201.25.12-blue)](https://go.dev/)

## 特性

✅ **领域驱动设计** - 按功能领域组织，清晰的分层架构
✅ **生产级代码** - 经过实战验证的高质量实现
✅ **接口驱动** - 易于扩展和测试
✅ **零拷贝优化** - 高性能字符串/字节操作
✅ **完整监控** - Prometheus 指标支持
✅ **泛型支持** - Go 泛型实现类型安全
✅ **安全优先** - SSRF 防护（IPv6）、HMAC 恒定时间比较、AES-GCM 推荐
✅ **AI 生态** - OpenAI/Claude/Gemini 等 14+ 平台预设客户端与流式响应处理
✅ **多层缓存** - Local → Redis → DB 三层防护（防击穿/穿透/雪崩）
✅ **HTTP 连接池** - 连接复用、重试重放、限流、断路器中间件
✅ **熔断保护** - AI API 专用熔断器预设，多实例管理

## 快速开始

```bash
go get github.com/hexagon-codes/toolkit@v0.3.0
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

// 安全转换（结果不共享可变底层内存）
str := stringx.BytesToString([]byte("hello"))
bytes := stringx.StringToBytes("world")

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
me.Append(err1, err2)
if err := me.ErrorOrNil(); err != nil {
    return err
}

// 使用默认上限并行执行
err := errorx.Go(
    func() error { return task1() },
    func() error { return task2() },
    func() error { return task3() },
)
if err != nil {
    return err
}

// 限制并发数
err = errorx.GoWithLimit(5,
    func() error { return process(item1) },
    func() error { return process(item2) },
    // ... 更多任务
)
if err != nil {
    return err
}

// 安全启动 goroutine，并等待唯一一次完成结果
if err := <-errorx.SafeGo(func() { runTask() }); err != nil {
    return err
}

// Result 在失败时计算替代值；回调本身也会被校验
fallback, err := errorx.Err[int](loadErr).UnwrapOrElse(func(err error) int {
    log.Printf("load failed: %v", err)
    return 0
})
if err != nil {
    return err
}

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

// 运行控制：任务通过传入的 context 协作响应取消
if err := contextx.Run(ctx, func(taskCtx context.Context) error {
    return runTask(taskCtx)
}); err != nil {
    return err
}
if err := contextx.RunTimeout(ctx, 5*time.Second, func(taskCtx context.Context) error {
    return runTask(taskCtx)
}); err != nil {
    return err
}

// Detach - 脱离父 context 取消控制，保留值
detached := contextx.Detach(ctx)

// WaitGroup with Context
wg := contextx.NewWaitGroupContext(ctx)
wg.Go(func(ctx context.Context) error { ... })
wg.Wait()

// 协程池
pool, err := contextx.NewPool(ctx, 10)
if err != nil {
    return err
}
defer pool.Close()
pool.Go(func(ctx context.Context) error { ... })
if err := pool.Wait(); err != nil {
    return err
}
```

### 多层缓存

```go
import "github.com/hexagon-codes/toolkit/cache/multi"

cache, err := multi.NewCache([]multi.LayerConfig{
    {Layer: localCache, TTL: 10 * time.Minute, Name: "local"},
    {Layer: redisCache, TTL: 60 * time.Minute, Name: "redis"},
})
if err != nil {
    return err
}

builtCache, err := multi.NewBuilder().
    WithLocal(localCache, 10*time.Minute).
    WithRedis(redisCache, 60*time.Minute).
    Build()
if err != nil {
    return err
}
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
if err != nil {
    log.Fatal(err)
}

// PEM 导出
privatePEM, err := kp.PrivateKeyToPEM()
if err != nil {
    log.Fatal(err)
}
publicPEM, err := kp.PublicKeyToPEM()
if err != nil {
    log.Fatal(err)
}
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
resp, err := httpx.Get(ctx, "https://api.example.com/users")
if err != nil {
    log.Fatal(err)
}
resp, err = httpx.Post(ctx, "https://api.example.com/users", body)
if err != nil {
    log.Fatal(err)
}

// 链式调用
client, err := httpx.NewClient(
    httpx.WithTimeout(10*time.Second),
    httpx.WithRetry(3, time.Second),
)
if err != nil {
    log.Fatal(err)
}
resp, err = client.R(ctx).
    SetHeader("Authorization", "Bearer token").
    SetQuery("page", "1").
    Get("/api/users")
if err != nil {
    log.Fatal(err)
}

// 解析响应
var users []User
if err := resp.JSON(&users); err != nil {
    log.Fatal(err)
}

// SSRF 防护（阻止访问内网地址，支持 IPv6 白名单）
secureClient, err := httpx.NewClient(
    httpx.WithSSRFProtection("api.trusted.com", "[::1]:8080"),
)
if err != nil {
    log.Fatal(err)
}
resp, err = secureClient.R(ctx).Get(userProvidedURL)
if errors.Is(err, httpx.ErrSSRFBlocked) {
    // 请求被拦截
} else if err != nil {
    log.Fatal(err)
}
```

### HTTP 连接池

```go
import "github.com/hexagon-codes/toolkit/net/httpx"

// 创建连接池
poolConfig := httpx.DefaultPoolConfig()
poolConfig.MaxConnsPerHost = 20
pool, err := httpx.NewPool(poolConfig)
if err != nil {
    log.Fatal(err)
}
defer pool.Close()

// 执行请求
req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com", http.NoBody)
resp, _ := pool.Do(req)

// 查看统计信息
stats := pool.GetStats()
fmt.Printf("总请求: %d, 活跃: %d, 错误: %d\n",
    stats.TotalRequests, stats.ActiveRequests, stats.ErrorCount)

// 全局连接池
if err := httpx.SetGlobalPool(pool); err != nil {
    log.Fatal(err)
}
p := httpx.GlobalPool()

// 有界主机级连接池（自动按主机分配独立连接池）
hostPoolConfig := httpx.DefaultHostPoolConfig()
hostPoolConfig.MaxHosts = 128
hostPool, err := httpx.NewHostPool(hostPoolConfig)
if err != nil {
    log.Fatal(err)
}
defer hostPool.Close()
hostConfig := httpx.DefaultPoolConfig()
hostConfig.MaxConnsPerHost = 20
if err := hostPool.SetHostConfig("api.example.com", hostConfig); err != nil {
    log.Fatal(err)
}
resp, _ = hostPool.Do(req)
// 移除主机时关闭对应连接池并释放容量
if err := hostPool.RemoveHost("api.example.com"); err != nil {
    log.Fatal(err)
}

// 带重试的连接池（带 Body 的请求必须提供 GetBody）
retryPool, err := httpx.NewRetryPool(pool, httpx.RetryConfig{
    MaxRetries:   3,
    RetryWait:    100 * time.Millisecond,
    MaxRetryWait: 5 * time.Second,
    RetryCondition: func(resp *http.Response, err error) bool {
        return err != nil || resp.StatusCode >= 500
    },
})
if err != nil {
    log.Fatal(err)
}

// 带限流的连接池
rateLimitedPool, err := httpx.NewRateLimitedPool(pool, 100)  // 100 QPS
if err != nil {
    log.Fatal(err)
}
defer rateLimitedPool.Close()

// 带熔断器的连接池，复用 util/circuit 状态机
cbPool, err := httpx.NewCircuitBreakerPool(
    pool,
    circuit.WithThreshold(5),
    circuit.WithSuccessThreshold(2),
    circuit.WithTimeout(30*time.Second),
)
if err != nil {
    log.Fatal(err)
}
```

### AI 客户端预设

```go
import "github.com/hexagon-codes/toolkit/net/httpx"

// 各大 AI 平台预设客户端（自动配置 BaseURL、认证头、超时等）
openai, err := httpx.OpenAIClient("sk-xxx")
if err != nil {
    log.Fatal(err)
}
claude, err := httpx.ClaudeClient("sk-ant-xxx")
if err != nil {
    log.Fatal(err)
}
gemini, err := httpx.GeminiClient("AIza-xxx")
if err != nil {
    log.Fatal(err)
}
deepseek, err := httpx.DeepSeekClient("sk-xxx")
if err != nil {
    log.Fatal(err)
}
qwen, err := httpx.QwenClient("sk-xxx") // 通义千问
if err != nil {
    log.Fatal(err)
}
zhipu, err := httpx.ZhipuClient("xxx.xxx") // 智谱清言
if err != nil {
    log.Fatal(err)
}
moonshot, err := httpx.MoonshotClient("sk-xxx") // 月之暗面
if err != nil {
    log.Fatal(err)
}
doubao, err := httpx.DoubaoClient("xxx") // 字节豆包
if err != nil {
    log.Fatal(err)
}

// 自定义 AI 客户端
custom, err := httpx.CustomAIClient("https://my-api.com", "my-token")
if err != nil {
    log.Fatal(err)
}

// 流式请求
stream, err := claude.R(ctx).
    SetJSONBody(requestBody).
    PostStream("/v1/messages")
if err != nil {
    log.Fatal(err)
}
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
client, err := sse.NewClient("https://api.example.com/events",
    sse.WithTimeout(30*time.Second),
    sse.WithLastEventID("last-id"),
)
if err != nil {
    log.Fatal(err)
}
stream, err := client.Connect(ctx)
if err != nil {
    log.Fatal(err)
}
defer stream.Close()

for event := range stream.Events() {
    fmt.Printf("Event: %s, Data: %s\n", event.Event, event.Data)
    var data MyData
    event.JSON(&data)
}

// 服务端 - 发送 SSE 事件
func handler(w http.ResponseWriter, r *http.Request) {
    writer, err := sse.NewWriter(w)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    defer writer.Close()

    for {
        if err := writer.Write(&sse.Event{
            ID:    "1",
            Event: "message",
            Data:  "Hello, World!",
        }); err != nil {
            return
        }
        if err := writer.WriteJSON(myData); err != nil {
            return
        }
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
breaker, err := circuit.New(
    circuit.WithThreshold(5),           // 5次失败后熔断
    circuit.WithTimeout(30*time.Second), // 熔断持续30秒
    circuit.WithHalfOpenMaxRequests(3), // 半开状态最多3个探测请求
    circuit.WithSuccessThreshold(2),    // 2次成功后恢复
)

result, err := breaker.Execute(func() (any, error) {
    return callAPI()
})

// AI API 专用熔断器（内置预设配置）
openaiBreaker, err := circuit.NewAIBreaker(circuit.OpenAIConfig())
claudeBreaker, err := circuit.NewAIBreaker(circuit.ClaudeConfig())
geminiBreaker, err := circuit.NewAIBreaker(circuit.GeminiConfig())

// 预设风格
aggressiveBreaker, err := circuit.NewAIBreaker(circuit.AggressiveConfig())       // 快速熔断
conservativeBreaker, err := circuit.NewAIBreaker(circuit.ConservativeConfig())   // 慢速熔断

// 自定义错误判断
breaker, err = circuit.New(
    circuit.WithIsFailure(circuit.IsRateLimitOrServerError),  // 仅 429/5xx 触发
)

// 多熔断器管理（按名称隔离）
manager, err := circuit.NewBreakerManager(circuit.OpenAIConfig()...)
result, err = manager.Execute("gpt-4", func() (any, error) {
    return callGPT4()
})
manager.Execute("claude", func() (any, error) {
    return callClaude()
})
states := manager.States()  // map[string]State

// 状态监听
if err := breaker.OnStateChange(func(from, to circuit.State) {
    log.Printf("熔断器状态: %s -> %s", from, to)
}); err != nil {
    log.Fatal(err)
}
```

### 事件总线

```go
import "github.com/hexagon-codes/toolkit/event"

// 创建事件总线
bus, err := event.New()
if err != nil {
    log.Fatal(err)
}
defer func() {
    if err := bus.Shutdown(ctx); err != nil {
        log.Print(err)
    }
}()

// 订阅指定类型事件
unsub, err := bus.Subscribe("agent.start", func(e event.Event) {
    fmt.Printf("Agent 启动: %v (来源: %s)\n", e.Payload, e.Source)
})
if err != nil {
    log.Fatal(err)
}
defer unsub()  // 取消订阅

// 订阅所有事件（全局订阅）
unsubAll, err := bus.SubscribeAll(func(e event.Event) {
    fmt.Printf("[%s] %v\n", e.Type, e.Payload)
})
if err != nil {
    log.Fatal(err)
}
defer unsubAll()

// 发布事件
if err := bus.Publish(event.Event{
    Type:    "agent.start",
    Payload: "my-agent",
    Source:  "scheduler",
}); err != nil {
    log.Fatal(err)
}

// 同步发布在当前 goroutine 中执行处理器，完成后返回
if err := bus.PublishSync(event.Event{Type: "agent.ready"}); err != nil {
    log.Fatal(err)
}

// 预定义事件类型常量
for _, evt := range []event.Event{
    {Type: event.EventLLMRequest, Payload: req},
    {Type: event.EventLLMResponse, Payload: resp},
    {Type: event.EventToolCall, Payload: toolName},
    {Type: event.EventCostUpdate, Payload: cost},
    {Type: event.EventAgentError, Payload: callErr},
} {
    if err := bus.Publish(evt); err != nil {
        log.Fatal(err)
    }
}

// 配置选项
configuredBus, err := event.New(
    event.WithMaxGoroutines(512),              // 限制并发 goroutine 数
    event.WithPanicHandler(func(e event.Event, v any) {
        log.Printf("handler panic: %v", v)     // 捕获 handler panic
    }),
)
if err != nil {
    log.Fatal(err)
}
configuredBus.Close() // 仅启动关闭流程，不等待正在执行的处理器
if err := configuredBus.Shutdown(ctx); err != nil {
    log.Print(err)
}

// 订阅数量统计
count := bus.Len()
```

### IP 工具

```go
import (
    "context"

    "github.com/hexagon-codes/toolkit/net/ip"
)

ip.IsValid("192.168.1.1")           // true
ip.IsPrivate("192.168.1.1")         // true
ip.IsIPv4("192.168.1.1")            // true
ip.IsInCIDR("192.168.1.100", "192.168.1.0/24")  // true

// 从 HTTP 请求获取客户端 IP
clientIP := ip.FromRequest(r)

// 本机 IP
localIP, _ := ip.GetLocalIP(context.Background())
```

### 日志

```go
import "github.com/hexagon-codes/toolkit/util/logger"

// 快速使用
logger.Info("user login", "userId", 123, "ip", "192.168.1.1")
logger.Error("request failed", "error", err)

// 配置
if err := logger.Init(&logger.Config{
    Level:  "info",
    Format: "json",
    Output: "stdout",
}); err != nil {
    log.Fatal(err)
}

// 带字段
serviceLog := logger.With("service", "user-api")
serviceLog.Info("started", "port", 8080)
```

### OpenTelemetry

```go
import "github.com/hexagon-codes/toolkit/infra/otel"

tracer := otel.NewTracer(otel.WithServiceName("agent-service"))
exporter, err := otel.NewOTLPExporter("http://localhost:4318")
if err != nil {
    return err
}

// SetExporter 接管 exporter 所有权；替换时会关闭旧 exporter
if err := tracer.SetExporter(ctx, exporter); err != nil {
    return err
}
defer func() {
    if err := tracer.Shutdown(ctx); err != nil {
        log.Print(err)
    }
}()
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

if err := p.Submit(func() {
    // task
}); err != nil {
    log.Fatal(err)
}

// Future 模式
future := poolx.SubmitFunc(p, func() (int, error) {
    return compute(), nil
})
result, err := future.Get()

// 等待第一个完成（通过 cancel context 防止 goroutine 泄漏）
f1 := poolx.SubmitFunc(p, func() (int, error) { return callAPI1() })
f2 := poolx.SubmitFunc(p, func() (int, error) { return callAPI2() })
val, idx, err := poolx.AwaitFirst(f1, f2)

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
if err := cfg.LoadEnv("APP"); err != nil { // APP_NAME 映射到 name，APP_PORT 映射到 port
    log.Fatal(err)
}

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

### 结构化命令沙箱

`Sandbox.Exec` 仅接受结构化的 `sandbox.Command`，不会把命令和参数拼成 shell
文本。`Path` 表示可执行文件路径，`Args` 按原始 argv 元素传递；`Dir` 为空时使用
沙箱工作区，非空时必须位于工作区内。

Toolkit 不负责语言识别、临时源码文件或构建流程；调用方应先准备可执行产物，再通过
结构化 `Exec` 提交命令。这样隔离层不会把 `go run` 等多进程编排误当成单次载荷执行。

`Env == nil` 时由平台提供最小安全环境；`Env` 非 nil 时表示完整环境，而不是在
宿主环境上增量覆盖。需要显式环境时，调用方必须给出全部必要变量。

```go
sb, err := sandbox.New(sandbox.Config{
    Workspace:            workspace,
    RequiredCapabilities: sandbox.UntrustedCodeIsolationCapabilities,
})
if err != nil {
    return err
}

result, execErr := sb.Exec(ctx, sandbox.Command{
    Path: executablePath,
    Args: []string{"structured-command"},
    Dir:  workspace,
    Env:  nil,
})
if err := errors.Join(execErr, sb.Close()); err != nil {
    return err
}
fmt.Print(result.Stdout)
```

完整的跨平台可编译示例见 `examples/os/sandbox`。

### 威胁模型与能力合同

沙箱用于保护宿主免受沙箱载荷侵害。宿主进程、调用方提供的配置，以及载荷启动前
完成的工作区 staging 属于可信边界；本合同不承诺抵抗已经在宿主上以同 UID 任意
执行的恶意进程，因为该进程本身已经拥有相应用户数据的访问权限。这个边界不豁免
沙箱载荷自身的逃逸防护：工作区根链接、指向工作区外对象的静态 hardlink，以及在
最终 preflight 复核时已经发生且可观察到的路径身份变更仍必须失败关闭。可信调用方
或已取得同 UID 宿主执行能力的进程在最终复核后继续并发修改路径，不属于本合同。

`RequiredCapabilities` 是必填的执行前合同，`RequiredCapabilities == 0` 会在创建工作区
前被 `New` 拒绝。执行不可信代码应从 `UntrustedCodeIsolationCapabilities` 开始，它只保证
filesystem、network、process-containment 和 output 隔离，不声称提供 Memory、Processes
或 Storage 抗拒绝服务配额。`MaxMemoryBytes`、`MaxProcesses`、`MaxWorkspaceBytes` 和
`MaxArtifactBytes` 为 0 时表示未请求且不设置上限；设置正值时必须分别追加
`CapabilityMemory`、`CapabilityProcesses` 或 `CapabilityStorage`。输出使用安全的有界默认值，
因此隔离集合始终包含 `CapabilityOutput`。后端无法证明任一必需能力时，必须在载荷启动前拒绝。
`LimitReport` 只记录本次执行事实：可选配额分别以 `not_requested`、`enforced` 或
`unsupported` 表示未请求、已真实执行或后端不支持；平台可用能力应通过
`sandbox.AvailableCapabilities(ctx, sb)` 查询。

固定且可信的构建工具处理已暂存源码时，可以单独创建要求
`TrustedBuildIsolationCapabilities` 的沙箱。该集合增加 `CapabilityProcessCreation`，并刻意
不要求 `CapabilityProcessContainment`；macOS 因而允许工具链派生子进程，但会如实报告后代
收容为 `unsupported`。此合同绝不能用于执行不可信产物：构建结束后必须关闭构建沙箱，再用
要求 `UntrustedCodeIsolationCapabilities` 的新沙箱执行已经校验的产物。

`CapabilityProcessContainment` 的统一合同是：从载荷启动到 `Exec` 返回，根进程及其后代
要么不能产生，要么始终位于不可逃逸的生命周期边界；只有确认全部退出后才能报告
`enforced`。macOS 通过严格 no-fork、Linux 通过 PID namespace、Windows 通过 Job Object
分别证明同一不变量。任一证明不成立时，必须在任何载荷副作用发生前拒绝。

## 近期更新

- **infra/redisconn**: 新增 Single / Cluster / Sentinel 统一 Redis 连接工厂，支持 ACL 账号密码、Sentinel 双凭据、TLS、动态凭据与启动探活；项目级策略主动拒绝 password-only
- **net/httpx**: `RateLimitedPool.Close()` 无返回值，并复用底层连接池的原子关闭保证幂等，多次调用安全
- **util/poolx**: `AwaitFirst` 使用可取消 context，首个结果返回后自动取消剩余等待，防止 goroutine 泄漏
- **util/poolx**: 修复 `workerStack.retrieveExpiry` 环形缓冲区压缩逻辑，过期回收后正确重建存活 worker 队列

## 项目结构

```
toolkit/
├── blobstore/          # Blob 存储（本地落盘 + S3/R2 后端 seam + 流式 + TTL）
│
├── event/              # 事件总线（发布-订阅，线程安全）
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
│   ├── redisconn/     # Redis 统一连接工厂（显式拓扑 / ACL / TLS）
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
│   ├── sse/           # Server-Sent Events
│   └── ssrf/          # URL 级 SSRF 校验
│
├── os/                 # 操作系统能力
│   └── sandbox/       # 命令沙箱（文件系统/网络/进程/输出隔离）
│
├── util/               # 工具组件
│   ├── circuit/       # 熔断器（AI 预设/多实例管理）
│   ├── config/        # 配置管理
│   ├── encoding/      # 编码（Base64/Hex/URL）
│   ├── env/           # 环境变量
│   ├── file/          # 文件操作
│   ├── hash/          # 哈希（SHA-256/SHA-512/bcrypt）
│   ├── idgen/         # ID 生成（Snowflake/NanoID）
│   ├── json/          # JSON 辅助
│   ├── lease/         # 分布式互斥租约（FencingToken）
│   ├── logger/        # 日志（基于 slog）
│   ├── pagination/    # 分页
│   ├── poolx/         # 高性能协程池
│   ├── rand/          # 随机数（含返回 error 的 Try* 安全变体）
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
| infra/redisconn | 98.0% |
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

- AES-GCM 提供带认证的加密能力
- HMAC 验证使用恒定时间比较
- HTTP 客户端内置 SSRF 防护
- 签名验证支持时间戳过期和 nonce 防重放

### 4. 性能优化

- 字符串与字节切片使用安全、无别名转换
- 对象池和缓存复用
- 最小化反射使用
- Singleflight 防止缓存击穿

### 5. 泛型优先

所有集合和工具函数优先使用泛型实现类型安全。

## 依赖

核心依赖（按需引入，仅相关子包才会拉取对应依赖）：
```
github.com/hibiken/asynq                  # 任务队列（infra/queue/asynq）
github.com/redis/go-redis/v9              # Redis 客户端（infra/redisconn、cache/redis、infra/db/redis）
github.com/go-sql-driver/mysql            # MySQL 驱动（infra/db/mysql）
go.mongodb.org/mongo-driver               # MongoDB 驱动（infra/db/mongodb）
github.com/ClickHouse/clickhouse-go/v2    # ClickHouse 驱动（infra/db/clickhouse）
github.com/elastic/go-elasticsearch/v8    # Elasticsearch 客户端（infra/db/elasticsearch）
github.com/prometheus/client_golang       # 监控指标（infra/prometheus）
golang.org/x/sync                         # singleflight
golang.org/x/crypto                       # 加密扩展
github.com/google/uuid                    # UUID 生成
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
