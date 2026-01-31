# Hash 哈希工具

提供常用的哈希算法和密码加密功能。

## 特性

- ✅ MD5、SHA1、SHA256、SHA512 哈希
- ✅ Bcrypt 密码加密
- ✅ 简单易用的 API
- ✅ 安全的密码存储

## 快速开始

### 哈希函数

```go
import "github.com/everyday-items/toolkit/util/hash"

// MD5
md5Hash := hash.MD5("data")
// 输出: "8d777f385d3dfec8815d20f7496026dc"

// SHA256
sha256Hash := hash.SHA256("data")
// 输出: "3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7"

// SHA512
sha512Hash := hash.SHA512("data")
```

### 密码加密（Bcrypt）

```go
// 加密密码
password := "mySecretPassword"
hashedPassword, err := hash.BcryptHash(password)
if err != nil {
    // 处理错误
}

// 验证密码
isValid := hash.BcryptCheck("mySecretPassword", hashedPassword)
if isValid {
    // 密码正确
}
```

## API 文档

### 哈希函数

| 函数 | 说明 | 输出长度 |
|------|------|---------|
| `MD5(string)` | MD5 哈希 | 32 字符 |
| `SHA1(string)` | SHA1 哈希 | 40 字符 |
| `SHA256(string)` | SHA256 哈希 | 64 字符 |
| `SHA512(string)` | SHA512 哈希 | 128 字符 |
| `MD5Bytes([]byte)` | MD5 哈希（字节） | 32 字符 |
| `SHA256Bytes([]byte)` | SHA256 哈希（字节） | 64 字符 |

### 密码加密

```go
// BcryptHash 使用默认 cost 加密密码
BcryptHash(password string) (string, error)

// BcryptHashWithCost 使用指定 cost 加密密码
BcryptHashWithCost(password string, cost int) (string, error)

// BcryptCheck 验证密码
BcryptCheck(password, hash string) bool

// MustBcryptHash 加密密码，失败时 panic
MustBcryptHash(password string) string
```

## 使用场景

### 1. 数据签名

```go
// 对数据生成签名
data := "user_id=123&amount=100.00"
signature := hash.SHA256(data + secretKey)
```

### 2. 文件校验

```go
// 计算文件 MD5
fileContent, _ := os.ReadFile("file.zip")
fileMD5 := hash.MD5Bytes(fileContent)
```

### 3. 用户密码存储

```go
// 注册时加密密码
func Register(username, password string) error {
    hashedPassword, err := hash.BcryptHash(password)
    if err != nil {
        return err
    }

    // 存储到数据库
    return db.SaveUser(username, hashedPassword)
}

// 登录时验证密码
func Login(username, password string) bool {
    user, _ := db.GetUser(username)
    return hash.BcryptCheck(password, user.Password)
}
```

### 4. API Token 生成

```go
// 生成 API Token
data := fmt.Sprintf("%s:%s:%d", userID, apiKey, time.Now().Unix())
token := hash.SHA256(data)
```

## Bcrypt Cost 说明

Bcrypt 的 `cost` 参数控制加密强度：

- **范围**：4-31
- **默认**：10
- **推荐**：10-12

```go
// Cost 10（默认，适合大多数场景）
hash.BcryptHash(password)

// Cost 12（更安全，但更慢）
hash.BcryptHashWithCost(password, 12)
```

| Cost | 耗时（约） | 使用场景 |
|------|----------|----------|
| 10 | 100ms | 普通应用 |
| 12 | 400ms | 高安全要求 |
| 14 | 1.6s | 极高安全要求 |

## 安全建议

### ✅ 推荐做法

```go
// ✅ 密码存储：使用 Bcrypt
hashedPassword, _ := hash.BcryptHash(password)

// ✅ 数据签名：使用 SHA256
signature := hash.SHA256(data + secret)

// ✅ 文件校验：使用 MD5 或 SHA256
fileMD5 := hash.MD5Bytes(fileContent)
```

### ❌ 不推荐做法

```go
// ❌ 不要用 MD5 存储密码
badHash := hash.MD5(password) // 不安全！

// ❌ 不要用 SHA256 直接存储密码（无 salt）
badHash := hash.SHA256(password) // 容易被彩虹表破解！
```

## 注意事项

1. **密码存储**：
   - ✅ 使用 Bcrypt（自动加盐）
   - ❌ 不要使用 MD5、SHA256 直接存储密码

2. **性能**：
   - MD5、SHA 系列：非常快
   - Bcrypt：较慢（故意设计）

3. **哈希不可逆**：
   - 哈希是单向的，无法解密
   - 只能通过比对验证

4. **Bcrypt 特点**：
   - 自动加盐（同一密码每次生成的 hash 不同）
   - 验证时自动提取 salt
   - 防暴力破解

## 依赖

```bash
go get -u golang.org/x/crypto/bcrypt
```

## 性能

```
BenchmarkMD5          5000000    250 ns/op
BenchmarkSHA256       3000000    450 ns/op
BenchmarkBcryptHash        100  100 ms/op
BenchmarkBcryptCheck       100  100 ms/op
```

Bcrypt 慢是设计特性，防止暴力破解！
