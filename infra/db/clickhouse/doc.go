// Package clickhouse 提供 ClickHouse 客户端单例管理
//
// 封装官方 ClickHouse Go 驱动，提供单例模式、连接池、健康检查和优雅关闭功能。
//
// 基本用法:
//
//	// 应用启动时初始化
//	err := clickhouse.Init(ctx, &clickhouse.Config{
//	    Addrs:    []string{"localhost:9000"},
//	    Database: "default",
//	    Username: "default",
//	    Password: "",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer clickhouse.Close()
//
//	// 执行查询
//	conn := clickhouse.Conn()
//	rows, _ := conn.Query(ctx, "SELECT * FROM users")
//
//	// 或直接使用客户端
//	client := clickhouse.GetClient()
//	client.Exec(ctx, "INSERT INTO logs VALUES (?)", data)
//
// 批量插入:
//
//	batch, _ := clickhouse.GetClient().PrepareBatch(ctx, "INSERT INTO logs")
//	batch.Append(...)
//	batch.Send()
//
// 健康检查:
//
//	if err := clickhouse.GetClient().Ping(ctx); err != nil {
//	    // 处理不健康状态
//	}
//
// --- English ---
//
// Package clickhouse provides ClickHouse client singleton management.
//
// It wraps the official ClickHouse Go driver with singleton pattern,
// connection pooling, health checks, and graceful shutdown.
//
// Basic usage:
//
//	// Initialize at application startup
//	err := clickhouse.Init(ctx, &clickhouse.Config{
//	    Addrs:    []string{"localhost:9000"},
//	    Database: "default",
//	    Username: "default",
//	    Password: "",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer clickhouse.Close()
//
//	// Execute queries
//	conn := clickhouse.Conn()
//	rows, _ := conn.Query(ctx, "SELECT * FROM users")
//
//	// Or use the client directly
//	client := clickhouse.GetClient()
//	client.Exec(ctx, "INSERT INTO logs VALUES (?)", data)
//
// Batch insert:
//
//	batch, _ := clickhouse.GetClient().PrepareBatch(ctx, "INSERT INTO logs")
//	batch.Append(...)
//	batch.Send()
//
// Health check:
//
//	if err := clickhouse.GetClient().Ping(ctx); err != nil {
//	    // handle unhealthy
//	}
package clickhouse
