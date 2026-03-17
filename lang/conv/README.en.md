[中文](README.md) | English

# Conv - Type Conversion Utilities

Provides comprehensive Go type conversion capabilities, supporting conversions between primitive types, JSON/Map operations, and other common scenarios.

## Features

- ✅ Zero external dependencies (uses only Go standard library)
- ✅ Interface-driven type conversion (supports custom types)
- ✅ No panic on failure (returns zero value)
- ✅ Intelligent type inference
- ✅ Primitive type conversion (String/Int/Uint/Float/Bool)
- ✅ Bidirectional JSON-Map conversion
- ✅ Map operations (merge, extract keys/values)
- ✅ Concurrency safe (pure functions, stateless)

## Quick Start

### Primitive Type Conversion

```go
import "github.com/everyday-items/toolkit/lang/conv"

// String conversion
s := conv.String(123)          // "123"
s := conv.String(3.14)         // "3.14"
s := conv.String(true)         // "true"
s := conv.String([]byte("hi")) // "hi"

// Integer conversion
i := conv.Int("456")           // 456
i := conv.Int(3.99)            // 3 (float truncation)
i := conv.Int(true)            // 1
i := conv.Int("invalid")       // 0 (returns zero value on failure)

// Float conversion
f := conv.Float64("3.14")      // 3.14
f := conv.Float64(123)         // 123.0

// Boolean conversion
b := conv.Bool(1)              // true
b := conv.Bool(0)              // false
b := conv.Bool("true")         // true
b := conv.Bool("yes")          // true
```

### JSON-Map Interconversion

```go
// JSON to Map
m, err := conv.JSONToMap(`{"name":"Alice","age":30}`)
// m = map[string]any{"name": "Alice", "age": 30}

// Map to JSON
json, err := conv.MapToJSON(map[string]any{"name": "Bob"})
// json = `{"name":"Bob"}`
```

### Map Operations

```go
// Merge Maps (later ones override earlier ones)
m1 := map[string]any{"a": 1, "b": 2}
m2 := map[string]any{"b": 3, "c": 4}
merged := conv.MergeMaps(m1, m2)
// merged = map[string]any{"a": 1, "b": 3, "c": 4}

// Extract all keys
keys := conv.MapKeys(m)
// keys = []string{"a", "b", "c"}

// Extract all values
values := conv.MapValues(m)
// values = []any{1, 3, 4}
```

## API Reference

### String Conversion

| Function | Description | Failure Return |
|----------|-------------|----------------|
| `String(any)` | Convert any type to string | `""` |

**Supported types**:
- Primitive types: `int`, `uint`, `float`, `bool`, `string`, `[]byte`
- Interface: `iString` (calls the `String()` method)
- Others: uses `fmt.Sprintf("%v", value)`

### Integer Conversion

| Function | Description | Failure Return |
|----------|-------------|----------------|
| `Int(any)` | Convert any type to int | `0` |
| `Int32(any)` | Convert any type to int32 | `0` |
| `Int64(any)` | Convert any type to int64 | `0` |
| `Uint(any)` | Convert any type to uint | `0` |
| `Uint32(any)` | Convert any type to uint32 | `0` |
| `Uint64(any)` | Convert any type to uint64 | `0` |

**Conversion rules**:
- Strings parsed as decimal
- Floats truncate decimal part
- `bool`: `true=1`, `false=0`

### Float Conversion

| Function | Description | Failure Return |
|----------|-------------|----------------|
| `Float32(any)` | Convert any type to float32 | `0.0` |
| `Float64(any)` | Convert any type to float64 | `0.0` |

**Special features**:
- Supports `[]byte` binary decoding (little-endian)
- Supports interfaces: `iFloat32`, `iFloat64`

### Boolean Conversion

| Function | Description | Failure Return |
|----------|-------------|----------------|
| `Bool(any)` | Convert any type to bool | `false` |

**Conversion rules**:
- Numbers: `0` = `false`, others = `true`
- Strings: `"true"`, `"1"`, `"yes"`, `"on"` = `true` (case-insensitive)

### JSON/Map Operations

| Function | Description |
|----------|-------------|
| `JSONToMap(string)` | Convert JSON string to Map |
| `MapToJSON(map)` | Convert Map to JSON string |
| `MergeMaps(...map)` | Merge multiple Maps |
| `MapKeys(map)` | Extract all keys |
| `MapValues(map)` | Extract all values |

### Custom Type Conversion Interfaces

Implement these interfaces to support custom type conversions:

```go
type iString interface { String() string }
type iInt64 interface { Int64() int64 }
type iUint64 interface { Uint64() uint64 }
type iFloat32 interface { Float32() float32 }
type iFloat64 interface { Float64() float64 }
type iBool interface { Bool() bool }
```

## Use Cases

### 1. HTTP Request Parameter Parsing

```go
// Parse URL parameters
func GetUserByID(r *http.Request) (*User, error) {
    idStr := r.URL.Query().Get("id")
    id := conv.Int(idStr)  // safe conversion, returns 0 on failure

    if id == 0 {
        return nil, errors.New("invalid id")
    }

    return db.FindUser(id)
}
```

### 2. JSON Config File Parsing

```go
// Parse JSON config
func LoadConfig(jsonStr string) (*Config, error) {
    m, err := conv.JSONToMap(jsonStr)
    if err != nil {
        return nil, err
    }

    return &Config{
        Host:     conv.String(m["host"]),
        Port:     conv.Int(m["port"]),
        Timeout:  conv.Int(m["timeout"]),
        Debug:    conv.Bool(m["debug"]),
        MaxConns: conv.Int(m["max_connections"]),
    }, nil
}
```

### 3. Dynamic Data Processing

```go
// Handle dynamic type data
func ProcessDynamicData(data any) {
    switch v := data.(type) {
    case map[string]any:
        // Handle Map
        for key, val := range v {
            fmt.Printf("%s: %s\n", key, conv.String(val))
        }
    default:
        // Handle other types
        fmt.Println(conv.String(data))
    }
}
```

### 4. Database Field Mapping

```go
// Map database row to struct
func ScanUser(row *sql.Row) (*User, error) {
    var data map[string]any
    // Assume map data retrieved from database

    return &User{
        ID:        conv.Int64(data["id"]),
        Name:      conv.String(data["name"]),
        Age:       conv.Int(data["age"]),
        Email:     conv.String(data["email"]),
        IsActive:  conv.Bool(data["is_active"]),
        Balance:   conv.Float64(data["balance"]),
    }, nil
}
```

### 5. Config Merging

```go
// Merge default config and user config
func MergeConfig(defaults, user map[string]any) map[string]any {
    return conv.MergeMaps(defaults, user)  // user overrides defaults
}

// Example usage
defaults := map[string]any{"timeout": 30, "debug": false}
user := map[string]any{"debug": true, "host": "localhost"}
config := MergeConfig(defaults, user)
// Result: {"timeout": 30, "debug": true, "host": "localhost"}
```

### 6. Log Field Extraction

```go
// Extract fields from log Map
func ExtractLogFields(logMap map[string]any) {
    fields := conv.MapKeys(logMap)

    fmt.Println("Available fields:", fields)

    for _, field := range fields {
        value := conv.String(logMap[field])
        fmt.Printf("%s: %s\n", field, value)
    }
}
```

### 7. API Response Conversion

```go
// Normalize API response format
func NormalizeAPIResponse(resp any) map[string]any {
    // If string, try to parse as JSON
    if jsonStr, ok := resp.(string); ok {
        m, err := conv.JSONToMap(jsonStr)
        if err == nil {
            return m
        }
    }

    // Otherwise convert to Map
    return map[string]any{"data": resp}
}
```

### 8. Environment Variable Reading

```go
// Safely read environment variables
func GetEnvInt(key string, defaultVal int) int {
    val := os.Getenv(key)
    if val == "" {
        return defaultVal
    }

    result := conv.Int(val)
    if result == 0 {
        return defaultVal
    }

    return result
}

// Example usage
maxWorkers := GetEnvInt("MAX_WORKERS", 10)
timeout := GetEnvInt("TIMEOUT_SECONDS", 30)
```

### 9. Form Data Processing

```go
// Parse form data
func ParseForm(r *http.Request) (*FormData, error) {
    r.ParseForm()

    return &FormData{
        Name:      r.FormValue("name"),
        Age:       conv.Int(r.FormValue("age")),
        Email:     r.FormValue("email"),
        Subscribe: conv.Bool(r.FormValue("subscribe")),
        Amount:    conv.Float64(r.FormValue("amount")),
    }, nil
}
```

### 10. Cache Key-Value Conversion

```go
// Redis cache reading (Redis values are usually strings)
func GetFromCache(key string) (*CachedData, error) {
    val, err := redisClient.Get(ctx, key).Result()
    if err != nil {
        return nil, err
    }

    // JSON string to Map
    m, err := conv.JSONToMap(val)
    if err != nil {
        return nil, err
    }

    return &CachedData{
        ID:        conv.Int64(m["id"]),
        Value:     conv.String(m["value"]),
        Timestamp: conv.Int64(m["timestamp"]),
    }, nil
}
```

## Conversion Principles

All conversion functions attempt conversion in the following order:

1. **Nil check**: Returns zero value if input is `nil`
2. **Type assertion**: Directly handles common Go types
3. **Interface check**: Checks if conversion interface is implemented (e.g., `iString`, `iFloat32`)
4. **Fallback conversion**: Uses standard library functions (`strconv`, `fmt`)
5. **Returns zero value on failure**: Conversion failures do not panic

## Notes

### 1. No Panic on Failure

```go
// ✅ Safe: returns zero value on failure
i := conv.Int("invalid")  // returns 0, no panic
f := conv.Float64("abc")  // returns 0.0, no panic
```

### 2. Float Truncation

```go
// Converting float to integer truncates the decimal part
conv.Int(3.99)   // 3 (not 4)
conv.Int(-2.5)   // -2 (not -3)
```

### 3. Boolean Conversion

```go
// String to bool (case-insensitive)
conv.Bool("true")   // true
conv.Bool("TRUE")   // true
conv.Bool("1")      // true
conv.Bool("yes")    // true
conv.Bool("on")     // true
conv.Bool("false")  // false
conv.Bool("0")      // false
conv.Bool("no")     // false
conv.Bool("off")    // false
conv.Bool("other")  // false (unknown string)
```

### 4. Map Order Not Guaranteed

```go
// MapKeys and MapValues return in non-deterministic order
m := map[string]any{"a": 1, "b": 2, "c": 3}
keys := conv.MapKeys(m)
// keys may be ["a", "b", "c"] or ["b", "c", "a"], etc.
```

### 5. Map Merge Rules

```go
// Later Maps override duplicate keys from earlier Maps
m1 := map[string]any{"a": 1, "b": 2}
m2 := map[string]any{"b": 3}
result := conv.MergeMaps(m1, m2)
// result["b"] = 3 (m2 overrides m1)
```

### 6. JSON Must Be an Object

```go
// ✅ Correct: JSON object
m, err := conv.JSONToMap(`{"name":"Alice"}`)

// ❌ Wrong: JSON array
m, err := conv.JSONToMap(`[1,2,3]`)  // returns error
```

### 7. Binary Decoding Format

```go
// Float32/Float64 from []byte uses little-endian decoding
bytes := []byte{...}  // 4 or 8 bytes
f32 := conv.Float32(bytes)  // little-endian decode
f64 := conv.Float64(bytes)  // little-endian decode
```

### 8. Concurrency Safety

```go
// ✅ All functions are concurrency-safe (pure functions, stateless)
go func() { conv.Int("123") }()
go func() { conv.String(456) }()
```

## Custom Type Conversion Example

```go
// Custom type implementing conversion interfaces
type UserID int64

func (u UserID) Int64() int64 {
    return int64(u)
}

func (u UserID) String() string {
    return fmt.Sprintf("user_%d", u)
}

// Usage example
uid := UserID(1001)
conv.Int64(uid)   // 1001
conv.String(uid)  // "user_1001"
```

## Performance Considerations

### High-performance Scenarios

```go
// ✅ Direct type assertion (fastest)
if s, ok := value.(string); ok {
    // use s
}

// ✅ Use standard library when type is known
strconv.Atoi(str)
strconv.ParseFloat(str, 64)
```

### General Scenarios

```go
// ✅ Use conv when type is uncertain (safe + convenient)
i := conv.Int(value)  // value may be string/int/float/any
```

### Avoid

```go
// ❌ Avoid frequent conversions in hot paths
for i := 0; i < 1000000; i++ {
    conv.String(i)  // performance sensitive
}

// ✅ Optimization: batch conversion
strs := make([]string, 1000000)
for i := 0; i < 1000000; i++ {
    strs[i] = strconv.Itoa(i)  // use standard library directly
}
```

## Zero External Dependencies

```
Depends only on Go standard library:
- encoding/json     # JSON encoding/decoding
- encoding/binary   # binary encoding
- strconv           # string conversion
- fmt               # formatting
- math              # math functions
```

## Comparison with Other Libraries

| Library | Dependencies | Failure Handling | Interface Support | JSON/Map |
|---------|-------------|------------------|-------------------|----------|
| **conv** | zero deps | returns zero | ✅ | ✅ |
| cast (spf13) | zero deps | returns zero | ❌ | ❌ |
| gconv (goframe) | many deps | returns zero | ✅ | ✅ |
| stdlib strconv | zero deps | returns error | ❌ | ❌ |

## Best Practices

### ✅ Recommended

```go
// ✅ Use when type is uncertain
func Process(data any) {
    i := conv.Int(data)
    // safe, returns 0 on failure
}

// ✅ API parameter parsing
id := conv.Int(r.URL.Query().Get("id"))

// ✅ Config file parsing
config, _ := conv.JSONToMap(jsonStr)
timeout := conv.Int(config["timeout"])

// ✅ Merge configs
finalConfig := conv.MergeMaps(defaults, userConfig)
```

### ❌ Not Recommended

```go
// ❌ Unnecessary when type is already known
var i int = 123
s := conv.String(i)  // unnecessary, use strconv.Itoa(i) directly

// ❌ Not suitable when error handling is needed
i := conv.Int(str)  // can't distinguish "0" from conversion failure
// Should use: i, err := strconv.Atoi(str)

// ❌ Performance-critical paths
for i := 0; i < 1000000; i++ {
    conv.Float64(arr[i])  // too slow
}
```

## Contributing

Issues and Pull Requests are welcome!

## License

MIT License
