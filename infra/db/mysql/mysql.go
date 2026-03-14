package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql" // MySQL 驱动
)

var (
	// 全局实例（单例模式）
	globalDB   *DB
	globalOnce sync.Once
)

// DB MySQL 数据库封装
type DB struct {
	*sql.DB
	config *Config
}

// Init 初始化全局 MySQL 实例
func Init(config *Config) (*DB, error) {
	var err error
	globalOnce.Do(func() {
		globalDB, err = New(config)
	})
	return globalDB, err
}

// GetGlobal 获取全局 MySQL 实例
func GetGlobal() *DB {
	return globalDB
}

// New 创建新的 MySQL 连接
func New(config *Config) (*DB, error) {
	if config == nil {
		return nil, fmt.Errorf("mysql config is nil")
	}

	dsn := config.BuildDSN()
	if dsn == "" {
		return nil, fmt.Errorf("mysql DSN is empty")
	}

	// 打开数据库连接
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		if config.Logger != nil {
			config.Logger.Error("failed to open mysql connection", err)
		}
		return nil, fmt.Errorf("failed to open mysql: %w", err)
	}

	// 配置连接池
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), config.ConnectTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		if config.Logger != nil {
			config.Logger.Error("failed to ping mysql", err)
		}
		return nil, fmt.Errorf("failed to ping mysql: %w", err)
	}

	if config.Logger != nil {
		config.Logger.Printf("mysql connected successfully: %s", maskDSN(dsn))
	}

	return &DB{
		DB:     db,
		config: config,
	}, nil
}

// Health 健康检查
func (db *DB) Health(ctx context.Context) error {
	if db == nil || db.DB == nil {
		return fmt.Errorf("mysql db is nil")
	}

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("mysql health check failed: %w", err)
	}

	return nil
}

// Stats 返回数据库统计信息
func (db *DB) Stats() sql.DBStats {
	if db == nil || db.DB == nil {
		return sql.DBStats{}
	}
	return db.DB.Stats()
}

// ExecWithTimeout 带超时的 Exec
func (db *DB) ExecWithTimeout(ctx context.Context, timeout time.Duration, query string, args ...any) (sql.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return db.ExecContext(ctx, query, args...)
}

// QueryWithTimeout 带超时的 Query
//
// 警告：此函数存在 context 生命周期问题，推荐使用 QueryWithTimeoutEx
// 或直接使用 QueryContext 并自行管理 context。
// 原因：Rows 返回后 cancel 立即调用，但 Scan() 仍需要有效的 context。
func (db *DB) QueryWithTimeout(ctx context.Context, timeout time.Duration, query string, args ...any) (*sql.Rows, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return db.QueryContext(ctx, query, args...)
}

// QueryWithTimeoutEx 带超时的 Query（返回 cancel 函数）
//
// 调用者必须在 Rows 处理完成后调用 cancel 函数释放资源
//
// 示例：
//
//	rows, cancel, err := db.QueryWithTimeoutEx(ctx, 5*time.Second, "SELECT id, name FROM users")
//	if err != nil { return err }
//	defer cancel()
//	defer rows.Close()
//	for rows.Next() { ... }
func (db *DB) QueryWithTimeoutEx(ctx context.Context, timeout time.Duration, query string, args ...any) (*sql.Rows, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return rows, cancel, nil
}

// QueryRowWithTimeout 带超时的 QueryRow
//
// 警告：此函数存在 context 生命周期问题，推荐使用 QueryRowWithTimeoutEx
// 或直接使用 QueryRowContext 并自行管理 context
func (db *DB) QueryRowWithTimeout(ctx context.Context, timeout time.Duration, query string, args ...any) *sql.Row {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return db.QueryRowContext(ctx, query, args...)
}

// QueryRowWithTimeoutEx 带超时的 QueryRow（返回 cancel 函数）
//
// 调用者必须在 Scan 完成后调用 cancel 函数释放资源
//
// 示例：
//
//	row, cancel := db.QueryRowWithTimeoutEx(ctx, 5*time.Second, "SELECT name FROM users WHERE id = ?", 1)
//	defer cancel()
//	var name string
//	err := row.Scan(&name)
func (db *DB) QueryRowWithTimeoutEx(ctx context.Context, timeout time.Duration, query string, args ...any) (*sql.Row, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	return db.QueryRowContext(ctx, query, args...), cancel
}

// Transaction 事务封装
func (db *DB) Transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("tx error: %v, rollback error: %v", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Close 关闭数据库连接
func (db *DB) Close() error {
	if db == nil || db.DB == nil {
		return nil
	}
	return db.DB.Close()
}

// maskDSN 隐藏 DSN 中的敏感信息
// 解析 DSN 中 @ 前的 user:password 部分，仅遮蔽 password
func maskDSN(dsn string) string {
	// MySQL DSN 格式: user:password@tcp(host:port)/dbname?params
	atIdx := strings.Index(dsn, "@")
	if atIdx < 0 {
		// 没有 @ 符号，无法解析，安全起见全部遮蔽
		return "***"
	}

	userPass := dsn[:atIdx]
	rest := dsn[atIdx:] // 包含 @

	colonIdx := strings.Index(userPass, ":")
	if colonIdx < 0 {
		// 没有密码部分，直接返回
		return userPass + rest
	}

	// 保留用户名，遮蔽密码
	return userPass[:colonIdx] + ":***" + rest
}
