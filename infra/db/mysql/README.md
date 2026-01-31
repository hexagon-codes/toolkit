# MySQL 数据库封装

生产级 MySQL 数据库连接池封装，支持连接池管理、健康检查、事务等功能。

## 特性

- ✅ 单例模式 - 全局统一实例
- ✅ 连接池管理 - 自动管理连接生命周期
- ✅ 健康检查 - Ping 检测连接状态
- ✅ 超时控制 - 支持查询超时
- ✅ 事务封装 - 自动 Rollback/Commit
- ✅ 统计信息 - 连接池状态监控
- ✅ 日志接口 - 可插拔的日志系统

## 快速开始

### 1. 初始化

```go
package main

import (
    "github.com/everyday-items/toolkit/infra/db/mysql"
)

func main() {
    // 使用默认配置
    config := mysql.DefaultConfig("user:password@tcp(localhost:3306)/dbname?parseTime=true&charset=utf8mb4")

    // 初始化全局实例
    db, err := mysql.Init(config)
    if err != nil {
        panic(err)
    }
    defer db.Close()
}
```

### 2. 自定义配置

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

## 使用示例

### 基础查询

```go
// 获取全局实例
db := mysql.GetGlobal()

// 查询单行
var name string
err := db.QueryRow("SELECT name FROM users WHERE id = ?", 1).Scan(&name)

// 查询多行
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

### 带超时查询

```go
ctx := context.Background()

// 5秒超时查询
rows, err := db.QueryWithTimeout(ctx, 5*time.Second,
    "SELECT * FROM large_table")
```

### 执行语句

```go
// 插入
result, err := db.Exec("INSERT INTO users (name, age) VALUES (?, ?)", "Alice", 25)
if err != nil {
    log.Fatal(err)
}

id, _ := result.LastInsertId()
affected, _ := result.RowsAffected()
```

### 事务

```go
ctx := context.Background()

err := db.Transaction(ctx, func(tx *sql.Tx) error {
    // 操作1
    _, err := tx.Exec("UPDATE accounts SET balance = balance - 100 WHERE id = ?", 1)
    if err != nil {
        return err // 自动回滚
    }

    // 操作2
    _, err = tx.Exec("UPDATE accounts SET balance = balance + 100 WHERE id = ?", 2)
    if err != nil {
        return err // 自动回滚
    }

    return nil // 自动提交
})

if err != nil {
    log.Printf("Transaction failed: %v", err)
}
```

### 健康检查

```go
ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()

if err := db.Health(ctx); err != nil {
    log.Printf("MySQL unhealthy: %v", err)
}
```

### 连接池统计

```go
stats := db.Stats()

fmt.Printf("MaxOpenConnections: %d\n", stats.MaxOpenConnections)
fmt.Printf("OpenConnections: %d\n", stats.OpenConnections)
fmt.Printf("InUse: %d\n", stats.InUse)
fmt.Printf("Idle: %d\n", stats.Idle)
fmt.Printf("WaitCount: %d\n", stats.WaitCount)
fmt.Printf("WaitDuration: %v\n", stats.WaitDuration)
```

## 配置说明

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `DSN` | string | - | 数据源名称（必填） |
| `MaxOpenConns` | int | 100 | 最大打开连接数 |
| `MaxIdleConns` | int | 10 | 最大空闲连接数 |
| `ConnMaxLifetime` | Duration | 1h | 连接最大生命周期 |
| `ConnMaxIdleTime` | Duration | 10m | 连接最大空闲时间 |
| `ConnectTimeout` | Duration | 10s | 连接超时 |
| `ReadTimeout` | Duration | 30s | 读超时 |
| `WriteTimeout` | Duration | 30s | 写超时 |
| `ParseTime` | bool | true | 解析时间类型 |
| `Charset` | string | utf8mb4 | 字符集 |
| `Collation` | string | utf8mb4_unicode_ci | 排序规则 |

## DSN 格式

```
[username[:password]@][protocol[(address)]]/dbname[?param1=value1&...&paramN=valueN]
```

示例：
```
user:password@tcp(localhost:3306)/dbname?parseTime=true&charset=utf8mb4
```

常用参数：
- `parseTime=true` - 自动解析 DATE/DATETIME 到 time.Time
- `charset=utf8mb4` - 使用 UTF-8 字符集
- `loc=Local` - 时区设置
- `timeout=10s` - 连接超时
- `readTimeout=30s` - 读超时
- `writeTimeout=30s` - 写超时

## 最佳实践

### 1. 使用单例模式

```go
// 初始化一次
func init() {
    config := mysql.DefaultConfig(os.Getenv("MYSQL_DSN"))
    if _, err := mysql.Init(config); err != nil {
        log.Fatal(err)
    }
}

// 全局使用
func GetUser(id int) (*User, error) {
    db := mysql.GetGlobal()
    // ...
}
```

### 2. 使用 Context 控制超时

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

rows, err := db.QueryContext(ctx, "SELECT * FROM users")
```

### 3. 正确处理连接

```go
// ✅ 正确：使用 defer 关闭
rows, err := db.Query("SELECT * FROM users")
if err != nil {
    return err
}
defer rows.Close() // 确保关闭

for rows.Next() {
    // 处理数据
}
```

### 4. 使用预编译语句（防 SQL 注入）

```go
// ✅ 使用占位符
db.Query("SELECT * FROM users WHERE id = ?", userID)

// ❌ 拼接字符串（危险）
db.Query(fmt.Sprintf("SELECT * FROM users WHERE id = %d", userID))
```

### 5. 合理配置连接池

```go
// 根据服务规模调整
config := mysql.DefaultConfig(dsn)

// 小型服务
config.MaxOpenConns = 50
config.MaxIdleConns = 5

// 中型服务
config.MaxOpenConns = 100
config.MaxIdleConns = 10

// 大型服务
config.MaxOpenConns = 200
config.MaxIdleConns = 20
```

## 依赖

```bash
go get -u github.com/go-sql-driver/mysql
```

## 注意事项

1. **连接数限制**：MaxOpenConns 不应超过 MySQL 的 max_connections
2. **超时设置**：根据业务需求合理设置超时时间
3. **事务使用**：长事务会占用连接，影响性能
4. **连接泄漏**：务必 defer rows.Close()
5. **SQL 注入**：始终使用占位符，不要拼接 SQL
