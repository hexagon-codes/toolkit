package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/everyday-items/toolkit/infra/db/mysql"
)

// User 示例用户结构
type User struct {
	ID        int64
	Username  string
	Email     string
	CreatedAt time.Time
}

func main() {
	fmt.Println("=== MySQL 使用示例 ===")

	// 1. 连接初始化
	fmt.Println("1. 初始化 MySQL 连接")
	db := initMySQL()
	defer db.Close()

	// 2. 基本 CRUD 操作
	fmt.Println("\n2. 基本 CRUD 操作")
	demonstrateCRUD(db)

	// 3. 事务使用
	fmt.Println("\n3. 事务使用")
	demonstrateTransaction(db)

	// 4. 连接池配置和监控
	fmt.Println("\n4. 连接池监控")
	monitorConnectionPool(db)

	// 5. 高级查询
	fmt.Println("\n5. 高级查询")
	demonstrateAdvancedQuery(db)
}

// initMySQL 初始化 MySQL 连接
func initMySQL() *mysql.DB {
	// 方式1: 使用默认配置
	dsn := "user:password@tcp(127.0.0.1:3306)/testdb?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci"
	config := mysql.DefaultConfig(dsn)

	// 方式2: 自定义配置
	config = &mysql.Config{
		DSN:              dsn,
		MaxOpenConns:     50,               // 最大打开连接数
		MaxIdleConns:     10,               // 最大空闲连接数
		ConnMaxLifetime:  time.Hour,        // 连接最大生命周期
		ConnMaxIdleTime:  10 * time.Minute, // 连接最大空闲时间
		ConnectTimeout:   10 * time.Second, // 连接超时
		ReadTimeout:      30 * time.Second, // 读超时
		WriteTimeout:     30 * time.Second, // 写超时
		ParseTime:        true,             // 解析时间类型
		Charset:          "utf8mb4",        // 字符集
		Collation:        "utf8mb4_unicode_ci",
		Loc:              "Local",            // 时区
		MaxAllowedPacket: 4 << 20,            // 4MB
		Logger:           &mysql.StdLogger{}, // 可选的日志
	}

	// 创建连接
	db, err := mysql.New(config)
	if err != nil {
		log.Fatalf("Failed to connect to MySQL: %v", err)
	}

	fmt.Printf("✓ MySQL 连接成功\n")

	// 健康检查
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.Health(ctx); err != nil {
		log.Fatalf("Health check failed: %v", err)
	}

	fmt.Printf("✓ 健康检查通过\n")

	return db
}

// demonstrateCRUD 演示基本 CRUD 操作
func demonstrateCRUD(db *mysql.DB) {
	ctx := context.Background()

	// CREATE - 插入数据
	fmt.Println("\n  [CREATE] 插入用户")
	insertSQL := `INSERT INTO users (username, email, created_at) VALUES (?, ?, ?)`
	result, err := db.ExecContext(ctx, insertSQL, "john_doe", "john@example.com", time.Now())
	if err != nil {
		log.Printf("插入失败: %v", err)
	} else {
		lastID, _ := result.LastInsertId()
		rowsAffected, _ := result.RowsAffected()
		fmt.Printf("  ✓ 插入成功 - ID: %d, 影响行数: %d\n", lastID, rowsAffected)
	}

	// READ - 查询单条数据
	fmt.Println("\n  [READ] 查询单个用户")
	var user User
	querySQL := `SELECT id, username, email, created_at FROM users WHERE username = ?`
	err = db.QueryRowContext(ctx, querySQL, "john_doe").Scan(
		&user.ID, &user.Username, &user.Email, &user.CreatedAt,
	)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("查询失败: %v", err)
	} else if err == sql.ErrNoRows {
		fmt.Printf("  未找到用户\n")
	} else {
		fmt.Printf("  ✓ 用户信息: ID=%d, Username=%s, Email=%s\n",
			user.ID, user.Username, user.Email)
	}

	// UPDATE - 更新数据
	fmt.Println("\n  [UPDATE] 更新用户邮箱")
	updateSQL := `UPDATE users SET email = ? WHERE username = ?`
	result, err = db.ExecContext(ctx, updateSQL, "newemail@example.com", "john_doe")
	if err != nil {
		log.Printf("更新失败: %v", err)
	} else {
		rowsAffected, _ := result.RowsAffected()
		fmt.Printf("  ✓ 更新成功 - 影响行数: %d\n", rowsAffected)
	}

	// DELETE - 删除数据（示例，实际可能不执行）
	fmt.Println("\n  [DELETE] 删除用户")
	deleteSQL := `DELETE FROM users WHERE username = ?`
	result, err = db.ExecContext(ctx, deleteSQL, "john_doe")
	if err != nil {
		log.Printf("删除失败: %v", err)
	} else {
		rowsAffected, _ := result.RowsAffected()
		fmt.Printf("  ✓ 删除成功 - 影响行数: %d\n", rowsAffected)
	}
}

// demonstrateTransaction 演示事务使用
func demonstrateTransaction(db *mysql.DB) {
	ctx := context.Background()

	fmt.Println("\n  场景：转账事务（扣款 + 加款必须同时成功）")

	// 使用封装的事务方法
	err := db.Transaction(ctx, func(tx *sql.Tx) error {
		// 第一步：从账户 A 扣款
		_, err := tx.ExecContext(ctx,
			`UPDATE accounts SET balance = balance - ? WHERE user_id = ?`,
			100.0, 1)
		if err != nil {
			return fmt.Errorf("扣款失败: %w", err)
		}
		fmt.Printf("  ✓ 账户 1 扣款 100 元\n")

		// 第二步：给账户 B 加款
		_, err = tx.ExecContext(ctx,
			`UPDATE accounts SET balance = balance + ? WHERE user_id = ?`,
			100.0, 2)
		if err != nil {
			return fmt.Errorf("加款失败: %w", err)
		}
		fmt.Printf("  ✓ 账户 2 加款 100 元\n")

		// 模拟错误回滚（取消注释测试）
		// return fmt.Errorf("模拟错误，触发回滚")

		return nil
	})

	if err != nil {
		fmt.Printf("  ✗ 事务失败（已回滚）: %v\n", err)
	} else {
		fmt.Printf("  ✓ 事务提交成功\n")
	}

	// 手动事务控制示例
	fmt.Println("\n  手动事务控制示例")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		log.Printf("开始事务失败: %v", err)
		return
	}

	// 记得 defer rollback（如果 commit 成功则 rollback 无效）
	defer tx.Rollback()

	// 执行操作
	_, err = tx.ExecContext(ctx, `INSERT INTO logs (message) VALUES (?)`, "事务日志")
	if err != nil {
		fmt.Printf("  ✗ 插入失败，自动回滚\n")
		return
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		fmt.Printf("  ✗ 提交失败: %v\n", err)
		return
	}

	fmt.Printf("  ✓ 手动事务提交成功\n")
}

// monitorConnectionPool 连接池监控
func monitorConnectionPool(db *mysql.DB) {
	// 获取连接池统计信息
	stats := db.Stats()

	fmt.Printf("\n  连接池状态:\n")
	fmt.Printf("  - 打开连接数: %d\n", stats.OpenConnections)
	fmt.Printf("  - 使用中连接: %d\n", stats.InUse)
	fmt.Printf("  - 空闲连接数: %d\n", stats.Idle)
	fmt.Printf("  - 等待连接数: %d\n", stats.WaitCount)
	fmt.Printf("  - 等待总时长: %v\n", stats.WaitDuration)
	fmt.Printf("  - 最大空闲关闭: %d\n", stats.MaxIdleClosed)
	fmt.Printf("  - 最大生命周期关闭: %d\n", stats.MaxLifetimeClosed)

	// 健康检查
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := db.Health(ctx); err != nil {
		fmt.Printf("  ✗ 健康检查失败: %v\n", err)
	} else {
		fmt.Printf("  ✓ 健康检查通过\n")
	}
}

// demonstrateAdvancedQuery 高级查询示例
func demonstrateAdvancedQuery(db *mysql.DB) {
	ctx := context.Background()

	// 1. 批量查询
	fmt.Println("\n  [批量查询] 查询多个用户")
	rows, err := db.QueryContext(ctx, `SELECT id, username, email FROM users LIMIT 10`)
	if err != nil {
		log.Printf("查询失败: %v", err)
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.Username, &user.Email); err != nil {
			log.Printf("扫描失败: %v", err)
			continue
		}
		count++
		fmt.Printf("  - User: ID=%d, Username=%s\n", user.ID, user.Username)
	}

	if err := rows.Err(); err != nil {
		log.Printf("遍历失败: %v", err)
	}
	fmt.Printf("  ✓ 查询到 %d 个用户\n", count)

	// 2. 使用带超时的查询
	fmt.Println("\n  [带超时查询] 5秒超时")
	row := db.QueryRowWithTimeout(ctx, 5*time.Second,
		`SELECT COUNT(*) FROM users`)

	var totalUsers int
	if err := row.Scan(&totalUsers); err != nil {
		log.Printf("查询失败: %v", err)
	} else {
		fmt.Printf("  ✓ 总用户数: %d\n", totalUsers)
	}

	// 3. 预处理语句（性能优化）
	fmt.Println("\n  [预处理语句] 批量插入")
	stmt, err := db.PrepareContext(ctx, `INSERT INTO users (username, email, created_at) VALUES (?, ?, ?)`)
	if err != nil {
		log.Printf("预处理失败: %v", err)
		return
	}
	defer stmt.Close()

	users := []struct {
		username string
		email    string
	}{
		{"user1", "user1@example.com"},
		{"user2", "user2@example.com"},
		{"user3", "user3@example.com"},
	}

	for _, u := range users {
		_, err := stmt.ExecContext(ctx, u.username, u.email, time.Now())
		if err != nil {
			log.Printf("插入 %s 失败: %v", u.username, err)
		}
	}
	fmt.Printf("  ✓ 批量插入完成\n")
}
