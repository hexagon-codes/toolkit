[中文](README.md) | English

# MySQL Database Wrapper

Production-grade MySQL database connection pool wrapper with connection pool management, health check, transaction support, and more.

## Features

- ✅ Singleton pattern - global unified instance
- ✅ Connection pool management - automatic connection lifecycle management
- ✅ Health check - Ping to detect connection state
- ✅ Timeout control - supports query timeout
- ✅ Transaction wrapper - automatic Rollback/Commit
- ✅ Statistics - connection pool status monitoring
- ✅ Logger interface - pluggable logging system

## Quick Start

### 1. Initialization

```go
package main

import (
    "github.com/everyday-items/toolkit/infra/db/mysql"
)

func main() {
    // Use default configuration
    config := mysql.DefaultConfig("user:password@tcp(localhost:3306)/dbname?parseTime=true&charset=utf8mb4")

    // Initialize global instance
    db, err := mysql.Init(config)
    if err != nil {
        panic(err)
    }
    defer db.Close()
}
```

### 2. Custom Configuration

```go
config := &mysql.Config{
    DSN:              "user:password@tcp(localhost:3306)/dbname",
    MaxOpenConns:     200,
    MaxIdleConns:     20,
    ConnMaxLifetime:  time.Hour,
    ConnMaxIdleTime:  10 * time.Minute,
    ConnectTimeout:   10 * time.Second,
    ReadTimeout:      30 * time.Second,
    WriteTimeout:     30 * time.Second,
    ParseTime:        true,
    Charset:          "utf8mb4",
    Collation:        "utf8mb4_unicode_ci",
    Loc:              "Local",
}

db, err := mysql.New(config)
```

## Usage Examples

### Basic Query

```go
// Get global instance
db := mysql.GetGlobal()

// Query single row
var name string
err := db.QueryRow("SELECT name FROM users WHERE id = ?", 1).Scan(&name)

// Query multiple rows
rows, err := db.Query("SELECT id, name FROM users WHERE age > ?", 18)
if err != nil {
    log.Fatal(err)
}
defer rows.Close()

for rows.Next() {
    var id int
    var name string
    if err := rows.Scan(&id, &name); err != nil {
        log.Fatal(err)
    }
    fmt.Printf("User: %d - %s\n", id, name)
}
```

### Query with Timeout

```go
ctx := context.Background()

// Query with 5-second timeout
rows, err := db.QueryWithTimeout(ctx, 5*time.Second,
    "SELECT * FROM large_table")
```

### Execute Statement

```go
// Insert
result, err := db.Exec("INSERT INTO users (name, age) VALUES (?, ?)", "Alice", 25)
if err != nil {
    log.Fatal(err)
}

id, _ := result.LastInsertId()
affected, _ := result.RowsAffected()
```

### Transaction

```go
ctx := context.Background()

err := db.Transaction(ctx, func(tx *sql.Tx) error {
    // Operation 1
    _, err := tx.Exec("UPDATE accounts SET balance = balance - 100 WHERE id = ?", 1)
    if err != nil {
        return err // auto rollback
    }

    // Operation 2
    _, err = tx.Exec("UPDATE accounts SET balance = balance + 100 WHERE id = ?", 2)
    if err != nil {
        return err // auto rollback
    }

    return nil // auto commit
})

if err != nil {
    log.Printf("Transaction failed: %v", err)
}
```

### Health Check

```go
ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()

if err := db.Health(ctx); err != nil {
    log.Printf("MySQL unhealthy: %v", err)
}
```

### Connection Pool Statistics

```go
stats := db.Stats()

fmt.Printf("MaxOpenConnections: %d\n", stats.MaxOpenConnections)
fmt.Printf("OpenConnections: %d\n", stats.OpenConnections)
fmt.Printf("InUse: %d\n", stats.InUse)
fmt.Printf("Idle: %d\n", stats.Idle)
fmt.Printf("WaitCount: %d\n", stats.WaitCount)
fmt.Printf("WaitDuration: %v\n", stats.WaitDuration)
```

## Configuration Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `DSN` | string | - | Data Source Name (required) |
| `MaxOpenConns` | int | 100 | Maximum open connections |
| `MaxIdleConns` | int | 10 | Maximum idle connections |
| `ConnMaxLifetime` | Duration | 1h | Maximum connection lifetime |
| `ConnMaxIdleTime` | Duration | 10m | Maximum connection idle time |
| `ConnectTimeout` | Duration | 10s | Connection timeout |
| `ReadTimeout` | Duration | 30s | Read timeout |
| `WriteTimeout` | Duration | 30s | Write timeout |
| `ParseTime` | bool | true | Parse time types |
| `Charset` | string | utf8mb4 | Character set |
| `Collation` | string | utf8mb4_unicode_ci | Collation |

## DSN Format

```
[username[:password]@][protocol[(address)]]/dbname[?param1=value1&...&paramN=valueN]
```

Example:
```
user:password@tcp(localhost:3306)/dbname?parseTime=true&charset=utf8mb4
```

Common parameters:
- `parseTime=true` - automatically parse DATE/DATETIME to time.Time
- `charset=utf8mb4` - use UTF-8 character set
- `loc=Local` - timezone setting
- `timeout=10s` - connection timeout
- `readTimeout=30s` - read timeout
- `writeTimeout=30s` - write timeout

## Best Practices

### 1. Use Singleton Pattern

```go
// Initialize once
func init() {
    config := mysql.DefaultConfig(os.Getenv("MYSQL_DSN"))
    if _, err := mysql.Init(config); err != nil {
        log.Fatal(err)
    }
}

// Use globally
func GetUser(id int) (*User, error) {
    db := mysql.GetGlobal()
    // ...
}
```

### 2. Use Context for Timeout Control

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

rows, err := db.QueryContext(ctx, "SELECT * FROM users")
```

### 3. Properly Handle Connections

```go
// ✅ Correct: use defer to close
rows, err := db.Query("SELECT * FROM users")
if err != nil {
    return err
}
defer rows.Close() // ensure close

for rows.Next() {
    // process data
}
```

### 4. Use Prepared Statements (prevent SQL injection)

```go
// ✅ Use placeholders
db.Query("SELECT * FROM users WHERE id = ?", userID)

// ❌ String concatenation (dangerous)
db.Query(fmt.Sprintf("SELECT * FROM users WHERE id = %d", userID))
```

### 5. Properly Configure Connection Pool

```go
// Adjust according to service scale
config := mysql.DefaultConfig(dsn)

// Small service
config.MaxOpenConns = 50
config.MaxIdleConns = 5

// Medium service
config.MaxOpenConns = 100
config.MaxIdleConns = 10

// Large service
config.MaxOpenConns = 200
config.MaxIdleConns = 20
```

## Dependencies

```bash
go get -u github.com/go-sql-driver/mysql
```

## Notes

1. **Connection limit**: MaxOpenConns should not exceed MySQL's max_connections
2. **Timeout settings**: Set timeouts appropriately based on business requirements
3. **Transaction usage**: Long transactions hold connections and impact performance
4. **Connection leak**: Always use defer rows.Close()
5. **SQL injection**: Always use placeholders, never concatenate SQL strings
