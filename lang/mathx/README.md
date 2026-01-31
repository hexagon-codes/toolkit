# Mathx - 泛型数学工具

提供泛型版本的数学运算函数，支持所有数值类型。

## 特性

- ✅ 泛型支持 - 支持 int/float/string 等所有可比较类型
- ✅ Min/Max - 可变参数，一次比较多个值
- ✅ Abs/AbsDiff - 泛型绝对值计算
- ✅ Clamp - 值范围限制
- ✅ 四舍五入 - Round/RoundTo/Ceil/Floor/Trunc
- ✅ 零外部依赖 - 只使用 Go 标准库
- ✅ 类型安全 - 编译时类型检查

## 快速开始

### 基础操作

```go
import "github.com/everyday-items/toolkit/lang/mathx"

// Min/Max - 支持可变参数
min := mathx.Min(3, 1, 4, 1, 5)           // 1 (int)
max := mathx.Max(3.14, 2.71, 1.41)        // 3.14 (float64)
minStr := mathx.Min("c", "a", "b")        // "a" (string)

// MinMax - 同时返回最小值和最大值
min, max := mathx.MinMax(3, 1, 4, 1, 5)   // 1, 5

// Clamp - 限制值在范围内
clamped := mathx.Clamp(15, 0, 10)         // 10
clamped := mathx.Clamp(-5, 0, 10)         // 0
clamped := mathx.Clamp(5, 0, 10)          // 5

// Abs - 绝对值（泛型）
abs := mathx.Abs(-5)                      // 5 (int)
absf := mathx.Abs(-3.14)                  // 3.14 (float64)

// AbsDiff - 差的绝对值
diff := mathx.AbsDiff(5, 3)               // 2
diff := mathx.AbsDiff(3, 5)               // 2
```

### 四舍五入

```go
// Round - 四舍五入到整数
rounded := mathx.Round(3.14)              // 3.0
rounded := mathx.Round(3.5)               // 4.0

// RoundTo - 四舍五入到指定小数位
rounded := mathx.RoundTo(3.14159, 2)      // 3.14
rounded := mathx.RoundTo(123.456, 1)      // 123.5

// Ceil - 向上取整
ceiled := mathx.Ceil(3.14)                // 4.0

// Floor - 向下取整
floored := mathx.Floor(3.14)              // 3.0

// Trunc - 截断小数部分
truncated := mathx.Trunc(3.14)            // 3.0
truncated := mathx.Trunc(-3.14)           // -3.0
```

## API 文档

### 类型约束

```go
// Ordered - 可排序类型（支持 <、> 比较）
type Ordered interface {
    ~int | ~int8 | ~int16 | ~int32 | ~int64 |
    ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
    ~float32 | ~float64 |
    ~string
}

// Signed - 有符号数类型
type Signed interface {
    ~int | ~int8 | ~int16 | ~int32 | ~int64 |
    ~float32 | ~float64
}

// Float - 浮点数类型
type Float interface {
    ~float32 | ~float64
}
```

### 比较和限制

```go
// Min - 返回最小值
Min[T Ordered](values ...T) T

// Max - 返回最大值
Max[T Ordered](values ...T) T

// MinMax - 同时返回最小值和最大值
MinMax[T Ordered](values ...T) (T, T)

// Clamp - 将值限制在范围内
Clamp[T Ordered](value, min, max T) T
```

### 绝对值

```go
// Abs - 返回绝对值
Abs[T Signed](value T) T

// AbsDiff - 返回两数差的绝对值
AbsDiff[T Signed](a, b T) T
```

### 四舍五入

```go
// Round - 四舍五入到整数
Round(value float64) float64

// RoundTo - 四舍五入到指定小数位
RoundTo(value float64, decimals int) float64

// Ceil - 向上取整
Ceil(value float64) float64

// Floor - 向下取整
Floor(value float64) float64

// Trunc - 截断小数部分
Trunc(value float64) float64
```

## 使用场景

### 1. 通用 Min/Max（替代标准库）

```go
// 标准库 math 包只支持 float64
import "math"
min := math.Min(3.14, 2.71)  // 只能用于 float64

// mathx 支持所有类型
import "github.com/everyday-items/toolkit/lang/mathx"
min := mathx.Min(3, 1, 4)                  // int
min := mathx.Min(3.14, 2.71, 1.41)        // float64
min := mathx.Min("c", "a", "b")           // string
min := mathx.Min(time.Second, time.Minute) // time.Duration
```

### 2. 批量比较（可变参数）

```go
// 标准库需要多次调用
min := math.Min(math.Min(a, b), c)

// mathx 一次调用
min := mathx.Min(a, b, c, d, e)

// 实用示例
prices := []float64{99.99, 149.99, 79.99, 199.99}
minPrice := mathx.Min(prices...)  // 79.99
maxPrice := mathx.Max(prices...)  // 199.99
```

### 3. 同时获取最小值和最大值

```go
// 一次遍历获取 min 和 max
scores := []int{85, 92, 78, 95, 88}
minScore, maxScore := mathx.MinMax(scores...)
fmt.Printf("分数范围: %d - %d\n", minScore, maxScore)
```

### 4. 值范围限制

```go
// 限制用户输入
age := mathx.Clamp(userAge, 0, 150)          // 年龄 0-150
percentage := mathx.Clamp(value, 0.0, 100.0) // 百分比 0-100

// 限制颜色值
r := mathx.Clamp(red, 0, 255)
g := mathx.Clamp(green, 0, 255)
b := mathx.Clamp(blue, 0, 255)

// 限制价格
price := mathx.Clamp(userPrice, minPrice, maxPrice)
```

### 5. 距离计算

```go
// 计算坐标距离
dx := mathx.AbsDiff(x1, x2)
dy := mathx.AbsDiff(y1, y2)

// 计算时间差（绝对值）
diff := mathx.AbsDiff(time1.Unix(), time2.Unix())
hours := float64(diff) / 3600

// 计算价格差异
priceDiff := mathx.AbsDiff(price1, price2)
```

### 6. 数值处理

```go
// 四舍五入价格
price := 99.999
finalPrice := mathx.RoundTo(price, 2)  // 100.00

// 向上取整（页数）
totalPages := mathx.Ceil(float64(totalItems) / float64(pageSize))

// 向下取整（折扣）
discount := mathx.Floor(price * 0.9)

// 截断小数（税费）
tax := mathx.Trunc(price * 0.13)
```

### 7. 统计分析

```go
// 计算范围
values := []float64{1.2, 3.4, 2.1, 5.6, 4.3}
min, max := mathx.MinMax(values...)
dataRange := max - min

// 归一化到 [0, 1]
normalized := make([]float64, len(values))
for i, v := range values {
    normalized[i] = (v - min) / dataRange
}

// 限制异常值
cleaned := make([]float64, len(values))
for i, v := range values {
    cleaned[i] = mathx.Clamp(v, min, max)
}
```

### 8. 游戏开发

```go
// 限制玩家位置
playerX := mathx.Clamp(newX, 0, mapWidth)
playerY := mathx.Clamp(newY, 0, mapHeight)

// 限制生命值
health := mathx.Clamp(currentHealth-damage, 0, maxHealth)

// 计算伤害（绝对值）
damage := mathx.Abs(attackPower - defense)

// 计算距离
distance := mathx.AbsDiff(playerPos, enemyPos)
```

### 9. 配置验证

```go
type Config struct {
    Port        int
    Timeout     time.Duration
    MaxConns    int
    CacheSize   int
}

func ValidateConfig(cfg *Config) *Config {
    // 限制端口范围
    cfg.Port = mathx.Clamp(cfg.Port, 1024, 65535)

    // 限制超时时间
    cfg.Timeout = mathx.Clamp(cfg.Timeout, time.Second, time.Minute)

    // 限制连接数
    cfg.MaxConns = mathx.Clamp(cfg.MaxConns, 10, 10000)

    // 限制缓存大小
    cfg.CacheSize = mathx.Clamp(cfg.CacheSize, 100, 100000)

    return cfg
}
```

### 10. 图形计算

```go
// 计算矩形边界
type Rect struct {
    X, Y, Width, Height float64
}

func (r Rect) Left() float64   { return r.X }
func (r Rect) Right() float64  { return r.X + r.Width }
func (r Rect) Top() float64    { return r.Y }
func (r Rect) Bottom() float64 { return r.Y + r.Height }

func Intersect(r1, r2 Rect) Rect {
    left := mathx.Max(r1.Left(), r2.Left())
    right := mathx.Min(r1.Right(), r2.Right())
    top := mathx.Max(r1.Top(), r2.Top())
    bottom := mathx.Min(r1.Bottom(), r2.Bottom())

    if left < right && top < bottom {
        return Rect{left, top, right - left, bottom - top}
    }
    return Rect{} // 无交集
}
```

## 与标准库对比

### 标准库 math 包

```go
import "math"

// 只支持 float64
min := math.Min(3.14, 2.71)
max := math.Max(3.14, 2.71)
abs := math.Abs(-3.14)

// 不支持可变参数
min3 := math.Min(math.Min(a, b), c)

// 不支持其他类型
// min := math.Min(1, 2)  // 编译错误
```

### mathx 包

```go
import "github.com/everyday-items/toolkit/lang/mathx"

// 支持泛型
min := mathx.Min(3, 1, 4)           // int
min := mathx.Min(3.14, 2.71)        // float64
min := mathx.Min("a", "b")          // string

// 支持可变参数
min := mathx.Min(a, b, c, d, e)

// 一次获取 min 和 max
min, max := mathx.MinMax(1, 2, 3, 4, 5)
```

### 功能对比表

| 功能 | math 包 | mathx 包 |
|------|---------|----------|
| Min/Max | ✅ float64 only | ✅ 泛型（所有类型） |
| 可变参数 | ❌ 只支持 2 个 | ✅ 支持任意多个 |
| Abs | ✅ float64 only | ✅ 泛型（有符号数） |
| Clamp | ❌ 不支持 | ✅ 支持 |
| MinMax | ❌ 需两次调用 | ✅ 一次调用 |
| AbsDiff | ❌ 不支持 | ✅ 支持 |
| Round | ✅ | ✅ |
| RoundTo | ❌ | ✅ |

## 性能说明

### 内联优化

所有函数都很简单，编译器会自动内联，性能与手写代码相同：

```go
// 以下两种写法性能相同
min := mathx.Min(a, b)

// 等价于
min := a
if b < a {
    min = b
}
```

### 性能基准

```
Min (2 个值):      2 ns/op
Min (5 个值):      5 ns/op
Max (2 个值):      2 ns/op
MinMax (5 个值):   8 ns/op
Abs:              1 ns/op
Clamp:            3 ns/op
RoundTo:          15 ns/op
```

### 性能建议

1. **避免在循环中使用可变参数**：
   ```go
   // 不推荐
   for _, v := range values {
       min := mathx.Min(v, otherValues...)
   }

   // 推荐
   min := mathx.Min(values...)
   ```

2. **优先使用 MinMax**：
   ```go
   // 不推荐（两次遍历）
   min := mathx.Min(values...)
   max := mathx.Max(values...)

   // 推荐（一次遍历）
   min, max := mathx.MinMax(values...)
   ```

3. **性能关键路径使用标准库**：
   ```go
   // 极端性能场景（仅 float64）
   import "math"
   min := math.Min(a, b)  // 略快于泛型版本

   // 通用场景（推荐）
   import "github.com/everyday-items/toolkit/lang/mathx"
   min := mathx.Min(a, b)  // 类型安全 + 可读性更好
   ```

## 设计原则

1. **类型安全**：使用泛型约束，编译时检查类型
2. **API 简洁**：与标准库保持一致的命名和语义
3. **零依赖**：只使用 Go 标准库
4. **性能优先**：函数简单，易于内联

## 注意事项

1. **空参数处理**：
   ```go
   min := mathx.Min()  // 返回类型的零值
   ```

2. **浮点数精度**：
   ```go
   // 浮点数运算遵循 IEEE 754 标准
   result := mathx.RoundTo(0.1+0.2, 1)  // 0.3（可能有精度误差）
   ```

3. **泛型约束**：
   ```go
   // Ordered 类型（支持 < > 比较）
   min := mathx.Min(1, 2, 3)           // ✅ int
   min := mathx.Min(1.0, 2.0)          // ✅ float64
   min := mathx.Min("a", "b")          // ✅ string

   // Signed 类型（有符号数）
   abs := mathx.Abs(-5)                // ✅ int
   abs := mathx.Abs(-3.14)             // ✅ float64
   // abs := mathx.Abs(uint(5))        // ❌ 编译错误（uint 是无符号数）
   ```

4. **并发安全**：
   - 所有函数都是纯函数，无状态
   - 可以安全地在多个 goroutine 中使用

## 依赖

```bash
# 仅依赖标准库 math 包
import "math"
```

## 扩展建议

如需更多数学功能，可考虑：
- `math` - Go 标准库（三角函数、对数、指数等）
- `math/big` - 大数运算
- `gonum.org/v1/gonum/mathext` - 扩展数学函数
- `github.com/shopspring/decimal` - 高精度十进制运算
