package clickhouse

import (
	"context"
	"crypto/tls"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Package errors.
var (
	ErrNotInitialized = errors.New("clickhouse: client not initialized, call Init first")
	ErrAlreadyClosed  = errors.New("clickhouse: client already closed")
)

// Client wraps the ClickHouse connection with additional functionality.
type Client struct {
	conn      driver.Conn
	config    *Config
	closed    atomic.Bool
	closeOnce sync.Once
	closeErr  error
}

// Global singleton.
var (
	instance    *Client
	initialized atomic.Bool
	initErr     error
	mu          sync.RWMutex
)

// Init 初始化全局 ClickHouse 客户端单例。
// 可安全多次调用，仅首次调用生效。使用 atomic.Bool + sync.Mutex 双重检查，线程安全。
func Init(ctx context.Context, cfg *Config, opts ...Option) error {
	if initialized.Load() {
		mu.RLock()
		err := initErr
		mu.RUnlock()
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	if initialized.Load() {
		return initErr
	}
	instance, initErr = New(ctx, cfg, opts...)
	if initErr == nil {
		initialized.Store(true)
	}
	return initErr
}

// New creates a new ClickHouse client (non-singleton).
// Use this when you need multiple clients or dependency injection.
func New(ctx context.Context, cfg *Config, opts ...Option) (*Client, error) {
	if ctx == nil {
		return nil, errors.New("clickhouse: context must not be nil")
	}
	cfg = cloneConfig(cfg)

	// Apply options
	cfg.Apply(opts...)
	cfg = cloneConfig(cfg)

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	chOpts := buildClickHouseOptions(cfg)

	// 建立连接
	conn, err := clickhouse.Open(chOpts)
	if err != nil {
		return nil, err
	}

	// 校验连接
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := conn.Ping(pingCtx); err != nil {
		return nil, errors.Join(err, conn.Close())
	}

	return &Client{
		conn:   conn,
		config: cfg,
	}, nil
}

// buildClickHouseOptions 将领域配置转换为驱动配置，不执行网络操作。
func buildClickHouseOptions(cfg *Config) *clickhouse.Options {
	chOpts := &clickhouse.Options{
		Addr: cfg.Addrs,
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		DialTimeout:     cfg.DialTimeout,
		ReadTimeout:     cfg.ReadTimeout,
		MaxOpenConns:    cfg.MaxOpenConns,
		MaxIdleConns:    cfg.MaxIdleConns,
		ConnMaxLifetime: cfg.ConnMaxLifetime,
		Debug:           cfg.Debug,
		BlockBufferSize: cfg.BlockBufferSize,
	}

	// Settings
	if len(cfg.Settings) > 0 {
		chOpts.Settings = make(clickhouse.Settings)
		for k, v := range cfg.Settings {
			chOpts.Settings[k] = v
		}
	}

	// Compression
	switch cfg.Compression {
	case "lz4":
		chOpts.Compression = &clickhouse.Compression{Method: clickhouse.CompressionLZ4}
	case "zstd":
		chOpts.Compression = &clickhouse.Compression{Method: clickhouse.CompressionZSTD}
	case "none", "":
		chOpts.Compression = &clickhouse.Compression{Method: clickhouse.CompressionNone}
	}

	// TLS
	if cfg.TLS {
		chOpts.TLS = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	return chOpts
}

// GetClient returns the global singleton client.
// Returns nil if Init has not been called.
func GetClient() *Client {
	mu.RLock()
	defer mu.RUnlock()
	return instance
}

// MustGetClient returns the global client or panics if not initialized.
func MustGetClient() *Client {
	c := GetClient()
	if c == nil {
		panic(ErrNotInitialized)
	}
	return c
}

// Conn returns the raw ClickHouse connection from the global client.
// Returns nil if client is not initialized.
func Conn() driver.Conn {
	c := GetClient()
	if c == nil {
		return nil
	}
	return c.conn
}

// Close closes the global client.
func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		return nil
	}
	err := instance.Close()
	instance = nil
	initialized.Store(false)
	initErr = nil
	return err
}

// Reset 重置单例，允许重新初始化。
// 主要用于测试场景。
func Reset() error {
	mu.Lock()
	defer mu.Unlock()

	var closeErr error
	if instance != nil {
		closeErr = instance.Close()
		instance = nil
	}
	initialized.Store(false)
	initErr = nil
	return closeErr
}

// --- Client methods ---

// Ping performs a health check.
func (c *Client) Ping(ctx context.Context) error {
	if c.closed.Load() {
		return ErrAlreadyClosed
	}
	return c.conn.Ping(ctx)
}

// Close closes the ClickHouse connection.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		if c.conn != nil {
			c.closeErr = c.conn.Close()
		}
	})
	return c.closeErr
}

// Name returns the client name for the db.Client interface.
func (c *Client) Name() string {
	return "clickhouse"
}

// RawConn returns the underlying driver.Conn.
func (c *Client) RawConn() driver.Conn {
	return c.conn
}

// Config returns a copy of the client configuration.
func (c *Client) Config() Config {
	return *cloneConfig(c.config)
}

// IsClosed returns true if the client has been closed.
func (c *Client) IsClosed() bool {
	return c.closed.Load()
}

// Exec executes a query without returning results.
func (c *Client) Exec(ctx context.Context, query string, args ...any) error {
	if c.closed.Load() {
		return ErrAlreadyClosed
	}
	return c.conn.Exec(ctx, query, args...)
}

// Query executes a query and returns rows.
func (c *Client) Query(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	if c.closed.Load() {
		return nil, ErrAlreadyClosed
	}
	return c.conn.Query(ctx, query, args...)
}

// QueryRow executes a query and returns a single row.
func (c *Client) QueryRow(ctx context.Context, query string, args ...any) driver.Row {
	return c.conn.QueryRow(ctx, query, args...)
}

// PrepareBatch prepares a batch for insertion.
func (c *Client) PrepareBatch(ctx context.Context, query string) (driver.Batch, error) {
	if c.closed.Load() {
		return nil, ErrAlreadyClosed
	}
	return c.conn.PrepareBatch(ctx, query)
}

// Stats returns connection statistics.
func (c *Client) Stats() driver.Stats {
	return c.conn.Stats()
}
