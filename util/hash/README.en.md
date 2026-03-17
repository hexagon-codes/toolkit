[中文](README.md) | English

# Hash Utility

Provides common hash algorithms and password encryption functionality.

## Features

- ✅ MD5, SHA1, SHA256, SHA512 hashing
- ✅ Bcrypt password encryption
- ✅ Simple and easy-to-use API
- ✅ Secure password storage

## Quick Start

### Hash Functions

```go
import "github.com/everyday-items/toolkit/util/hash"

// MD5
md5Hash := hash.MD5("data")
// Output: "8d777f385d3dfec8815d20f7496026dc"

// SHA256
sha256Hash := hash.SHA256("data")
// Output: "3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7"

// SHA512
sha512Hash := hash.SHA512("data")
```

### Password Encryption (Bcrypt)

```go
// Encrypt password
password := "mySecretPassword"
hashedPassword, err := hash.BcryptHash(password)
if err != nil {
    // Handle error
}

// Verify password
isValid := hash.BcryptCheck("mySecretPassword", hashedPassword)
if isValid {
    // Password is correct
}
```

## API Reference

### Hash Functions

| Function | Description | Output Length |
|----------|-------------|---------------|
| `MD5(string)` | MD5 hash | 32 characters |
| `SHA1(string)` | SHA1 hash | 40 characters |
| `SHA256(string)` | SHA256 hash | 64 characters |
| `SHA512(string)` | SHA512 hash | 128 characters |
| `MD5Bytes([]byte)` | MD5 hash (bytes) | 32 characters |
| `SHA256Bytes([]byte)` | SHA256 hash (bytes) | 64 characters |

### Password Encryption

```go
// BcryptHash encrypts password with default cost
BcryptHash(password string) (string, error)

// BcryptHashWithCost encrypts password with specified cost
BcryptHashWithCost(password string, cost int) (string, error)

// BcryptCheck verifies a password
BcryptCheck(password, hash string) bool

// MustBcryptHash encrypts password, panics on failure
MustBcryptHash(password string) string
```

## Use Cases

### 1. Data Signing

```go
// Generate signature for data
data := "user_id=123&amount=100.00"
signature := hash.SHA256(data + secretKey)
```

### 2. File Checksum

```go
// Calculate file MD5
fileContent, _ := os.ReadFile("file.zip")
fileMD5 := hash.MD5Bytes(fileContent)
```

### 3. User Password Storage

```go
// Encrypt password on registration
func Register(username, password string) error {
    hashedPassword, err := hash.BcryptHash(password)
    if err != nil {
        return err
    }

    // Store in database
    return db.SaveUser(username, hashedPassword)
}

// Verify password on login
func Login(username, password string) bool {
    user, _ := db.GetUser(username)
    return hash.BcryptCheck(password, user.Password)
}
```

### 4. API Token Generation

```go
// Generate API Token
data := fmt.Sprintf("%s:%s:%d", userID, apiKey, time.Now().Unix())
token := hash.SHA256(data)
```

## Bcrypt Cost Explanation

The `cost` parameter of Bcrypt controls encryption strength:

- **Range**: 4-31
- **Default**: 10
- **Recommended**: 10-12

```go
// Cost 10 (default, suitable for most cases)
hash.BcryptHash(password)

// Cost 12 (more secure but slower)
hash.BcryptHashWithCost(password, 12)
```

| Cost | Approximate Time | Use Case |
|------|-----------------|----------|
| 10 | 100ms | General applications |
| 12 | 400ms | High security requirements |
| 14 | 1.6s | Extremely high security requirements |

## Security Recommendations

### ✅ Recommended Practices

```go
// ✅ Password storage: use Bcrypt
hashedPassword, _ := hash.BcryptHash(password)

// ✅ Data signing: use SHA256
signature := hash.SHA256(data + secret)

// ✅ File checksum: use MD5 or SHA256
fileMD5 := hash.MD5Bytes(fileContent)
```

### ❌ Practices to Avoid

```go
// ❌ Do not use MD5 to store passwords
badHash := hash.MD5(password) // Insecure!

// ❌ Do not use SHA256 directly to store passwords (no salt)
badHash := hash.SHA256(password) // Vulnerable to rainbow table attacks!
```

## Notes

1. **Password Storage**:
   - ✅ Use Bcrypt (automatically adds salt)
   - ❌ Do not use MD5 or SHA256 directly to store passwords

2. **Performance**:
   - MD5, SHA family: very fast
   - Bcrypt: intentionally slow (by design)

3. **Hashes Are One-Way**:
   - Hashing is irreversible; cannot be decrypted
   - Can only verify by comparison

4. **Bcrypt Characteristics**:
   - Automatically adds salt (same password generates different hash each time)
   - Automatically extracts salt during verification
   - Resistant to brute-force attacks

## Dependencies

```bash
go get -u golang.org/x/crypto/bcrypt
```

## Performance

```
BenchmarkMD5          5000000    250 ns/op
BenchmarkSHA256       3000000    450 ns/op
BenchmarkBcryptHash        100  100 ms/op
BenchmarkBcryptCheck       100  100 ms/op
```

Bcrypt being slow is a design feature to prevent brute-force attacks!
