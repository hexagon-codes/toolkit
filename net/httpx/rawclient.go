package httpx

import (
	"net/http"
	"time"
)

// RawClient 返回原生的 *http.Client，带业务常用的 Transport 预设。
//
// 与 NewClient() 的关系：
//   - NewClient() 是业务级封装（Response 缓存成 []byte、.R().Post() 链式 API），
//     适合"POST 一个 JSON 拿一个 JSON"这种同步场景
//   - RawClient() 返回原生 *http.Client，**保留 .Do(req) 调用契约**，适合：
//   - 流式 SSE / WebSocket upgrade（不能预读 body）
//   - 需要自己注入 Transport 做 mock（如 test 中的 http.RoundTripper）
//   - 长耗时请求（thinking 模型 / 视频生成），超时由调用方 ctx 控制而非 Client.Timeout
//
// 默认行为（不传 options）：
//   - 无全局 Timeout（由 request ctx 控制）
//   - ResponseHeaderTimeout 120s（防止服务端连接成功但不发数据的挂死）
//   - 合理的连接池参数（MaxIdleConns=100、IdleConnTimeout=90s）
//
// 典型用法：
//
//	// LLM 流式客户端（thinking 模型可能跑数分钟）
//	c := httpx.RawClient(httpx.WithResponseHeaderTimeout(120 * time.Second))
//	req, _ := http.NewRequestWithContext(ctx, "POST", url, body)
//	resp, err := c.Do(req)  // 契约不变
//
//	// 测试 mock 注入
//	c := httpx.RawClient(httpx.WithRawTransport(mockTransport))
func RawClient(opts ...RawOption) *http.Client {
	cfg := &rawConfig{
		responseHeaderTimeout: 120 * time.Second,
		maxIdleConns:          100,
		maxIdleConnsPerHost:   10,
		idleConnTimeout:       90 * time.Second,
		tlsHandshakeTimeout:   10 * time.Second,
		expectContinueTimeout: 1 * time.Second,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	transport := cfg.customTransport
	if transport == nil {
		transport = &http.Transport{
			// 与 net/http.DefaultTransport 一致地遵循 HTTP(S)_PROXY/NO_PROXY 环境变量，
			// 使 RawClient 与宿主机上其余 HTTP 客户端走同一代理出口。
			Proxy:                 http.ProxyFromEnvironment,
			ResponseHeaderTimeout: cfg.responseHeaderTimeout,
			MaxIdleConns:          cfg.maxIdleConns,
			MaxIdleConnsPerHost:   cfg.maxIdleConnsPerHost,
			IdleConnTimeout:       cfg.idleConnTimeout,
			TLSHandshakeTimeout:   cfg.tlsHandshakeTimeout,
			ExpectContinueTimeout: cfg.expectContinueTimeout,
		}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   cfg.timeout, // 默认 0 = 不限，由 ctx 控制
	}
}

// RawOption 配置 RawClient 的选项。
type RawOption func(*rawConfig)

type rawConfig struct {
	timeout               time.Duration
	responseHeaderTimeout time.Duration
	maxIdleConns          int
	maxIdleConnsPerHost   int
	idleConnTimeout       time.Duration
	tlsHandshakeTimeout   time.Duration
	expectContinueTimeout time.Duration
	customTransport       http.RoundTripper
}

// WithRawTimeout 设置整体请求超时（默认 0 = 不限，推荐由 ctx 控制）。
// 注意：对流式请求应留 0，否则 Client.Timeout 会强制切断长流。
func WithRawTimeout(d time.Duration) RawOption {
	return func(c *rawConfig) { c.timeout = d }
}

// WithResponseHeaderTimeout 响应头超时 —— 从建连成功到收到首个响应头的最长时间。
// 对流式请求关键：防止服务端建连后不发数据的挂死。默认 120s。
func WithResponseHeaderTimeout(d time.Duration) RawOption {
	return func(c *rawConfig) { c.responseHeaderTimeout = d }
}

// WithMaxIdleConns 连接池总上限，默认 100。
func WithMaxIdleConns(n int) RawOption {
	return func(c *rawConfig) { c.maxIdleConns = n }
}

// WithMaxIdleConnsPerHost 每主机连接池上限，默认 10。
// LLM/图像场景建议 20+ 避免高并发短暂抖动。
func WithMaxIdleConnsPerHost(n int) RawOption {
	return func(c *rawConfig) { c.maxIdleConnsPerHost = n }
}

// WithIdleConnTimeout 空闲连接存活时间，默认 90s。
func WithIdleConnTimeout(d time.Duration) RawOption {
	return func(c *rawConfig) { c.idleConnTimeout = d }
}

// WithRawTransport 注入自定义 Transport（测试 mock / 代理场景）。
// 注入后其它 transport 配置项（ResponseHeaderTimeout 等）会被忽略。
func WithRawTransport(t http.RoundTripper) RawOption {
	return func(c *rawConfig) { c.customTransport = t }
}
