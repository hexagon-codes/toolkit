[中文](README.md) | English

# Validator - Data Validation Utility

Provides comprehensive data validation functionality supporting common format validation and custom rules.

## Features

- ✅ Common format validation (email, phone, URL, IP)
- ✅ ID card number validation (China mainland 18-digit)
- ✅ Password strength validation
- ✅ String length and content validation
- ✅ Range validation (integers, floats)
- ✅ Generic support (In/NotIn)
- ✅ Regular expression matching
- ✅ Zero external dependencies

## Quick Start

### Format Validation

```go
import "github.com/everyday-items/toolkit/util/validator"

// Email validation
valid := validator.Email("user@example.com")  // true
valid := validator.Email("invalid")           // false

// Phone number validation (China mainland)
valid := validator.Phone("13800138000")  // true
valid := validator.Phone("12345678901")  // false

// URL validation
valid := validator.URL("https://example.com")  // true
valid := validator.URL("not-a-url")            // false

// IP address validation
valid := validator.IP("192.168.1.1")                  // true
valid := validator.IPv4("192.168.1.1")                // true
valid := validator.IPv6("2001:0db8:85a3::8a2e:0370:7334")  // true
```

### ID Card Number Validation

```go
// Validate 18-digit ID card number
valid := validator.IDCard("110101199001011234")  // true
valid := validator.IDCard("12345")               // false
```

### Password and Username

```go
// Password strength validation (at least 8 chars, must include uppercase, lowercase, and digits)
valid := validator.Password("Aa123456")   // true
valid := validator.Password("password")   // false (no digits or uppercase)
valid := validator.Password("Pass1")      // false (too short)

// Username validation (4-20 chars: letters, digits, underscores)
valid := validator.Username("user_123")   // true
valid := validator.Username("abc")        // false (too short)
valid := validator.Username("user-name")  // false (contains invalid characters)
```

### String Validation

```go
// String length validation
valid := validator.MinLength("hello", 3)          // true
valid := validator.MaxLength("hello", 10)         // true
valid := validator.LengthBetween("hello", 3, 10)  // true

// String content validation
valid := validator.IsNumeric("12345")       // true
valid := validator.IsAlpha("abc")           // true
valid := validator.IsAlphaNumeric("abc123") // true

// String contains validation
valid := validator.Contains("hello world", "world")  // true
valid := validator.HasPrefix("hello", "he")          // true
valid := validator.HasSuffix("hello", "lo")          // true

// Empty string validation
valid := validator.IsEmpty("   ")      // true
valid := validator.NotEmpty("hello")   // true
```

### Range Validation

```go
// Integer range validation [min, max]
valid := validator.InRange(5, 1, 10)   // true
valid := validator.InRange(0, 1, 10)   // false

// Float range validation [min, max]
valid := validator.InRangeFloat(3.5, 1.0, 10.0)  // true
```

### List Validation

```go
// Generic list validation
colors := []string{"red", "green", "blue"}
valid := validator.In("red", colors)        // true
valid := validator.NotIn("yellow", colors)  // true

// Integer list
numbers := []int{1, 2, 3, 4, 5}
valid := validator.In(3, numbers)  // true
```

### Regular Expression Matching

```go
// Custom regex validation
valid := validator.Match("abc123", `^[a-z0-9]+$`)  // true
valid := validator.Match("ABC", `^[a-z]+$`)        // false
```

## API Reference

### Format Validation

| Function | Description | Example |
|----------|-------------|---------|
| `Email(email)` | Email format | user@example.com |
| `Phone(phone)` | Phone number (China) | 13800138000 |
| `URL(url)` | URL format | https://example.com |
| `IP(ip)` | IP address (v4/v6) | 192.168.1.1 |
| `IPv4(ip)` | IPv4 address | 8.8.8.8 |
| `IPv6(ip)` | IPv6 address | 2001:db8::1 |
| `IDCard(id)` | ID card (18 digits) | 110101199001011234 |

### Password and Username

```go
// Password validates password strength (at least 8 chars, with uppercase, lowercase, and digits)
Password(password string) bool

// Username validates username (4-20 chars: letters, digits, underscores)
Username(username string) bool
```

### String Validation

```go
// Length validation
MinLength(str string, min int) bool
MaxLength(str string, max int) bool
LengthBetween(str string, min, max int) bool

// Content validation
IsNumeric(str string) bool        // digits only
IsAlpha(str string) bool          // letters only
IsAlphaNumeric(str string) bool   // letters + digits

// Contains validation
Contains(str, substr string) bool
HasPrefix(str, prefix string) bool
HasSuffix(str, suffix string) bool

// Empty validation
IsEmpty(str string) bool    // empty or whitespace only
NotEmpty(str string) bool   // non-empty
```

### Range Validation

```go
// InRange validates integer range [min, max]
InRange(value, min, max int) bool

// InRangeFloat validates float range [min, max]
InRangeFloat(value, min, max float64) bool
```

### List Validation (Generic)

```go
// In checks if value is in the list
In[T comparable](value T, list []T) bool

// NotIn checks if value is not in the list
NotIn[T comparable](value T, list []T) bool
```

### Regular Expression Validation

```go
// Match checks if string matches the regex pattern
Match(str, pattern string) bool
```

## Use Cases

### 1. User Registration Validation

```go
func ValidateRegister(req RegisterRequest) error {
    // Validate email
    if !validator.Email(req.Email) {
        return errors.New("invalid email format")
    }

    // Validate username
    if !validator.Username(req.Username) {
        return errors.New("username must be 4-20 characters (letters, numbers, underscore)")
    }

    // Validate password strength
    if !validator.Password(req.Password) {
        return errors.New("password must be at least 8 characters with uppercase, lowercase and numbers")
    }

    // Validate phone number
    if !validator.Phone(req.Phone) {
        return errors.New("invalid phone number")
    }

    return nil
}
```

### 2. API Parameter Validation

```go
func ValidateQueryParams(page, pageSize int) error {
    // Validate page range
    if !validator.InRange(page, 1, 1000) {
        return errors.New("page must be between 1 and 1000")
    }

    // Validate page size
    if !validator.InRange(pageSize, 1, 100) {
        return errors.New("page_size must be between 1 and 100")
    }

    return nil
}
```

### 3. Real-Name Verification

```go
func ValidateRealName(name, idCard string) error {
    // Validate name length
    if !validator.LengthBetween(name, 2, 20) {
        return errors.New("name must be 2-20 characters")
    }

    // Validate ID card number
    if !validator.IDCard(idCard) {
        return errors.New("invalid ID card number")
    }

    return nil
}
```

### 4. Contact Information Validation

```go
func ValidateContact(email, phone, website string) error {
    // Email validation
    if email != "" && !validator.Email(email) {
        return errors.New("invalid email")
    }

    // Phone validation
    if phone != "" && !validator.Phone(phone) {
        return errors.New("invalid phone")
    }

    // Website validation
    if website != "" && !validator.URL(website) {
        return errors.New("invalid website URL")
    }

    return nil
}
```

### 5. IP Whitelist Validation

```go
func ValidateIPWhitelist(ip string, whitelist []string) error {
    // Validate IP format
    if !validator.IP(ip) {
        return errors.New("invalid IP address")
    }

    // Check if in whitelist
    if !validator.In(ip, whitelist) {
        return errors.New("IP not in whitelist")
    }

    return nil
}
```

### 6. File Upload Validation

```go
func ValidateUpload(filename string, allowedExts []string) error {
    // Validate filename is not empty
    if validator.IsEmpty(filename) {
        return errors.New("filename is required")
    }

    // Extract extension
    ext := strings.ToLower(filepath.Ext(filename))
    ext = strings.TrimPrefix(ext, ".")

    // Validate extension
    if !validator.In(ext, allowedExts) {
        return fmt.Errorf("file type %s not allowed", ext)
    }

    return nil
}
```

### 7. Data Filter Validation

```go
func ValidateFilter(field string, validFields []string) error {
    // Validate field name format
    if !validator.Match(field, `^[a-z_]+$`) {
        return errors.New("field name must contain only lowercase letters and underscores")
    }

    // Validate field name is in allowed list
    if !validator.In(field, validFields) {
        return errors.New("field not allowed for filtering")
    }

    return nil
}
```

## Validation Rules

### Phone Number Rules

- Length: 11 digits
- Format: starts with 1 + [3-9] + 9 digits
- Examples: `13800138000`, `15912345678`

### Password Rules

- Minimum length: 8 characters
- Must contain: uppercase letters, lowercase letters, digits
- Examples: `Aa123456`, `Password1`

### Username Rules

- Length: 4-20 characters
- Allowed characters: letters, digits, underscores
- Examples: `user_123`, `test_user`

### ID Card Number Rules

- Length: 18 digits
- Format: region code (6 digits) + date of birth (8 digits) + sequence code (3 digits) + check digit (1 digit)
- Birth year: 18xx, 19xx, 20xx
- Month: 01-12
- Day: 01-31

## Combined Validation Example

```go
// Create a validator struct
type UserValidator struct {
    Email    string
    Phone    string
    Password string
    Age      int
}

func (v *UserValidator) Validate() []string {
    var errors []string

    // Email validation
    if !validator.NotEmpty(v.Email) {
        errors = append(errors, "email is required")
    } else if !validator.Email(v.Email) {
        errors = append(errors, "invalid email format")
    }

    // Phone validation
    if validator.NotEmpty(v.Phone) && !validator.Phone(v.Phone) {
        errors = append(errors, "invalid phone number")
    }

    // Password validation
    if !validator.NotEmpty(v.Password) {
        errors = append(errors, "password is required")
    } else if !validator.Password(v.Password) {
        errors = append(errors, "password too weak")
    }

    // Age validation
    if !validator.InRange(v.Age, 0, 150) {
        errors = append(errors, "invalid age")
    }

    return errors
}
```

## Notes

1. **Phone Number Validation**:
   - Only supports China mainland phone numbers (11 digits, starting with 1)
   - International phone numbers require custom regex

2. **ID Card Validation**:
   - Only validates format, not check digit
   - Does not validate region code validity
   - Only supports 18-digit second-generation ID cards

3. **Password Validation**:
   - Only checks basic strength (length + character types)
   - Does not check common passwords or dictionary attacks

4. **Performance**:
   - Most validation functions perform well (< 1μs)
   - Regex validation is slightly slower (1-10μs)

5. **Error Handling**:
   - All functions return `bool`, not `error`
   - Recommended to convert to specific error messages at the application layer

## Dependencies

```bash
# Zero external dependencies, uses only standard library
import (
    "net"
    "net/mail"
    "net/url"
    "regexp"
    "strings"
    "unicode"
)
```

## Extension Suggestions

For more complex validation, consider:
- `github.com/go-playground/validator` - Struct tag-based validation
- `github.com/asaskevich/govalidator` - More built-in validation rules
