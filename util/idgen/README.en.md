[中文](README.md) | English

# IDGen - ID Generator

Provides multiple ID generation schemes: UUID, Snowflake, and NanoID.

## Features

- ✅ UUID - Standard UUID v4
- ✅ Snowflake - Distributed unique ID
- ✅ NanoID - Short unique ID
- ✅ High performance - fast generation
- ✅ Concurrency-safe - supports concurrent calls

## Quick Start

### UUID

```go
import "github.com/everyday-items/toolkit/util/idgen"

// Generate UUID
id := idgen.UUID()
// Output: "550e8400-e29b-41d4-a716-446655440000"

// UUID without hyphens
id := idgen.UUIDWithoutHyphen()
// Output: "550e8400e29b41d4a716446655440000"
```

### Snowflake

```go
// Initialize (call once in main function)
idgen.InitSnowflake(1) // worker ID: 1

// Generate Snowflake ID
id := idgen.SnowflakeID()
// Output: 1234567890123456789
```

### NanoID

```go
// Default length (21 characters)
id := idgen.NanoID()
// Output: "V1StGXR8_Z5jdHi6B-myT"

// Specified length
id := idgen.NanoIDSize(10)
// Output: "4f90d13a42"

// Short ID (8 characters)
id := idgen.ShortID()
// Output: "xK3s9d2a"

// Medium ID (16 characters)
id := idgen.MediumID()
// Output: "3f2hK9s1pL4m8nR5"
```

## Comparison

| Type | Length | Performance | Ordered | Distributed | Use Cases |
|------|--------|-------------|---------|-------------|-----------|
| UUID | 36 chars | Fast | ❌ | ✅ | General unique identifier |
| Snowflake | 19-digit integer | Very fast | ✅ | ✅ | Order IDs, User IDs |
| NanoID | Variable (default 21) | Fast | ❌ | ✅ | Short links, filenames |

## UUID

### Characteristics
- 128-bit unique identifier
- Globally unique without centralization
- Standard format: 8-4-4-4-12

### Use Cases
```go
// User ID
userID := idgen.UUID()

// Request tracing ID
requestID := idgen.UUID()

// Filename
filename := idgen.UUIDWithoutHyphen() + ".jpg"
```

## Snowflake

### Characteristics
- 64-bit integer
- Time-ordered (sorted by generation time)
- Distributed support (up to 1024 nodes)
- Up to 4096 IDs per millisecond

### Structure
```
|---41-bit timestamp---|---10-bit machine ID---|---12-bit sequence---|
```

### Initialization
```go
func main() {
    // Initialize once at application startup
    // workerID: 0-1023
    if err := idgen.InitSnowflake(1); err != nil {
        log.Fatal(err)
    }
}
```

### Use Cases
```go
// Order ID
orderID := idgen.SnowflakeID()

// Message ID
messageID := idgen.SnowflakeID()

// Log ID
logID := idgen.SnowflakeID()
```

### Multi-Instance Deployment
```go
// Server 1
idgen.InitSnowflake(1)

// Server 2
idgen.InitSnowflake(2)

// Server 3
idgen.InitSnowflake(3)

// Each server uses a different workerID
```

## NanoID

### Characteristics
- Shorter than UUID (default 21 characters)
- URL-safe characters
- Customizable character set and length
- Extremely low collision probability

### Custom Character Set
```go
// Numbers only
alphabet := "0123456789"
id := idgen.NanoIDCustom(alphabet, 10)
// Output: "4839274920"

// Lowercase letters only
alphabet := "abcdefghijklmnopqrstuvwxyz"
id := idgen.NanoIDCustom(alphabet, 10)
// Output: "xfbdekmpqr"
```

### Use Cases
```go
// Short URL
shortURL := idgen.ShortID()
// Output: "xK3s9d2a"

// Verification code (numbers only)
code := idgen.NanoIDCustom("0123456789", 6)
// Output: "492837"

// Filename
filename := idgen.MediumID() + ".pdf"
```

## Performance Comparison

```go
// Benchmark results
BenchmarkUUID           10000000    120 ns/op
BenchmarkSnowflake      50000000     25 ns/op
BenchmarkNanoID         5000000     280 ns/op
```

## Best Practices

### 1. Choose the Right ID Type

```go
// ✅ Database primary key: Snowflake (ordered, better performance)
userID := idgen.SnowflakeID()

// ✅ External API: UUID (standard, good compatibility)
apiKey := idgen.UUID()

// ✅ Short links: NanoID (short, URL-friendly)
shortURL := idgen.ShortID()
```

### 2. Snowflake Must Be Pre-Initialized

```go
// ✅ Initialize in main function
func main() {
    idgen.InitSnowflake(getWorkerID())
    // ...
}

// ❌ Do not create on every use
func bad() {
    gen, _ := idgen.NewSnowflake(1) // Wrong!
    id := gen.Generate()
}
```

### 3. Distributed Deployment workerID Management

```go
// Option 1: Configuration file
workerID := config.GetInt("worker_id")

// Option 2: Environment variable
workerID := os.Getenv("WORKER_ID")

// Option 3: Calculated from IP
workerID := hashIP(getLocalIP()) % 1024
```

## Dependencies

```bash
go get -u github.com/google/uuid
```

## Notes

1. **UUID**: No initialization needed, use directly
2. **Snowflake**: Must initialize first; workerID range 0-1023
3. **NanoID**: Ensure no duplicate characters in custom character set
4. **Clock Skew**: Snowflake panics when clock rollback is detected
5. **Concurrency Safety**: All methods are concurrency-safe
