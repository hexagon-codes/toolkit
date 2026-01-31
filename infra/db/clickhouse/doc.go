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
