package elasticsearch

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
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
	client    *elasticsearch.Client
	transport *http.Transport
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

// Init initializes the global Elasticsearch client singleton.
// It is safe to call multiple times; only the first call takes effect.
func Init(cfg *Config, opts ...Option) error {
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
	instance, initErr = New(cfg, opts...)
	if initErr == nil {
		initialized.Store(true)
	}
	return initErr
}

// New creates a new Elasticsearch client (non-singleton).
// Use this when you need multiple clients or dependency injection.
func New(cfg *Config, opts ...Option) (result *Client, err error) {
	cfg = cloneConfig(cfg)

	// Apply options
	cfg.Apply(opts...)
	cfg = cloneConfig(cfg)

	// Validate
	if validationErr := cfg.Validate(); validationErr != nil {
		return nil, validationErr
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
		EnableDebugLogger:     cfg.EnableDebugLogger,
	}

	transport, err := buildHTTPTransport(cfg)
	if err != nil {
		return nil, err
	}
	esCfg.Transport = transport

	// Create client
	client, err := elasticsearch.NewClient(esCfg)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, err
	}
	defer func() {
		if err == nil {
			return
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = errors.Join(err, client.Close(closeCtx))
		transport.CloseIdleConnections()
		result = nil
	}()

	// 校验连接
	res, err := client.Info()
	if err != nil {
		return nil, err
	}
	if res == nil || res.Body == nil {
		return nil, errors.New("elasticsearch: connection verification returned an empty response")
	}
	defer func() { err = errors.Join(err, res.Body.Close()) }()

	if res.IsError() {
		return nil, errors.New("elasticsearch: connection failed - " + res.String())
	}

	return &Client{
		client:    client,
		transport: transport,
		config:    cfg,
	}, nil
}

// buildHTTPTransport 构造启用 TLS 1.2 下限和可选自定义 CA 的 HTTP 传输层。
func buildHTTPTransport(cfg *Config) (*http.Transport, error) {
	transport := &http.Transport{
		MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: cfg.RequestTimeout,
		IdleConnTimeout:       90 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	if cfg.CACert != "" {
		caCert, readErr := os.ReadFile(cfg.CACert) // #nosec G304 -- CA 路径来自显式连接配置。
		if readErr != nil {
			return nil, fmt.Errorf("elasticsearch: failed to read CA cert: %w", readErr)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("elasticsearch: failed to parse CA cert")
		}
		transport.TLSClientConfig.RootCAs = caCertPool
	}
	return transport, nil
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
	initialized.Store(false)
	initErr = nil
	return err
}

// Reset resets the singleton, allowing re-initialization.
// This is primarily useful for testing.
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
func (c *Client) Ping(ctx context.Context) (err error) {
	if c.closed.Load() {
		return ErrAlreadyClosed
	}

	res, err := c.client.Ping(c.client.Ping.WithContext(ctx))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, res.Body.Close()) }()

	if res.IsError() {
		return ErrPingFailed
	}
	return nil
}

// Close 关闭 Elasticsearch 客户端及底层 HTTP 连接池。
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		if c.client != nil {
			closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			c.closeErr = c.client.Close(closeCtx)
		}
		if c.transport != nil {
			c.transport.CloseIdleConnections()
		}
	})
	return c.closeErr
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
	return *cloneConfig(c.config)
}

// IsClosed returns true if the client has been closed.
func (c *Client) IsClosed() bool {
	return c.closed.Load()
}

// Info returns cluster information.
//
// 注意：调用者必须关闭返回的 Response.Body，推荐使用 InfoParsed 替代
func (c *Client) Info(ctx context.Context) (*esapi.Response, error) {
	if c.closed.Load() {
		return nil, ErrAlreadyClosed
	}
	return c.client.Info(c.client.Info.WithContext(ctx))
}

// ClusterInfo 集群信息结构体
type ClusterInfo struct {
	Name        string `json:"name"`
	ClusterName string `json:"cluster_name"`
	ClusterUUID string `json:"cluster_uuid"`
	Version     struct {
		Number string `json:"number"`
	} `json:"version"`
}

// InfoParsed 返回解析后的集群信息（推荐使用，无需手动关闭 Body）
func (c *Client) InfoParsed(ctx context.Context) (_ *ClusterInfo, err error) {
	res, err := c.Info(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, res.Body.Close()) }()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch: info request failed: %s", res.Status())
	}

	var info ClusterInfo
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("elasticsearch: failed to decode info response: %w", err)
	}
	return &info, nil
}

// Health returns cluster health.
//
// 注意：调用者必须关闭返回的 Response.Body，推荐使用 HealthParsed 替代
func (c *Client) Health(ctx context.Context) (*esapi.Response, error) {
	if c.closed.Load() {
		return nil, ErrAlreadyClosed
	}
	return c.client.Cluster.Health(c.client.Cluster.Health.WithContext(ctx))
}

// ClusterHealth 集群健康状态结构体
type ClusterHealth struct {
	ClusterName         string `json:"cluster_name"`
	Status              string `json:"status"` // green, yellow, red
	NumberOfNodes       int    `json:"number_of_nodes"`
	NumberOfDataNodes   int    `json:"number_of_data_nodes"`
	ActivePrimaryShards int    `json:"active_primary_shards"`
	ActiveShards        int    `json:"active_shards"`
	RelocatingShards    int    `json:"relocating_shards"`
	InitializingShards  int    `json:"initializing_shards"`
	UnassignedShards    int    `json:"unassigned_shards"`
}

// HealthParsed 返回解析后的集群健康状态（推荐使用，无需手动关闭 Body）
func (c *Client) HealthParsed(ctx context.Context) (_ *ClusterHealth, err error) {
	res, err := c.Health(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, res.Body.Close()) }()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch: health request failed: %s", res.Status())
	}

	var health ClusterHealth
	if err := json.NewDecoder(res.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("elasticsearch: failed to decode health response: %w", err)
	}
	return &health, nil
}
