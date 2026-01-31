# toolkit

一个生产级 Go 通用工具包，采用领域驱动设计理念。

[![Go Version](https://img.shields.io/badge/Go-%3E%3D%201.22-blue)](https://golang.org/)

## 特性

✅ **领域驱动设计** - 按功能领域组织，清晰的分层架构
✅ **生产级代码** - 经过实战验证的高质量实现
✅ **接口驱动** - 易于扩展和测试
✅ **零拷贝优化** - 高性能字符串/字节操作
✅ **完整监控** - Prometheus 指标支持
✅ **泛型支持** - Go 1.18+ 泛型实现类型安全

## 快速开始

```bash
go get github.com/everyday-items/toolkit
```

### 类型转换

```go
import "github.com/everyday-items/toolkit/lang/conv"

str := conv.String(123)           // "123"
i := conv.Int("42")               // 42
f := conv.Float64("3.14")         // 3.14

// JSON-Map 互转
m, _ := conv.JsonToMap(`{"key":"value"}`)
json, _ := conv.MapToJson(m)
```

### 字符串工具

```go
import "github.com/everyday-items/toolkit/lang/stringx"

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
import "github.com/everyday-items/toolkit/lang/mapx"

m := map[string]int{"a": 1, "b": 2, "c": 3}

keys := mapx.Keys(m)                           // ["a", "b", "c"]
values := mapx.Values(m)                       // [1, 2, 3]
filtered := mapx.Filter(m, func(k string, v int) bool { return v > 1 })
merged := mapx.Merge(m1, m2)
inverted := mapx.Invert(m)                     // map[int]string
```

### 错误处理

```go
import "github.com/everyday-items/toolkit/lang/errorx"

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
import "github.com/everyday-items/toolkit/lang/timex"

timex.IsToday(t)                    // 是否今天
timex.IsWeekend(t)                  // 是否周末
timex.StartOfDay(t)                 // 当天 00:00:00
timex.EndOfMonth(t)                 // 月末
timex.DaysBetween(t1, t2)           // 间隔天数
timex.Age(birthday)                 // 计算年龄
```

### Context 工具

```go
import "github.com/everyday-items/toolkit/lang/contextx"

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
import "github.com/everyday-items/toolkit/crypto/aes"

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
import "github.com/everyday-items/toolkit/crypto/rsa"

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
import "github.com/everyday-items/toolkit/crypto/sign"

sig := sign.HMACSHA256String("message", "secret-key")
ok := sign.VerifyHMACSHA256String("message", "secret-key", sig)

// API 签名
signer := sign.NewAPISigner("app-key", "app-secret")
sig := signer.Sign(params, timestamp, nonce)
```

### HTTP 客户端

```go
import "github.com/everyday-items/toolkit/net/httpx"

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
```

### IP 工具

```go
import "github.com/everyday-items/toolkit/net/ip"

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
import "github.com/everyday-items/toolkit/util/logger"

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
import "github.com/everyday-items/toolkit/util/env"

port := env.GetIntDefault("PORT", 8080)
debug := env.GetBool("DEBUG")
hosts := env.GetSlice("HOSTS")  // 逗号分隔

if env.IsProd() {
    // 生产环境
}
```

### 编码工具

```go
import "github.com/everyday-items/toolkit/util/encoding"

// Base64
encoded := encoding.Base64EncodeString("hello")
decoded, _ := encoding.Base64DecodeString(encoded)

// Hex
hex := encoding.HexEncodeString("hello")

// URL
query := encoding.BuildQuery(map[string]string{"name": "test"})
params, _ := encoding.ParseQuery("name=test&age=18")
```

### Poolx 协程池

```go
import "github.com/everyday-items/toolkit/util/poolx"

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
import "github.com/everyday-items/toolkit/util/config"

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
import "github.com/everyday-items/toolkit/collection/list"

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
import "github.com/everyday-items/toolkit/collection/stack"

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
import "github.com/everyday-items/toolkit/collection/queue"

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
import "github.com/everyday-items/toolkit/collection/set"

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
gopkg/
├── collection/         # 数据结构
│   ├── list/          # 双向链表
│   ├── queue/         # 队列（FIFO/双端/优先级）
│   ├── set/           # 泛型 HashSet
│   └── stack/         # 栈（LIFO）
│
├── crypto/             # 加密工具
│   ├── aes/           # AES 对称加密
│   ├── rsa/           # RSA 非对称加密
│   └── sign/          # 签名验签
│
├── lang/               # 语言增强（零外部依赖）
│   ├── contextx/      # Context 工具
│   ├── conv/          # 类型转换
│   ├── stringx/       # 字符串扩展
│   ├── timex/         # 时间工具
│   ├── slicex/        # 切片工具（泛型）
│   ├── mapx/          # Map 工具（泛型）
│   ├── mathx/         # 数学工具（泛型）
│   ├── errorx/        # 错误处理
│   └── syncx/         # 并发工具
│
├── net/                # 网络工具
│   ├── httpx/         # HTTP 客户端
│   └── ip/            # IP 工具
│
├── util/               # 工具组件
│   ├── config/        # 配置管理
│   ├── encoding/      # 编码（Base64/Hex/URL）
│   ├── env/           # 环境变量
│   ├── logger/        # 日志（基于 slog）
│   ├── poolx/         # 高性能协程池
│   ├── hash/          # 哈希（MD5/SHA/Bcrypt）
│   ├── idgen/         # ID 生成
│   ├── rand/          # 随机数
│   ├── retry/         # 重试机制
│   ├── rate/          # 限流器
│   ├── validator/     # 数据验证
│   ├── pagination/    # 分页
│   ├── file/          # 文件操作
│   ├── json/          # JSON 辅助
│   └── slice/         # 切片工具
│
├── cache/              # 缓存
│   ├── local/         # 本地缓存（LRU）
│   ├── redis/         # Redis 缓存
│   └── multi/         # 多层缓存
│
├── infra/              # 基础设施
│   ├── db/            # 数据库
│   │   ├── mysql/
│   │   └── redis/
│   └── queue/         # 消息队列
│       └── asynq/
│
└── examples/           # 使用示例
```

## 测试覆盖率

| 包 | 覆盖率 |
|---|--------|
| collection/list | 98.8% |
| collection/queue | 100.0% |
| collection/set | 95.5% |
| collection/stack | 100.0% |
| lang/contextx | 97.2% |
| lang/conv | 100.0% |
| lang/stringx | 95.9% |
| lang/timex | 96.0% |
| lang/slicex | 100.0% |
| lang/mapx | 96.5% |
| lang/mathx | 100.0% |
| lang/errorx | 80.8% |
| lang/syncx | 100.0% |
| crypto/aes | 83.2% |
| crypto/rsa | 81.4% |
| crypto/sign | 95.9% |
| net/httpx | 90.1% |
| net/ip | 87.5% |
| cache/local | 91.2% |
| cache/multi | 72.8% |
| cache/redis | 78.3% |
| util/config | 77.7% |
| util/encoding | 93.7% |
| util/env | 97.4% |
| util/file | 84.7% |
| util/hash | 100.0% |
| util/poolx | 61.6% |
| util/idgen | 84.5% |
| util/json | 79.6% |
| util/logger | 93.7% |
| util/pagination | 94.1% |
| util/rand | 96.8% |
| util/rate | 97.9% |
| util/retry | 91.9% |
| util/slice | 100.0% |
| util/validator | 100.0% |
| infra/db/mysql | 54.3% |
| infra/db/redis | 80.2% |
| infra/queue/asynq | 23.3% |

## 设计哲学

### 1. 领域驱动组织

代码按功能领域分组，而非技术类型：

```
❌ 不推荐：util/string.go, util/time.go
✅ 推荐：lang/stringx/, lang/timex/
```

### 2. 清晰的分层

```
crypto (加密) → net (网络) → util (工具) → lang (零依赖)
     ↓              ↓            ↓           ↓
  外部依赖       可能依赖      可能依赖    纯标准库
```

### 3. 接口优先

所有组件提供接口，易于 mock 和测试。

### 4. 性能优化

- 零拷贝字符串操作（unsafe）
- 对象池和缓存复用
- 最小化反射使用

## 依赖

核心依赖：
```
github.com/hibiken/asynq           # 任务队列
github.com/redis/go-redis/v9       # Redis 客户端
github.com/prometheus/client_golang # 监控指标
golang.org/x/sync                  # singleflight
```

**注意**：`lang/` 包零外部依赖，只使用 Go 标准库。

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
