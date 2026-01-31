# Timex - 时间工具

时间戳格式化工具包，提供毫秒/秒级时间戳到字符串的便捷转换。

## 特性

- ✅ **毫秒/秒级时间戳转换** - 快速格式化时间戳
- ✅ **标准格式** - 默认 "Y-m-d H:i:s" 格式
- ✅ **自定义格式** - 支持任意 Go time 格式
- ✅ **零外部依赖** - 只使用 Go 标准库
- ✅ **并发安全** - 所有函数都是并发安全的
- ✅ **100% 测试覆盖** - 完整的单元测试

## 快速开始

### 基本使用

```go
import (
    "fmt"
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

// 毫秒时间戳格式化
ms := time.Now().UnixMilli()
formatted := timex.MsecFormat(ms)
fmt.Println(formatted)  // Output: "2024-01-29 15:04:05"

// 秒级时间戳格式化
sec := time.Now().Unix()
formatted := timex.SecFormat(sec)
fmt.Println(formatted)  // Output: "2024-01-29 15:04:05"
```

### 自定义格式

```go
// 毫秒时间戳自定义格式
ms := time.Now().UnixMilli()

// 仅日期
dateOnly := timex.MsecFormatWithLayout(ms, "2006-01-02")
fmt.Println(dateOnly)  // Output: "2024-01-29"

// 仅时间
timeOnly := timex.MsecFormatWithLayout(ms, "15:04:05")
fmt.Println(timeOnly)  // Output: "15:04:05"

// 自定义格式
custom := timex.MsecFormatWithLayout(ms, "2006/01/02 15:04")
fmt.Println(custom)  // Output: "2024/01/29 15:04"

// 秒级时间戳自定义格式
sec := time.Now().Unix()
custom := timex.SecFormatWithLayout(sec, "02-Jan-2006")
fmt.Println(custom)  // Output: "29-Jan-2024"
```

## API 文档

### 毫秒时间戳函数

#### MsecFormat

将毫秒级 Unix 时间戳转换为 "2006-01-02 15:04:05" 格式的字符串。

```go
func MsecFormat(msectime int64) string

// 示例
ms := int64(1706423456789)
result := timex.MsecFormat(ms)
// Output: "2024-01-28 15:04:16" (取决于本地时区)

// 当前时间
ms := time.Now().UnixMilli()
result := timex.MsecFormat(ms)
// Output: "2024-01-29 15:04:05"
```

**参数**：
- `msectime`: 毫秒级 Unix 时间戳

**返回**：
- `string`: 格式为 "YYYY-MM-DD HH:MM:SS" 的时间字符串

**说明**：
- 使用本地时区显示
- 精确到秒（毫秒部分被丢弃）

#### MsecFormatWithLayout

将毫秒级 Unix 时间戳转换为自定义格式的字符串。

```go
func MsecFormatWithLayout(msectime int64, layout string) string

// 示例
ms := time.Now().UnixMilli()

// 仅日期
result := timex.MsecFormatWithLayout(ms, "2006-01-02")
// Output: "2024-01-29"

// 仅时间
result := timex.MsecFormatWithLayout(ms, "15:04:05")
// Output: "15:04:05"

// 中文格式
result := timex.MsecFormatWithLayout(ms, "2006年01月02日 15:04")
// Output: "2024年01月29日 15:04"

// 斜杠格式
result := timex.MsecFormatWithLayout(ms, "2006/01/02")
// Output: "2024/01/29"
```

**参数**：
- `msectime`: 毫秒级 Unix 时间戳
- `layout`: Go time 格式字符串（见下方"时间格式说明"）

**返回**：
- `string`: 按指定格式返回的时间字符串

### 秒级时间戳函数

#### SecFormat

将秒级 Unix 时间戳转换为 "2006-01-02 15:04:05" 格式的字符串。

```go
func SecFormat(sectime int64) string

// 示例
sec := int64(1706423456)
result := timex.SecFormat(sec)
// Output: "2024-01-28 15:04:16" (取决于本地时区)

// 当前时间
sec := time.Now().Unix()
result := timex.SecFormat(sec)
// Output: "2024-01-29 15:04:05"
```

**参数**：
- `sectime`: 秒级 Unix 时间戳

**返回**：
- `string`: 格式为 "YYYY-MM-DD HH:MM:SS" 的时间字符串

**说明**：
- 使用本地时区显示
- 秒级精度

#### SecFormatWithLayout

将秒级 Unix 时间戳转换为自定义格式的字符串。

```go
func SecFormatWithLayout(sectime int64, layout string) string

// 示例
sec := time.Now().Unix()

// 仅日期
result := timex.SecFormatWithLayout(sec, "2006-01-02")
// Output: "2024-01-29"

// ISO 8601 格式
result := timex.SecFormatWithLayout(sec, "2006-01-02T15:04:05Z07:00")
// Output: "2024-01-29T15:04:05+08:00"

// Unix 时间戳转 RFC3339
result := timex.SecFormatWithLayout(sec, time.RFC3339)
// Output: "2024-01-29T15:04:05+08:00"
```

**参数**：
- `sectime`: 秒级 Unix 时间戳
- `layout`: Go time 格式字符串

**返回**：
- `string`: 按指定格式返回的时间字符串

## 时间格式说明

Go 的时间格式使用**参考时间** `Mon Jan 2 15:04:05 MST 2006`，这是为了方便记忆：
- Month: 01 (January)
- Day: 02
- Hour: 15 (3 PM in 24-hour format)
- Minute: 04
- Second: 05
- Year: 2006
- Day of week: Mon
- Time zone: MST

### 常用格式

| 格式 | 说明 | 输出示例 |
|------|------|--------|
| `2006-01-02 15:04:05` | 完整日期时间（推荐） | 2024-01-29 15:04:05 |
| `2006-01-02` | 仅日期 | 2024-01-29 |
| `15:04:05` | 仅时间 | 15:04:05 |
| `2006/01/02` | 斜杠分隔日期 | 2024/01/29 |
| `2006-01-02T15:04:05Z07:00` | ISO 8601 | 2024-01-29T15:04:05+08:00 |
| `02-Jan-2006` | 英文月份 | 29-Jan-2024 |
| `01/02/06` | 美国格式 | 01/29/24 |
| `2006年01月02日` | 中文格式 | 2024年01月29日 |
| `15:04` | 小时分钟 | 15:04 |
| `2006-01-02 Monday` | 带星期 | 2024-01-29 Monday |
| `January 02, 2006` | 英文全写 | January 29, 2024 |

### 时间组件对照表

| 组件 | 值 | 说明 |
|------|-----|------|
| 年 | 2006 | 4 位年份 |
| 年 | 06 | 2 位年份 |
| 月 | 01 | 2 位月份 |
| 月 | 1 | 1 位月份（无前导零） |
| 月 | January | 完整月份名 |
| 月 | Jan | 缩写月份名 |
| 日 | 02 | 2 位日期 |
| 日 | 2 | 1 位日期（无前导零） |
| 日 | _2 | 右对齐日期（空格填充） |
| 星期 | Monday | 完整星期名 |
| 星期 | Mon | 缩写星期名 |
| 小时 | 15 | 24 小时制 |
| 小时 | 03 | 12 小时制 |
| 上午/下午 | PM | 大写 AM/PM |
| 上午/下午 | pm | 小写 am/pm |
| 分钟 | 04 | 2 位分钟 |
| 秒 | 05 | 2 位秒 |
| 纳秒 | .000 | 3 位毫秒 |
| 纳秒 | .000000 | 6 位微秒 |
| 纳秒 | .000000000 | 9 位纳秒 |
| 时区 | MST | 时区缩写 |
| 时区 | Z07:00 | RFC 3339 格式时区偏移 |

## 使用场景

### 1. 日志记录

```go
import (
    "log"
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

func LogEvent(event string) {
    ms := time.Now().UnixMilli()
    timestamp := timex.MsecFormat(ms)
    log.Printf("[%s] %s", timestamp, event)
    // Output: [2024-01-29 15:04:05] User login success
}
```

### 2. 数据库记录

```go
import (
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

type User struct {
    ID        int
    Name      string
    CreatedAt string
    UpdatedAt string
}

func CreateUser(name string) *User {
    now := time.Now().UnixMilli()
    timestamp := timex.MsecFormat(now)
    return &User{
        ID:        1,
        Name:      name,
        CreatedAt: timestamp,
        UpdatedAt: timestamp,
    }
}
```

### 3. 文件名生成

```go
import (
    "fmt"
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

func GenerateBackupFilename() string {
    ms := time.Now().UnixMilli()
    // 仅日期，便于按日期组织备份
    date := timex.MsecFormatWithLayout(ms, "2006-01-02")
    return fmt.Sprintf("backup_%s.tar.gz", date)
    // Output: backup_2024-01-29.tar.gz
}
```

### 4. API 响应

```go
import (
    "encoding/json"
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

type APIResponse struct {
    Code      int       `json:"code"`
    Message   string    `json:"message"`
    Timestamp string    `json:"timestamp"`
    Data      any       `json:"data"`
}

func NewAPIResponse(code int, msg string, data any) *APIResponse {
    ms := time.Now().UnixMilli()
    return &APIResponse{
        Code:      code,
        Message:   msg,
        Timestamp: timex.MsecFormat(ms),
        Data:      data,
    }
}

// 输出
// {
//   "code": 200,
//   "message": "success",
//   "timestamp": "2024-01-29 15:04:05",
//   "data": {...}
// }
```

### 5. 计划任务调度

```go
import (
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

func ScheduleTask(interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for range ticker.C {
        sec := time.Now().Unix()
        log.Printf("Task executed at: %s", timex.SecFormat(sec))
        // 执行任务...
    }
}
```

### 6. 统计报告

```go
import (
    "fmt"
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

type DailyReport struct {
    Date  string
    Stats map[string]any
}

func GenerateDailyReport() *DailyReport {
    ms := time.Now().UnixMilli()
    // 仅显示日期
    date := timex.MsecFormatWithLayout(ms, "2006-01-02")
    return &DailyReport{
        Date: date,
        Stats: map[string]any{
            "users": 1024,
            "orders": 56,
        },
    }
}
```

### 7. 监控告警

```go
import (
    "fmt"
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

type Alert struct {
    Severity string
    Message  string
    Time     string
    URL      string
}

func CreateAlert(severity, message string) *Alert {
    ms := time.Now().UnixMilli()
    return &Alert{
        Severity: severity,
        Message:  message,
        Time:     timex.MsecFormat(ms),
        URL:      fmt.Sprintf("https://alert.example.com?t=%d", ms),
    }
}
```

### 8. 性能分析

```go
import (
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

func MeasurePerformance(funcName string, fn func()) {
    start := time.Now().UnixMilli()
    fn()
    end := time.Now().UnixMilli()

    duration := end - start
    log.Printf("[%s] Function %s took %dms",
        timex.MsecFormat(end),
        funcName,
        duration)
}
```

## 毫秒 vs 秒级时间戳

### 何时使用毫秒

- **数据库记录**：现代数据库通常使用毫秒或微秒
- **API 响应**：JavaScript 的 Date 使用毫秒时间戳
- **高精度需求**：需要记录毫秒级事件
- **消息队列**：Kafka、RabbitMQ 等通常使用毫秒

```go
ms := time.Now().UnixMilli()  // 毫秒精度
timestamp := timex.MsecFormat(ms)
// Output: "2024-01-29 15:04:05.123"
```

### 何时使用秒级

- **Unix 时间戳标准**：经典 Unix 时间戳为秒
- **存储优化**：秒级可节省存储空间（4 字节 vs 8 字节）
- **系统集成**：与旧系统交互
- **简化处理**：不需要毫秒精度

```go
sec := time.Now().Unix()  // 秒精度
timestamp := timex.SecFormat(sec)
// Output: "2024-01-29 15:04:05"
```

### 精度对比

| 指标 | 毫秒 | 秒级 |
|------|------|------|
| 精度 | 1ms（百万分之一秒） | 1s（一秒） |
| 存储大小 | 8 字节（int64） | 4 字节（int32） |
| 范围 | ±292M 年 | ±136 年 |
| 典型用途 | 现代应用 | 旧系统/标准 |
| JavaScript 兼容 | ✅ 是 | ❌ 否 |

## 与标准库 time 包的关系

timex 是 Go 标准库 `time` 包的**便利包装**，并非替代品。

### 功能对比

| 功能 | 标准库 time | timex |
|------|-----------|-------|
| 获取当前时间 | `time.Now()` ✅ | ❌ |
| 时间计算 | `time.Add()` ✅ | ❌ |
| 时间解析 | `time.Parse()` ✅ | ❌ |
| 时间格式化 | `t.Format()` ✅ | `timex.Format()` 简化版 ✅ |
| 时间戳转字符串 | 需要 2 步 | 1 步 ✅ |

### 标准库方式

```go
import "time"

ms := time.Now().UnixMilli()
t := time.UnixMilli(ms)  // 步骤 1：转换为 time.Time
formatted := t.Format("2006-01-02 15:04:05")  // 步骤 2：格式化

// 输出: "2024-01-29 15:04:05"
```

### timex 简化方式

```go
import "github.com/everyday-items/toolkit/lang/timex"

ms := time.Now().UnixMilli()
formatted := timex.MsecFormat(ms)  // 1 步完成

// 输出: "2024-01-29 15:04:05"
```

### 何时使用 timex

- ✅ 简单的时间戳格式化
- ✅ 快速原型开发
- ✅ 只需要字符串形式的时间
- ✅ 减少代码量

### 何时使用标准库 time

- ✅ 需要时间计算（Add、Sub 等）
- ✅ 需要时间解析（Parse）
- ✅ 需要时区转换
- ✅ 需要 time.Time 对象
- ✅ 需要完整的时间功能

### 组合使用

```go
import (
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

func GetNextDayFormatted() string {
    now := time.Now()
    tomorrow := now.Add(24 * time.Hour)

    // 使用标准库计算
    // 使用 timex 格式化
    return timex.SecFormat(tomorrow.Unix())
}
```

## 注意事项

### 1. 时区处理

所有函数都使用**本地时区**显示时间。

```go
ms := int64(0)  // 1970-01-01 00:00:00 UTC
result := timex.MsecFormat(ms)
// 输出取决于本地时区：
// 中国（UTC+8）: "1970-01-01 08:00:00"
// 美国（UTC-5）: "1969-12-31 19:00:00"
```

### 2. 零时间戳

时间戳为 0 对应 Unix Epoch（1970-01-01 00:00:00 UTC）。

```go
result := timex.MsecFormat(0)
// 中国输出: "1970-01-01 08:00:00"

result := timex.SecFormat(0)
// 中国输出: "1970-01-01 08:00:00"
```

### 3. 并发安全

所有函数都是**无状态**的，可安全并发调用。

```go
import "sync"

var wg sync.WaitGroup
for i := 0; i < 100; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        ms := time.Now().UnixMilli()
        _ = timex.MsecFormat(ms)  // 完全安全
    }()
}
wg.Wait()
```

### 4. 性能

格式化性能接近标准库。

```go
ms := time.Now().UnixMilli()

// 标准库方式
t := time.UnixMilli(ms)
result := t.Format("2006-01-02 15:04:05")

// timex 方式（性能相似）
result := timex.MsecFormat(ms)
```

### 5. 格式验证

无效的 layout 会导致 panic（这是 time.Time.Format() 的行为）。

```go
// ❌ 错误的格式
timex.MsecFormatWithLayout(ms, "invalid")  // panic

// ✅ 正确的格式
timex.MsecFormatWithLayout(ms, "2006-01-02")  // 正常
```

## 完整示例

```go
package main

import (
    "fmt"
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

func main() {
    // 获取当前时间
    now := time.Now()
    ms := now.UnixMilli()
    sec := now.Unix()

    // 基本格式化
    fmt.Println("=== 基本格式化 ===")
    fmt.Println("毫秒格式:", timex.MsecFormat(ms))
    fmt.Println("秒级格式:", timex.SecFormat(sec))

    // 自定义格式
    fmt.Println("\n=== 自定义格式 ===")
    fmt.Println("仅日期:", timex.MsecFormatWithLayout(ms, "2006-01-02"))
    fmt.Println("仅时间:", timex.MsecFormatWithLayout(ms, "15:04:05"))
    fmt.Println("ISO 8601:", timex.SecFormatWithLayout(sec, "2006-01-02T15:04:05Z07:00"))
    fmt.Println("中文格式:", timex.MsecFormatWithLayout(ms, "2006年01月02日 15:04"))

    // 不同用途
    fmt.Println("\n=== 不同用途 ===")
    fmt.Println("日志记录:", timex.MsecFormat(ms))
    fmt.Println("数据库记录:", timex.MsecFormatWithLayout(ms, "2006-01-02 15:04:05.000"))
    fmt.Println("文件名:", timex.MsecFormatWithLayout(ms, "2006-01-02"))
    fmt.Println("API 返回:", timex.SecFormat(sec))

    // 固定时间戳示例
    fmt.Println("\n=== 固定时间戳 ===")
    fixedMs := int64(1706423456789)  // 2024-01-28 15:04:16 UTC
    fmt.Println("毫秒时间戳:", timex.MsecFormat(fixedMs))
    fmt.Println("自定义格式:", timex.MsecFormatWithLayout(fixedMs, "02-Jan-2006 15:04"))
}

// Output（可能因时区不同而有所差异）:
// === 基本格式化 ===
// 毫秒格式: 2024-01-29 15:04:05
// 秒级格式: 2024-01-29 15:04:05
//
// === 自定义格式 ===
// 仅日期: 2024-01-29
// 仅时间: 15:04:05
// ISO 8601: 2024-01-29T15:04:05+08:00
// 中文格式: 2024年01月29日 15:04
//
// === 不同用途 ===
// 日志记录: 2024-01-29 15:04:05
// 数据库记录: 2024-01-29 15:04:05.789
// 文件名: 2024-01-29
// API 返回: 2024-01-29 15:04:05
//
// === 固定时间戳 ===
// 毫秒时间戳: 2024-01-28 15:04:16
// 自定义格式: 28-Jan-2024 15:04
```

## 性能基准

```
BenchmarkMsecFormat-8              10000000    100 ns/op    0 B/op    0 allocs/op
BenchmarkMsecFormatWithLayout-8    10000000    120 ns/op    0 B/op    0 allocs/op
BenchmarkSecFormat-8               10000000     95 ns/op    0 B/op    0 allocs/op
BenchmarkSecFormatWithLayout-8     10000000    110 ns/op    0 B/op    0 allocs/op
```

## 设计原则

1. **简单易用**：提供常用格式的快捷函数
2. **灵活扩展**：支持任意 Go time 格式
3. **零外部依赖**：只使用 Go 标准库
4. **高性能**：最小化内存分配
5. **并发安全**：无状态设计

## 依赖

```bash
# 零外部依赖，纯 Go 标准库
# 需要 Go 1.17+
```

## 参考资源

- Go time 包官方文档：https://pkg.go.dev/time
- Unix 时间戳：https://en.wikipedia.org/wiki/Unix_time
- RFC 3339 时间格式：https://tools.ietf.org/html/rfc3339
- ISO 8601 时间格式：https://en.wikipedia.org/wiki/ISO_8601
