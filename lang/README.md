# Lang - Go 语言增强工具

纯 Go 语言工具，**零外部依赖**，提供常用的类型转换、字符串操作和时间工具。

[![Test Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen.svg)](TEST_SUMMARY.md)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.20-blue.svg)](https://go.dev/)

## 特性

- **零外部依赖**：只使用 Go 标准库
- **100% 测试覆盖**：所有代码路径都经过测试验证 ⭐⭐⭐
  - conv: 100% ✅
  - stringx: 100% ✅
  - timex: 100% ✅
  - **mathx: 100%** ✅ NEW
  - **slicex: 100%** ✅ NEW
  - **syncx: 100%** ✅ NEW
- **类型安全**：完善的类型转换和泛型支持
- **高性能**：针对常见场景优化，使用零拷贝等优化技术
- **易于使用**：简洁的 API 设计
- **代码现代化**：使用 Go 1.20+ 最新特性（泛型等）

## 包列表

### conv - 类型转换

通用类型转换工具，支持各种 Go 类型之间的转换。

```go
import "github.com/everyday-items/toolkit/lang/conv"

// 字符串转换
conv.String(123)        // "123"
conv.String(45.67)      // "45.67"
conv.String(true)       // "true"

// 整数转换
conv.Int("123")         // 123
conv.Int64(45.67)       // 45
conv.Uint("100")        // 100

// 浮点数转换
conv.Float32("3.14")    // 3.14
conv.Float64([]byte{...}) // 从二进制解码

// 布尔值转换
conv.Bool(1)            // true
conv.Bool("yes")        // true

// JSON/Map 互转
m, _ := conv.JSONToMap(`{"name":"Alice"}`)
json, _ := conv.MapToJSON(m)

// Map 操作
merged := conv.MergeMaps(m1, m2)
keys := conv.MapKeys(m)
values := conv.MapValues(m)
```

**特性**：
- 支持所有基础类型转换
- 智能类型推断
- 失败时返回零值（不 panic）
- 支持 []byte 二进制解码
- 接口驱动的自定义转换

### stringx - 字符串工具

高性能字符串操作，包括零拷贝转换。

```go
import "github.com/everyday-items/toolkit/lang/stringx"

// 零拷贝转换（使用 unsafe，需谨慎）
str := stringx.BytesToString([]byte("hello"))
bytes := stringx.String2Bytes("world")

// 字符串切片转换（使用 reflect）
result := stringx.StringToSlice("1,2,3", ",")
// result = []int{1, 2, 3}
```

**警告**：
- `BytesToString` 和 `String2Bytes` 使用 unsafe 指针
- 不要修改转换后的数据
- 仅在性能关键路径使用

### timex - 时间工具

时间戳格式化工具。

```go
import "github.com/everyday-items/toolkit/lang/timex"

// 毫秒时间戳转字符串
ms := time.Now().UnixMilli()
formatted := timex.MsecFormat(ms)
// Output: "2024-01-29 15:04:05"

// 自定义格式
custom := timex.MsecFormatWithLayout(ms, "2006/01/02")
// Output: "2024/01/29"

// 秒级时间戳
timex.SecFormat(time.Now().Unix())
timex.SecFormatWithLayout(ts, "15:04:05")
```

**新增功能**（v2）：
- `MsecFormatWithLayout` - 自定义时间格式
- `SecFormat` - 秒级时间戳转换
- `SecFormatWithLayout` - 秒级自定义格式

### slicex - 切片工具 ⭐ **NEW**

Go 标准库缺失的切片操作，使用泛型实现类型安全。

```go
import "github.com/everyday-items/toolkit/lang/slicex"

// 包含检查
found := slicex.Contains([]int{1, 2, 3}, 2)  // true

// 过滤
even := slicex.Filter([]int{1, 2, 3, 4}, func(n int) bool {
    return n%2 == 0  // [2, 4]
})

// 映射
doubled := slicex.Map([]int{1, 2, 3}, func(n int) int {
    return n * 2  // [2, 4, 6]
})

// 去重
unique := slicex.Unique([]int{1, 2, 2, 3})  // [1, 2, 3]

// 查找
user, found := slicex.Find(users, func(u User) bool {
    return u.Role == "admin"
})

// 聚合
sum := slicex.Reduce([]int{1, 2, 3}, 0, func(acc, n int) int {
    return acc + n  // 6
})

// 分组
groups := slicex.GroupBy(users, func(u User) string {
    return u.City
})
```

**主要功能**：
- **查找检查**: Contains, Find, IndexOf
- **转换映射**: Map, Filter, Unique, FlatMap
- **聚合**: Reduce, GroupBy, Count, Some, Every
- **工具**: Reverse, Chunk, Take, Drop

### mathx - 数学工具 ⭐ **NEW**

标准库 math 包的泛型增强版本。

```go
import "github.com/everyday-items/toolkit/lang/mathx"

// 泛型 Min/Max（支持 int, float64, string 等）
min := mathx.Min(3, 1, 4, 1, 5)           // 1 (int)
max := mathx.Max(3.14, 2.71, 1.41)        // 3.14 (float64)
minStr := mathx.Min("c", "a", "b")        // "a" (string)

// 同时获取最小最大值
min, max := mathx.MinMax(3, 1, 4, 1, 5)   // 1, 5

// 限制值范围
clamped := mathx.Clamp(15, 0, 10)         // 10

// 泛型绝对值
abs := mathx.Abs(-5)                      // 5 (int)
absf := mathx.Abs(-3.14)                  // 3.14 (float64)
diff := mathx.AbsDiff(5, 3)               // 2

// 四舍五入
rounded := mathx.RoundTo(3.14159, 2)      // 3.14
ceil := mathx.Ceil(3.14)                  // 4.0
floor := mathx.Floor(3.14)                // 3.0
```

**主要功能**：
- **比较**: Min, Max, MinMax, Clamp
- **绝对值**: Abs, AbsDiff（泛型）
- **四舍五入**: Round, RoundTo, Ceil, Floor, Trunc

### syncx - 并发工具 ⭐ **NEW**

并发同步的工具函数，对标准库 sync 的增强。

```go
import "github.com/everyday-items/toolkit/lang/syncx"

// Singleflight - 防缓存击穿
sf := syncx.NewSingleflight()
result, err := sf.Do("user:123", func() (any, error) {
    return db.GetUser(123)  // 并发请求只执行一次
})

// Pool - 对象复用
pool := syncx.NewPool(func() any {
    return &bytes.Buffer{}
})
buf := pool.Get().(*bytes.Buffer)
defer pool.Put(buf)

// TypedPool - 类型安全的对象池（泛型）
pool := syncx.NewTypedPool(func() *bytes.Buffer {
    return &bytes.Buffer{}
})
buf := pool.Get()  // 无需类型断言
defer pool.Put(buf)
```

**主要功能**：
- **Singleflight**: 防止缓存击穿，合并重复请求
- **Pool**: sync.Pool 的友好封装
- **TypedPool**: 类型安全的对象池（泛型）

**典型场景**：
- Singleflight: 防缓存击穿、减少数据库压力、API去重
- Pool: 减少 GC 压力、高频对象复用

## 安装

```bash
go get github.com/everyday-items/toolkit/lang
```

## 完整示例

### 类型转换示例

```go
package main

import (
    "fmt"
    "github.com/everyday-items/toolkit/lang/conv"
)

func main() {
    // 各种类型到字符串
    fmt.Println(conv.String(123))           // "123"
    fmt.Println(conv.String([]byte("abc"))) // "abc"

    // 字符串到数字
    fmt.Println(conv.Int("456"))      // 456
    fmt.Println(conv.Float64("3.14")) // 3.14

    // JSON 处理
    m, err := conv.JSONToMap(`{"name":"Alice","age":30}`)
    if err != nil {
        panic(err)
    }
    fmt.Println(m["name"]) // "Alice"

    // Map 合并
    m1 := map[string]any{"a": 1}
    m2 := map[string]any{"b": 2}
    merged := conv.MergeMaps(m1, m2)
    fmt.Println(merged) // map[a:1 b:2]
}
```

### 时间格式化示例

```go
package main

import (
    "fmt"
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

func main() {
    // 当前时间（毫秒）
    ms := time.Now().UnixMilli()

    // 标准格式
    fmt.Println(timex.MsecFormat(ms))
    // Output: "2024-01-29 15:04:05"

    // 自定义格式
    fmt.Println(timex.MsecFormatWithLayout(ms, "2006-01-02"))
    // Output: "2024-01-29"

    fmt.Println(timex.MsecFormatWithLayout(ms, "15:04:05"))
    // Output: "15:04:05"
}
```

## 设计原则

### 1. 零外部依赖

**原则**：Lang 包只依赖 Go 标准库

**优势**：
- 减少依赖冲突
- 提高稳定性
- 降低维护成本

**已移除依赖**：
- ~~github.com/gogf/gf/v2~~ → 使用标准库替代

### 2. 失败不 panic

**原则**：转换失败返回零值，不抛出 panic

```go
// ✅ 好的设计
conv.Int("invalid")  // 返回 0，不 panic

// ❌ 避免的设计
mustInt("invalid")   // panic
```

### 3. 接口驱动

**原则**：支持自定义类型通过接口实现转换

```go
// 自定义类型实现 iString 接口
type User struct {
    Name string
}

func (u User) String() string {
    return u.Name
}

// 自动使用接口方法
conv.String(User{Name: "Alice"}) // "Alice"
```

## 性能考虑

### unsafe 操作

`stringx.BytesToString()` 和 `String2Bytes()` 使用 unsafe 指针：

**优点**：
- 零拷贝，性能极高
- 避免内存分配

**缺点**：
- 修改数据会导致未定义行为
- 需要确保数据生命周期正确

**使用场景**：
- ✅ 只读操作
- ✅ 性能关键路径
- ❌ 需要修改数据
- ❌ 数据生命周期不确定

### reflect 使用

`stringx.StringToSlice()` 使用反射：

**性能开销**：较大
**建议**：仅在必要时使用

## 迁移指南

### 从 GoFrame v2 迁移

如果你之前使用 GoFrame v2 的 gconv：

```go
// 旧代码
import "github.com/gogf/gf/v2/util/gconv"
gconv.String(123)
gconv.Int("456")

// 新代码
import "github.com/everyday-items/toolkit/lang/conv"
conv.String(123)
conv.Int("456")
```

**API 兼容性**：基本兼容，只需替换包名

## 更新日志

### v1.1 (2026-01-29) ⭐ **NEW**

**新增包**：
- ✅ **slicex** - 切片工具（Contains, Filter, Map, Unique, Reduce 等）
- ✅ **mathx** - 数学工具（泛型 Min/Max, Abs, Round 等）
- ✅ **syncx** - 并发工具（Singleflight, Pool, TypedPool）

**测试覆盖率**：
- ✅ **整体覆盖率 100%** ⬆️⬆️⬆️ 完美覆盖！
- ✅ **mathx: 100%** (从 83.7% 提升)
- ✅ **slicex: 100%** (从 46.7% 提升)
- ✅ **syncx: 100%** (新增)
- ✅ conv/stringx/timex: 保持 100%

**改进**：
- ✅ 所有包都有完整的单元测试和边界测试
- ✅ 所有包都有 Benchmark 性能测试
- ✅ 完整的中文文档和使用示例

### v1.0

**核心包**：
- ✅ **conv** - 类型转换
- ✅ **stringx** - 字符串工具
- ✅ **timex** - 时间工具

## 后续计划

### 优先级 1（进行中）
- [ ] 为 conv/stringx/timex 补充测试到 100%
- [x] 添加 slicex 包 ✅
- [x] 添加 mathx 包 ✅
- [x] 添加 syncx 包 ✅

### 优先级 2（计划中）
- [ ] 添加更多 slice 工具（Partition, Zip 等）
- [ ] 添加更多时间工具函数
- [ ] 性能优化和 Benchmark 对比

### 优先级 3（考虑中）
- [ ] 添加字符串模板功能
- [ ] 考虑添加更多并发原语（如果有需求）

## License

MIT
