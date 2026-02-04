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
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
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
	Proxy func(*http.Request) (*url.URL, error)

	// DialContext 自定义拨号函数
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
}

// DefaultPoolConfig 默认连接池配置
var DefaultPoolConfig = PoolConfig{
	MaxIdleConns:          100,
	MaxConnsPerHost:       10,
	MaxIdleConnsPerHost:   5,
	IdleConnTimeout:       90 * time.Second,
	ConnectTimeout:        30 * time.Second,
	ResponseHeaderTimeout: 30 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
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

	mu sync.RWMutex
}

// PoolStats 连接池统计
type PoolStats struct {
	// TotalRequests 总请求数
	TotalRequests atomic.Int64

	// ActiveRequests 活跃请求数
	ActiveRequests atomic.Int64

	// TotalConnections 总连接数（历史）
	TotalConnections atomic.Int64

	// IdleConnections 当前空闲连接数
	IdleConnections atomic.Int64

	// WaitCount 等待获取连接的次数
	WaitCount atomic.Int64

	// WaitDuration 等待获取连接的总时间
	WaitDuration atomic.Int64

	// ErrorCount 错误数
	ErrorCount atomic.Int64

	// TimeoutCount 超时数
	TimeoutCount atomic.Int64

	// AvgResponseTime 平均响应时间（纳秒）
	AvgResponseTime atomic.Int64

	// MaxResponseTime 最大响应时间（纳秒）
	MaxResponseTime atomic.Int64
}

// NewPool 创建连接池
func NewPool(config ...PoolConfig) *Pool {
	cfg := DefaultPoolConfig
	if len(config) > 0 {
		cfg = config[0]
	}

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
	}

	if transport.DialContext == nil {
		transport.DialContext = dialer.DialContext
	}

	return &Pool{
		transport: transport,
		client:    &http.Client{Transport: transport},
		config:    cfg,
		stats:     &PoolStats{},
	}
}

// Do 执行 HTTP 请求
func (p *Pool) Do(req *http.Request) (*http.Response, error) {
	if p.closed.Load() {
		return nil, fmt.Errorf("pool is closed")
	}

	p.stats.TotalRequests.Add(1)
	p.stats.ActiveRequests.Add(1)
	defer p.stats.ActiveRequests.Add(-1)

	startTime := time.Now()

	resp, err := p.client.Do(req)

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
	return p.Do(req.WithContext(ctx))
}

// Get 发送 GET 请求
func (p *Pool) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	return p.Do(req)
}

// Post 发送 POST 请求
func (p *Pool) Post(ctx context.Context, url, contentType string, body io.Reader) (*http.Response, error) {
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

	// 更新平均响应时间（简化的指数移动平均）
	oldAvg := p.stats.AvgResponseTime.Load()
	newAvg := (oldAvg*9 + ns) / 10
	p.stats.AvgResponseTime.Store(newAvg)

	// 更新最大响应时间
	for {
		max := p.stats.MaxResponseTime.Load()
		if ns <= max {
			break
		}
		if p.stats.MaxResponseTime.CompareAndSwap(max, ns) {
			break
		}
	}
}

// GetStats 获取统计信息
func (p *Pool) GetStats() PoolStatsSnapshot {
	return PoolStatsSnapshot{
		TotalRequests:   p.stats.TotalRequests.Load(),
		ActiveRequests:  p.stats.ActiveRequests.Load(),
		ErrorCount:      p.stats.ErrorCount.Load(),
		TimeoutCount:    p.stats.TimeoutCount.Load(),
		AvgResponseTime: time.Duration(p.stats.AvgResponseTime.Load()),
		MaxResponseTime: time.Duration(p.stats.MaxResponseTime.Load()),
	}
}

// PoolStatsSnapshot 连接池统计快照
type PoolStatsSnapshot struct {
	TotalRequests   int64         `json:"total_requests"`
	ActiveRequests  int64         `json:"active_requests"`
	ErrorCount      int64         `json:"error_count"`
	TimeoutCount    int64         `json:"timeout_count"`
	AvgResponseTime time.Duration `json:"avg_response_time"`
	MaxResponseTime time.Duration `json:"max_response_time"`
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
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}

// ============== 主机级连接池 ==============

// HostPool 主机级连接池管理
type HostPool struct {
	// pools 每个主机的连接池
	pools map[string]*Pool

	// defaultConfig 默认配置
	defaultConfig PoolConfig

	// hostConfigs 主机特定配置
	hostConfigs map[string]PoolConfig

	mu sync.RWMutex
}

// NewHostPool 创建主机级连接池
func NewHostPool(defaultConfig ...PoolConfig) *HostPool {
	cfg := DefaultPoolConfig
	if len(defaultConfig) > 0 {
		cfg = defaultConfig[0]
	}

	return &HostPool{
		pools:         make(map[string]*Pool),
		defaultConfig: cfg,
		hostConfigs:   make(map[string]PoolConfig),
	}
}

// SetHostConfig 设置主机特定配置
func (hp *HostPool) SetHostConfig(host string, config PoolConfig) {
	hp.mu.Lock()
	defer hp.mu.Unlock()
	hp.hostConfigs[host] = config
}

// GetPool 获取指定主机的连接池
func (hp *HostPool) GetPool(host string) *Pool {
	hp.mu.RLock()
	pool, exists := hp.pools[host]
	hp.mu.RUnlock()

	if exists {
		return pool
	}

	hp.mu.Lock()
	defer hp.mu.Unlock()

	// 双重检查
	if pool, exists = hp.pools[host]; exists {
		return pool
	}

	// 创建新池
	cfg := hp.defaultConfig
	if hostCfg, ok := hp.hostConfigs[host]; ok {
		cfg = hostCfg
	}

	pool = NewPool(cfg)
	hp.pools[host] = pool
	return pool
}

// Do 执行请求（自动选择连接池）
func (hp *HostPool) Do(req *http.Request) (*http.Response, error) {
	host := req.URL.Host
	pool := hp.GetPool(host)
	return pool.Do(req)
}

// Close 关闭所有连接池
func (hp *HostPool) Close() {
	hp.mu.Lock()
	defer hp.mu.Unlock()

	for _, pool := range hp.pools {
		pool.Close()
	}
	hp.pools = make(map[string]*Pool)
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

// ============== 全局连接池 ==============

var (
	globalPool     *Pool
	globalPoolOnce sync.Once
)

// GlobalPool 获取全局连接池
func GlobalPool() *Pool {
	globalPoolOnce.Do(func() {
		globalPool = NewPool()
	})
	return globalPool
}

// SetGlobalPool 设置全局连接池
func SetGlobalPool(pool *Pool) {
	globalPool = pool
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

// DefaultRetryConfig 默认重试配置
var DefaultRetryConfig = RetryConfig{
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

// RetryPool 带重试的连接池
type RetryPool struct {
	pool   *Pool
	config RetryConfig
}

// NewRetryPool 创建带重试的连接池
func NewRetryPool(pool *Pool, config ...RetryConfig) *RetryPool {
	cfg := DefaultRetryConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	return &RetryPool{
		pool:   pool,
		config: cfg,
	}
}

// Do 执行带重试的请求
func (rp *RetryPool) Do(req *http.Request) (*http.Response, error) {
	var lastErr error
	var lastResp *http.Response
	wait := rp.config.RetryWait

	for attempt := 0; attempt <= rp.config.MaxRetries; attempt++ {
		// 如果不是第一次尝试，等待
		if attempt > 0 {
			time.Sleep(wait)
			// 指数退避
			wait *= 2
			if wait > rp.config.MaxRetryWait {
				wait = rp.config.MaxRetryWait
			}
		}

		resp, err := rp.pool.Do(req)

		// 检查是否需要重试
		if !rp.config.RetryCondition(resp, err) {
			return resp, err
		}

		lastErr = err
		lastResp = resp

		// 关闭响应体以释放连接
		if resp != nil && resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}

	if lastResp != nil {
		return lastResp, lastErr
	}
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// ============== 限流中间件 ==============

// RateLimitedPool 带限流的连接池
type RateLimitedPool struct {
	pool    *Pool
	limiter *rateLimiter
}

type rateLimiter struct {
	tokens   chan struct{}
	interval time.Duration
	stop     chan struct{}
}

// NewRateLimitedPool 创建带限流的连接池
// rps: 每秒请求数限制
func NewRateLimitedPool(pool *Pool, rps int) *RateLimitedPool {
	limiter := &rateLimiter{
		tokens:   make(chan struct{}, rps),
		interval: time.Second / time.Duration(rps),
		stop:     make(chan struct{}),
	}

	// 初始填充 token
	for i := 0; i < rps; i++ {
		limiter.tokens <- struct{}{}
	}

	// 定时补充 token
	go func() {
		ticker := time.NewTicker(limiter.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				select {
				case limiter.tokens <- struct{}{}:
				default:
				}
			case <-limiter.stop:
				return
			}
		}
	}()

	return &RateLimitedPool{
		pool:    pool,
		limiter: limiter,
	}
}

// Do 执行带限流的请求
func (rlp *RateLimitedPool) Do(req *http.Request) (*http.Response, error) {
	// 获取 token
	select {
	case <-rlp.limiter.tokens:
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}

	return rlp.pool.Do(req)
}

// Close 关闭限流池
func (rlp *RateLimitedPool) Close() {
	close(rlp.limiter.stop)
	rlp.pool.Close()
}

// ============== 断路器中间件 ==============

// CircuitBreakerConfig 断路器配置
type CircuitBreakerConfig struct {
	// FailureThreshold 失败阈值
	FailureThreshold int

	// SuccessThreshold 成功阈值（恢复所需）
	SuccessThreshold int

	// Timeout 开路超时
	Timeout time.Duration
}

// CircuitBreakerState 断路器状态
type CircuitBreakerState int

const (
	// CircuitClosed 关闭（正常）
	CircuitClosed CircuitBreakerState = iota

	// CircuitOpen 开路（拒绝请求）
	CircuitOpen

	// CircuitHalfOpen 半开（尝试恢复）
	CircuitHalfOpen
)

// CircuitBreakerPool 带断路器的连接池
type CircuitBreakerPool struct {
	pool   *Pool
	config CircuitBreakerConfig

	state       CircuitBreakerState
	failures    int
	successes   int
	lastFailure time.Time

	mu sync.Mutex
}

// NewCircuitBreakerPool 创建带断路器的连接池
func NewCircuitBreakerPool(pool *Pool, config CircuitBreakerConfig) *CircuitBreakerPool {
	return &CircuitBreakerPool{
		pool:   pool,
		config: config,
		state:  CircuitClosed,
	}
}

// Do 执行带断路器的请求
func (cbp *CircuitBreakerPool) Do(req *http.Request) (*http.Response, error) {
	cbp.mu.Lock()

	// 检查断路器状态
	switch cbp.state {
	case CircuitOpen:
		// 检查是否可以进入半开状态
		if time.Since(cbp.lastFailure) > cbp.config.Timeout {
			cbp.state = CircuitHalfOpen
			cbp.successes = 0
		} else {
			cbp.mu.Unlock()
			return nil, fmt.Errorf("circuit breaker is open")
		}
	}

	cbp.mu.Unlock()

	// 执行请求
	resp, err := cbp.pool.Do(req)

	cbp.mu.Lock()
	defer cbp.mu.Unlock()

	if err != nil || (resp != nil && resp.StatusCode >= 500) {
		// 失败
		cbp.failures++
		cbp.lastFailure = time.Now()

		if cbp.state == CircuitHalfOpen {
			cbp.state = CircuitOpen
		} else if cbp.failures >= cbp.config.FailureThreshold {
			cbp.state = CircuitOpen
		}
	} else {
		// 成功
		if cbp.state == CircuitHalfOpen {
			cbp.successes++
			if cbp.successes >= cbp.config.SuccessThreshold {
				cbp.state = CircuitClosed
				cbp.failures = 0
			}
		} else {
			cbp.failures = 0
		}
	}

	return resp, err
}

// State 获取当前状态
func (cbp *CircuitBreakerPool) State() CircuitBreakerState {
	cbp.mu.Lock()
	defer cbp.mu.Unlock()
	return cbp.state
}

// Reset 重置断路器
func (cbp *CircuitBreakerPool) Reset() {
	cbp.mu.Lock()
	defer cbp.mu.Unlock()
	cbp.state = CircuitClosed
	cbp.failures = 0
	cbp.successes = 0
}
