# Conv 类型转换工具

提供全面的 Go 类型转换功能，支持基础类型互转、JSON/Map 操作等常用场景。

## 特性

- ✅ 零外部依赖（仅使用 Go 标准库）
- ✅ 接口驱动的类型转换（支持自定义类型）
- ✅ 失败不 panic（返回零值）
- ✅ 智能类型推断
- ✅ 基础类型转换（String/Int/Uint/Float/Bool）
- ✅ JSON-Map 双向转换
- ✅ Map 操作（合并、提取 key/value）
- ✅ 并发安全（纯函数，无状态）

## 快速开始

### 基础类型转换

```go
import "github.com/everyday-items/toolkit/lang/conv"

// 字符串转换
s := conv.String(123)          // "123"
s := conv.String(3.14)         // "3.14"
s := conv.String(true)         // "true"
s := conv.String([]byte("hi")) // "hi"

// 整数转换
i := conv.Int("456")           // 456
i := conv.Int(3.99)            // 3 (浮点截断)
i := conv.Int(true)            // 1
i := conv.Int("invalid")       // 0 (失败返回零值)

// 浮点数转换
f := conv.Float64("3.14")      // 3.14
f := conv.Float64(123)         // 123.0

// 布尔转换
b := conv.Bool(1)              // true
b := conv.Bool(0)              // false
b := conv.Bool("true")         // true
b := conv.Bool("yes")          // true
```

### JSON-Map 互转

```go
// JSON 转 Map
m, err := conv.JSONToMap(`{"name":"Alice","age":30}`)
// m = map[string]any{"name": "Alice", "age": 30}

// Map 转 JSON
json, err := conv.MapToJSON(map[string]any{"name": "Bob"})
// json = `{"name":"Bob"}`
```

### Map 操作

```go
// 合并 Map（后面的覆盖前面的）
m1 := map[string]any{"a": 1, "b": 2}
m2 := map[string]any{"b": 3, "c": 4}
merged := conv.MergeMaps(m1, m2)
// merged = map[string]any{"a": 1, "b": 3, "c": 4}

// 提取所有 key
keys := conv.MapKeys(m)
// keys = []string{"a", "b", "c"}

// 提取所有 value
values := conv.MapValues(m)
// values = []any{1, 3, 4}
```

## API 文档

### 字符串转换

| 函数 | 说明 | 失败返回 |
|------|------|---------|
| `String(any)` | 任意类型转字符串 | `""` |

**支持类型**：
- 基础类型：`int`, `uint`, `float`, `bool`, `string`, `[]byte`
- 接口：`iString` (调用 `String()` 方法)
- 其他：使用 `fmt.Sprintf("%v", value)`

### 整数转换

| 函数 | 说明 | 失败返回 |
|------|------|---------|
| `Int(any)` | 任意类型转 int | `0` |
| `Int32(any)` | 任意类型转 int32 | `0` |
| `Int64(any)` | 任意类型转 int64 | `0` |
| `Uint(any)` | 任意类型转 uint | `0` |
| `Uint32(any)` | 任意类型转 uint32 | `0` |
| `Uint64(any)` | 任意类型转 uint64 | `0` |

**转换规则**：
- 字符串解析为十进制
- 浮点数截断小数部分
- `bool`: `true=1`, `false=0`

### 浮点数转换

| 函数 | 说明 | 失败返回 |
|------|------|---------|
| `Float32(any)` | 任意类型转 float32 | `0.0` |
| `Float64(any)` | 任意类型转 float64 | `0.0` |

**特殊功能**：
- 支持 `[]byte` 二进制解码（小端序）
- 支持接口：`iFloat32`, `iFloat64`

### 布尔转换

| 函数 | 说明 | 失败返回 |
|------|------|---------|
| `Bool(any)` | 任意类型转布尔 | `false` |

**转换规则**：
- 数字：`0` = `false`，其他 = `true`
- 字符串：`"true"`, `"1"`, `"yes"`, `"on"` = `true`（不区分大小写）

### JSON/Map 操作

| 函数 | 说明 |
|------|------|
| `JSONToMap(string)` | JSON 字符串转 Map |
| `MapToJSON(map)` | Map 转 JSON 字符串 |
| `MergeMaps(...map)` | 合并多个 Map |
| `MapKeys(map)` | 提取所有 key |
| `MapValues(map)` | 提取所有 value |

### 自定义类型转换接口

实现这些接口以支持自定义类型转换：

```go
type iString interface { String() string }
type iInt64 interface { Int64() int64 }
type iUint64 interface { Uint64() uint64 }
type iFloat32 interface { Float32() float32 }
type iFloat64 interface { Float64() float64 }
type iBool interface { Bool() bool }
```

## 使用场景

### 1. HTTP 请求参数解析

```go
// 解析 URL 参数
func GetUserByID(r *http.Request) (*User, error) {
    idStr := r.URL.Query().Get("id")
    id := conv.Int(idStr)  // 安全转换，失败返回 0

    if id == 0 {
        return nil, errors.New("invalid id")
    }

    return db.FindUser(id)
}
```

### 2. JSON 配置文件解析

```go
// 解析 JSON 配置
func LoadConfig(jsonStr string) (*Config, error) {
    m, err := conv.JSONToMap(jsonStr)
    if err != nil {
        return nil, err
    }

    return &Config{
        Host:     conv.String(m["host"]),
        Port:     conv.Int(m["port"]),
        Timeout:  conv.Int(m["timeout"]),
        Debug:    conv.Bool(m["debug"]),
        MaxConns: conv.Int(m["max_connections"]),
    }, nil
}
```

### 3. 动态数据处理

```go
// 处理动态类型数据
func ProcessDynamicData(data any) {
    switch v := data.(type) {
    case map[string]any:
        // 处理 Map
        for key, val := range v {
            fmt.Printf("%s: %s\n", key, conv.String(val))
        }
    default:
        // 处理其他类型
        fmt.Println(conv.String(data))
    }
}
```

### 4. 数据库字段映射

```go
// 从数据库行映射到结构体
func ScanUser(row *sql.Row) (*User, error) {
    var data map[string]any
    // 假设从数据库获取了 map 数据

    return &User{
        ID:        conv.Int64(data["id"]),
        Name:      conv.String(data["name"]),
        Age:       conv.Int(data["age"]),
        Email:     conv.String(data["email"]),
        IsActive:  conv.Bool(data["is_active"]),
        Balance:   conv.Float64(data["balance"]),
    }, nil
}
```

### 5. 配置合并

```go
// 合并默认配置和用户配置
func MergeConfig(defaults, user map[string]any) map[string]any {
    return conv.MergeMaps(defaults, user)  // user 覆盖 defaults
}

// 使用示例
defaults := map[string]any{"timeout": 30, "debug": false}
user := map[string]any{"debug": true, "host": "localhost"}
config := MergeConfig(defaults, user)
// 结果: {"timeout": 30, "debug": true, "host": "localhost"}
```

### 6. 日志字段提取

```go
// 从日志 Map 提取字段
func ExtractLogFields(logMap map[string]any) {
    fields := conv.MapKeys(logMap)

    fmt.Println("Available fields:", fields)

    for _, field := range fields {
        value := conv.String(logMap[field])
        fmt.Printf("%s: %s\n", field, value)
    }
}
```

### 7. API 响应转换

```go
// 将 API 响应转换为统一格式
func NormalizeAPIResponse(resp any) map[string]any {
    // 如果是字符串，尝试解析为 JSON
    if jsonStr, ok := resp.(string); ok {
        m, err := conv.JSONToMap(jsonStr)
        if err == nil {
            return m
        }
    }

    // 否则转为 Map
    return map[string]any{"data": resp}
}
```

### 8. 环境变量读取

```go
// 安全读取环境变量
func GetEnvInt(key string, defaultVal int) int {
    val := os.Getenv(key)
    if val == "" {
        return defaultVal
    }

    result := conv.Int(val)
    if result == 0 {
        return defaultVal
    }

    return result
}

// 使用示例
maxWorkers := GetEnvInt("MAX_WORKERS", 10)
timeout := GetEnvInt("TIMEOUT_SECONDS", 30)
```

### 9. 表单数据处理

```go
// 解析表单数据
func ParseForm(r *http.Request) (*FormData, error) {
    r.ParseForm()

    return &FormData{
        Name:      r.FormValue("name"),
        Age:       conv.Int(r.FormValue("age")),
        Email:     r.FormValue("email"),
        Subscribe: conv.Bool(r.FormValue("subscribe")),
        Amount:    conv.Float64(r.FormValue("amount")),
    }, nil
}
```

### 10. 缓存键值转换

```go
// Redis 缓存读取（Redis 返回的值通常是 string）
func GetFromCache(key string) (*CachedData, error) {
    val, err := redisClient.Get(ctx, key).Result()
    if err != nil {
        return nil, err
    }

    // JSON 字符串转 Map
    m, err := conv.JSONToMap(val)
    if err != nil {
        return nil, err
    }

    return &CachedData{
        ID:        conv.Int64(m["id"]),
        Value:     conv.String(m["value"]),
        Timestamp: conv.Int64(m["timestamp"]),
    }, nil
}
```

## 转换原则

所有转换函数按照以下顺序尝试转换：

1. **Nil 检查**：输入为 `nil` 时返回零值
2. **类型断言**：直接处理常见 Go 类型
3. **接口检查**：检查是否实现了转换接口（如 `iString`, `iFloat32`）
4. **降级转换**：使用标准库函数（`strconv`, `fmt`）
5. **失败返回零值**：转换失败不会 panic

## 注意事项

### 1. 失败不 panic

```go
// ✅ 安全：转换失败返回零值
i := conv.Int("invalid")  // 返回 0，不会 panic
f := conv.Float64("abc")  // 返回 0.0，不会 panic
```

### 2. 浮点数截断

```go
// 浮点数转整数会截断小数部分
conv.Int(3.99)   // 3 (不是 4)
conv.Int(-2.5)   // -2 (不是 -3)
```

### 3. 布尔转换

```go
// 字符串转布尔（不区分大小写）
conv.Bool("true")   // true
conv.Bool("TRUE")   // true
conv.Bool("1")      // true
conv.Bool("yes")    // true
conv.Bool("on")     // true
conv.Bool("false")  // false
conv.Bool("0")      // false
conv.Bool("no")     // false
conv.Bool("off")    // false
conv.Bool("other")  // false (未知字符串)
```

### 4. Map 顺序不保证

```go
// MapKeys 和 MapValues 返回顺序不确定
m := map[string]any{"a": 1, "b": 2, "c": 3}
keys := conv.MapKeys(m)
// keys 可能是 ["a", "b", "c"] 或 ["b", "c", "a"] 等
```

### 5. Map 合并规则

```go
// 后面的 Map 覆盖前面的重复键
m1 := map[string]any{"a": 1, "b": 2}
m2 := map[string]any{"b": 3}
result := conv.MergeMaps(m1, m2)
// result["b"] = 3 (m2 覆盖了 m1)
```

### 6. JSON 必须是对象

```go
// ✅ 正确：JSON 对象
m, err := conv.JSONToMap(`{"name":"Alice"}`)

// ❌ 错误：JSON 数组
m, err := conv.JSONToMap(`[1,2,3]`)  // 返回 error
```

### 7. 二进制解码格式

```go
// Float32/Float64 从 []byte 解码时使用小端序
bytes := []byte{...}  // 4 字节或 8 字节
f32 := conv.Float32(bytes)  // 小端序解码
f64 := conv.Float64(bytes)  // 小端序解码
```

### 8. 并发安全

```go
// ✅ 所有函数都是并发安全的（纯函数，无状态）
go func() { conv.Int("123") }()
go func() { conv.String(456) }()
```

## 自定义类型转换示例

```go
// 自定义类型实现转换接口
type UserID int64

func (u UserID) Int64() int64 {
    return int64(u)
}

func (u UserID) String() string {
    return fmt.Sprintf("user_%d", u)
}

// 使用示例
uid := UserID(1001)
conv.Int64(uid)   // 1001
conv.String(uid)  // "user_1001"
```

## 性能考虑

### 高性能场景

```go
// ✅ 直接类型断言（最快）
if s, ok := value.(string); ok {
    // 使用 s
}

// ✅ 类型已知时使用标准库
strconv.Atoi(str)
strconv.ParseFloat(str, 64)
```

### 通用场景

```go
// ✅ 类型不确定时使用 conv（安全 + 便利）
i := conv.Int(value)  // value 可能是 string/int/float/any
```

### 避免

```go
// ❌ 避免在热路径中频繁转换
for i := 0; i < 1000000; i++ {
    conv.String(i)  // 性能敏感
}

// ✅ 优化：批量转换
strs := make([]string, 1000000)
for i := 0; i < 1000000; i++ {
    strs[i] = strconv.Itoa(i)  // 直接使用标准库
}
```

## 零外部依赖

```
只依赖 Go 标准库:
- encoding/json     # JSON 编解码
- encoding/binary   # 二进制编码
- strconv           # 字符串转换
- fmt               # 格式化
- math              # 数学函数
```

## 与其他库对比

| 库 | 依赖 | 失败处理 | 接口支持 | JSON/Map |
|----|------|---------|---------|----------|
| **conv** | 零依赖 | 返回零值 | ✅ | ✅ |
| cast (spf13) | 零依赖 | 返回零值 | ❌ | ❌ |
| gconv (goframe) | 大量依赖 | 返回零值 | ✅ | ✅ |
| 标准库 strconv | 零依赖 | 返回 error | ❌ | ❌ |

## 最佳实践

### ✅ 推荐做法

```go
// ✅ 用于类型不确定的场景
func Process(data any) {
    i := conv.Int(data)
    // 安全，失败返回 0
}

// ✅ API 参数解析
id := conv.Int(r.URL.Query().Get("id"))

// ✅ 配置文件解析
config, _ := conv.JSONToMap(jsonStr)
timeout := conv.Int(config["timeout"])

// ✅ 合并配置
finalConfig := conv.MergeMaps(defaults, userConfig)
```

### ❌ 不推荐做法

```go
// ❌ 类型已知时不必要使用
var i int = 123
s := conv.String(i)  // 不必要，直接用 strconv.Itoa(i)

// ❌ 需要错误处理时不适合
i := conv.Int(str)  // 无法区分 "0" 和转换失败
// 应该用: i, err := strconv.Atoi(str)

// ❌ 性能关键路径
for i := 0; i < 1000000; i++ {
    conv.Float64(arr[i])  // 太慢
}
```

## 贡献

欢迎提交 Issue 和 Pull Request！

## 许可证

MIT License
