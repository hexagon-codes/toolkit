// Package mysql 提供 MySQL 数据库连接池
//
// 支持连接池管理、健康检查和 Prometheus 监控指标。
//
// 基本用法:
//
//	db, err := mysql.New(ctx, &mysql.Config{
//	    Host:     "localhost",
//	    Port:     3306,
//	    User:     "root",
//	    Password: "password",
//	    Database: "mydb",
//	})
//	defer db.Close()
//
// 带选项:
//
//	db, err := mysql.New(ctx, cfg,
//	    mysql.WithMaxOpenConns(100),
//	    mysql.WithMaxIdleConns(10),
//	    mysql.WithConnMaxLifetime(time.Hour),
//	)
//
// 健康检查:
//
//	if err := db.Ping(ctx); err != nil {
//	    // 处理连接错误
//	}
//
// --- English ---
//
// Package mysql provides MySQL database connection pool.
//
// Features connection pooling, health checks, and Prometheus metrics.
//
// Basic usage:
//
//	db, err := mysql.New(ctx, &mysql.Config{
//	    Host:     "localhost",
//	    Port:     3306,
//	    User:     "root",
//	    Password: "password",
//	    Database: "mydb",
//	})
//	defer db.Close()
//
// With options:
//
//	db, err := mysql.New(ctx, cfg,
//	    mysql.WithMaxOpenConns(100),
//	    mysql.WithMaxIdleConns(10),
//	    mysql.WithConnMaxLifetime(time.Hour),
//	)
//
// Health check:
//
//	if err := db.Ping(ctx); err != nil {
//	    // handle connection error
//	}
package mysql
