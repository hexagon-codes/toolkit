中文 | [English](README.en.md)

# IDGen ID 生成器

提供多种 ID 生成方案：UUID、Snowflake、NanoID。

## 特性

- ✅ UUID - 标准 UUID v4
- ✅ Snowflake - 分布式唯一 ID
- ✅ NanoID - 短小的唯一 ID
- ✅ 高性能 - 快速生成
- ✅ 并发安全 - 支持并发调用

## 快速开始

### UUID

```go
import "github.com/hexagon-codes/toolkit/util/idgen"

// 需要传播熵源错误时使用 TryUUID
id, err := idgen.TryUUID()
if err != nil {
    return err
}
// 输出: "550e8400-e29b-41d4-a716-446655440000"

// 不带连字符的 UUID
id := idgen.UUIDWithoutHyphen()
// 输出: "550e8400e29b41d4a716446655440000"
```

### Snowflake

```go
// 初始化（在 main 函数中执行一次）
if err := idgen.InitSnowflake(1); err != nil { // worker ID: 1
    return err
}

// 生成 Snowflake ID
id := idgen.SnowflakeID()
// 输出: 1234567890123456789
```

### NanoID

```go
// 默认长度（21 位）
id := idgen.NanoID()
// 输出: "V1StGXR8_Z5jdHi6B-myT"

// 指定长度
id := idgen.NanoIDSize(10)
// 输出: "4f90d13a42"

// 输入来自外部或需要传播熵源错误时使用 Try 版本
id, err := idgen.TryNanoIDSize(10)

// 短 ID（8 位）
id := idgen.ShortID()
// 输出: "xK3s9d2a"

// 中等 ID（16 位）
id := idgen.MediumID()
// 输出: "3f2hK9s1pL4m8nR5"
```

## 对比

| 类型 | 长度 | 性能 | 有序 | 分布式 | 适用场景 |
|------|------|------|------|--------|----------|
| UUID | 36字符 | 快 | ❌ | ✅ | 通用唯一标识 |
| Snowflake | 19位数字 | 很快 | ✅ | ✅ | 订单号、用户ID |
| NanoID | 可变(默认21) | 快 | ❌ | ✅ | 短链接、文件名 |

## UUID

### 特点
- 128位唯一标识
- 全局唯一，无需中心化
- 标准格式：8-4-4-4-12

### 使用场景
```go
// 用户 ID
userID := idgen.UUID()

// 请求追踪 ID
requestID := idgen.UUID()

// 文件名
filename := idgen.UUIDWithoutHyphen() + ".jpg"
```

## Snowflake

### 特点
- 64位整数
- 时间有序（按生成时间排序）
- 支持分布式（最多 1024 个节点）
- 每毫秒最多生成 4096 个 ID

### 结构
```
|---41位时间戳---|---10位机器ID---|---12位序列号---|
```

### 初始化
```go
func main() {
    // 在应用启动时初始化一次
    // workerID: 0-1023
    if err := idgen.InitSnowflake(1); err != nil {
        log.Fatal(err)
    }
}
```

### 使用场景
```go
// 订单 ID
orderID := idgen.SnowflakeID()

// 消息 ID
messageID := idgen.SnowflakeID()

// 日志 ID
logID := idgen.SnowflakeID()
```

### 多实例部署
```go
// 服务器 1
idgen.InitSnowflake(1)

// 服务器 2
idgen.InitSnowflake(2)

// 服务器 3
idgen.InitSnowflake(3)

// 每个服务器使用不同的 workerID
```

## NanoID

### 特点
- 比 UUID 更短（默认21位）
- URL 安全字符
- 可自定义字符集和长度；字符集必须包含 2-256 个互不重复的字符
- 长度必须为 1-1048576
- 碰撞概率极低

### 自定义字符集
```go
// 只使用数字
alphabet := "0123456789"
id := idgen.NanoIDCustom(alphabet, 10)
// 输出: "4839274920"

// 只使用小写字母
alphabet := "abcdefghijklmnopqrstuvwxyz"
id := idgen.NanoIDCustom(alphabet, 10)
// 输出: "xfbdekmpqr"
```

### 使用场景
```go
// 短链接
shortURL := idgen.ShortID()
// 输出: "xK3s9d2a"

// 验证码（只用数字）
code := idgen.NanoIDCustom("0123456789", 6)
// 输出: "492837"

// 文件名
filename := idgen.MediumID() + ".pdf"
```

## 错误处理

`UUID`、`NanoID` 及其便捷变体在输入无效或系统熵源失败时 panic。服务请求链路应优先使用
`TryUUID`、`TryUUIDWithoutHyphen`、`TryNanoID`、`TryNanoIDSize` 和 `TryNanoIDCustom`，并传播错误。
`GenerateSafe` 返回 Snowflake 的时钟回拨、序列耗尽和时间戳越界错误；`Generate` 在这些错误上 panic。

## 最佳实践

### 1. 选择合适的 ID 类型

```go
// ✅ 数据库主键：Snowflake（有序，性能好）
userID := idgen.SnowflakeID()

// ✅ 外部 API：UUID（标准，兼容性好）
apiKey := idgen.UUID()

// ✅ 短链接：NanoID（短小，URL 友好）
shortURL := idgen.ShortID()
```

### 2. Snowflake 需要提前初始化

```go
// ✅ 在 main 函数中初始化
func main() {
    idgen.InitSnowflake(getWorkerID())
    // ...
}

// ❌ 不要在每次使用时创建
func bad() {
    gen, _ := idgen.NewSnowflake(1) // 错误！
    id := gen.Generate()
}
```

### 3. 分布式部署 workerID 管理

```go
// 方案1：配置文件
workerID := config.GetInt("worker_id")

// 方案2：环境变量
workerID := os.Getenv("WORKER_ID")

// 方案3：根据 IP 计算
workerID := hashIP(getLocalIP()) % 1024
```

## 依赖

```bash
go get -u github.com/google/uuid
```

## 注意事项

1. **UUID**: 无需初始化，直接使用
2. **Snowflake**: 必须先初始化，workerID 范围 0-1023
3. **NanoID**: 自定义字符集时注意字符不重复
4. **时钟与范围**: Snowflake 默认最多等待 100ms；41 位时间字段约覆盖 2020-01-01 起 69 年
5. **碰撞风险**: 随机 ID 不提供绝对唯一保证，短 ID 的容量规划必须考虑生日碰撞
6. **并发安全**: 所有生成入口都可并发调用
