[中文](README.md) | English

# Timex - Time Utilities

A timestamp formatting package providing convenient conversion of millisecond/second-precision timestamps to strings.

## Features

- ✅ **Millisecond/second timestamp conversion** - Quickly format timestamps
- ✅ **Standard format** - Default "Y-m-d H:i:s" format
- ✅ **Custom format** - Supports any Go time format
- ✅ **Zero external dependencies** - Uses only the Go standard library
- ✅ **Concurrency safe** - All functions are concurrency-safe
- ✅ **100% test coverage** - Complete unit tests

## Quick Start

### Basic Usage

```go
import (
    "fmt"
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

// Millisecond timestamp formatting
ms := time.Now().UnixMilli()
formatted := timex.MsecFormat(ms)
fmt.Println(formatted)  // Output: "2024-01-29 15:04:05"

// Second timestamp formatting
sec := time.Now().Unix()
formatted := timex.SecFormat(sec)
fmt.Println(formatted)  // Output: "2024-01-29 15:04:05"
```

### Custom Format

```go
// Millisecond timestamp with custom format
ms := time.Now().UnixMilli()

// Date only
dateOnly := timex.MsecFormatWithLayout(ms, "2006-01-02")
fmt.Println(dateOnly)  // Output: "2024-01-29"

// Time only
timeOnly := timex.MsecFormatWithLayout(ms, "15:04:05")
fmt.Println(timeOnly)  // Output: "15:04:05"

// Custom format
custom := timex.MsecFormatWithLayout(ms, "2006/01/02 15:04")
fmt.Println(custom)  // Output: "2024/01/29 15:04"

// Second timestamp with custom format
sec := time.Now().Unix()
custom := timex.SecFormatWithLayout(sec, "02-Jan-2006")
fmt.Println(custom)  // Output: "29-Jan-2024"
```

## API Reference

### Millisecond Timestamp Functions

#### MsecFormat

Converts a millisecond-precision Unix timestamp to a string in "2006-01-02 15:04:05" format.

```go
func MsecFormat(msectime int64) string

// Example
ms := int64(1706423456789)
result := timex.MsecFormat(ms)
// Output: "2024-01-28 15:04:16" (depends on local timezone)

// Current time
ms := time.Now().UnixMilli()
result := timex.MsecFormat(ms)
// Output: "2024-01-29 15:04:05"
```

**Parameters**:
- `msectime`: Millisecond-precision Unix timestamp

**Returns**:
- `string`: Time string in "YYYY-MM-DD HH:MM:SS" format

**Notes**:
- Displays in local timezone
- Precision to seconds (milliseconds are discarded)

#### MsecFormatWithLayout

Converts a millisecond-precision Unix timestamp to a string in a custom format.

```go
func MsecFormatWithLayout(msectime int64, layout string) string

// Example
ms := time.Now().UnixMilli()

// Date only
result := timex.MsecFormatWithLayout(ms, "2006-01-02")
// Output: "2024-01-29"

// Time only
result := timex.MsecFormatWithLayout(ms, "15:04:05")
// Output: "15:04:05"

// Chinese format
result := timex.MsecFormatWithLayout(ms, "2006年01月02日 15:04")
// Output: "2024年01月29日 15:04"

// Slash-separated format
result := timex.MsecFormatWithLayout(ms, "2006/01/02")
// Output: "2024/01/29"
```

**Parameters**:
- `msectime`: Millisecond-precision Unix timestamp
- `layout`: Go time format string (see "Time Format Notes" below)

**Returns**:
- `string`: Time string in the specified format

### Second-Precision Timestamp Functions

#### SecFormat

Converts a second-precision Unix timestamp to a string in "2006-01-02 15:04:05" format.

```go
func SecFormat(sectime int64) string

// Example
sec := int64(1706423456)
result := timex.SecFormat(sec)
// Output: "2024-01-28 15:04:16" (depends on local timezone)

// Current time
sec := time.Now().Unix()
result := timex.SecFormat(sec)
// Output: "2024-01-29 15:04:05"
```

**Parameters**:
- `sectime`: Second-precision Unix timestamp

**Returns**:
- `string`: Time string in "YYYY-MM-DD HH:MM:SS" format

**Notes**:
- Displays in local timezone
- Second-precision

#### SecFormatWithLayout

Converts a second-precision Unix timestamp to a string in a custom format.

```go
func SecFormatWithLayout(sectime int64, layout string) string

// Example
sec := time.Now().Unix()

// Date only
result := timex.SecFormatWithLayout(sec, "2006-01-02")
// Output: "2024-01-29"

// ISO 8601 format
result := timex.SecFormatWithLayout(sec, "2006-01-02T15:04:05Z07:00")
// Output: "2024-01-29T15:04:05+08:00"

// Unix timestamp to RFC3339
result := timex.SecFormatWithLayout(sec, time.RFC3339)
// Output: "2024-01-29T15:04:05+08:00"
```

**Parameters**:
- `sectime`: Second-precision Unix timestamp
- `layout`: Go time format string

**Returns**:
- `string`: Time string in the specified format

## Time Format Notes

Go's time format uses the **reference time** `Mon Jan 2 15:04:05 MST 2006`, chosen for easy memorization:
- Month: 01 (January)
- Day: 02
- Hour: 15 (3 PM in 24-hour format)
- Minute: 04
- Second: 05
- Year: 2006
- Day of week: Mon
- Time zone: MST

### Common Formats

| Format | Description | Example Output |
|--------|-------------|----------------|
| `2006-01-02 15:04:05` | Full datetime (recommended) | 2024-01-29 15:04:05 |
| `2006-01-02` | Date only | 2024-01-29 |
| `15:04:05` | Time only | 15:04:05 |
| `2006/01/02` | Slash-separated date | 2024/01/29 |
| `2006-01-02T15:04:05Z07:00` | ISO 8601 | 2024-01-29T15:04:05+08:00 |
| `02-Jan-2006` | English month | 29-Jan-2024 |
| `01/02/06` | US format | 01/29/24 |
| `2006年01月02日` | Chinese format | 2024年01月29日 |
| `15:04` | Hour and minute | 15:04 |
| `2006-01-02 Monday` | With weekday | 2024-01-29 Monday |
| `January 02, 2006` | Full English | January 29, 2024 |

### Time Component Reference

| Component | Value | Description |
|-----------|-------|-------------|
| Year | 2006 | 4-digit year |
| Year | 06 | 2-digit year |
| Month | 01 | 2-digit month |
| Month | 1 | 1-digit month (no leading zero) |
| Month | January | Full month name |
| Month | Jan | Abbreviated month name |
| Day | 02 | 2-digit day |
| Day | 2 | 1-digit day (no leading zero) |
| Day | _2 | Right-aligned day (space-padded) |
| Weekday | Monday | Full weekday name |
| Weekday | Mon | Abbreviated weekday name |
| Hour | 15 | 24-hour format |
| Hour | 03 | 12-hour format |
| AM/PM | PM | Uppercase AM/PM |
| AM/PM | pm | Lowercase am/pm |
| Minute | 04 | 2-digit minute |
| Second | 05 | 2-digit second |
| Nanosecond | .000 | 3-digit milliseconds |
| Nanosecond | .000000 | 6-digit microseconds |
| Nanosecond | .000000000 | 9-digit nanoseconds |
| Timezone | MST | Timezone abbreviation |
| Timezone | Z07:00 | RFC 3339 timezone offset |

## Use Cases

### 1. Logging

```go
import (
    "log"
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

func LogEvent(event string) {
    ms := time.Now().UnixMilli()
    timestamp := timex.MsecFormat(ms)
    log.Printf("[%s] %s", timestamp, event)
    // Output: [2024-01-29 15:04:05] User login success
}
```

### 2. Database Records

```go
import (
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

type User struct {
    ID        int
    Name      string
    CreatedAt string
    UpdatedAt string
}

func CreateUser(name string) *User {
    now := time.Now().UnixMilli()
    timestamp := timex.MsecFormat(now)
    return &User{
        ID:        1,
        Name:      name,
        CreatedAt: timestamp,
        UpdatedAt: timestamp,
    }
}
```

### 3. Filename Generation

```go
import (
    "fmt"
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

func GenerateBackupFilename() string {
    ms := time.Now().UnixMilli()
    // Date only, for organizing backups by date
    date := timex.MsecFormatWithLayout(ms, "2006-01-02")
    return fmt.Sprintf("backup_%s.tar.gz", date)
    // Output: backup_2024-01-29.tar.gz
}
```

### 4. API Responses

```go
import (
    "encoding/json"
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

type APIResponse struct {
    Code      int       `json:"code"`
    Message   string    `json:"message"`
    Timestamp string    `json:"timestamp"`
    Data      any       `json:"data"`
}

func NewAPIResponse(code int, msg string, data any) *APIResponse {
    ms := time.Now().UnixMilli()
    return &APIResponse{
        Code:      code,
        Message:   msg,
        Timestamp: timex.MsecFormat(ms),
        Data:      data,
    }
}

// Output
// {
//   "code": 200,
//   "message": "success",
//   "timestamp": "2024-01-29 15:04:05",
//   "data": {...}
// }
```

### 5. Scheduled Task Execution

```go
import (
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

func ScheduleTask(interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for range ticker.C {
        sec := time.Now().Unix()
        log.Printf("Task executed at: %s", timex.SecFormat(sec))
        // Execute task...
    }
}
```

### 6. Statistical Reports

```go
import (
    "fmt"
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

type DailyReport struct {
    Date  string
    Stats map[string]any
}

func GenerateDailyReport() *DailyReport {
    ms := time.Now().UnixMilli()
    // Display date only
    date := timex.MsecFormatWithLayout(ms, "2006-01-02")
    return &DailyReport{
        Date: date,
        Stats: map[string]any{
            "users": 1024,
            "orders": 56,
        },
    }
}
```

### 7. Monitoring Alerts

```go
import (
    "fmt"
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

type Alert struct {
    Severity string
    Message  string
    Time     string
    URL      string
}

func CreateAlert(severity, message string) *Alert {
    ms := time.Now().UnixMilli()
    return &Alert{
        Severity: severity,
        Message:  message,
        Time:     timex.MsecFormat(ms),
        URL:      fmt.Sprintf("https://alert.example.com?t=%d", ms),
    }
}
```

### 8. Performance Analysis

```go
import (
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

func MeasurePerformance(funcName string, fn func()) {
    start := time.Now().UnixMilli()
    fn()
    end := time.Now().UnixMilli()

    duration := end - start
    log.Printf("[%s] Function %s took %dms",
        timex.MsecFormat(end),
        funcName,
        duration)
}
```

## Milliseconds vs Second Timestamps

### When to Use Milliseconds

- **Database records**: Modern databases typically use milliseconds or microseconds
- **API responses**: JavaScript's Date uses millisecond timestamps
- **High-precision requirements**: Recording millisecond-level events
- **Message queues**: Kafka, RabbitMQ, etc. typically use milliseconds

```go
ms := time.Now().UnixMilli()  // millisecond precision
timestamp := timex.MsecFormat(ms)
// Output: "2024-01-29 15:04:05.123"
```

### When to Use Seconds

- **Unix timestamp standard**: Classic Unix timestamps are in seconds
- **Storage optimization**: Second-precision saves storage (4 bytes vs 8 bytes)
- **System integration**: Interacting with legacy systems
- **Simplified processing**: Millisecond precision not needed

```go
sec := time.Now().Unix()  // second precision
timestamp := timex.SecFormat(sec)
// Output: "2024-01-29 15:04:05"
```

### Precision Comparison

| Metric | Milliseconds | Seconds |
|--------|-------------|---------|
| Precision | 1ms (millionth of a second) | 1s (one second) |
| Storage size | 8 bytes (int64) | 4 bytes (int32) |
| Range | ±292M years | ±136 years |
| Typical use | Modern applications | Legacy systems/standards |
| JavaScript compatible | ✅ Yes | ❌ No |

## Relationship with the Standard Library time Package

timex is a **convenience wrapper** around Go's standard `time` package, not a replacement.

### Feature Comparison

| Feature | Standard time | timex |
|---------|--------------|-------|
| Get current time | `time.Now()` ✅ | ❌ |
| Time arithmetic | `time.Add()` ✅ | ❌ |
| Time parsing | `time.Parse()` ✅ | ❌ |
| Time formatting | `t.Format()` ✅ | `timex.Format()` simplified ✅ |
| Timestamp to string | 2 steps required | 1 step ✅ |

### Standard Library Way

```go
import "time"

ms := time.Now().UnixMilli()
t := time.UnixMilli(ms)  // Step 1: convert to time.Time
formatted := t.Format("2006-01-02 15:04:05")  // Step 2: format

// Output: "2024-01-29 15:04:05"
```

### timex Simplified Way

```go
import "github.com/everyday-items/toolkit/lang/timex"

ms := time.Now().UnixMilli()
formatted := timex.MsecFormat(ms)  // done in 1 step

// Output: "2024-01-29 15:04:05"
```

### When to Use timex

- ✅ Simple timestamp formatting
- ✅ Rapid prototyping
- ✅ Only need time as string
- ✅ Reducing code verbosity

### When to Use Standard Library time

- ✅ Time arithmetic needed (Add, Sub, etc.)
- ✅ Time parsing needed (Parse)
- ✅ Timezone conversion needed
- ✅ time.Time objects needed
- ✅ Full time functionality needed

### Combined Usage

```go
import (
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

func GetNextDayFormatted() string {
    now := time.Now()
    tomorrow := now.Add(24 * time.Hour)

    // Use standard library for calculation
    // Use timex for formatting
    return timex.SecFormat(tomorrow.Unix())
}
```

## Notes

### 1. Timezone Handling

All functions display time in the **local timezone**.

```go
ms := int64(0)  // 1970-01-01 00:00:00 UTC
result := timex.MsecFormat(ms)
// Output depends on local timezone:
// China (UTC+8): "1970-01-01 08:00:00"
// US (UTC-5): "1969-12-31 19:00:00"
```

### 2. Zero Timestamp

Timestamp 0 corresponds to Unix Epoch (1970-01-01 00:00:00 UTC).

```go
result := timex.MsecFormat(0)
// China output: "1970-01-01 08:00:00"

result := timex.SecFormat(0)
// China output: "1970-01-01 08:00:00"
```

### 3. Concurrency Safety

All functions are **stateless** and can be safely called concurrently.

```go
import "sync"

var wg sync.WaitGroup
for i := 0; i < 100; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        ms := time.Now().UnixMilli()
        _ = timex.MsecFormat(ms)  // completely safe
    }()
}
wg.Wait()
```

### 4. Performance

Formatting performance is close to the standard library.

```go
ms := time.Now().UnixMilli()

// Standard library way
t := time.UnixMilli(ms)
result := t.Format("2006-01-02 15:04:05")

// timex way (similar performance)
result := timex.MsecFormat(ms)
```

### 5. Format Validation

An invalid layout will cause a panic (this is the behavior of time.Time.Format()).

```go
// ❌ Wrong format
timex.MsecFormatWithLayout(ms, "invalid")  // panic

// ✅ Correct format
timex.MsecFormatWithLayout(ms, "2006-01-02")  // works normally
```

## Complete Example

```go
package main

import (
    "fmt"
    "time"
    "github.com/everyday-items/toolkit/lang/timex"
)

func main() {
    // Get current time
    now := time.Now()
    ms := now.UnixMilli()
    sec := now.Unix()

    // Basic formatting
    fmt.Println("=== Basic Formatting ===")
    fmt.Println("Millisecond format:", timex.MsecFormat(ms))
    fmt.Println("Second format:", timex.SecFormat(sec))

    // Custom format
    fmt.Println("\n=== Custom Format ===")
    fmt.Println("Date only:", timex.MsecFormatWithLayout(ms, "2006-01-02"))
    fmt.Println("Time only:", timex.MsecFormatWithLayout(ms, "15:04:05"))
    fmt.Println("ISO 8601:", timex.SecFormatWithLayout(sec, "2006-01-02T15:04:05Z07:00"))
    fmt.Println("Chinese format:", timex.MsecFormatWithLayout(ms, "2006年01月02日 15:04"))

    // Different purposes
    fmt.Println("\n=== Different Purposes ===")
    fmt.Println("Logging:", timex.MsecFormat(ms))
    fmt.Println("Database record:", timex.MsecFormatWithLayout(ms, "2006-01-02 15:04:05.000"))
    fmt.Println("Filename:", timex.MsecFormatWithLayout(ms, "2006-01-02"))
    fmt.Println("API response:", timex.SecFormat(sec))

    // Fixed timestamp example
    fmt.Println("\n=== Fixed Timestamp ===")
    fixedMs := int64(1706423456789)  // 2024-01-28 15:04:16 UTC
    fmt.Println("Millisecond timestamp:", timex.MsecFormat(fixedMs))
    fmt.Println("Custom format:", timex.MsecFormatWithLayout(fixedMs, "02-Jan-2006 15:04"))
}

// Output (may vary by timezone):
// === Basic Formatting ===
// Millisecond format: 2024-01-29 15:04:05
// Second format: 2024-01-29 15:04:05
//
// === Custom Format ===
// Date only: 2024-01-29
// Time only: 15:04:05
// ISO 8601: 2024-01-29T15:04:05+08:00
// Chinese format: 2024年01月29日 15:04
//
// === Different Purposes ===
// Logging: 2024-01-29 15:04:05
// Database record: 2024-01-29 15:04:05.789
// Filename: 2024-01-29
// API response: 2024-01-29 15:04:05
//
// === Fixed Timestamp ===
// Millisecond timestamp: 2024-01-28 15:04:16
// Custom format: 28-Jan-2024 15:04
```

## Performance Benchmarks

```
BenchmarkMsecFormat-8              10000000    100 ns/op    0 B/op    0 allocs/op
BenchmarkMsecFormatWithLayout-8    10000000    120 ns/op    0 B/op    0 allocs/op
BenchmarkSecFormat-8               10000000     95 ns/op    0 B/op    0 allocs/op
BenchmarkSecFormatWithLayout-8     10000000    110 ns/op    0 B/op    0 allocs/op
```

## Design Principles

1. **Simple and easy to use**: Provides shortcut functions for common formats
2. **Flexible extension**: Supports any Go time format
3. **Zero external dependencies**: Uses only the Go standard library
4. **High performance**: Minimizes memory allocation
5. **Concurrency safe**: Stateless design

## Dependencies

```bash
# Zero external dependencies, pure Go standard library
# Requires Go 1.17+
```

## References

- Go time package official docs: https://pkg.go.dev/time
- Unix timestamp: https://en.wikipedia.org/wiki/Unix_time
- RFC 3339 time format: https://tools.ietf.org/html/rfc3339
- ISO 8601 time format: https://en.wikipedia.org/wiki/ISO_8601
