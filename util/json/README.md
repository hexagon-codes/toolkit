# JSON 辅助工具

简化 JSON 操作的便捷工具包，提供易用的 JSON 序列化和反序列化函数。

## 特性

- ✅ 简化的 JSON 序列化/反序列化
- ✅ 忽略错误的便捷函数
- ✅ Must 函数（失败时 panic）
- ✅ JSON 美化和压缩
- ✅ JSON 验证
- ✅ 快速转换为 Map/Slice
- ✅ 零外部依赖

## 快速开始

### JSON 序列化

```go
import "github.com/everyday-items/toolkit/util/json"

type User struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}

user := User{Name: "Alice", Age: 30}

// 序列化（忽略错误）
str := json.Marshal(user)
// 输出: {"name":"Alice","age":30}

// 序列化（失败时 panic）
str := json.MustMarshal(user)

// 美化输出
str := json.MarshalIndent(user)
// 输出:
// {
//   "name": "Alice",
//   "age": 30
// }
```

### JSON 反序列化

```go
jsonStr := `{"name":"Alice","age":30}`

// 反序列化
var user User
err := json.Unmarshal(jsonStr, &user)
if err != nil {
    log.Fatal(err)
}

// 反序列化（失败时 panic）
var user User
json.MustUnmarshal(jsonStr, &user)

// 反序列化字节数组
data := []byte(jsonStr)
err = json.UnmarshalBytes(data, &user)
```

### JSON 验证

```go
// 验证 JSON 是否合法
valid := json.Valid(`{"name":"Alice"}`)  // true
valid := json.Valid(`{invalid}`)         // false

// 验证字节数组
data := []byte(`{"name":"Alice"}`)
valid := json.ValidBytes(data)
```

### JSON 美化和压缩

```go
// 美化 JSON
ugly := `{"name":"Alice","age":30}`
pretty := json.Pretty(ugly)
// 输出:
// {
//   "name": "Alice",
//   "age": 30
// }

// 压缩 JSON
pretty := `{
  "name": "Alice",
  "age": 30
}`
compact := json.Compact(pretty)
// 输出: {"name":"Alice","age":30}
```

### 转换为 Map/Slice

```go
// JSON 转 Map
jsonStr := `{"name":"Alice","age":30}`
m, err := json.ToMap(jsonStr)
// m = map[string]any{"name": "Alice", "age": 30}

// JSON 转 Map（失败时 panic）
m := json.MustToMap(jsonStr)

// JSON 转 Slice
jsonStr := `[1, 2, 3, 4, 5]`
s, err := json.ToSlice(jsonStr)
// s = []any{1, 2, 3, 4, 5}

// JSON 转 Slice（失败时 panic）
s := json.MustToSlice(jsonStr)
```

### 快速打印

```go
// 美化打印到控制台
user := User{Name: "Alice", Age: 30}
json.Print(user)
// 输出:
// {
//   "name": "Alice",
//   "age": 30
// }
```

## API 文档

### 序列化函数

```go
// Marshal JSON序列化（忽略错误）
Marshal(v any) string

// MustMarshal JSON序列化，失败时panic
MustMarshal(v any) string

// MarshalIndent JSON序列化（美化输出）
MarshalIndent(v any) string

// MustMarshalIndent JSON序列化（美化输出），失败时panic
MustMarshalIndent(v any) string
```

### 反序列化函数

```go
// Unmarshal JSON反序列化
Unmarshal(data string, v any) error

// MustUnmarshal JSON反序列化，失败时panic
MustUnmarshal(data string, v any)

// UnmarshalBytes JSON反序列化（字节数组）
UnmarshalBytes(data []byte, v any) error
```

### 验证函数

```go
// Valid 验证JSON是否合法
Valid(data string) bool

// ValidBytes 验证JSON是否合法（字节数组）
ValidBytes(data []byte) bool
```

### 格式化函数

```go
// Pretty 美化JSON字符串
Pretty(data string) string

// PrettyBytes 美化JSON字节数组
PrettyBytes(data []byte) []byte

// Compact 压缩JSON字符串
Compact(data string) string

// CompactBytes 压缩JSON字节数组
CompactBytes(data []byte) []byte
```

### 转换函数

```go
// ToMap JSON字符串转Map
ToMap(data string) (map[string]any, error)

// MustToMap JSON字符串转Map，失败时panic
MustToMap(data string) map[string]any

// ToSlice JSON字符串转Slice
ToSlice(data string) ([]any, error)

// MustToSlice JSON字符串转Slice，失败时panic
MustToSlice(data string) []any
```

### 工具函数

```go
// Print 打印JSON（美化输出）
Print(v any)
```

## 使用场景

### 1. HTTP 响应

```go
func HandleRequest(c *gin.Context) {
    user := GetUser(123)

    // 快速序列化并返回
    c.String(200, json.Marshal(user))
}

// 或使用 gin 自带的 JSON
func HandleRequest(c *gin.Context) {
    user := GetUser(123)
    c.JSON(200, user)
}
```

### 2. 日志记录

```go
func LogEvent(event string, data any) {
    // 将结构体转为 JSON 记录日志
    jsonStr := json.MarshalIndent(data)
    log.Printf("[%s] %s", event, jsonStr)
}

// 使用
user := User{Name: "Alice", Age: 30}
LogEvent("USER_CREATED", user)
// 输出:
// [USER_CREATED] {
//   "name": "Alice",
//   "age": 30
// }
```

### 3. 配置文件解析

```go
func LoadConfig(path string) (*Config, error) {
    // 读取配置文件
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    // 验证 JSON 格式
    if !json.ValidBytes(data) {
        return nil, errors.New("invalid JSON format")
    }

    // 解析配置
    var config Config
    if err := json.UnmarshalBytes(data, &config); err != nil {
        return nil, err
    }

    return &config, nil
}
```

### 4. API 请求参数解析

```go
func ParseRequestBody(c *gin.Context) (*Request, error) {
    // 读取请求体
    body, err := io.ReadAll(c.Request.Body)
    if err != nil {
        return nil, err
    }

    // 验证 JSON 格式
    if !json.ValidBytes(body) {
        return nil, errors.New("invalid JSON")
    }

    // 解析请求
    var req Request
    if err := json.UnmarshalBytes(body, &req); err != nil {
        return nil, err
    }

    return &req, nil
}
```

### 5. 数据缓存

```go
type Cache struct {
    store map[string]string
}

// 存储对象到缓存
func (c *Cache) Set(key string, value any) {
    jsonStr := json.Marshal(value)
    c.store[key] = jsonStr
}

// 从缓存读取对象
func (c *Cache) Get(key string, v any) error {
    jsonStr, ok := c.store[key]
    if !ok {
        return errors.New("key not found")
    }

    return json.Unmarshal(jsonStr, v)
}

// 使用
cache := &Cache{store: make(map[string]string)}
user := User{Name: "Alice", Age: 30}

cache.Set("user:123", user)

var loadedUser User
cache.Get("user:123", &loadedUser)
```

### 6. 动态 JSON 处理

```go
// 将 JSON 转为 Map 进行动态处理
jsonStr := `{"name":"Alice","age":30,"city":"New York"}`
m := json.MustToMap(jsonStr)

// 动态访问字段
name := m["name"].(string)
age := int(m["age"].(float64))

// 修改字段
m["age"] = 31
m["email"] = "alice@example.com"

// 转回 JSON
updatedJSON := json.MustMarshal(m)
```

### 7. JSON 美化工具

```go
func BeautifyJSON(input string) {
    // 验证 JSON
    if !json.Valid(input) {
        fmt.Println("Invalid JSON")
        return
    }

    // 美化输出
    pretty := json.Pretty(input)
    fmt.Println(pretty)
}

// 使用
ugly := `{"name":"Alice","age":30,"address":{"city":"New York","zip":"10001"}}`
BeautifyJSON(ugly)
// 输出:
// {
//   "name": "Alice",
//   "age": 30,
//   "address": {
//     "city": "New York",
//     "zip": "10001"
//   }
// }
```

### 8. 测试数据生成

```go
func TestUserAPI(t *testing.T) {
    // 创建测试数据
    user := User{Name: "Alice", Age: 30}

    // 序列化为 JSON
    payload := json.MustMarshal(user)

    // 发送请求
    resp, err := http.Post("/api/users", "application/json", strings.NewReader(payload))
    if err != nil {
        t.Fatal(err)
    }

    // 解析响应
    var result User
    body, _ := io.ReadAll(resp.Body)
    json.UnmarshalBytes(body, &result)

    // 验证结果
    if result.Name != user.Name {
        t.Errorf("Expected %s, got %s", user.Name, result.Name)
    }
}
```

## Must 函数说明

`Must*` 函数在失败时会 panic，适合以下场景：

```go
// ✅ 适合：初始化阶段（失败应该立即停止）
var config = json.MustToMap(`{"host":"localhost","port":8080}`)

// ✅ 适合：测试代码
func TestSomething(t *testing.T) {
    data := json.MustMarshal(testUser)
    // ...
}

// ❌ 不适合：运行时处理用户输入
func HandleUserInput(input string) {
    // 不要这样做！用户输入可能非法
    m := json.MustToMap(input)  // 可能 panic
}

// ✅ 应该这样：
func HandleUserInput(input string) error {
    m, err := json.ToMap(input)
    if err != nil {
        return err
    }
    // ...
}
```

## 性能

```
Marshal():         1000 ns/op
Unmarshal():       1500 ns/op
Valid():           500 ns/op
Pretty():          2000 ns/op
Compact():         1500 ns/op
```

性能与标准库 `encoding/json` 基本一致。

## 注意事项

1. **错误处理**：
   - `Marshal()` 等函数忽略错误，失败返回空字符串
   - `MustMarshal()` 等函数失败时 panic
   - 生产代码建议使用标准库或检查错误

2. **类型转换**：
   - `ToMap()` 返回 `map[string]any`
   - 数字会被解析为 `float64`
   - 需要手动类型断言

3. **内存占用**：
   - `Marshal()` 会创建新的字符串
   - 大对象序列化会消耗较多内存

4. **并发安全**：
   - 所有函数都是无状态的
   - 可安全并发调用

5. **空值处理**：
   - `Marshal(nil)` 返回 `"null"`
   - `Unmarshal("null", &v)` 不会报错

## 依赖

```bash
# 零外部依赖，仅使用标准库
import (
    "bytes"
    "encoding/json"
    "fmt"
)
```

## 对比标准库

| 特性 | 本包 | encoding/json |
|------|------|---------------|
| 易用性 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |
| 错误处理 | 可选（Must* 或 忽略） | 必须处理 |
| 性能 | 相同 | 相同 |
| 功能 | 基本功能 | 全功能 |

**推荐**：
- ✅ 简单场景：使用本包（更方便）
- ✅ 复杂场景：使用标准库（更灵活）
- ✅ 测试代码：使用 Must* 函数

## 扩展建议

如需更高级的 JSON 操作，可考虑：
- `github.com/tidwall/gjson` - 快速 JSON 查询
- `github.com/json-iterator/go` - 高性能 JSON 库
- `github.com/valyala/fastjson` - 零分配 JSON 解析
