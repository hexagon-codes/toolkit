# Stringx 高性能字符串工具

提供零拷贝的字符串转换工具，适用于性能关键路径。

## 特性

- ✅ 零拷贝转换 - 使用 unsafe 实现极致性能
- ✅ 类型转换 - 任意切片/数组转通用切片
- ✅ 安全保证 - 明确的使用警告和边界条件
- ✅ 零外部依赖 - 只使用 Go 标准库
- ✅ 完整测试 - 包含性能基准测试

## 快速开始

### 基础使用

```go
package main

import (
    "github.com/everyday-items/toolkit/lang/stringx"
)

func main() {
    // []byte 转 string（零拷贝）
    b := []byte("hello world")
    s := stringx.BytesToString(b)

    // string 转 []byte（零拷贝）
    str := "hello world"
    bytes := stringx.String2Bytes(str)
}
```

### 切片转换

```go
// 任意类型切片转 []any
strSlice := []string{"apple", "banana", "cherry"}
result := stringx.StringToSlice(strSlice)
// result = []any{"apple", "banana", "cherry"}

// 整数切片
intSlice := []int{1, 2, 3, 4, 5}
result := stringx.StringToSlice(intSlice)
// result = []any{1, 2, 3, 4, 5}
```

## 核心函数

### 1. BytesToString - 零拷贝 []byte 转 string

将 `[]byte` 零拷贝转换为 `string`。

```go
func BytesToString(b []byte) string
```

**特点**：
- 零内存分配
- 零数据拷贝
- 性能比标准转换快 10-100 倍

**使用场景**：
- ✅ 从网络读取数据后只读取
- ✅ 解析协议、JSON 等只读操作
- ✅ 日志输出、字符串比较
- ❌ 需要修改原始 []byte
- ❌ []byte 生命周期短暂

**示例**：

```go
// ✅ 正确用法：只读操作
data := []byte("hello world")
str := stringx.BytesToString(data)
fmt.Println(str)  // 安全：只读取

// ❌ 错误用法：修改原始数据
data := []byte("hello")
str := stringx.BytesToString(data)
data[0] = 'H'  // 危险！会修改 str 的内容
fmt.Println(str)  // 输出 "Hello"（原本是 "hello"）
```

**性能对比**：

```
BenchmarkBytesToString/unsafe-8       1000000000    0.25 ns/op    0 B/op    0 allocs/op
BenchmarkBytesToString/standard-8      50000000    28.5 ns/op    48 B/op    1 allocs/op
```

### 2. String2Bytes - 零拷贝 string 转 []byte

将 `string` 零拷贝转换为 `[]byte`。

```go
func String2Bytes(s string) []byte
```

**特点**：
- 零内存分配
- 零数据拷贝
- 性能比标准转换快 10-100 倍

**使用场景**：
- ✅ 传递给只读函数（如 hash.Write）
- ✅ 字节级比较操作
- ✅ 协议编码（只读取）
- ❌ 需要修改返回的 []byte
- ❌ 跨 goroutine 传递可变数据

**示例**：

```go
// ✅ 正确用法：只读操作
str := "hello world"
data := stringx.String2Bytes(str)
n := len(data)  // 安全：只读取

// ✅ 正确用法：传递给只读函数
hash := sha256.New()
hash.Write(stringx.String2Bytes(str))  // 安全：Write 不会修改数据

// ❌ 错误用法：修改返回的数据
str := "hello"
data := stringx.String2Bytes(str)
data[0] = 'H'  // 危险！会导致 panic（string 是不可变的）
```

**性能对比**：

```
BenchmarkString2Bytes/unsafe-8        1000000000    0.25 ns/op    0 B/op    0 allocs/op
BenchmarkString2Bytes/standard-8       50000000    28.5 ns/op    48 B/op    1 allocs/op
```

### 3. StringToSlice - 切片类型转换

将任意类型的切片或数组转换为 `[]any`。

```go
func StringToSlice(arr any) []any
```

**特点**：
- 使用反射实现
- 支持切片和数组
- 类型安全检查

**使用场景**：
- ✅ 泛型操作前的类型转换
- ✅ 接口参数传递
- ✅ 动态类型处理
- ❌ 高性能路径（反射开销大）

**示例**：

```go
// 字符串切片
strSlice := []string{"apple", "banana", "cherry"}
result := stringx.StringToSlice(strSlice)
fmt.Println(result)  // [apple banana cherry]

// 整数切片
intSlice := []int{1, 2, 3, 4, 5}
result := stringx.StringToSlice(intSlice)
fmt.Println(result)  // [1 2 3 4 5]

// 数组也支持
arr := [3]string{"red", "green", "blue"}
result := stringx.StringToSlice(arr)
fmt.Println(result)  // [red green blue]

// 混合类型
mixed := []any{1, "hello", 3.14, true}
result := stringx.StringToSlice(mixed)
fmt.Println(result)  // [1 hello 3.14 true]

// 非切片类型返回 nil
result := stringx.StringToSlice("not a slice")
fmt.Println(result)  // nil
```

**注意事项**：
- 输入非切片/数组类型会打印错误信息并返回 nil
- 使用反射，性能开销较大
- 返回新切片，不是零拷贝

## 零拷贝技术详解

### 什么是零拷贝

零拷贝（Zero-Copy）是指在数据转换过程中，不分配新内存、不复制数据，而是直接复用原始数据的底层内存。

**标准转换**：
```go
// string -> []byte（标准方式）
str := "hello"
b := []byte(str)  // 分配新内存，复制数据
```

**零拷贝转换**：
```go
// string -> []byte（零拷贝）
str := "hello"
b := stringx.String2Bytes(str)  // 不分配内存，不复制数据
```

### unsafe 包的使用

本包使用 Go 1.20+ 的新 API：
- `unsafe.String()` - 从 `*byte` 和长度构造 string
- `unsafe.StringData()` - 获取 string 的底层 `*byte`
- `unsafe.Slice()` - 从指针和长度构造切片

这些 API 比旧的 `(*reflect.StringHeader)` 和 `(*reflect.SliceHeader)` 更安全、更高效。

### 性能优势

**小数据（48 字节）**：
- 零拷贝：0.25 ns/op，0 次分配
- 标准方式：28.5 ns/op，1 次分配
- **快约 100 倍**

**大数据（1MB）**：
- 零拷贝：0.25 ns/op，0 次分配
- 标准方式：~50 µs/op，1 次分配
- **快约 200,000 倍**

### 何时使用零拷贝

#### ✅ 适合使用的场景

1. **只读操作**
```go
// HTTP 响应体转 string
body, _ := io.ReadAll(resp.Body)
str := stringx.BytesToString(body)
fmt.Println(str)  // 只读取，安全
```

2. **性能关键路径**
```go
// 高频调用的序列化函数
func serialize(data []byte) error {
    str := stringx.BytesToString(data)
    return json.Unmarshal([]byte(str), &result)
}
```

3. **临时转换**
```go
// 传递给只读函数
hash := sha256.New()
hash.Write(stringx.String2Bytes(str))
```

4. **数据生命周期明确**
```go
// 数据在整个函数周期内有效
func process(data []byte) {
    str := stringx.BytesToString(data)
    // ... 使用 str，不修改 data
}
```

#### ❌ 不适合使用的场景

1. **需要修改数据**
```go
// ❌ 错误：修改会影响原数据
data := []byte("hello")
str := stringx.BytesToString(data)
data[0] = 'H'  // str 也变成了 "Hello"
```

2. **跨 goroutine 传递**
```go
// ❌ 危险：数据竞争风险
go func() {
    data[0] = 'X'  // goroutine 1 修改
}()
str := stringx.BytesToString(data)  // goroutine 2 读取
```

3. **数据生命周期不确定**
```go
// ❌ 危险：data 可能被回收
var str string
{
    data := []byte("temporary")
    str = stringx.BytesToString(data)  // data 超出作用域后可能无效
}
fmt.Println(str)  // 可能崩溃
```

4. **不信任的数据源**
```go
// ❌ 不安全：外部可能修改数据
str := stringx.BytesToString(externalBuffer)
// 如果 external 修改 buffer，str 会变化
```

### 安全使用原则

1. **只读原则**：转换后不修改原始数据
2. **生命周期原则**：确保数据在使用期间有效
3. **单线程原则**：避免并发访问和修改
4. **信任原则**：只对受控数据使用零拷贝

## 使用场景

### 1. HTTP 请求处理

```go
func handleRequest(w http.ResponseWriter, r *http.Request) {
    // 读取请求体
    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }

    // 零拷贝转换（只读操作）
    bodyStr := stringx.BytesToString(body)

    // 解析 JSON
    var req Request
    if err := json.Unmarshal(body, &req); err != nil {
        http.Error(w, err.Error(), 400)
        return
    }

    // 日志输出
    log.Printf("Received: %s", bodyStr)
}
```

### 2. 序列化和反序列化

```go
// JSON 序列化
func SerializeJSON(v any) (string, error) {
    data, err := json.Marshal(v)
    if err != nil {
        return "", err
    }
    // 零拷贝转换
    return stringx.BytesToString(data), nil
}

// JSON 反序列化
func DeserializeJSON(str string, v any) error {
    // 零拷贝转换
    data := stringx.String2Bytes(str)
    return json.Unmarshal(data, v)
}
```

### 3. 哈希计算

```go
func HashString(str string) string {
    h := sha256.New()
    // 零拷贝转换（Write 不会修改数据）
    h.Write(stringx.String2Bytes(str))
    return hex.EncodeToString(h.Sum(nil))
}
```

### 4. 协议解析

```go
// 解析二进制协议
func parseProtocol(data []byte) (*Message, error) {
    // 零拷贝转换消息头
    header := stringx.BytesToString(data[:16])

    // 解析消息类型
    msgType := header[0:4]
    msgLen := binary.BigEndian.Uint32(data[4:8])

    // 解析消息体
    body := data[16:]
    return &Message{
        Type: msgType,
        Body: stringx.BytesToString(body),
    }, nil
}
```

### 5. 缓存键生成

```go
// 从多个字段生成缓存键
func generateCacheKey(parts ...string) string {
    var buf bytes.Buffer
    for _, part := range parts {
        buf.WriteString(part)
        buf.WriteByte(':')
    }

    // 零拷贝转换
    return stringx.BytesToString(buf.Bytes())
}
```

### 6. 切片类型转换

```go
// 处理多种类型的输入
func processItems(items any) error {
    // 转换为通用切片
    slice := stringx.StringToSlice(items)
    if slice == nil {
        return errors.New("invalid input: not a slice")
    }

    // 处理每个元素
    for i, item := range slice {
        fmt.Printf("Item %d: %v\n", i, item)
    }
    return nil
}

// 使用示例
processItems([]string{"a", "b", "c"})
processItems([]int{1, 2, 3})
processItems([3]float64{1.1, 2.2, 3.3})
```

## 最佳实践

### 1. 优先考虑安全性

```go
// ✅ 不确定时使用标准转换
str := string(data)

// ✅ 确定安全时使用零拷贝
str := stringx.BytesToString(data)
```

### 2. 在性能关键路径使用

```go
// ✅ 高频调用的函数
func processLoop(items [][]byte) {
    for _, item := range items {
        str := stringx.BytesToString(item)  // 零拷贝，高性能
        process(str)
    }
}

// ❌ 低频调用不值得冒险
func initialize(config []byte) {
    str := string(config)  // 标准转换，安全第一
}
```

### 3. 添加明确的注释

```go
// ✅ 说明为什么使用零拷贝
// 零拷贝转换，data 在此函数周期内不会被修改
str := stringx.BytesToString(data)
```

### 4. 测试覆盖

```go
func TestZeroCopy(t *testing.T) {
    // 测试正常情况
    data := []byte("test")
    str := stringx.BytesToString(data)
    if str != "test" {
        t.Errorf("expected 'test', got '%s'", str)
    }

    // 测试边界条件
    empty := []byte{}
    str = stringx.BytesToString(empty)
    if str != "" {
        t.Errorf("expected empty string, got '%s'", str)
    }
}
```

### 5. 性能基准测试

```go
func BenchmarkConversion(b *testing.B) {
    data := []byte("benchmark test data")

    b.Run("zero-copy", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            _ = stringx.BytesToString(data)
        }
    })

    b.Run("standard", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            _ = string(data)
        }
    })
}
```

### 6. StringToSlice 使用注意

```go
// ✅ 检查返回值
slice := stringx.StringToSlice(input)
if slice == nil {
    return errors.New("invalid input type")
}

// ✅ 避免在性能关键路径使用（反射开销大）
// 适合用于配置解析、初始化等低频操作
config := []string{"opt1", "opt2", "opt3"}
options := stringx.StringToSlice(config)
```

## 注意事项

### 1. 内存安全

- **不要修改**转换后共享内存的数据
- **不要**跨 goroutine 传递可变数据
- **确保**数据生命周期足够长

### 2. 性能权衡

- BytesToString 和 String2Bytes：极致性能，但需要谨慎
- StringToSlice：使用反射，性能开销较大

### 3. Go 版本要求

- 需要 Go 1.20+ 版本
- 使用了新的 unsafe API（`unsafe.String`, `unsafe.StringData`, `unsafe.Slice`）

### 4. 代码审查

使用零拷贝转换的代码需要特别注意：
- 在代码审查时重点检查
- 添加明确的注释说明安全性
- 确保有充分的测试覆盖

## 性能对比

### 小数据（48 字节）

```
BenchmarkBytesToString/unsafe-8       1000000000    0.25 ns/op    0 B/op    0 allocs/op
BenchmarkBytesToString/standard-8      50000000    28.5 ns/op    48 B/op    1 allocs/op

BenchmarkString2Bytes/unsafe-8        1000000000    0.25 ns/op    0 B/op    0 allocs/op
BenchmarkString2Bytes/standard-8       50000000    28.5 ns/op    48 B/op    1 allocs/op
```

### 大数据（1MB）

```
BenchmarkBytesToString_1MB-8    1000000000    0.25 ns/op    0 B/op    0 allocs/op
BenchmarkString2Bytes_1MB-8     1000000000    0.25 ns/op    0 B/op    0 allocs/op
```

**结论**：
- 零拷贝转换性能稳定，不受数据大小影响
- 数据越大，性能优势越明显
- 完全零内存分配

## 运行测试

```bash
# 运行所有测试
go test ./lang/stringx

# 运行基准测试
go test -bench=. ./lang/stringx

# 查看内存分配
go test -bench=. -benchmem ./lang/stringx

# 运行示例
go test -run=Example ./lang/stringx -v
```

## 参考资料

- [Go unsafe 包文档](https://pkg.go.dev/unsafe)
- [Go 1.20 Release Notes](https://go.dev/doc/go1.20)
- [零拷贝技术详解](https://en.wikipedia.org/wiki/Zero-copy)

## 总结

`lang/stringx` 提供了高性能的字符串转换工具：

- **BytesToString** 和 **String2Bytes**：零拷贝转换，极致性能，需要谨慎使用
- **StringToSlice**：通用切片转换，使用反射，适合非性能关键路径

**使用建议**：
- 在性能关键路径使用零拷贝转换
- 确保数据只读、生命周期明确
- 不确定时优先使用标准转换
- 添加清晰的注释和测试

**安全第一，性能第二。**
