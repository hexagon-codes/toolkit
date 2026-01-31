package mongodb

import (
	"errors"
	"fmt"
	"time"
)

// Config errors.
var (
	ErrEmptyURI      = errors.New("mongodb: URI is required")
	ErrEmptyDatabase = errors.New("mongodb: database name is required")
)

// Config holds MongoDB connection configuration.
type Config struct {
	// URI is the MongoDB connection string (required).
	// Format: mongodb://[username:password@]host1[:port1][,...hostN[:portN]][/[database][?options]]
	URI string `json:"uri" yaml:"uri" mapstructure:"uri"`

	// Database is the default database name (required).
	Database string `json:"database" yaml:"database" mapstructure:"database"`

	// Connection Pool
	MaxPoolSize     uint64        `json:"max_pool_size" yaml:"max_pool_size" mapstructure:"max_pool_size"`
	MinPoolSize     uint64        `json:"min_pool_size" yaml:"min_pool_size" mapstructure:"min_pool_size"`
	MaxConnIdleTime time.Duration `json:"max_conn_idle_time" yaml:"max_conn_idle_time" mapstructure:"max_conn_idle_time"`

	// Timeouts
	ConnectTimeout         time.Duration `json:"connect_timeout" yaml:"connect_timeout" mapstructure:"connect_timeout"`
	SocketTimeout          time.Duration `json:"socket_timeout" yaml:"socket_timeout" mapstructure:"socket_timeout"`
	ServerSelectionTimeout time.Duration `json:"server_selection_timeout" yaml:"server_selection_timeout" mapstructure:"server_selection_timeout"`
	HeartbeatInterval      time.Duration `json:"heartbeat_interval" yaml:"heartbeat_interval" mapstructure:"heartbeat_interval"`

	// Auth (alternative to URI)
	Username   string `json:"username" yaml:"username" mapstructure:"username"`
	Password   string `json:"password" yaml:"password" mapstructure:"password"`
	AuthSource string `json:"auth_source" yaml:"auth_source" mapstructure:"auth_source"`

	// Replica Set
	ReplicaSet     string `json:"replica_set" yaml:"replica_set" mapstructure:"replica_set"`
	ReadPreference string `json:"read_preference" yaml:"read_preference" mapstructure:"read_preference"`
	Direct         bool   `json:"direct" yaml:"direct" mapstructure:"direct"`

	// Other
	AppName     string   `json:"app_name" yaml:"app_name" mapstructure:"app_name"`
	Compressors []string `json:"compressors" yaml:"compressors" mapstructure:"compressors"`
}

// DefaultConfig returns sensible default configuration.
func DefaultConfig() *Config {
	return &Config{
		URI:                    "mongodb://localhost:27017",
		Database:               "test",
		MaxPoolSize:            100,
		MinPoolSize:            10,
		MaxConnIdleTime:        30 * time.Minute,
		ConnectTimeout:         10 * time.Second,
		SocketTimeout:          30 * time.Second,
		ServerSelectionTimeout: 30 * time.Second,
		HeartbeatInterval:      10 * time.Second,
		AuthSource:             "admin",
		ReadPreference:         "primary",
	}
}

// Validate validates the configuration and returns an error if invalid.
func (c *Config) Validate() error {
	if c.URI == "" {
		return ErrEmptyURI
	}
	if c.Database == "" {
		return ErrEmptyDatabase
	}
	return nil
}

// String returns a safe string representation (hides password).
func (c *Config) String() string {
	return fmt.Sprintf("MongoDB{URI: %s, Database: %s, MaxPool: %d, MinPool: %d}",
		maskURI(c.URI), c.Database, c.MaxPoolSize, c.MinPoolSize)
}

// maskURI masks sensitive parts of the URI.
func maskURI(uri string) string {
	if len(uri) <= 20 {
		return "***"
	}
	return uri[:20] + "***"
}

// Option is a functional option for Config.
type Option func(*Config)

// WithURI sets the MongoDB URI.
func WithURI(uri string) Option {
	return func(c *Config) { c.URI = uri }
}

// WithDatabase sets the default database.
func WithDatabase(db string) Option {
	return func(c *Config) { c.Database = db }
}

// WithMaxPoolSize sets the maximum pool size.
func WithMaxPoolSize(size uint64) Option {
	return func(c *Config) { c.MaxPoolSize = size }
}

// WithMinPoolSize sets the minimum pool size.
func WithMinPoolSize(size uint64) Option {
	return func(c *Config) { c.MinPoolSize = size }
}

// WithConnectTimeout sets the connection timeout.
func WithConnectTimeout(d time.Duration) Option {
	return func(c *Config) { c.ConnectTimeout = d }
}

// WithAuth sets authentication credentials.
func WithAuth(username, password, authSource string) Option {
	return func(c *Config) {
		c.Username = username
		c.Password = password
		if authSource != "" {
			c.AuthSource = authSource
		}
	}
}

// WithAppName sets the application name.
func WithAppName(name string) Option {
	return func(c *Config) { c.AppName = name }
}

// WithReadPreference sets the read preference.
func WithReadPreference(pref string) Option {
	return func(c *Config) { c.ReadPreference = pref }
}

// Apply applies options to the config.
func (c *Config) Apply(opts ...Option) *Config {
	for _, opt := range opts {
		opt(c)
	}
	return c
}
