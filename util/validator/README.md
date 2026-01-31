# Validator 数据验证工具

提供全面的数据验证功能，支持常见格式验证和自定义规则。

## 特性

- ✅ 常见格式验证（邮箱、手机、URL、IP）
- ✅ 身份证号验证（中国大陆18位）
- ✅ 密码强度验证
- ✅ 字符串长度和内容验证
- ✅ 范围验证（数字、浮点数）
- ✅ 泛型支持（In/NotIn）
- ✅ 正则表达式匹配
- ✅ 零外部依赖

## 快速开始

### 格式验证

```go
import "github.com/everyday-items/toolkit/util/validator"

// 邮箱验证
valid := validator.Email("user@example.com")  // true
valid := validator.Email("invalid")           // false

// 手机号验证（中国大陆）
valid := validator.Phone("13800138000")  // true
valid := validator.Phone("12345678901")  // false

// URL 验证
valid := validator.URL("https://example.com")  // true
valid := validator.URL("not-a-url")            // false

// IP 地址验证
valid := validator.IP("192.168.1.1")                  // true
valid := validator.IPv4("192.168.1.1")                // true
valid := validator.IPv6("2001:0db8:85a3::8a2e:0370:7334")  // true
```

### 身份证号验证

```go
// 验证18位身份证号
valid := validator.IDCard("110101199001011234")  // true
valid := validator.IDCard("12345")               // false
```

### 密码和用户名

```go
// 密码强度验证（至少8位，包含大小写字母和数字）
valid := validator.Password("Aa123456")   // true
valid := validator.Password("password")   // false（无数字和大写）
valid := validator.Password("Pass1")      // false（太短）

// 用户名验证（4-20位字母、数字、下划线）
valid := validator.Username("user_123")   // true
valid := validator.Username("abc")        // false（太短）
valid := validator.Username("user-name")  // false（包含非法字符）
```

### 字符串验证

```go
// 字符串长度验证
valid := validator.MinLength("hello", 3)          // true
valid := validator.MaxLength("hello", 10)         // true
valid := validator.LengthBetween("hello", 3, 10)  // true

// 字符串内容验证
valid := validator.IsNumeric("12345")       // true
valid := validator.IsAlpha("abc")           // true
valid := validator.IsAlphaNumeric("abc123") // true

// 字符串包含验证
valid := validator.Contains("hello world", "world")  // true
valid := validator.HasPrefix("hello", "he")          // true
valid := validator.HasSuffix("hello", "lo")          // true

// 空字符串验证
valid := validator.IsEmpty("   ")      // true
valid := validator.NotEmpty("hello")   // true
```

### 范围验证

```go
// 整数范围验证 [min, max]
valid := validator.InRange(5, 1, 10)   // true
valid := validator.InRange(0, 1, 10)   // false

// 浮点数范围验证 [min, max]
valid := validator.InRangeFloat(3.5, 1.0, 10.0)  // true
```

### 列表验证

```go
// 泛型列表验证
colors := []string{"red", "green", "blue"}
valid := validator.In("red", colors)     // true
valid := validator.NotIn("yellow", colors)  // true

// 整数列表
numbers := []int{1, 2, 3, 4, 5}
valid := validator.In(3, numbers)  // true
```

### 正则匹配

```go
// 自定义正则验证
valid := validator.Match("abc123", `^[a-z0-9]+$`)  // true
valid := validator.Match("ABC", `^[a-z]+$`)        // false
```

## API 文档

### 格式验证

| 函数 | 说明 | 示例 |
|------|------|------|
| `Email(email)` | 邮箱格式 | user@example.com |
| `Phone(phone)` | 手机号（中国） | 13800138000 |
| `URL(url)` | URL 格式 | https://example.com |
| `IP(ip)` | IP 地址（v4/v6） | 192.168.1.1 |
| `IPv4(ip)` | IPv4 地址 | 8.8.8.8 |
| `IPv6(ip)` | IPv6 地址 | 2001:db8::1 |
| `IDCard(id)` | 身份证号（18位） | 110101199001011234 |

### 密码和用户名

```go
// Password 验证密码强度（至少8位，包含大小写字母和数字）
Password(password string) bool

// Username 验证用户名（4-20位字母、数字、下划线）
Username(username string) bool
```

### 字符串验证

```go
// 长度验证
MinLength(str string, min int) bool
MaxLength(str string, max int) bool
LengthBetween(str string, min, max int) bool

// 内容验证
IsNumeric(str string) bool        // 纯数字
IsAlpha(str string) bool          // 纯字母
IsAlphaNumeric(str string) bool   // 字母+数字

// 包含验证
Contains(str, substr string) bool
HasPrefix(str, prefix string) bool
HasSuffix(str, suffix string) bool

// 空值验证
IsEmpty(str string) bool    // 空或空白
NotEmpty(str string) bool   // 非空
```

### 范围验证

```go
// InRange 整数范围验证 [min, max]
InRange(value, min, max int) bool

// InRangeFloat 浮点数范围验证 [min, max]
InRangeFloat(value, min, max float64) bool
```

### 列表验证（泛型）

```go
// In 值在列表中
In[T comparable](value T, list []T) bool

// NotIn 值不在列表中
NotIn[T comparable](value T, list []T) bool
```

### 正则验证

```go
// Match 正则表达式匹配
Match(str, pattern string) bool
```

## 使用场景

### 1. 用户注册验证

```go
func ValidateRegister(req RegisterRequest) error {
    // 验证邮箱
    if !validator.Email(req.Email) {
        return errors.New("invalid email format")
    }

    // 验证用户名
    if !validator.Username(req.Username) {
        return errors.New("username must be 4-20 characters (letters, numbers, underscore)")
    }

    // 验证密码强度
    if !validator.Password(req.Password) {
        return errors.New("password must be at least 8 characters with uppercase, lowercase and numbers")
    }

    // 验证手机号
    if !validator.Phone(req.Phone) {
        return errors.New("invalid phone number")
    }

    return nil
}
```

### 2. API 参数验证

```go
func ValidateQueryParams(page, pageSize int) error {
    // 验证页码范围
    if !validator.InRange(page, 1, 1000) {
        return errors.New("page must be between 1 and 1000")
    }

    // 验证每页大小
    if !validator.InRange(pageSize, 1, 100) {
        return errors.New("page_size must be between 1 and 100")
    }

    return nil
}
```

### 3. 实名认证验证

```go
func ValidateRealName(name, idCard string) error {
    // 验证姓名长度
    if !validator.LengthBetween(name, 2, 20) {
        return errors.New("name must be 2-20 characters")
    }

    // 验证身份证号
    if !validator.IDCard(idCard) {
        return errors.New("invalid ID card number")
    }

    return nil
}
```

### 4. 联系方式验证

```go
func ValidateContact(email, phone, website string) error {
    // 邮箱验证
    if email != "" && !validator.Email(email) {
        return errors.New("invalid email")
    }

    // 手机验证
    if phone != "" && !validator.Phone(phone) {
        return errors.New("invalid phone")
    }

    // 网站验证
    if website != "" && !validator.URL(website) {
        return errors.New("invalid website URL")
    }

    return nil
}
```

### 5. IP 白名单验证

```go
func ValidateIPWhitelist(ip string, whitelist []string) error {
    // 验证 IP 格式
    if !validator.IP(ip) {
        return errors.New("invalid IP address")
    }

    // 验证是否在白名单
    if !validator.In(ip, whitelist) {
        return errors.New("IP not in whitelist")
    }

    return nil
}
```

### 6. 文件上传验证

```go
func ValidateUpload(filename string, allowedExts []string) error {
    // 验证文件名非空
    if validator.IsEmpty(filename) {
        return errors.New("filename is required")
    }

    // 提取扩展名
    ext := strings.ToLower(filepath.Ext(filename))
    ext = strings.TrimPrefix(ext, ".")

    // 验证扩展名
    if !validator.In(ext, allowedExts) {
        return fmt.Errorf("file type %s not allowed", ext)
    }

    return nil
}
```

### 7. 数据过滤验证

```go
func ValidateFilter(field string, validFields []string) error {
    // 验证字段名格式
    if !validator.Match(field, `^[a-z_]+$`) {
        return errors.New("field name must contain only lowercase letters and underscores")
    }

    // 验证字段名在允许列表中
    if !validator.In(field, validFields) {
        return errors.New("field not allowed for filtering")
    }

    return nil
}
```

## 验证规则说明

### 手机号规则

- 长度：11位
- 格式：1开头 + [3-9] + 9位数字
- 示例：`13800138000`、`15912345678`

### 密码规则

- 最小长度：8位
- 必须包含：大写字母、小写字母、数字
- 示例：`Aa123456`、`Password1`

### 用户名规则

- 长度：4-20位
- 允许字符：字母、数字、下划线
- 示例：`user_123`、`test_user`

### 身份证号规则

- 长度：18位
- 格式：地区码（6位）+ 出生日期（8位）+ 顺序码（3位）+ 校验码（1位）
- 出生年份：18xx、19xx、20xx
- 月份：01-12
- 日期：01-31

## 组合验证示例

```go
// 创建一个验证器结构
type UserValidator struct {
    Email    string
    Phone    string
    Password string
    Age      int
}

func (v *UserValidator) Validate() []string {
    var errors []string

    // 邮箱验证
    if !validator.NotEmpty(v.Email) {
        errors = append(errors, "email is required")
    } else if !validator.Email(v.Email) {
        errors = append(errors, "invalid email format")
    }

    // 手机验证
    if validator.NotEmpty(v.Phone) && !validator.Phone(v.Phone) {
        errors = append(errors, "invalid phone number")
    }

    // 密码验证
    if !validator.NotEmpty(v.Password) {
        errors = append(errors, "password is required")
    } else if !validator.Password(v.Password) {
        errors = append(errors, "password too weak")
    }

    // 年龄验证
    if !validator.InRange(v.Age, 0, 150) {
        errors = append(errors, "invalid age")
    }

    return errors
}
```

## 注意事项

1. **手机号验证**：
   - 仅支持中国大陆手机号（11位，1开头）
   - 国际手机号需自定义正则

2. **身份证验证**：
   - 仅验证格式，不验证校验码
   - 不验证地区码有效性
   - 仅支持18位二代身份证

3. **密码验证**：
   - 仅检查基本强度（长度 + 字符类型）
   - 不检查常见密码、字典攻击

4. **性能**：
   - 大部分验证函数性能良好（< 1μs）
   - 正则验证稍慢（1-10μs）

5. **错误处理**：
   - 所有函数返回 `bool`，不返回 `error`
   - 建议在应用层转换为具体错误信息

## 依赖

```bash
# 零外部依赖，仅使用标准库
import (
    "net"
    "net/mail"
    "net/url"
    "regexp"
    "strings"
    "unicode"
)
```

## 扩展建议

如需更复杂的验证，可考虑以下开源库：
- `github.com/go-playground/validator` - 结构体标签验证
- `github.com/asaskevich/govalidator` - 更多内置验证规则
