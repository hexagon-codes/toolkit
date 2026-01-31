package mongodb

import (
	"context"
	"errors"
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
	client   *mongo.Client
	database *mongo.Database
	config   *Config
	closed   atomic.Bool
}

// Global singleton.
var (
	instance *Client
	once     sync.Once
	initErr  error
	mu       sync.RWMutex
)

// Init initializes the global MongoDB client singleton.
// It is safe to call multiple times; only the first call takes effect.
// Returns any error from the initial connection attempt.
func Init(ctx context.Context, cfg *Config, opts ...Option) error {
	once.Do(func() {
		instance, initErr = New(ctx, cfg, opts...)
	})
	return initErr
}

// New creates a new MongoDB client (non-singleton).
// Use this when you need multiple clients or dependency injection.
func New(ctx context.Context, cfg *Config, opts ...Option) (*Client, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Apply options
	cfg.Apply(opts...)

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Build client options
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

	// Connect with timeout
	connectCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	client, err := mongo.Connect(connectCtx, clientOpts)
	if err != nil {
		return nil, err
	}

	// Verify connection
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()

	if err := client.Ping(pingCtx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}

	return &Client{
		client:   client,
		database: client.Database(cfg.Database),
		config:   cfg,
	}, nil
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
	return err
}

// Reset resets the singleton, allowing re-initialization.
// This is primarily useful for testing.
func Reset() {
	mu.Lock()
	defer mu.Unlock()

	if instance != nil {
		_ = instance.Close()
		instance = nil
	}
	once = sync.Once{}
	initErr = nil
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
	if c.closed.Swap(true) {
		return ErrAlreadyClosed
	}
	return c.client.Disconnect(context.Background())
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
	return *c.config
}

// IsClosed returns true if the client has been closed.
func (c *Client) IsClosed() bool {
	return c.closed.Load()
}
