package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/hexagon-codes/toolkit/infra/db/mysql"
)

// User 示例用户结构
type User struct {
	ID        int64
	Username  string
	Email     string
	CreatedAt time.Time
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() (resultErr error) {
	dsn, mysqlConfigured := configuredMySQLDSN()
	if !mysqlConfigured {
		fmt.Println("Set MYSQL_DSN to run this example.")
		return nil
	}

	fmt.Println("=== MySQL 使用示例 ===")

	// 1. 连接初始化
	fmt.Println("1. 初始化 MySQL 连接")
	db, err := initMySQL(dsn)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, db.Close())
	}()

	// 2. 基本 CRUD 操作
	fmt.Println("\n2. 基本 CRUD 操作")
	if err := demonstrateCRUD(db); err != nil {
		return err
	}

	// 3. 事务使用
	fmt.Println("\n3. 事务使用")
	if err := demonstrateTransaction(db); err != nil {
		return err
	}

	// 4. 连接池配置和监控
	fmt.Println("\n4. 连接池监控")
	if err := monitorConnectionPool(db); err != nil {
		return err
	}

	// 5. 高级查询
	fmt.Println("\n5. 高级查询")
	if err := demonstrateAdvancedQuery(db); err != nil {
		return err
	}
	return nil
}

func configuredMySQLDSN() (string, bool) {
	dsn := strings.TrimSpace(os.Getenv("MYSQL_DSN"))
	return dsn, dsn != ""
}

// initMySQL 初始化 MySQL 连接
func initMySQL(dsn string) (*mysql.DB, error) {
	// 方式1: 使用默认配置
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
		return nil, fmt.Errorf("connect to MySQL: %w", err)
	}

	fmt.Printf("✓ MySQL 连接成功\n")

	// 健康检查
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.Health(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("check MySQL health: %w", err), db.Close())
	}

	fmt.Printf("✓ 健康检查通过\n")

	return db, nil
}

// demonstrateCRUD 演示基本 CRUD 操作
func demonstrateCRUD(db *mysql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// CREATE - 插入数据
	fmt.Println("\n  [CREATE] 插入用户")
	insertSQL := `INSERT INTO users (username, email, created_at) VALUES (?, ?, ?)`
	result, err := db.ExecContext(ctx, insertSQL, "john_doe", "john@example.com", time.Now())
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	lastID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("read inserted user ID: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read inserted user row count: %w", err)
	}
	fmt.Printf("  ✓ User inserted - ID: %d, rows affected: %d\n", lastID, rowsAffected)

	// READ - 查询单条数据
	fmt.Println("\n  [READ] 查询单个用户")
	var user User
	querySQL := `SELECT id, username, email, created_at FROM users WHERE username = ?`
	err = db.QueryRowContext(ctx, querySQL, "john_doe").Scan(
		&user.ID, &user.Username, &user.Email, &user.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("query user: %w", err)
	}
	fmt.Printf("  ✓ User: ID=%d, Username=%s, Email=%s\n",
		user.ID, user.Username, user.Email)

	// UPDATE - 更新数据
	fmt.Println("\n  [UPDATE] 更新用户邮箱")
	updateSQL := `UPDATE users SET email = ? WHERE username = ?`
	result, err = db.ExecContext(ctx, updateSQL, "newemail@example.com", "john_doe")
	if err != nil {
		return fmt.Errorf("update user email: %w", err)
	}
	rowsAffected, err = result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read updated user row count: %w", err)
	}
	fmt.Printf("  ✓ User updated - rows affected: %d\n", rowsAffected)

	// DELETE - 删除数据（示例，实际可能不执行）
	fmt.Println("\n  [DELETE] 删除用户")
	deleteSQL := `DELETE FROM users WHERE username = ?`
	result, err = db.ExecContext(ctx, deleteSQL, "john_doe")
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	rowsAffected, err = result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted user row count: %w", err)
	}
	fmt.Printf("  ✓ User deleted - rows affected: %d\n", rowsAffected)
	return nil
}

// demonstrateTransaction 演示事务使用
func demonstrateTransaction(db *mysql.DB) (resultErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("\n  场景：转账事务（扣款 + 加款必须同时成功）")

	// 使用封装的事务方法
	err := db.Transaction(ctx, func(tx *sql.Tx) error {
		// 第一步：从账户 A 扣款
		_, err := tx.ExecContext(ctx,
			`UPDATE accounts SET balance = balance - ? WHERE user_id = ?`,
			100.0, 1)
		if err != nil {
			return fmt.Errorf("debit account 1: %w", err)
		}
		fmt.Printf("  ✓ 账户 1 扣款 100 元\n")

		// 第二步：给账户 B 加款
		_, err = tx.ExecContext(ctx,
			`UPDATE accounts SET balance = balance + ? WHERE user_id = ?`,
			100.0, 2)
		if err != nil {
			return fmt.Errorf("credit account 2: %w", err)
		}
		fmt.Printf("  ✓ 账户 2 加款 100 元\n")

		// 模拟错误回滚（取消注释测试）
		// return fmt.Errorf("模拟错误，触发回滚")

		return nil
	})

	if err != nil {
		return fmt.Errorf("execute transfer transaction: %w", err)
	}
	fmt.Printf("  ✓ Transaction committed\n")

	// 手动事务控制示例
	fmt.Println("\n  手动事务控制示例")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin manual transaction: %w", err)
	}

	// Commit 成功后 Rollback 返回 sql.ErrTxDone，不属于关闭失败。
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			resultErr = errors.Join(resultErr, fmt.Errorf("roll back manual transaction: %w", err))
		}
	}()

	// 执行操作
	_, err = tx.ExecContext(ctx, `INSERT INTO logs (message) VALUES (?)`, "事务日志")
	if err != nil {
		return fmt.Errorf("insert manual transaction log: %w", err)
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit manual transaction: %w", err)
	}

	fmt.Printf("  ✓ 手动事务提交成功\n")
	return nil
}

// monitorConnectionPool 连接池监控
func monitorConnectionPool(db *mysql.DB) error {
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
		return fmt.Errorf("check MySQL pool health: %w", err)
	}
	fmt.Printf("  ✓ Health check passed\n")
	return nil
}

// demonstrateAdvancedQuery 高级查询示例
func demonstrateAdvancedQuery(db *mysql.DB) error {
	ctx, cancelContext := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelContext()

	// 1. 批量查询
	fmt.Println("\n  [批量查询] 查询多个用户")
	rows, err := db.QueryContext(ctx, `SELECT id, username, email FROM users LIMIT 10`)
	if err != nil {
		return fmt.Errorf("query users: %w", err)
	}

	count := 0
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.Username, &user.Email); err != nil {
			return errors.Join(fmt.Errorf("scan user row: %w", err), rows.Close())
		}
		count++
		fmt.Printf("  - User: ID=%d, Username=%s\n", user.ID, user.Username)
	}

	if err := rows.Err(); err != nil {
		return errors.Join(fmt.Errorf("iterate user rows: %w", err), rows.Close())
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close user rows: %w", err)
	}
	fmt.Printf("  ✓ 查询到 %d 个用户\n", count)

	// 2. 使用带超时的查询
	fmt.Println("\n  [带超时查询] 5秒超时")
	row, cancel := db.QueryRowWithTimeout(ctx, 5*time.Second,
		`SELECT COUNT(*) FROM users`)
	defer cancel()

	var totalUsers int
	if err := row.Scan(&totalUsers); err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	fmt.Printf("  ✓ Total users: %d\n", totalUsers)

	// 3. 预处理语句（性能优化）
	fmt.Println("\n  [预处理语句] 批量插入")
	stmt, err := db.PrepareContext(ctx, `INSERT INTO users (username, email, created_at) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare user insert: %w", err)
	}

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
			return errors.Join(fmt.Errorf("insert user %s: %w", u.username, err), stmt.Close())
		}
	}
	if err := stmt.Close(); err != nil {
		return fmt.Errorf("close user insert statement: %w", err)
	}
	fmt.Printf("  ✓ 批量插入完成\n")
	return nil
}
