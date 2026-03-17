[中文](README.md) | English

# Rand - Random Number Generator

Provides cryptographically secure random number generation based on `crypto/rand`.

## Features

- ✅ Cryptographically secure random number generation
- ✅ Multiple character sets (digits, letters, alphanumeric)
- ✅ Random integers, byte arrays, and booleans
- ✅ Convenient verification code and token generation
- ✅ Zero external dependencies

## Quick Start

### Random Strings

```go
import "github.com/everyday-items/toolkit/util/rand"

// Generate 16-character random string (letters + digits)
token := rand.String(16)
// Output: "a8Kx9pLm2Qz7Bn3Y"

// Generate 6-digit numeric verification code
code := rand.NumericString(6)
// Output: "382947"

// Generate 8-character alphabetic string
name := rand.AlphaString(8)
// Output: "AbCdEfGh"

// Generate lowercase alphabetic string
lower := rand.LowerString(10)
// Output: "abcdefghij"

// Generate uppercase alphabetic string
upper := rand.UpperString(10)
// Output: "ABCDEFGHIJ"
```

### Custom Character Set

```go
// Custom character set
charset := "ABCD1234"
str := rand.StringFrom(charset, 8)
// Output: "A2C4B1D3"

// Generate string containing only specific characters
hexStr := rand.StringFrom("0123456789ABCDEF", 16)
// Output: "3F7A9B2C8D1E4F6A"
```

### Random Integers

```go
// Generate random integer in range [1, 100)
num := rand.Int(1, 100)
// Output: 42

// Generate random int64 in range
bigNum := rand.Int64(1, 1000000)
// Output: 384756
```

### Random Bytes and Booleans

```go
// Generate 32 random bytes
bytes := rand.Bytes(32)

// Generate random boolean
flag := rand.Bool()
// Output: true or false
```

### Convenience Functions

```go
// Generate 6-digit verification code
verifyCode := rand.Code(6)
// Output: "548392"

// Generate 32-character token
apiToken := rand.Token(32)
// Output: "7kLm9pQz2Wx5Vy3Bn8Cx1Fy4Gx6Hz0Jx"
```

## API Reference

### String Generation

| Function | Description | Character Set |
|----------|-------------|---------------|
| `String(length)` | Generates alphanumeric string | A-Z, a-z, 0-9 |
| `NumericString(length)` | Generates numeric-only string | 0-9 |
| `AlphaString(length)` | Generates alphabetic-only string | A-Z, a-z |
| `LowerString(length)` | Generates lowercase alphabetic string | a-z |
| `UpperString(length)` | Generates uppercase alphabetic string | A-Z |
| `StringFrom(charset, length)` | Generates from custom character set | Custom |

### Number Generation

```go
// Int generates random integer in [min, max)
Int(min, max int) int

// Int64 generates random int64 in [min, max)
Int64(min, max int64) int64

// Bool generates random boolean
Bool() bool
```

### Byte Generation

```go
// Bytes generates a random byte array of specified length
Bytes(length int) []byte
```

### Convenience Functions

```go
// Code generates a numeric verification code
Code(length int) string

// Token generates an alphanumeric token
Token(length int) string
```

## Use Cases

### 1. Generate Verification Codes

```go
// SMS verification code (6 digits)
smsCode := rand.Code(6)
// Output: "384756"

// Email verification code (4 digits)
emailCode := rand.Code(4)
// Output: "8392"

// Captcha (6 alphanumeric, case-sensitive)
captcha := rand.String(6)
// Output: "aB9Kx2"
```

### 2. Generate API Tokens

```go
// API access token (32 characters)
apiToken := rand.Token(32)
// Output: "7kLm9pQz2Wx5Vy3Bn8Cx1Fy4Gx6Hz0Jx"

// Temporary session ID (16 characters)
sessionID := rand.Token(16)
// Output: "a8Kx9pLm2Qz7Bn3Y"
```

### 3. Generate Passwords

```go
// Generate random password (letters and digits)
password := rand.String(12)
// Output: "aB3Xy9Km2Lz7"

// Generate strong password (custom charset with special characters)
charset := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*"
strongPassword := rand.StringFrom(charset, 16)
// Output: "aB3!Xy9@Km2#Lz7$"
```

### 4. Generate Unique Filenames

```go
// Generate unique filename
filename := fmt.Sprintf("upload_%s.jpg", rand.String(16))
// Output: "upload_a8Kx9pLm2Qz7Bn3Y.jpg"

// Generate temporary filename
tmpFile := fmt.Sprintf("/tmp/tmp_%s", rand.LowerString(10))
// Output: "/tmp/tmp_abcdefghij"
```

### 5. Generate Invite Codes

```go
// Generate uppercase alphanumeric invite code (easy to type)
inviteCode := rand.StringFrom("ABCDEFGHJKLMNPQRSTUVWXYZ23456789", 8)
// Output: "A8KX9PLM" (excludes easily confused characters 0/O, 1/I)
```

### 6. A/B Testing Assignment

```go
// Randomly assign users to experiment groups
if rand.Bool() {
    // Assign to group A
} else {
    // Assign to group B
}

// Multi-group assignment (using integers)
group := rand.Int(0, 3) // 0, 1, 2
switch group {
case 0:
    // Group A
case 1:
    // Group B
case 2:
    // Group C
}
```

### 7. Generate Test Data

```go
// Generate random test username
username := "user_" + rand.LowerString(8)

// Generate random quantity
quantity := rand.Int(1, 100)

// Generate random price (convert to float)
price := float64(rand.Int(100, 10000)) / 100.0 // 1.00 - 100.00
```

## Predefined Character Sets

The package provides the following predefined character set constants:

```go
const (
    Numeric       = "0123456789"                                          // digits
    Alpha         = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ" // letters
    AlphaNumeric  = Numeric + Alpha                                       // letters + digits
    AlphaLower    = "abcdefghijklmnopqrstuvwxyz"                          // lowercase letters
    AlphaUpper    = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"                          // uppercase letters
)
```

## Security Notes

### ✅ Cryptographically Secure

This package uses `crypto/rand` as the random source, providing cryptographic-level randomness:

```go
// ✅ Suitable for generating secure tokens
apiKey := rand.Token(32)

// ✅ Suitable for generating password reset tokens
resetToken := rand.String(48)

// ✅ Suitable for generating session IDs
sessionID := rand.Token(24)
```

### ⚠️ Range Note

```go
// Note: Int() returns [min, max) half-open interval
num := rand.Int(1, 10)  // may return 1-9, does not include 10

// For [1, 10] closed interval, use Int(1, 11)
num := rand.Int(1, 11)  // may return 1-10
```

### ✅ Uniqueness Guarantee

Although theoretically possible, collisions are extremely unlikely in practice:

```go
// Number of possible combinations for 32-char alphanumeric token: 62^32 ≈ 2^190
// Collision probability: < 10^-50 (virtually impossible)
token := rand.Token(32)
```

## Performance

```
BenchmarkString          500000       3000 ns/op
BenchmarkNumericString  1000000       2000 ns/op
BenchmarkInt            2000000        800 ns/op
BenchmarkBytes           500000       3500 ns/op
```

`crypto/rand` is slightly slower than `math/rand` but more secure.

## Notes

1. **Cryptographic Security**:
   - ✅ Uses `crypto/rand`, suitable for security-sensitive scenarios
   - ✅ Does not use pseudo-random number generators (PRNG)

2. **Performance**:
   - Slightly slower than `math/rand` (about 3-5x)
   - Sufficient performance for most application scenarios

3. **Range**:
   - `Int(min, max)` returns **[min, max)** half-open interval
   - To include upper bound, use `Int(min, max+1)`

4. **Error Handling**:
   - Internally ignores `crypto/rand` errors (very rare failures)
   - Returns a deterministic result on failure (rather than panic)

5. **Length Limit**:
   - May be slow for generating very long strings (> 1MB)
   - Recommended single generation length < 10000

## Dependencies

```bash
# Zero external dependencies, uses only standard library
import (
    "crypto/rand"
    "math/big"
)
```

## Comparison with math/rand

| Feature | crypto/rand (this package) | math/rand |
|---------|--------------------------|-----------|
| Security | Cryptographically secure | Insecure (predictable) |
| Performance | Slower | Fast |
| Use Case | Keys, tokens, passwords | Games, simulations, testing |
| Randomness | True random | Pseudo-random |

**Recommendation**:
- ✅ Use this package for security scenarios (tokens, passwords, keys)
- ⚠️ Consider `math/rand` for high-performance simulation scenarios
