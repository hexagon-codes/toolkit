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
