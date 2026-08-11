package mongodb

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// Package errors.
var (
	ErrNotInitialized = errors.New("mongodb: client not initialized, call Init first")
	ErrAlreadyClosed  = errors.New("mongodb: client already closed")
)

// Client wraps the MongoDB client with additional functionality.
type Client struct {
	client    *mongo.Client
	database  *mongo.Database
	config    *Config
	closed    atomic.Bool
	closeOnce sync.Once
	closeErr  error
}

// Global singleton.
var (
	instance    *Client
	initialized atomic.Bool // 使用 atomic.Bool 替代 sync.Once，支持安全重置
	initErr     error
	mu          sync.RWMutex
)

// Init initializes the global MongoDB client singleton.
// It is safe to call multiple times; only the first call takes effect.
// Returns any error from the initial connection attempt.
func Init(ctx context.Context, cfg *Config, opts ...Option) error {
	// 快速路径：已初始化则直接返回
	if initialized.Load() {
		mu.RLock()
		err := initErr
		mu.RUnlock()
		return err
	}

	mu.Lock()
	defer mu.Unlock()

	// double check：获取锁后再次检查
	if initialized.Load() {
		return initErr
	}

	instance, initErr = New(ctx, cfg, opts...)
	if initErr == nil {
		initialized.Store(true)
	}
	return initErr
}

// New creates a new MongoDB client (non-singleton).
// Use this when you need multiple clients or dependency injection.
func New(ctx context.Context, cfg *Config, opts ...Option) (*Client, error) {
	if ctx == nil {
		return nil, errors.New("mongodb: context must not be nil")
	}
	cfg = cloneConfig(cfg)

	// Apply options
	cfg.Apply(opts...)
	cfg = cloneConfig(cfg)

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	clientOpts := buildClientOptions(cfg)
	if err := clientOpts.Validate(); err != nil {
		return nil, fmt.Errorf("mongodb: invalid client options: %w", err)
	}

	connectCtx, cancel := contextWithOptionalTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	client, err := mongo.Connect(connectCtx, clientOpts)
	if err != nil {
		return nil, err
	}

	// 校验连接
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()

	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		return nil, errors.Join(err, client.Disconnect(cleanupCtx))
	}

	return &Client{
		client:   client,
		database: client.Database(cfg.Database),
		config:   cfg,
	}, nil
}

// buildClientOptions 将领域配置转换为 MongoDB 驱动配置，不执行网络操作。
func buildClientOptions(cfg *Config) *options.ClientOptions {
	clientOpts := options.Client().ApplyURI(cfg.URI)

	// Connection pool
	if cfg.MaxPoolSize > 0 {
		clientOpts.SetMaxPoolSize(cfg.MaxPoolSize)
	}
	if cfg.MinPoolSize > 0 {
		clientOpts.SetMinPoolSize(cfg.MinPoolSize)
	}
	if cfg.MaxConnIdleTime > 0 {
		clientOpts.SetMaxConnIdleTime(cfg.MaxConnIdleTime)
	}

	// Timeouts
	if cfg.ConnectTimeout > 0 {
		clientOpts.SetConnectTimeout(cfg.ConnectTimeout)
	}
	if cfg.SocketTimeout > 0 {
		clientOpts.SetSocketTimeout(cfg.SocketTimeout)
	}
	if cfg.ServerSelectionTimeout > 0 {
		clientOpts.SetServerSelectionTimeout(cfg.ServerSelectionTimeout)
	}
	if cfg.HeartbeatInterval > 0 {
		clientOpts.SetHeartbeatInterval(cfg.HeartbeatInterval)
	}

	// Auth (if not in URI)
	if cfg.Username != "" && cfg.Password != "" {
		cred := options.Credential{
			Username:   cfg.Username,
			Password:   cfg.Password,
			AuthSource: cfg.AuthSource,
		}
		clientOpts.SetAuth(cred)
	}

	// Replica set
	if cfg.ReplicaSet != "" {
		clientOpts.SetReplicaSet(cfg.ReplicaSet)
	}
	if cfg.Direct {
		clientOpts.SetDirect(true)
	}

	// Read preference
	if rp := parseReadPref(cfg.ReadPreference); rp != nil {
		clientOpts.SetReadPreference(rp)
	}

	// App name
	if cfg.AppName != "" {
		clientOpts.SetAppName(cfg.AppName)
	}

	// Compressors
	if len(cfg.Compressors) > 0 {
		clientOpts.SetCompressors(cfg.Compressors)
	}

	return clientOpts
}

// contextWithOptionalTimeout 仅在正超时时长下添加截止时间。
func contextWithOptionalTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func parseReadPref(pref string) *readpref.ReadPref {
	switch pref {
	case "primary":
		return readpref.Primary()
	case "primaryPreferred":
		return readpref.PrimaryPreferred()
	case "secondary":
		return readpref.Secondary()
	case "secondaryPreferred":
		return readpref.SecondaryPreferred()
	case "nearest":
		return readpref.Nearest()
	default:
		return nil
	}
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

// Database returns the default database.
// Returns nil if client is not initialized.
func Database() *mongo.Database {
	c := GetClient()
	if c == nil {
		return nil
	}
	return c.database
}

// Collection returns a collection from the default database.
// Returns nil if client is not initialized.
func Collection(name string) *mongo.Collection {
	db := Database()
	if db == nil {
		return nil
	}
	return db.Collection(name)
}

// DB returns a database by name from the global client.
// Returns nil if client is not initialized.
func DB(name string) *mongo.Database {
	c := GetClient()
	if c == nil {
		return nil
	}
	return c.client.Database(name)
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

// Reset resets the singleton, allowing re-initialization.
// This is primarily useful for testing.
// 此函数是线程安全的，使用 atomic.Bool 确保与 Init() 不会竞态
func Reset() error {
	mu.Lock()
	defer mu.Unlock()

	var closeErr error
	if instance != nil {
		closeErr = instance.Close()
		instance = nil
	}
	initialized.Store(false) // 原子操作，安全重置初始化状态
	initErr = nil
	return closeErr
}

// --- Client methods ---

// Ping performs a health check.
func (c *Client) Ping(ctx context.Context) error {
	if c.closed.Load() {
		return ErrAlreadyClosed
	}
	return c.client.Ping(ctx, readpref.Primary())
}

// Close closes the MongoDB connection.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		if c.client == nil {
			return
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c.closeErr = c.client.Disconnect(closeCtx)
	})
	return c.closeErr
}

// Name returns the client name for the db.Client interface.
func (c *Client) Name() string {
	return "mongodb"
}

// RawClient returns the underlying mongo.Client.
func (c *Client) RawClient() *mongo.Client {
	return c.client
}

// Database returns the default database.
func (c *Client) Database() *mongo.Database {
	return c.database
}

// DB returns a database by name.
func (c *Client) DB(name string) *mongo.Database {
	return c.client.Database(name)
}

// Coll returns a collection from the default database.
func (c *Client) Coll(name string) *mongo.Collection {
	return c.database.Collection(name)
}

// Config returns a copy of the client configuration.
func (c *Client) Config() Config {
	return *cloneConfig(c.config)
}

// IsClosed returns true if the client has been closed.
func (c *Client) IsClosed() bool {
	return c.closed.Load()
}
