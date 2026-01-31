package elasticsearch

import (
	"errors"
	"fmt"
	"time"
)

// Config errors.
var (
	ErrEmptyAddrs = errors.New("elasticsearch: at least one address or CloudID is required")
)

// Config holds Elasticsearch connection configuration.
type Config struct {
	// Addresses is the list of Elasticsearch node addresses.
	// Format: http://host:port or https://host:port
	Addresses []string `json:"addresses" yaml:"addresses" mapstructure:"addresses"`

	// Auth - Basic
	Username string `json:"username" yaml:"username" mapstructure:"username"`
	Password string `json:"password" yaml:"password" mapstructure:"password"`

	// Auth - API Key
	APIKey string `json:"api_key" yaml:"api_key" mapstructure:"api_key"`

	// Auth - Service Token
	ServiceToken string `json:"service_token" yaml:"service_token" mapstructure:"service_token"`

	// Elastic Cloud
	CloudID string `json:"cloud_id" yaml:"cloud_id" mapstructure:"cloud_id"`

	// Retry
	MaxRetries    int   `json:"max_retries" yaml:"max_retries" mapstructure:"max_retries"`
	RetryOnStatus []int `json:"retry_on_status" yaml:"retry_on_status" mapstructure:"retry_on_status"`
	DisableRetry  bool  `json:"disable_retry" yaml:"disable_retry" mapstructure:"disable_retry"`

	// Timeout
	RequestTimeout time.Duration `json:"request_timeout" yaml:"request_timeout" mapstructure:"request_timeout"`

	// TLS
	CACert             string `json:"ca_cert" yaml:"ca_cert" mapstructure:"ca_cert"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" yaml:"insecure_skip_verify" mapstructure:"insecure_skip_verify"`

	// Pool
	MaxIdleConnsPerHost int `json:"max_idle_conns_per_host" yaml:"max_idle_conns_per_host" mapstructure:"max_idle_conns_per_host"`

	// Features
	EnableDebugLogger     bool          `json:"enable_debug_logger" yaml:"enable_debug_logger" mapstructure:"enable_debug_logger"`
	CompressRequestBody   bool          `json:"compress_request_body" yaml:"compress_request_body" mapstructure:"compress_request_body"`
	DiscoverNodesOnStart  bool          `json:"discover_nodes_on_start" yaml:"discover_nodes_on_start" mapstructure:"discover_nodes_on_start"`
	DiscoverNodesInterval time.Duration `json:"discover_nodes_interval" yaml:"discover_nodes_interval" mapstructure:"discover_nodes_interval"`
}

// DefaultConfig returns sensible default configuration.
func DefaultConfig() *Config {
	return &Config{
		Addresses:           []string{"http://localhost:9200"},
		MaxRetries:          3,
		RetryOnStatus:       []int{502, 503, 504},
		RequestTimeout:      30 * time.Second,
		MaxIdleConnsPerHost: 10,
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if len(c.Addresses) == 0 && c.CloudID == "" {
		return ErrEmptyAddrs
	}
	return nil
}

// String returns a safe string representation.
func (c *Config) String() string {
	if c.CloudID != "" {
		return fmt.Sprintf("Elasticsearch{CloudID: %s, MaxRetries: %d}", maskString(c.CloudID, 10), c.MaxRetries)
	}
	return fmt.Sprintf("Elasticsearch{Addresses: %v, MaxRetries: %d}", c.Addresses, c.MaxRetries)
}

func maskString(s string, visible int) string {
	if len(s) <= visible {
		return "***"
	}
	return s[:visible] + "***"
}

// Option is a functional option for Config.
type Option func(*Config)

// WithAddresses sets the server addresses.
func WithAddresses(addrs ...string) Option {
	return func(c *Config) { c.Addresses = addrs }
}

// WithBasicAuth sets basic authentication.
func WithBasicAuth(username, password string) Option {
	return func(c *Config) {
		c.Username = username
		c.Password = password
	}
}

// WithAPIKey sets API key authentication.
func WithAPIKey(apiKey string) Option {
	return func(c *Config) { c.APIKey = apiKey }
}

// WithCloudID sets Elastic Cloud ID.
func WithCloudID(cloudID string) Option {
	return func(c *Config) { c.CloudID = cloudID }
}

// WithMaxRetries sets the maximum retries.
func WithMaxRetries(n int) Option {
	return func(c *Config) { c.MaxRetries = n }
}

// WithTimeout sets the request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Config) { c.RequestTimeout = d }
}

// WithCACert sets the CA certificate path.
func WithCACert(path string) Option {
	return func(c *Config) { c.CACert = path }
}

// WithInsecureSkipVerify skips TLS verification.
func WithInsecureSkipVerify(skip bool) Option {
	return func(c *Config) { c.InsecureSkipVerify = skip }
}

// WithDebugLogger enables debug logging.
func WithDebugLogger(enable bool) Option {
	return func(c *Config) { c.EnableDebugLogger = enable }
}

// WithCompression enables request body compression.
func WithCompression(enable bool) Option {
	return func(c *Config) { c.CompressRequestBody = enable }
}

// Apply applies options to the config.
func (c *Config) Apply(opts ...Option) *Config {
	for _, opt := range opts {
		opt(c)
	}
	return c
}
