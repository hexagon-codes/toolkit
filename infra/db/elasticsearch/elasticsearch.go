package elasticsearch

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// Package errors.
var (
	ErrNotInitialized = errors.New("elasticsearch: client not initialized, call Init first")
	ErrAlreadyClosed  = errors.New("elasticsearch: client already closed")
	ErrPingFailed     = errors.New("elasticsearch: ping failed")
)

// Client wraps the Elasticsearch client with additional functionality.
type Client struct {
	client *elasticsearch.Client
	config *Config
	closed atomic.Bool
}

// Global singleton.
var (
	instance *Client
	once     sync.Once
	initErr  error
	mu       sync.RWMutex
)

// Init initializes the global Elasticsearch client singleton.
// It is safe to call multiple times; only the first call takes effect.
func Init(cfg *Config, opts ...Option) error {
	once.Do(func() {
		instance, initErr = New(cfg, opts...)
	})
	return initErr
}

// New creates a new Elasticsearch client (non-singleton).
// Use this when you need multiple clients or dependency injection.
func New(cfg *Config, opts ...Option) (*Client, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Apply options
	cfg.Apply(opts...)

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Build ES config
	esCfg := elasticsearch.Config{
		Addresses:             cfg.Addresses,
		Username:              cfg.Username,
		Password:              cfg.Password,
		CloudID:               cfg.CloudID,
		APIKey:                cfg.APIKey,
		ServiceToken:          cfg.ServiceToken,
		MaxRetries:            cfg.MaxRetries,
		RetryOnStatus:         cfg.RetryOnStatus,
		DisableRetry:          cfg.DisableRetry,
		CompressRequestBody:   cfg.CompressRequestBody,
		DiscoverNodesOnStart:  cfg.DiscoverNodesOnStart,
		DiscoverNodesInterval: cfg.DiscoverNodesInterval,
	}

	// Custom transport
	transport := &http.Transport{
		MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: cfg.RequestTimeout,
		IdleConnTimeout:       90 * time.Second,
	}

	// TLS
	if cfg.InsecureSkipVerify || cfg.CACert != "" {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: cfg.InsecureSkipVerify,
		}
		// 加载 CA 证书
		if cfg.CACert != "" {
			caCert, err := os.ReadFile(cfg.CACert)
			if err != nil {
				return nil, fmt.Errorf("elasticsearch: failed to read CA cert: %w", err)
			}
			caCertPool := x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				return nil, fmt.Errorf("elasticsearch: failed to parse CA cert")
			}
			tlsConfig.RootCAs = caCertPool
		}
		transport.TLSClientConfig = tlsConfig
	}

	esCfg.Transport = transport

	// Create client
	client, err := elasticsearch.NewClient(esCfg)
	if err != nil {
		return nil, err
	}

	// Verify connection
	res, err := client.Info()
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, errors.New("elasticsearch: connection failed - " + res.String())
	}

	return &Client{
		client: client,
		config: cfg,
	}, nil
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

// ES returns the raw Elasticsearch client from the global client.
// Returns nil if client is not initialized.
func ES() *elasticsearch.Client {
	c := GetClient()
	if c == nil {
		return nil
	}
	return c.client
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

	res, err := c.client.Ping(c.client.Ping.WithContext(ctx))
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return ErrPingFailed
	}
	return nil
}

// Close closes the Elasticsearch client.
// Note: ES client uses HTTP and doesn't maintain persistent connections.
func (c *Client) Close() error {
	if c.closed.Swap(true) {
		return ErrAlreadyClosed
	}
	// ES client uses HTTP, no explicit close needed
	return nil
}

// Name returns the client name for the db.Client interface.
func (c *Client) Name() string {
	return "elasticsearch"
}

// RawClient returns the underlying elasticsearch.Client.
func (c *Client) RawClient() *elasticsearch.Client {
	return c.client
}

// Config returns a copy of the client configuration.
func (c *Client) Config() Config {
	return *c.config
}

// IsClosed returns true if the client has been closed.
func (c *Client) IsClosed() bool {
	return c.closed.Load()
}

// Info returns cluster information.
func (c *Client) Info(ctx context.Context) (*esapi.Response, error) {
	if c.closed.Load() {
		return nil, ErrAlreadyClosed
	}
	return c.client.Info(c.client.Info.WithContext(ctx))
}

// Health returns cluster health.
func (c *Client) Health(ctx context.Context) (*esapi.Response, error) {
	if c.closed.Load() {
		return nil, ErrAlreadyClosed
	}
	return c.client.Cluster.Health(c.client.Cluster.Health.WithContext(ctx))
}
