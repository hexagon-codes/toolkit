package clickhouse

import (
	"errors"
	"fmt"
	"time"
)

// Config errors.
var (
	ErrEmptyAddrs    = errors.New("clickhouse: at least one address is required")
	ErrEmptyDatabase = errors.New("clickhouse: database name is required")
)

// Config holds ClickHouse connection configuration.
type Config struct {
	// Addrs is the list of ClickHouse server addresses (required).
	// Format: host:port (default port is 9000 for native protocol)
	Addrs []string `json:"addrs" yaml:"addrs" mapstructure:"addrs"`

	// Database is the default database name (required).
	Database string `json:"database" yaml:"database" mapstructure:"database"`

	// Auth
	Username string `json:"username" yaml:"username" mapstructure:"username"`
	Password string `json:"password" yaml:"password" mapstructure:"password"`

	// Connection Pool
	MaxOpenConns    int           `json:"max_open_conns" yaml:"max_open_conns" mapstructure:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns" yaml:"max_idle_conns" mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime" yaml:"conn_max_lifetime" mapstructure:"conn_max_lifetime"`

	// Timeouts
	DialTimeout time.Duration `json:"dial_timeout" yaml:"dial_timeout" mapstructure:"dial_timeout"`
	ReadTimeout time.Duration `json:"read_timeout" yaml:"read_timeout" mapstructure:"read_timeout"`

	// Performance
	BlockBufferSize uint8 `json:"block_buffer_size" yaml:"block_buffer_size" mapstructure:"block_buffer_size"`

	// Compression: none, lz4, zstd
	Compression string `json:"compression" yaml:"compression" mapstructure:"compression"`

	// TLS
	TLS                bool `json:"tls" yaml:"tls" mapstructure:"tls"`
	InsecureSkipVerify bool `json:"insecure_skip_verify" yaml:"insecure_skip_verify" mapstructure:"insecure_skip_verify"`

	// Debug enables debug logging.
	Debug bool `json:"debug" yaml:"debug" mapstructure:"debug"`

	// Settings is a map of ClickHouse settings.
	Settings map[string]any `json:"settings" yaml:"settings" mapstructure:"settings"`
}

// DefaultConfig returns sensible default configuration.
func DefaultConfig() *Config {
	return &Config{
		Addrs:           []string{"localhost:9000"},
		Database:        "default",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
		DialTimeout:     10 * time.Second,
		ReadTimeout:     30 * time.Second,
		BlockBufferSize: 10,
		Compression:     "lz4",
		Settings: map[string]any{
			"max_execution_time": 60,
		},
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if len(c.Addrs) == 0 {
		return ErrEmptyAddrs
	}
	if c.Database == "" {
		return ErrEmptyDatabase
	}
	return nil
}

// String returns a safe string representation.
func (c *Config) String() string {
	return fmt.Sprintf("ClickHouse{Addrs: %v, Database: %s, MaxOpen: %d, MaxIdle: %d}",
		c.Addrs, c.Database, c.MaxOpenConns, c.MaxIdleConns)
}

// Option is a functional option for Config.
type Option func(*Config)

// WithAddrs sets the server addresses.
func WithAddrs(addrs ...string) Option {
	return func(c *Config) { c.Addrs = addrs }
}

// WithDatabase sets the default database.
func WithDatabase(db string) Option {
	return func(c *Config) { c.Database = db }
}

// WithAuth sets authentication credentials.
func WithAuth(username, password string) Option {
	return func(c *Config) {
		c.Username = username
		c.Password = password
	}
}

// WithMaxOpenConns sets the maximum open connections.
func WithMaxOpenConns(n int) Option {
	return func(c *Config) { c.MaxOpenConns = n }
}

// WithMaxIdleConns sets the maximum idle connections.
func WithMaxIdleConns(n int) Option {
	return func(c *Config) { c.MaxIdleConns = n }
}

// WithDialTimeout sets the dial timeout.
func WithDialTimeout(d time.Duration) Option {
	return func(c *Config) { c.DialTimeout = d }
}

// WithCompression sets the compression method.
func WithCompression(method string) Option {
	return func(c *Config) { c.Compression = method }
}

// WithTLS enables TLS.
func WithTLS(insecureSkipVerify bool) Option {
	return func(c *Config) {
		c.TLS = true
		c.InsecureSkipVerify = insecureSkipVerify
	}
}

// WithDebug enables debug mode.
func WithDebug(debug bool) Option {
	return func(c *Config) { c.Debug = debug }
}

// WithSettings sets ClickHouse settings.
func WithSettings(settings map[string]any) Option {
	return func(c *Config) { c.Settings = settings }
}

// Apply applies options to the config.
func (c *Config) Apply(opts ...Option) *Config {
	for _, opt := range opts {
		opt(c)
	}
	return c
}
