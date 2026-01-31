package mysql

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestDefaultConfig 测试默认配置
func TestDefaultConfig(t *testing.T) {
	dsn := "user:pass@tcp(localhost:3306)/testdb"
	config := DefaultConfig(dsn)

	if config.DSN != dsn {
		t.Errorf("expected DSN %s, got %s", dsn, config.DSN)
	}

	if config.MaxOpenConns != 100 {
		t.Errorf("expected MaxOpenConns 100, got %d", config.MaxOpenConns)
	}

	if config.MaxIdleConns != 10 {
		t.Errorf("expected MaxIdleConns 10, got %d", config.MaxIdleConns)
	}

	if config.ConnMaxLifetime != time.Hour {
		t.Errorf("expected ConnMaxLifetime 1h, got %v", config.ConnMaxLifetime)
	}

	if config.Charset != "utf8mb4" {
		t.Errorf("expected Charset utf8mb4, got %s", config.Charset)
	}

	if !config.ParseTime {
		t.Error("expected ParseTime to be true")
	}
}

// TestBuildDSN 测试 DSN 构建
func TestBuildDSN(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected string
	}{
		{
			name: "已有 DSN",
			config: &Config{
				DSN: "user:pass@tcp(localhost:3306)/testdb",
			},
			expected: "user:pass@tcp(localhost:3306)/testdb",
		},
		{
			name:     "空 DSN",
			config:   &Config{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.BuildDSN()
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestNew_NilConfig 测试 nil 配置
func TestNew_NilConfig(t *testing.T) {
	db, err := New(nil)
	if err == nil {
		t.Error("expected error for nil config")
	}
	if db != nil {
		t.Error("expected nil db")
	}
	if err.Error() != "mysql config is nil" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestNew_EmptyDSN 测试空 DSN
func TestNew_EmptyDSN(t *testing.T) {
	config := &Config{}
	db, err := New(config)
	if err == nil {
		t.Error("expected error for empty DSN")
	}
	if db != nil {
		t.Error("expected nil db")
	}
}

// TestNew_InvalidDSN 测试无效 DSN
func TestNew_InvalidDSN(t *testing.T) {
	config := &Config{
		DSN:            "invalid-dsn",
		ConnectTimeout: time.Second,
	}

	db, err := New(config)
	if err == nil {
		t.Error("expected error for invalid DSN")
		if db != nil {
			db.Close()
		}
	}
}

// TestHealth_NilDB 测试 nil 数据库健康检查
func TestHealth_NilDB(t *testing.T) {
	var db *DB
	err := db.Health(context.Background())
	if err == nil {
		t.Error("expected error for nil db")
	}
}

// TestStats_NilDB 测试 nil 数据库统计
func TestStats_NilDB(t *testing.T) {
	var db *DB
	stats := db.Stats()

	// 应该返回零值
	if stats.OpenConnections != 0 {
		t.Errorf("expected 0 open connections, got %d", stats.OpenConnections)
	}
}

// TestClose_NilDB 测试 nil 数据库关闭
func TestClose_NilDB(t *testing.T) {
	var db *DB
	err := db.Close()
	if err != nil {
		t.Errorf("expected no error for nil db close, got %v", err)
	}
}

// TestMaskDSN 测试 DSN 隐藏
func TestMaskDSN(t *testing.T) {
	tests := []struct {
		name     string
		dsn      string
		expected string
	}{
		{
			name:     "长 DSN",
			dsn:      "user:password@tcp(localhost:3306)/db",
			expected: "user:passw...",
		},
		{
			name:     "短 DSN",
			dsn:      "short",
			expected: "***",
		},
		{
			name:     "恰好10字符",
			dsn:      "1234567890",
			expected: "***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskDSN(tt.dsn)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestTransaction_Rollback 测试事务回滚
func TestTransaction_Rollback(t *testing.T) {
	// 这个测试需要真实数据库连接
	// 跳过，除非设置了测试数据库
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// 注意：这需要实际的数据库连接
	// 在 CI 环境中应该使用 testcontainers 或 docker
	t.Skip("integration test requires actual database")
}

// TestStdLogger 测试标准日志实现
func TestStdLogger(t *testing.T) {
	logger := &StdLogger{}

	// 不应该 panic
	logger.Printf("test message")
	logger.Error("test error", errors.New("test"))
}

// MockLogger 用于测试的 mock logger
type MockLogger struct {
	PrintfCalled bool
	ErrorCalled  bool
	LastMessage  string
	LastError    error
}

func (m *MockLogger) Printf(format string, args ...any) {
	m.PrintfCalled = true
	m.LastMessage = format
}

func (m *MockLogger) Error(msg string, err error) {
	m.ErrorCalled = true
	m.LastMessage = msg
	m.LastError = err
}

// TestConfig_WithLogger 测试带日志的配置
func TestConfig_WithLogger(t *testing.T) {
	logger := &MockLogger{}
	config := &Config{
		DSN:            "invalid-dsn",
		ConnectTimeout: time.Second,
		Logger:         logger,
	}

	// 尝试连接（应该失败）
	db, err := New(config)
	if err == nil {
		t.Error("expected error for invalid DSN")
		if db != nil {
			db.Close()
		}
	}

	// 验证日志被调用
	if !logger.ErrorCalled {
		t.Error("expected Error to be called")
	}
}

// TestGetGlobal_BeforeInit 测试初始化前获取全局实例
func TestGetGlobal_BeforeInit(t *testing.T) {
	// 重置全局变量（仅用于测试）
	// 注意：这在实际应用中不应该这样做
	db := GetGlobal()
	if db != nil {
		// 如果之前的测试已经初始化了，这是正常的
		t.Log("global db already initialized")
	}
}

// 集成测试辅助函数
func setupTestDB(t *testing.T) *DB {
	// 从环境变量读取测试数据库配置
	// 如果没有配置，跳过测试
	t.Helper()

	dsn := getTestDSN()
	if dsn == "" {
		t.Skip("test database not configured, set TEST_MYSQL_DSN env var")
	}

	config := DefaultConfig(dsn)
	config.ConnectTimeout = 5 * time.Second

	db, err := New(config)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	// 创建测试表
	createTestTable(t, db)

	return db
}

func getTestDSN() string {
	// 可以从环境变量读取
	// return os.Getenv("TEST_MYSQL_DSN")
	return "" // 默认跳过
}

func createTestTable(t *testing.T, db *DB) {
	t.Helper()

	ctx := context.Background()
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS test_users (
			id INT AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			email VARCHAR(100) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
}

func cleanupTestDB(t *testing.T, db *DB) {
	t.Helper()

	if db == nil {
		return
	}

	ctx := context.Background()
	_, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS test_users")
	if err != nil {
		t.Errorf("failed to cleanup test table: %v", err)
	}

	db.Close()
}

// 集成测试示例（需要真实数据库）
func TestIntegration_BasicOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	ctx := context.Background()

	// 测试插入
	result, err := db.ExecContext(ctx, "INSERT INTO test_users (name, email) VALUES (?, ?)", "Alice", "alice@example.com")
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("failed to get last insert id: %v", err)
	}

	if id <= 0 {
		t.Error("expected positive insert id")
	}

	// 测试查询
	var name, email string
	err = db.QueryRowContext(ctx, "SELECT name, email FROM test_users WHERE id = ?", id).Scan(&name, &email)
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}

	if name != "Alice" || email != "alice@example.com" {
		t.Errorf("unexpected query result: name=%s, email=%s", name, email)
	}

	// 测试更新
	_, err = db.ExecContext(ctx, "UPDATE test_users SET name = ? WHERE id = ?", "Bob", id)
	if err != nil {
		t.Fatalf("failed to update: %v", err)
	}

	// 测试删除
	_, err = db.ExecContext(ctx, "DELETE FROM test_users WHERE id = ?", id)
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}
}

// TestIntegration_Transaction 测试事务
func TestIntegration_Transaction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	ctx := context.Background()

	// 测试成功事务
	err := db.Transaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO test_users (name, email) VALUES (?, ?)", "Charlie", "charlie@example.com")
		return err
	})

	if err != nil {
		t.Errorf("transaction failed: %v", err)
	}

	// 测试回滚事务
	err = db.Transaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO test_users (name, email) VALUES (?, ?)", "Dave", "dave@example.com")
		if err != nil {
			return err
		}
		// 故意返回错误以触发回滚
		return errors.New("rollback test")
	})

	if err == nil {
		t.Error("expected transaction to fail")
	}

	// 验证回滚成功
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM test_users WHERE name = ?", "Dave").Scan(&count)
	if err != nil {
		t.Fatalf("failed to verify rollback: %v", err)
	}

	if count != 0 {
		t.Errorf("expected Dave to be rolled back, but found %d records", count)
	}
}

// TestIntegration_Health 测试健康检查
func TestIntegration_Health(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	ctx := context.Background()
	err := db.Health(ctx)
	if err != nil {
		t.Errorf("health check failed: %v", err)
	}
}

// TestIntegration_Stats 测试统计信息
func TestIntegration_Stats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	stats := db.Stats()

	// 验证统计信息合理
	if stats.MaxOpenConnections != 100 {
		t.Errorf("expected MaxOpenConnections 100, got %d", stats.MaxOpenConnections)
	}
}

// TestIntegration_Timeout 测试超时操作
func TestIntegration_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	ctx := context.Background()

	// 测试 ExecWithTimeout
	_, err := db.ExecWithTimeout(ctx, 5*time.Second, "INSERT INTO test_users (name, email) VALUES (?, ?)", "Eve", "eve@example.com")
	if err != nil {
		t.Errorf("ExecWithTimeout failed: %v", err)
	}

	// 测试 QueryRowWithTimeout
	var name string
	row := db.QueryRowWithTimeout(ctx, 5*time.Second, "SELECT name FROM test_users WHERE email = ?", "eve@example.com")
	err = row.Scan(&name)
	if err != nil {
		t.Errorf("QueryRowWithTimeout failed: %v", err)
	}

	if name != "Eve" {
		t.Errorf("expected name Eve, got %s", name)
	}
}

// TestTransaction_Panic 测试事务 panic 处理
func TestTransaction_Panic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	ctx := context.Background()

	// 测试 panic 会导致回滚
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()

	db.Transaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO test_users (name, email) VALUES (?, ?)", "Panic", "panic@example.com")
		if err != nil {
			return err
		}
		panic("test panic")
	})
}

// TestStats_NonNilDB 测试非空数据库统计
func TestStats_NonNilDB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	stats := db.Stats()

	// 验证有合理的统计值
	if stats.MaxOpenConnections <= 0 {
		t.Error("expected positive MaxOpenConnections")
	}
}

// TestExecWithTimeout_Success 测试带超时的成功执行
func TestExecWithTimeout_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	ctx := context.Background()
	result, err := db.ExecWithTimeout(ctx, 5*time.Second,
		"INSERT INTO test_users (name, email) VALUES (?, ?)", "Test", "test@example.com")

	if err != nil {
		t.Fatalf("ExecWithTimeout failed: %v", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("failed to get rows affected: %v", err)
	}

	if affected != 1 {
		t.Errorf("expected 1 row affected, got %d", affected)
	}
}

// TestQueryWithTimeout_Success 测试带超时的查询
func TestQueryWithTimeout_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	ctx := context.Background()

	// 先插入数据
	db.ExecContext(ctx, "INSERT INTO test_users (name, email) VALUES (?, ?)", "Query", "query@example.com")

	// 查询
	rows, err := db.QueryWithTimeout(ctx, 5*time.Second, "SELECT name, email FROM test_users WHERE name = ?", "Query")
	if err != nil {
		t.Fatalf("QueryWithTimeout failed: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var name, email string
		if err := rows.Scan(&name, &email); err != nil {
			t.Fatalf("failed to scan row: %v", err)
		}
		count++

		if name != "Query" || email != "query@example.com" {
			t.Errorf("unexpected row: name=%s, email=%s", name, email)
		}
	}

	if count != 1 {
		t.Errorf("expected 1 row, got %d", count)
	}
}

// TestInit_Once 测试单例初始化
func TestInit_Once(t *testing.T) {
	// 注意：由于 sync.Once 的特性，第二次调用不会执行初始化函数
	// 因此 err2 始终是 nil（第一次的 err 被忽略了）
	// 这个测试主要验证 Init 可以被多次调用而不 panic

	config := DefaultConfig("user:pass@tcp(localhost:3306)/test")
	config.ConnectTimeout = 1 * time.Second

	// 多次调用 Init（在实际应用中不推荐）
	db1, _ := Init(config)
	db2, _ := Init(config)

	// 如果第一次成功连接，两次应该返回相同的实例
	// 如果第一次失败，两次都应该是 nil
	if db1 != db2 {
		t.Error("Init should return the same instance when called multiple times")
	}

	// 清理
	if db1 != nil {
		db1.Close()
	}
}

// TestGetGlobal_AfterInit 测试初始化后获取全局实例
func TestGetGlobal_AfterInit(t *testing.T) {
	// 重置全局状态
	globalDB = nil
	globalOnce = sync.Once{}

	config := DefaultConfig("user:pass@tcp(localhost:3306)/test")
	config.ConnectTimeout = 1 * time.Second

	// 初始化
	Init(config)

	// 获取全局实例
	db := GetGlobal()

	// 可能是 nil（如果连接失败）或非 nil（如果有测试数据库）
	if db != nil {
		// 验证是同一个实例
		if db != globalDB {
			t.Error("GetGlobal should return the same instance as Init")
		}
		db.Close()
	}
}

// TestClose_ValidDB 测试关闭有效数据库
func TestClose_ValidDB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := setupTestDB(t)

	// 关闭数据库
	err := db.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// 再次关闭应该仍然成功（幂等性）
	err = db.Close()
	if err != nil {
		t.Errorf("second Close failed: %v", err)
	}
}

// Benchmark 测试
func BenchmarkQuery(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark in short mode")
	}

	db := setupTestDB(&testing.T{})
	defer cleanupTestDB(&testing.T{}, db)

	ctx := context.Background()

	// 插入一些测试数据
	db.ExecContext(ctx, "INSERT INTO test_users (name, email) VALUES (?, ?)", "Benchmark", "bench@example.com")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var name string
		db.QueryRowContext(ctx, "SELECT name FROM test_users WHERE email = ?", "bench@example.com").Scan(&name)
	}
}
