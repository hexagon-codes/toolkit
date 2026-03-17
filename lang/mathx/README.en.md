[中文](README.md) | English

# Mathx - Generic Math Utilities

Provides generic versions of mathematical functions, supporting all numeric types.

## Features

- ✅ Generics support - Supports int/float/string and all comparable types
- ✅ Min/Max - Variadic arguments, compare multiple values at once
- ✅ Abs/AbsDiff - Generic absolute value calculation
- ✅ Clamp - Value range clamping
- ✅ Rounding - Round/RoundTo/Ceil/Floor/Trunc
- ✅ Zero external dependencies - Uses only the Go standard library
- ✅ Type safety - Compile-time type checks

## Quick Start

### Basic Operations

```go
import "github.com/everyday-items/toolkit/lang/mathx"

// Min/Max - supports variadic arguments
min := mathx.Min(3, 1, 4, 1, 5)           // 1 (int)
max := mathx.Max(3.14, 2.71, 1.41)        // 3.14 (float64)
minStr := mathx.Min("c", "a", "b")        // "a" (string)

// MinMax - returns both minimum and maximum simultaneously
min, max := mathx.MinMax(3, 1, 4, 1, 5)   // 1, 5

// Clamp - restrict value to a range
clamped := mathx.Clamp(15, 0, 10)         // 10
clamped := mathx.Clamp(-5, 0, 10)         // 0
clamped := mathx.Clamp(5, 0, 10)          // 5

// Abs - absolute value (generic)
abs := mathx.Abs(-5)                      // 5 (int)
absf := mathx.Abs(-3.14)                  // 3.14 (float64)

// AbsDiff - absolute difference
diff := mathx.AbsDiff(5, 3)               // 2
diff := mathx.AbsDiff(3, 5)               // 2
```

### Rounding

```go
// Round - round to nearest integer
rounded := mathx.Round(3.14)              // 3.0
rounded := mathx.Round(3.5)               // 4.0

// RoundTo - round to specified decimal places
rounded := mathx.RoundTo(3.14159, 2)      // 3.14
rounded := mathx.RoundTo(123.456, 1)      // 123.5

// Ceil - round up
ceiled := mathx.Ceil(3.14)                // 4.0

// Floor - round down
floored := mathx.Floor(3.14)              // 3.0

// Trunc - truncate decimal part
truncated := mathx.Trunc(3.14)            // 3.0
truncated := mathx.Trunc(-3.14)           // -3.0
```

## API Reference

### Type Constraints

```go
// Ordered - orderable types (support <, > comparison)
type Ordered interface {
    ~int | ~int8 | ~int16 | ~int32 | ~int64 |
    ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
    ~float32 | ~float64 |
    ~string
}

// Signed - signed numeric types
type Signed interface {
    ~int | ~int8 | ~int16 | ~int32 | ~int64 |
    ~float32 | ~float64
}

// Float - floating-point types
type Float interface {
    ~float32 | ~float64
}
```

### Comparison and Clamping

```go
// Min - returns the minimum value
Min[T Ordered](values ...T) T

// Max - returns the maximum value
Max[T Ordered](values ...T) T

// MinMax - returns both minimum and maximum simultaneously
MinMax[T Ordered](values ...T) (T, T)

// Clamp - restricts value to a range
Clamp[T Ordered](value, min, max T) T
```

### Absolute Value

```go
// Abs - returns absolute value
Abs[T Signed](value T) T

// AbsDiff - returns absolute difference of two numbers
AbsDiff[T Signed](a, b T) T
```

### Rounding

```go
// Round - round to nearest integer
Round(value float64) float64

// RoundTo - round to specified decimal places
RoundTo(value float64, decimals int) float64

// Ceil - round up
Ceil(value float64) float64

// Floor - round down
Floor(value float64) float64

// Trunc - truncate decimal part
Trunc(value float64) float64
```

## Use Cases

### 1. Universal Min/Max (replacing standard library)

```go
// Standard math package only supports float64
import "math"
min := math.Min(3.14, 2.71)  // only works with float64

// mathx supports all types
import "github.com/everyday-items/toolkit/lang/mathx"
min := mathx.Min(3, 1, 4)                  // int
min := mathx.Min(3.14, 2.71, 1.41)        // float64
min := mathx.Min("c", "a", "b")           // string
min := mathx.Min(time.Second, time.Minute) // time.Duration
```

### 2. Multi-value Comparison (variadic arguments)

```go
// Standard library requires multiple calls
min := math.Min(math.Min(a, b), c)

// mathx: single call
min := mathx.Min(a, b, c, d, e)

// Practical example
prices := []float64{99.99, 149.99, 79.99, 199.99}
minPrice := mathx.Min(prices...)  // 79.99
maxPrice := mathx.Max(prices...)  // 199.99
```

### 3. Getting Min and Max Simultaneously

```go
// Get min and max in a single traversal
scores := []int{85, 92, 78, 95, 88}
minScore, maxScore := mathx.MinMax(scores...)
fmt.Printf("Score range: %d - %d\n", minScore, maxScore)
```

### 4. Value Range Clamping

```go
// Restrict user input
age := mathx.Clamp(userAge, 0, 150)          // age 0-150
percentage := mathx.Clamp(value, 0.0, 100.0) // percentage 0-100

// Restrict color values
r := mathx.Clamp(red, 0, 255)
g := mathx.Clamp(green, 0, 255)
b := mathx.Clamp(blue, 0, 255)

// Restrict price
price := mathx.Clamp(userPrice, minPrice, maxPrice)
```

### 5. Distance Calculation

```go
// Calculate coordinate distance
dx := mathx.AbsDiff(x1, x2)
dy := mathx.AbsDiff(y1, y2)

// Calculate time difference (absolute value)
diff := mathx.AbsDiff(time1.Unix(), time2.Unix())
hours := float64(diff) / 3600

// Calculate price difference
priceDiff := mathx.AbsDiff(price1, price2)
```

### 6. Numeric Processing

```go
// Round price
price := 99.999
finalPrice := mathx.RoundTo(price, 2)  // 100.00

// Round up for page count
totalPages := mathx.Ceil(float64(totalItems) / float64(pageSize))

// Round down for discount
discount := mathx.Floor(price * 0.9)

// Truncate for tax
tax := mathx.Trunc(price * 0.13)
```

### 7. Statistical Analysis

```go
// Calculate data range
values := []float64{1.2, 3.4, 2.1, 5.6, 4.3}
min, max := mathx.MinMax(values...)
dataRange := max - min

// Normalize to [0, 1]
normalized := make([]float64, len(values))
for i, v := range values {
    normalized[i] = (v - min) / dataRange
}

// Clamp outliers
cleaned := make([]float64, len(values))
for i, v := range values {
    cleaned[i] = mathx.Clamp(v, min, max)
}
```

### 8. Game Development

```go
// Restrict player position
playerX := mathx.Clamp(newX, 0, mapWidth)
playerY := mathx.Clamp(newY, 0, mapHeight)

// Restrict health points
health := mathx.Clamp(currentHealth-damage, 0, maxHealth)

// Calculate damage (absolute value)
damage := mathx.Abs(attackPower - defense)

// Calculate distance
distance := mathx.AbsDiff(playerPos, enemyPos)
```

### 9. Config Validation

```go
type Config struct {
    Port        int
    Timeout     time.Duration
    MaxConns    int
    CacheSize   int
}

func ValidateConfig(cfg *Config) *Config {
    // Restrict port range
    cfg.Port = mathx.Clamp(cfg.Port, 1024, 65535)

    // Restrict timeout
    cfg.Timeout = mathx.Clamp(cfg.Timeout, time.Second, time.Minute)

    // Restrict connection count
    cfg.MaxConns = mathx.Clamp(cfg.MaxConns, 10, 10000)

    // Restrict cache size
    cfg.CacheSize = mathx.Clamp(cfg.CacheSize, 100, 100000)

    return cfg
}
```

### 10. Graphics Calculation

```go
// Calculate rectangle bounds
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
    return Rect{} // no intersection
}
```

## Comparison with Standard Library

### Standard Library math Package

```go
import "math"

// Only supports float64
min := math.Min(3.14, 2.71)
max := math.Max(3.14, 2.71)
abs := math.Abs(-3.14)

// No variadic arguments
min3 := math.Min(math.Min(a, b), c)

// Does not support other types
// min := math.Min(1, 2)  // compile error
```

### mathx Package

```go
import "github.com/everyday-items/toolkit/lang/mathx"

// Supports generics
min := mathx.Min(3, 1, 4)           // int
min := mathx.Min(3.14, 2.71)        // float64
min := mathx.Min("a", "b")          // string

// Supports variadic arguments
min := mathx.Min(a, b, c, d, e)

// Get min and max at once
min, max := mathx.MinMax(1, 2, 3, 4, 5)
```

### Feature Comparison

| Feature | math package | mathx package |
|---------|-------------|---------------|
| Min/Max | ✅ float64 only | ✅ Generic (all types) |
| Variadic args | ❌ 2 args only | ✅ Unlimited args |
| Abs | ✅ float64 only | ✅ Generic (signed types) |
| Clamp | ❌ Not supported | ✅ Supported |
| MinMax | ❌ Two calls needed | ✅ Single call |
| AbsDiff | ❌ Not supported | ✅ Supported |
| Round | ✅ | ✅ |
| RoundTo | ❌ | ✅ |

## Performance Notes

### Inlining Optimization

All functions are simple enough that the compiler automatically inlines them, giving the same performance as hand-written code:

```go
// The following two approaches have the same performance
min := mathx.Min(a, b)

// Equivalent to
min := a
if b < a {
    min = b
}
```

### Performance Benchmarks

```
Min (2 values):      2 ns/op
Min (5 values):      5 ns/op
Max (2 values):      2 ns/op
MinMax (5 values):   8 ns/op
Abs:                 1 ns/op
Clamp:               3 ns/op
RoundTo:             15 ns/op
```

### Performance Recommendations

1. **Avoid variadic arguments in loops**:
   ```go
   // Not recommended
   for _, v := range values {
       min := mathx.Min(v, otherValues...)
   }

   // Recommended
   min := mathx.Min(values...)
   ```

2. **Prefer MinMax over separate calls**:
   ```go
   // Not recommended (two traversals)
   min := mathx.Min(values...)
   max := mathx.Max(values...)

   // Recommended (single traversal)
   min, max := mathx.MinMax(values...)
   ```

3. **Use standard library on extremely performance-critical paths**:
   ```go
   // Extreme performance scenario (float64 only)
   import "math"
   min := math.Min(a, b)  // slightly faster than generic version

   // General scenario (recommended)
   import "github.com/everyday-items/toolkit/lang/mathx"
   min := mathx.Min(a, b)  // type-safe + better readability
   ```

## Design Principles

1. **Type safety**: Uses generic constraints with compile-time type checks
2. **Clean API**: Consistent naming and semantics with the standard library
3. **Zero dependencies**: Uses only the Go standard library
4. **Performance-oriented**: Simple functions, easy to inline

## Notes

1. **Empty argument handling**:
   ```go
   min := mathx.Min()  // returns zero value of the type
   ```

2. **Floating-point precision**:
   ```go
   // Floating-point arithmetic follows IEEE 754 standard
   result := mathx.RoundTo(0.1+0.2, 1)  // 0.3 (may have precision errors)
   ```

3. **Generic constraints**:
   ```go
   // Ordered types (support < > comparison)
   min := mathx.Min(1, 2, 3)           // ✅ int
   min := mathx.Min(1.0, 2.0)          // ✅ float64
   min := mathx.Min("a", "b")          // ✅ string

   // Signed types (signed numbers)
   abs := mathx.Abs(-5)                // ✅ int
   abs := mathx.Abs(-3.14)             // ✅ float64
   // abs := mathx.Abs(uint(5))        // ❌ compile error (uint is unsigned)
   ```

4. **Concurrency safety**:
   - All functions are pure functions with no state
   - Can be safely used in multiple goroutines

## Dependencies

```bash
# Only depends on the standard math package
import "math"
```

## Extension Suggestions

For more math functionality, consider:
- `math` - Go standard library (trigonometric, logarithm, exponential, etc.)
- `math/big` - Big integer arithmetic
- `gonum.org/v1/gonum/mathext` - Extended math functions
- `github.com/shopspring/decimal` - High-precision decimal arithmetic
