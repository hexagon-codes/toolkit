package httpx

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"
)

// DefaultRawClientTimeout 是原生客户端默认的请求总超时。
const DefaultRawClientTimeout = 10 * time.Minute

// NewRawClient 校验配置并返回带常用 Transport 预设的原生 *http.Client。
//
// 与 NewClient() 的关系：
//   - NewClient() 是业务级封装（Response 缓存成 []byte、.R(ctx).Post() 链式 API），
//     适合"POST 一个 JSON 拿一个 JSON"这种同步场景
//   - NewRawClient() 返回原生 *http.Client，保留 .Do(req) 调用契约，适合：
//   - 流式 SSE / WebSocket upgrade（不能预读 body）
//   - 需要自己注入 Transport 做 mock（如 test 中的 http.RoundTripper）
//   - 长耗时请求（thinking 模型 / 视频生成），调用方 ctx 的更短截止时间优先
//
// 默认行为（不传 options）：
//   - 总超时 10 分钟，避免无 deadline 请求无限挂起
//   - ResponseHeaderTimeout 120s（防止服务端连接成功但不发数据的挂死）
//   - 合理的连接池参数（MaxIdleConns=100、IdleConnTimeout=90s）
//
// 典型用法：
//
//	// LLM 流式客户端（thinking 模型可能跑数分钟）
//	c, err := httpx.NewRawClient(httpx.WithResponseHeaderTimeout(120 * time.Second))
//	if err != nil { return err }
//	req, _ := http.NewRequestWithContext(ctx, "POST", url, body)
//	resp, err := c.Do(req)
//
//	// 测试 mock 注入
//	c := httpx.MustNewRawClient(httpx.WithRawTransport(mockTransport))
func NewRawClient(opts ...RawOption) (*http.Client, error) {
	cfg := &rawConfig{
		timeout:               DefaultRawClientTimeout,
		responseHeaderTimeout: 120 * time.Second,
		maxIdleConns:          100,
		maxIdleConnsPerHost:   10,
		idleConnTimeout:       90 * time.Second,
		tlsHandshakeTimeout:   10 * time.Second,
		expectContinueTimeout: 1 * time.Second,
	}
	for index, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("%w: option %d must not be nil", ErrInvalidRawClientConfig, index)
		}
		if err := opt(cfg); err != nil {
			return nil, fmt.Errorf("%w: option %d: %w", ErrInvalidRawClientConfig, index, err)
		}
	}
	if cfg.maxIdleConns > 0 && cfg.maxIdleConnsPerHost > cfg.maxIdleConns {
		return nil, fmt.Errorf(
			"%w: maximum idle connections per host must not exceed the global maximum",
			ErrInvalidRawClientConfig,
		)
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
			ForceAttemptHTTP2:     true,
		}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   cfg.timeout,
	}, nil
}

// MustNewRawClient 创建原生客户端，配置无效时触发 panic。
// 仅应用于编译期已知且经过测试的静态配置。
func MustNewRawClient(opts ...RawOption) *http.Client {
	client, err := NewRawClient(opts...)
	if err != nil {
		panic(err)
	}
	return client
}

// RawOption 配置原生 HTTP 客户端。
type RawOption func(*rawConfig) error

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

// WithRawTimeout 设置整体请求超时。
// 调用方上下文中更短的截止时间仍然优先；长流应显式设置足够长的正值。
func WithRawTimeout(d time.Duration) RawOption {
	return func(c *rawConfig) error {
		if d <= 0 {
			return errors.New("timeout must be positive")
		}
		c.timeout = d
		return nil
	}
}

// WithResponseHeaderTimeout 响应头超时 —— 从建连成功到收到首个响应头的最长时间。
// 对流式请求关键：防止服务端建连后不发数据的挂死。默认 120s。
func WithResponseHeaderTimeout(d time.Duration) RawOption {
	return func(c *rawConfig) error {
		if d < 0 {
			return errors.New("response header timeout must not be negative")
		}
		c.responseHeaderTimeout = d
		return nil
	}
}

// WithMaxIdleConns 连接池总上限，默认 100。
func WithMaxIdleConns(n int) RawOption {
	return func(c *rawConfig) error {
		if n < 0 {
			return errors.New("maximum idle connections must not be negative")
		}
		c.maxIdleConns = n
		return nil
	}
}

// WithMaxIdleConnsPerHost 每主机连接池上限，默认 10。
// LLM/图像场景建议 20+ 避免高并发短暂抖动。
func WithMaxIdleConnsPerHost(n int) RawOption {
	return func(c *rawConfig) error {
		if n < 0 {
			return errors.New("maximum idle connections per host must not be negative")
		}
		c.maxIdleConnsPerHost = n
		return nil
	}
}

// WithIdleConnTimeout 空闲连接存活时间，默认 90s。
func WithIdleConnTimeout(d time.Duration) RawOption {
	return func(c *rawConfig) error {
		if d < 0 {
			return errors.New("idle connection timeout must not be negative")
		}
		c.idleConnTimeout = d
		return nil
	}
}

// WithRawTransport 注入自定义 Transport（测试 mock / 代理场景）。
// 注入后其它 transport 配置项（ResponseHeaderTimeout 等）会被忽略。
func WithRawTransport(t http.RoundTripper) RawOption {
	return func(c *rawConfig) error {
		if isNilRoundTripper(t) {
			return errors.New("transport must not be nil")
		}
		c.customTransport = t
		return nil
	}
}

func isNilRoundTripper(transport http.RoundTripper) bool {
	if transport == nil {
		return true
	}
	value := reflect.ValueOf(transport)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
