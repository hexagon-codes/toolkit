# Rand 随机数生成工具

提供加密安全的随机数生成功能，基于 `crypto/rand`。

## 特性

- ✅ 加密安全的随机数生成
- ✅ 支持多种字符集（数字、字母、字母数字）
- ✅ 随机整数、字节数组、布尔值
- ✅ 便捷的验证码和 Token 生成
- ✅ 零外部依赖

## 快速开始

### 随机字符串

```go
import "github.com/everyday-items/toolkit/util/rand"

// 生成16位随机字符串（字母+数字）
token := rand.String(16)
// 输出: "a8Kx9pLm2Qz7Bn3Y"

// 生成6位数字验证码
code := rand.NumericString(6)
// 输出: "382947"

// 生成8位字母字符串
name := rand.AlphaString(8)
// 输出: "AbCdEfGh"

// 生成小写字母字符串
lower := rand.LowerString(10)
// 输出: "abcdefghij"

// 生成大写字母字符串
upper := rand.UpperString(10)
// 输出: "ABCDEFGHIJ"
```

### 自定义字符集

```go
// 自定义字符集
charset := "ABCD1234"
str := rand.StringFrom(charset, 8)
// 输出: "A2C4B1D3"

// 生成只包含特定字符的字符串
hexStr := rand.StringFrom("0123456789ABCDEF", 16)
// 输出: "3F7A9B2C8D1E4F6A"
```

### 随机整数

```go
// 生成 [1, 100) 范围的随机整数
num := rand.Int(1, 100)
// 输出: 42

// 生成 int64 范围的随机数
bigNum := rand.Int64(1, 1000000)
// 输出: 384756
```

### 随机字节和布尔值

```go
// 生成32字节的随机数据
bytes := rand.Bytes(32)

// 生成随机布尔值
flag := rand.Bool()
// 输出: true 或 false
```

### 便捷函数

```go
// 生成6位验证码
verifyCode := rand.Code(6)
// 输出: "548392"

// 生成32位 Token
apiToken := rand.Token(32)
// 输出: "7kLm9pQz2Wx5Vy3Bn8Cx1Fy4Gx6Hz0Jx"
```

## API 文档

### 字符串生成

| 函数 | 说明 | 字符集 |
|------|------|--------|
| `String(length)` | 生成字母+数字字符串 | A-Z, a-z, 0-9 |
| `NumericString(length)` | 生成纯数字字符串 | 0-9 |
| `AlphaString(length)` | 生成纯字母字符串 | A-Z, a-z |
| `LowerString(length)` | 生成小写字母字符串 | a-z |
| `UpperString(length)` | 生成大写字母字符串 | A-Z |
| `StringFrom(charset, length)` | 从自定义字符集生成 | 自定义 |

### 数值生成

```go
// Int 生成指定范围的随机整数 [min, max)
Int(min, max int) int

// Int64 生成指定范围的随机 int64 [min, max)
Int64(min, max int64) int64

// Bool 生成随机布尔值
Bool() bool
```

### 字节生成

```go
// Bytes 生成指定长度的随机字节数组
Bytes(length int) []byte
```

### 便捷函数

```go
// Code 生成数字验证码
Code(length int) string

// Token 生成字母数字 Token
Token(length int) string
```

## 使用场景

### 1. 生成验证码

```go
// 短信验证码（6位数字）
smsCode := rand.Code(6)
// 输出: "384756"

// 邮箱验证码（4位数字）
emailCode := rand.Code(4)
// 输出: "8392"

// 图形验证码（6位字母数字，区分大小写）
captcha := rand.String(6)
// 输出: "aB9Kx2"
```

### 2. 生成 API Token

```go
// API 访问令牌（32位）
apiToken := rand.Token(32)
// 输出: "7kLm9pQz2Wx5Vy3Bn8Cx1Fy4Gx6Hz0Jx"

// 临时会话 ID（16位）
sessionID := rand.Token(16)
// 输出: "a8Kx9pLm2Qz7Bn3Y"
```

### 3. 生成密码

```go
// 生成随机密码（包含字母和数字）
password := rand.String(12)
// 输出: "aB3Xy9Km2Lz7"

// 生成强密码（自定义字符集，包含特殊字符）
charset := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*"
strongPassword := rand.StringFrom(charset, 16)
// 输出: "aB3!Xy9@Km2#Lz7$"
```

### 4. 生成唯一文件名

```go
// 生成唯一文件名
filename := fmt.Sprintf("upload_%s.jpg", rand.String(16))
// 输出: "upload_a8Kx9pLm2Qz7Bn3Y.jpg"

// 生成临时文件名
tmpFile := fmt.Sprintf("/tmp/tmp_%s", rand.LowerString(10))
// 输出: "/tmp/tmp_abcdefghij"
```

### 5. 生成邀请码

```go
// 生成大写字母数字邀请码（易输入）
inviteCode := rand.StringFrom("ABCDEFGHJKLMNPQRSTUVWXYZ23456789", 8)
// 输出: "A8KX9PLM" (排除了容易混淆的字符 0/O, 1/I)
```

### 6. A/B 测试分组

```go
// 随机分配用户到实验组
if rand.Bool() {
    // 分配到 A 组
} else {
    // 分配到 B 组
}

// 多组分配（使用整数）
group := rand.Int(0, 3) // 0, 1, 2
switch group {
case 0:
    // A 组
case 1:
    // B 组
case 2:
    // C 组
}
```

### 7. 生成测试数据

```go
// 生成随机测试用户名
username := "user_" + rand.LowerString(8)

// 生成随机数量
quantity := rand.Int(1, 100)

// 生成随机价格（转换为 float）
price := float64(rand.Int(100, 10000)) / 100.0 // 1.00 - 100.00
```

## 预定义字符集

包提供了以下预定义字符集常量：

```go
const (
    Numeric       = "0123456789"                                          // 数字
    Alpha         = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ" // 字母
    AlphaNumeric  = Numeric + Alpha                                       // 字母+数字
    AlphaLower    = "abcdefghijklmnopqrstuvwxyz"                          // 小写字母
    AlphaUpper    = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"                          // 大写字母
)
```

## 安全性说明

### ✅ 加密安全

本包使用 `crypto/rand` 作为随机源，提供加密级别的随机数：

```go
// ✅ 适合用于生成安全令牌
apiKey := rand.Token(32)

// ✅ 适合用于生成密码重置令牌
resetToken := rand.String(48)

// ✅ 适合用于生成会话 ID
sessionID := rand.Token(24)
```

### ⚠️ 范围说明

```go
// 注意：Int() 返回 [min, max) 左闭右开区间
num := rand.Int(1, 10)  // 可能返回 1-9，不包含 10

// 如需 [1, 10] 闭区间，使用 Int(1, 11)
num := rand.Int(1, 11)  // 可能返回 1-10
```

### ✅ 唯一性保证

虽然理论上可能重复，但实际上极难发生：

```go
// 32位字母数字 Token 的可能组合数：62^32 ≈ 2^190
// 碰撞概率：< 10^-50（几乎不可能）
token := rand.Token(32)
```

## 性能

```
BenchmarkString          500000       3000 ns/op
BenchmarkNumericString  1000000       2000 ns/op
BenchmarkInt            2000000        800 ns/op
BenchmarkBytes           500000       3500 ns/op
```

相比 `math/rand`，`crypto/rand` 略慢但更安全。

## 注意事项

1. **加密安全**：
   - ✅ 使用 `crypto/rand`，适合安全敏感场景
   - ✅ 不使用伪随机数生成器（PRNG）

2. **性能**：
   - 相比 `math/rand` 稍慢（约3-5倍）
   - 对于大多数应用场景性能足够

3. **范围**：
   - `Int(min, max)` 返回 **[min, max)** 左闭右开区间
   - 如需包含上界，使用 `Int(min, max+1)`

4. **错误处理**：
   - 内部忽略了 `crypto/rand` 的错误（极少失败）
   - 失败时会返回确定性结果（而非 panic）

5. **长度限制**：
   - 生成长字符串时（> 1MB）可能较慢
   - 建议单次生成长度 < 10000

## 依赖

```bash
# 零外部依赖，仅使用标准库
import (
    "crypto/rand"
    "math/big"
)
```

## 对比 math/rand

| 特性 | crypto/rand (本包) | math/rand |
|------|-------------------|-----------|
| 安全性 | 加密安全 | 不安全（可预测） |
| 性能 | 较慢 | 快 |
| 用途 | 密钥、Token、密码 | 游戏、模拟、测试 |
| 随机性 | 真随机 | 伪随机 |

**推荐**：
- ✅ 安全场景（Token、密码、密钥）使用本包
- ⚠️ 高性能模拟场景可考虑 `math/rand`
