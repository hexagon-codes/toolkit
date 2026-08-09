// Package httpx 提供 HTTP 客户端增强功能
//
// 本文件实现 HTTP 连接池管理：
//   - 连接复用：减少建连开销
//   - 连接限制：防止资源耗尽
//   - 健康检查：自动剔除异常连接
//   - 指标监控：连接池状态监控
package httpx

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/hexagon-codes/toolkit/util/circuit"
)

// ============== 连接池配置 ==============

// PoolConfig 连接池配置
type PoolConfig struct {
	// MaxIdleConns 最大空闲连接数
	MaxIdleConns int

	// MaxConnsPerHost 每个主机最大连接数
	MaxConnsPerHost int

	// MaxIdleConnsPerHost 每个主机最大空闲连接数
	MaxIdleConnsPerHost int

	// IdleConnTimeout 空闲连接超时
	IdleConnTimeout time.Duration

	// ConnectTimeout 连接超时
	ConnectTimeout time.Duration

	// ResponseHeaderTimeout 响应头超时
	ResponseHeaderTimeout time.Duration

	// TLSHandshakeTimeout TLS 握手超时
	TLSHandshakeTimeout time.Duration

	// ExpectContinueTimeout Expect-Continue 超时
	ExpectContinueTimeout time.Duration

	// DisableKeepAlives 禁用 Keep-Alive
	DisableKeepAlives bool

	// DisableCompression 禁用压缩
	DisableCompression bool

	// TLSConfig TLS 配置
	TLSConfig *tls.Config

	// Proxy 代理设置
	Proxy func(*http.Request) (*neturl.URL, error)

	// DialContext 自定义拨号函数
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
}

// DefaultPoolConfig 返回默认连接池配置。
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxIdleConns:          100,
		MaxConnsPerHost:       10,
		MaxIdleConnsPerHost:   5,
		IdleConnTimeout:       90 * time.Second,
		ConnectTimeout:        30 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// ============== 连接池 ==============

// Pool HTTP 连接池
type Pool struct {
	// transport 底层 Transport
	transport *http.Transport

	// client HTTP 客户端
	client *http.Client

	// config 配置
	config PoolConfig

	// stats 统计信息
	stats *PoolStats

	// 关闭标记
	closed atomic.Bool
}

// PoolStats 连接池统计
type PoolStats struct {
	// TotalRequests 总请求数
	TotalRequests atomic.Int64

	// ActiveRequests 活跃请求数
	ActiveRequests atomic.Int64

	// ErrorCount 错误数
	ErrorCount atomic.Int64

	// TimeoutCount 超时数
	TimeoutCount atomic.Int64

	responseMu        sync.RWMutex
	completedRequests int64
	totalResponseTime int64
	maxResponseTime   int64
}

// NewPool 校验配置并创建连接池。
func NewPool(config PoolConfig) (*Pool, error) {
	if err := validatePoolConfig(config); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPoolConfig, err)
	}
	cfg := clonePoolConfig(config)

	dialer := &net.Dialer{
		Timeout:   cfg.ConnectTimeout,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
		TLSHandshakeTimeout:   cfg.TLSHandshakeTimeout,
		ExpectContinueTimeout: cfg.ExpectContinueTimeout,
		DisableKeepAlives:     cfg.DisableKeepAlives,
		DisableCompression:    cfg.DisableCompression,
		TLSClientConfig:       cfg.TLSConfig,
		Proxy:                 cfg.Proxy,
		DialContext:           cfg.DialContext,
		ForceAttemptHTTP2:     true,
	}

	if transport.DialContext == nil {
		transport.DialContext = dialer.DialContext
	}

	return &Pool{
		transport: transport,
		client:    &http.Client{Transport: transport},
		config:    cfg,
		stats:     &PoolStats{},
	}, nil
}

// NewDefaultPool 使用包内默认配置创建连接池。
func NewDefaultPool() *Pool {
	return MustNewPool(DefaultPoolConfig())
}

// MustNewPool 创建连接池，配置无效时触发 panic。
// 仅应用于编译期已知且经过测试的静态配置。
func MustNewPool(config PoolConfig) *Pool {
	pool, err := NewPool(config)
	if err != nil {
		panic(err)
	}
	return pool
}

func validatePoolConfig(config PoolConfig) error {
	switch {
	case config.MaxIdleConns < 0:
		return errors.New("maximum idle connections must not be negative")
	case config.MaxConnsPerHost < 0:
		return errors.New("maximum connections per host must not be negative")
	case config.MaxIdleConnsPerHost < 0:
		return errors.New("maximum idle connections per host must not be negative")
	case config.MaxIdleConns > 0 && config.MaxIdleConnsPerHost > config.MaxIdleConns:
		return errors.New("maximum idle connections per host must not exceed the global maximum")
	case config.MaxConnsPerHost > 0 && config.MaxIdleConnsPerHost > config.MaxConnsPerHost:
		return errors.New("maximum idle connections per host must not exceed maximum connections per host")
	case config.IdleConnTimeout < 0:
		return errors.New("idle connection timeout must not be negative")
	case config.ConnectTimeout < 0:
		return errors.New("connect timeout must not be negative")
	case config.ResponseHeaderTimeout < 0:
		return errors.New("response header timeout must not be negative")
	case config.TLSHandshakeTimeout < 0:
		return errors.New("TLS handshake timeout must not be negative")
	case config.ExpectContinueTimeout < 0:
		return errors.New("expect-continue timeout must not be negative")
	default:
		return nil
	}
}

func clonePoolConfig(config PoolConfig) PoolConfig {
	if config.TLSConfig != nil {
		config.TLSConfig = config.TLSConfig.Clone()
	}
	return config
}

// Do 执行调用方已构造且目标可信的 HTTP 请求。
//
// Pool 是基础传输组件，不判断业务 URL。来自不可信输入的 URL 应通过启用了
// WithSSRFProtection 的 Client 执行。
func (p *Pool) Do(req *http.Request) (*http.Response, error) {
	if err := validateHTTPRequest(req); err != nil {
		return nil, err
	}
	if p.closed.Load() {
		return nil, ErrPoolClosed
	}

	p.stats.TotalRequests.Add(1)
	p.stats.ActiveRequests.Add(1)
	defer p.stats.ActiveRequests.Add(-1)

	startTime := time.Now()

	resp, err := p.client.Do(req) // #nosec G704 -- 仅执行调用方构造的可信目标请求，安全边界已在方法契约中声明。

	duration := time.Since(startTime)
	p.updateResponseTime(duration)

	if err != nil {
		p.stats.ErrorCount.Add(1)
		if isTimeout(err) {
			p.stats.TimeoutCount.Add(1)
		}
		return nil, err
	}

	return resp, nil
}

// DoWithContext 带上下文执行请求
func (p *Pool) DoWithContext(ctx context.Context, req *http.Request) (*http.Response, error) {
	if ctx == nil {
		return nil, ErrInvalidContext
	}
	if err := validateHTTPRequest(req); err != nil {
		return nil, err
	}
	return p.Do(req.WithContext(ctx))
}

// Get 发送 GET 请求
func (p *Pool) Get(ctx context.Context, url string) (*http.Response, error) {
	if ctx == nil {
		return nil, ErrInvalidContext
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, err
	}
	return p.Do(req)
}

// Post 发送 POST 请求
func (p *Pool) Post(ctx context.Context, url, contentType string, body io.Reader) (*http.Response, error) {
	if ctx == nil {
		return nil, ErrInvalidContext
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return p.Do(req)
}

// updateResponseTime 更新响应时间统计
func (p *Pool) updateResponseTime(duration time.Duration) {
	ns := duration.Nanoseconds()
	p.stats.responseMu.Lock()
	defer p.stats.responseMu.Unlock()
	p.stats.completedRequests++
	p.stats.totalResponseTime += ns
	if ns > p.stats.maxResponseTime {
		p.stats.maxResponseTime = ns
	}
}

// GetStats 获取统计信息
func (p *Pool) GetStats() PoolStatsSnapshot {
	p.stats.responseMu.RLock()
	defer p.stats.responseMu.RUnlock()
	var average time.Duration
	if p.stats.completedRequests > 0 {
		average = time.Duration(p.stats.totalResponseTime / p.stats.completedRequests)
	}
	return PoolStatsSnapshot{
		TotalRequests:     p.stats.TotalRequests.Load(),
		ActiveRequests:    p.stats.ActiveRequests.Load(),
		CompletedRequests: p.stats.completedRequests,
		ErrorCount:        p.stats.ErrorCount.Load(),
		TimeoutCount:      p.stats.TimeoutCount.Load(),
		AvgResponseTime:   average,
		MaxResponseTime:   time.Duration(p.stats.maxResponseTime),
	}
}

// PoolStatsSnapshot 连接池统计快照
type PoolStatsSnapshot struct {
	TotalRequests     int64         `json:"total_requests"`
	ActiveRequests    int64         `json:"active_requests"`
	CompletedRequests int64         `json:"completed_requests"`
	ErrorCount        int64         `json:"error_count"`
	TimeoutCount      int64         `json:"timeout_count"`
	AvgResponseTime   time.Duration `json:"avg_response_time"`
	MaxResponseTime   time.Duration `json:"max_response_time"`
}

// Close 关闭连接池
func (p *Pool) Close() {
	if p.closed.CompareAndSwap(false, true) {
		p.transport.CloseIdleConnections()
	}
}

// CloseIdleConnections 关闭空闲连接
func (p *Pool) CloseIdleConnections() {
	p.transport.CloseIdleConnections()
}

// Client 获取底层 HTTP 客户端
func (p *Pool) Client() *http.Client {
	return p.client
}

// Transport 获取底层 Transport
func (p *Pool) Transport() *http.Transport {
	return p.transport
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func validateHTTPRequest(req *http.Request) error {
	if req == nil {
		return fmt.Errorf("%w: request must not be nil", ErrInvalidRequest)
	}
	if req.URL == nil || req.URL.Scheme == "" || req.URL.Host == "" {
		return fmt.Errorf("%w: request URL must be absolute", ErrInvalidRequest)
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return fmt.Errorf("%w: request URL scheme must be HTTP or HTTPS", ErrInvalidRequest)
	}
	return nil
}

// ============== 主机级连接池 ==============

// DefaultMaxHostPools 是默认允许保留的主机连接池数量。
const DefaultMaxHostPools = 256

// HostPoolConfig 配置主机级连接池。
type HostPoolConfig struct {
	// Pool 是每个主机连接池的默认配置。
	Pool PoolConfig
	// MaxHosts 是配置与已创建主机的联合上限。
	MaxHosts int
}

// DefaultHostPoolConfig 返回独立的默认主机连接池配置。
func DefaultHostPoolConfig() HostPoolConfig {
	return HostPoolConfig{
		Pool:     DefaultPoolConfig(),
		MaxHosts: DefaultMaxHostPools,
	}
}

// HostPool 主机级连接池管理
type HostPool struct {
	// pools 每个主机的连接池
	pools map[string]*Pool

	// defaultConfig 默认配置
	defaultConfig PoolConfig

	// hostConfigs 主机特定配置
	hostConfigs map[string]PoolConfig

	// maxHosts 配置与已创建主机的联合上限
	maxHosts int

	mu sync.RWMutex

	closed bool
}

// NewHostPool 校验配置并创建有界主机连接池。
func NewHostPool(config HostPoolConfig) (*HostPool, error) {
	if config.MaxHosts <= 0 {
		return nil, fmt.Errorf("%w: maximum hosts must be greater than zero", ErrInvalidPoolConfig)
	}
	if err := validatePoolConfig(config.Pool); err != nil {
		return nil, fmt.Errorf("%w: default host pool configuration: %w", ErrInvalidPoolConfig, err)
	}

	return &HostPool{
		pools:         make(map[string]*Pool),
		defaultConfig: clonePoolConfig(config.Pool),
		hostConfigs:   make(map[string]PoolConfig),
		maxHosts:      config.MaxHosts,
	}, nil
}

// NewDefaultHostPool 使用默认配置创建主机级连接池。
func NewDefaultHostPool() *HostPool {
	hostPool, err := NewHostPool(DefaultHostPoolConfig())
	if err != nil {
		panic(err)
	}
	return hostPool
}

// SetHostConfig 在主机连接池首次创建前设置专属配置。
func (hp *HostPool) SetHostConfig(host string, config PoolConfig) error {
	host, err := normalizeHostPoolKey(host)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPoolConfig, err)
	}
	if err = validatePoolConfig(config); err != nil {
		return fmt.Errorf("%w: host %q: %w", ErrInvalidPoolConfig, host, err)
	}
	hp.mu.Lock()
	defer hp.mu.Unlock()
	if hp.closed {
		return ErrPoolClosed
	}
	if _, exists := hp.pools[host]; exists {
		return fmt.Errorf("%w: host %q pool is already initialized", ErrInvalidPoolConfig, host)
	}
	if _, configured := hp.hostConfigs[host]; !configured && hp.hostCountLocked() >= hp.maxHosts {
		return fmt.Errorf("%w: maximum hosts %d", ErrHostPoolCapacity, hp.maxHosts)
	}
	hp.hostConfigs[host] = clonePoolConfig(config)
	return nil
}

// GetPool 获取指定主机的连接池
func (hp *HostPool) GetPool(host string) (*Pool, error) {
	host, err := normalizeHostPoolKey(host)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidRequest, err)
	}
	hp.mu.RLock()
	pool, exists := hp.pools[host]
	closed := hp.closed
	hp.mu.RUnlock()

	if closed {
		return nil, ErrPoolClosed
	}
	if exists {
		return pool, nil
	}

	hp.mu.Lock()
	defer hp.mu.Unlock()
	if hp.closed {
		return nil, ErrPoolClosed
	}

	// 双重检查
	if pool, exists = hp.pools[host]; exists {
		return pool, nil
	}
	if _, configured := hp.hostConfigs[host]; !configured && hp.hostCountLocked() >= hp.maxHosts {
		return nil, fmt.Errorf("%w: maximum hosts %d", ErrHostPoolCapacity, hp.maxHosts)
	}

	// 创建新池
	cfg := hp.defaultConfig
	if hostCfg, ok := hp.hostConfigs[host]; ok {
		cfg = hostCfg
	}

	pool, err = NewPool(cfg)
	if err != nil {
		return nil, err
	}
	hp.pools[host] = pool
	return pool, nil
}

// Do 执行请求（自动选择连接池）
func (hp *HostPool) Do(req *http.Request) (*http.Response, error) {
	if err := validateHTTPRequest(req); err != nil {
		return nil, err
	}
	host := req.URL.Host
	pool, err := hp.GetPool(host)
	if err != nil {
		return nil, err
	}
	return pool.Do(req)
}

// RemoveHost 移除主机专属配置与连接池，并释放对应容量。
func (hp *HostPool) RemoveHost(host string) error {
	host, err := normalizeHostPoolKey(host)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidRequest, err)
	}

	hp.mu.Lock()
	if hp.closed {
		hp.mu.Unlock()
		return ErrPoolClosed
	}
	pool := hp.pools[host]
	delete(hp.pools, host)
	delete(hp.hostConfigs, host)
	hp.mu.Unlock()

	if pool != nil {
		pool.Close()
	}
	return nil
}

// Close 关闭所有连接池
func (hp *HostPool) Close() {
	hp.mu.Lock()
	defer hp.mu.Unlock()
	if hp.closed {
		return
	}
	hp.closed = true

	for _, pool := range hp.pools {
		pool.Close()
	}
	hp.pools = make(map[string]*Pool)
	hp.hostConfigs = make(map[string]PoolConfig)
}

// GetAllStats 获取所有主机的统计
func (hp *HostPool) GetAllStats() map[string]PoolStatsSnapshot {
	hp.mu.RLock()
	defer hp.mu.RUnlock()

	stats := make(map[string]PoolStatsSnapshot)
	for host, pool := range hp.pools {
		stats[host] = pool.GetStats()
	}
	return stats
}

func (hp *HostPool) hostCountLocked() int {
	count := len(hp.pools)
	for host := range hp.hostConfigs {
		if _, exists := hp.pools[host]; !exists {
			count++
		}
	}
	return count
}

func normalizeHostPoolKey(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", errors.New("host must not be empty")
	}
	parsed, err := neturl.Parse("//" + host)
	if err != nil {
		return "", fmt.Errorf("invalid host %q: %w", host, err)
	}
	if parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("invalid host %q", host)
	}
	hostname := strings.TrimRight(strings.ToLower(parsed.Hostname()), ".")
	if hostname == "" {
		return "", fmt.Errorf("invalid host %q", host)
	}
	parsedPort := parsed.Port()
	if parsedPort == "" && strings.HasSuffix(parsed.Host, ":") {
		return "", fmt.Errorf("invalid host %q", host)
	}
	if parsedPort != "" {
		portNumber, parseErr := strconv.Atoi(parsedPort)
		if parseErr != nil || portNumber < 1 || portNumber > 65535 {
			return "", fmt.Errorf("invalid host %q", host)
		}
		return net.JoinHostPort(hostname, parsedPort), nil
	}
	if strings.Contains(hostname, ":") {
		return "[" + hostname + "]", nil
	}
	return hostname, nil
}

// ============== 全局连接池 ==============

var (
	globalPool   atomic.Pointer[Pool]
	globalPoolMu sync.Mutex
)

// GlobalPool 获取全局连接池
func GlobalPool() *Pool {
	// 快速路径：已经设置了 pool
	if p := globalPool.Load(); p != nil {
		return p
	}

	// 慢路径：使用互斥锁初始化默认 pool
	globalPoolMu.Lock()
	defer globalPoolMu.Unlock()

	// 双重检查
	if p := globalPool.Load(); p != nil {
		return p
	}

	p := NewDefaultPool()
	globalPool.Store(p)
	return p
}

// SetGlobalPool 设置全局连接池
//
// 注意：应在程序启动时调用，不建议在运行时频繁更换
// 旧的连接池不会被自动关闭，调用者负责管理其生命周期
func SetGlobalPool(pool *Pool) error {
	if pool == nil {
		return fmt.Errorf("%w: global pool must not be nil", ErrInvalidPoolConfig)
	}
	if pool.closed.Load() {
		return ErrPoolClosed
	}
	globalPoolMu.Lock()
	defer globalPoolMu.Unlock()
	globalPool.Store(pool)
	return nil
}

// ============== 重试中间件 ==============

// RetryConfig 重试配置
type RetryConfig struct {
	// MaxRetries 最大重试次数
	MaxRetries int

	// RetryWait 重试等待时间
	RetryWait time.Duration

	// MaxRetryWait 最大重试等待时间
	MaxRetryWait time.Duration

	// RetryCondition 重试条件判断
	RetryCondition func(resp *http.Response, err error) bool
}

// ErrInvalidRetryConfig 表示 HTTP 重试连接池配置无效。
var ErrInvalidRetryConfig = errors.New("httpx: invalid retry pool configuration")

// DefaultRetryConfig 返回独立的默认重试配置。
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:   3,
		RetryWait:    100 * time.Millisecond,
		MaxRetryWait: 5 * time.Second,
		RetryCondition: func(resp *http.Response, err error) bool {
			if err != nil {
				return true
			}
			return resp.StatusCode >= 500 || resp.StatusCode == 429
		},
	}
}

// RetryPool 带重试的连接池
type RetryPool struct {
	pool   *Pool
	config RetryConfig
}

// NewRetryPool 校验配置并创建带重试的连接池。
func NewRetryPool(pool *Pool, config RetryConfig) (*RetryPool, error) {
	switch {
	case pool == nil:
		return nil, fmt.Errorf("%w: pool must not be nil", ErrInvalidRetryConfig)
	case pool.closed.Load():
		return nil, fmt.Errorf("%w: pool: %w", ErrInvalidRetryConfig, ErrPoolClosed)
	case config.MaxRetries < 0:
		return nil, fmt.Errorf("%w: maximum retries must not be negative", ErrInvalidRetryConfig)
	case config.RetryWait < 0:
		return nil, fmt.Errorf("%w: retry wait must not be negative", ErrInvalidRetryConfig)
	case config.MaxRetryWait < config.RetryWait:
		return nil, fmt.Errorf("%w: maximum retry wait must not be shorter than retry wait", ErrInvalidRetryConfig)
	case config.RetryCondition == nil:
		return nil, fmt.Errorf("%w: retry condition must not be nil", ErrInvalidRetryConfig)
	}

	return &RetryPool{
		pool:   pool,
		config: config,
	}, nil
}

// Do 执行带重试的请求
//
// 带 Body 且需要重试的请求必须提供 GetBody。http.NewRequest 对 bytes.Buffer、
// bytes.Reader 和 strings.Reader 会自动设置 GetBody。
func (rp *RetryPool) Do(req *http.Request) (*http.Response, error) {
	if err := validateHTTPRequest(req); err != nil {
		return nil, fmt.Errorf("retry pool: %w", err)
	}
	// 非幂等请求未携带幂等键时绕过重试，避免重复业务副作用。
	if rp.config.MaxRetries == 0 || !isHTTPRetrySafe(req.Method, req.Header) {
		return rp.pool.Do(req)
	}
	hasBody := req.Body != nil && req.Body != http.NoBody
	if hasBody && req.GetBody == nil {
		return nil, fmt.Errorf("retry pool: %w", ErrRequestBodyNotReplayable)
	}

	var lastErr error
	wait := rp.config.RetryWait

	for attempt := 0; attempt <= rp.config.MaxRetries; attempt++ {
		// 如果不是第一次尝试，等待并重置 Body
		if attempt > 0 {
			// 等待重试间隔，同时监听 context 取消
			timer := time.NewTimer(wait)
			select {
			case <-timer.C:
			case <-req.Context().Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return nil, errors.Join(
					lastErr,
					fmt.Errorf("retry pool: backoff interrupted: %w", req.Context().Err()),
				)
			}
			// 指数退避
			if wait >= rp.config.MaxRetryWait-wait {
				wait = rp.config.MaxRetryWait
			} else {
				wait *= 2
			}
			// 重置 Body 以支持重放
			if hasBody {
				body, err := req.GetBody()
				if err != nil {
					return nil, errors.Join(
						lastErr,
						fmt.Errorf("retry pool: restore request body: %w", err),
					)
				}
				req.Body = body
			}
		}

		resp, err := rp.pool.Do(req)
		if errors.Is(err, ErrPoolClosed) {
			return nil, fmt.Errorf("retry pool: %w", err)
		}

		// 检查是否需要重试
		if !rp.config.RetryCondition(resp, err) {
			return resp, err
		}

		lastErr = err
		if lastErr == nil && resp != nil {
			lastErr = fmt.Errorf("retryable HTTP status: %s", resp.Status)
		}

		// 关闭响应体以释放连接（重试前必须清理）
		if resp != nil && resp.Body != nil {
			// 有界排空避免异常响应体让重试路径无限读取；超出上限时放弃连接复用。
			_, drainErr := io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
			closeErr := resp.Body.Close()
			lastErr = errors.Join(lastErr, drainErr, closeErr)
		}
	}

	// 所有重试均失败
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// ============== 限流中间件 ==============

// RateLimitedPool 带限流的连接池
type RateLimitedPool struct {
	pool    *Pool
	limiter *rate.Limiter
}

// NewRateLimitedPool 创建带限流的连接池
// rps: 每秒请求数限制
func NewRateLimitedPool(pool *Pool, rps int) (*RateLimitedPool, error) {
	if pool == nil {
		return nil, errors.New("httpx: rate limited pool requires a pool")
	}
	if pool.closed.Load() {
		return nil, fmt.Errorf("httpx: rate limited pool: %w", ErrPoolClosed)
	}
	if rps <= 0 {
		return nil, errors.New("httpx: requests per second must be positive")
	}
	return &RateLimitedPool{
		pool:    pool,
		limiter: rate.NewLimiter(rate.Limit(rps), rps),
	}, nil
}

// Do 执行带限流的请求
func (rlp *RateLimitedPool) Do(req *http.Request) (*http.Response, error) {
	if err := validateHTTPRequest(req); err != nil {
		return nil, fmt.Errorf("rate limited pool: %w", err)
	}
	if rlp.pool.closed.Load() {
		return nil, fmt.Errorf("rate limited pool: %w", ErrPoolClosed)
	}
	if err := rlp.limiter.Wait(req.Context()); err != nil {
		return nil, err
	}
	return rlp.pool.Do(req)
}

// Close 关闭底层连接池。
// 多次调用是安全的。
func (rlp *RateLimitedPool) Close() {
	rlp.pool.Close()
}

// ============== 熔断中间件 ==============

var errRetryableHTTPStatus = errors.New("httpx: retryable HTTP status")

// CircuitBreakerPool 组合连接池与通用熔断器。
type CircuitBreakerPool struct {
	pool    *Pool
	breaker *circuit.Breaker
}

// NewCircuitBreakerPool 使用 util/circuit 的统一状态机创建熔断连接池。
func NewCircuitBreakerPool(pool *Pool, opts ...circuit.Option) (*CircuitBreakerPool, error) {
	if pool == nil {
		return nil, errors.New("httpx: circuit breaker pool requires a pool")
	}
	if pool.closed.Load() {
		return nil, fmt.Errorf("httpx: circuit breaker pool: %w", ErrPoolClosed)
	}
	breaker, err := circuit.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("httpx: create circuit breaker: %w", err)
	}
	return &CircuitBreakerPool{pool: pool, breaker: breaker}, nil
}

// Do 执行带熔断保护的请求；触发熔断的首个 5xx 响应仍原样返回调用方。
func (cbp *CircuitBreakerPool) Do(req *http.Request) (*http.Response, error) {
	if err := validateHTTPRequest(req); err != nil {
		return nil, fmt.Errorf("circuit breaker pool: %w", err)
	}
	permit, err := cbp.breaker.Acquire()
	if err != nil {
		return nil, err
	}

	response, requestErr := cbp.pool.Do(req) //nolint:bodyclose // 成功响应体的所有权随返回值交给调用方。
	resultErr := requestErr
	if requestErr == nil && response != nil && response.StatusCode >= http.StatusInternalServerError {
		resultErr = errRetryableHTTPStatus
	}
	completeErr := permit.Complete(resultErr)
	if completeErr != nil {
		if response != nil && response.Body != nil {
			completeErr = errors.Join(completeErr, response.Body.Close())
		}
		return nil, errors.Join(requestErr, completeErr)
	}
	if requestErr != nil {
		return nil, requestErr
	}
	if response == nil {
		return nil, errors.New("httpx: pool returned an empty response")
	}
	return response, nil
}

// State 返回当前熔断状态。
func (cbp *CircuitBreakerPool) State() circuit.State { return cbp.breaker.State() }

// Reset 重置熔断器。
func (cbp *CircuitBreakerPool) Reset() error { return cbp.breaker.Reset() }

// Close 关闭熔断器与底层连接池，多次调用是安全的。
func (cbp *CircuitBreakerPool) Close() {
	cbp.breaker.Close()
	cbp.pool.Close()
}
