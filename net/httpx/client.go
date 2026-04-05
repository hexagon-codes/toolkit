package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	// ErrSSRFBlocked SSRF 防护拦截错误
	// 当请求目标为私有/内网 IP 时返回此错误
	ErrSSRFBlocked = errors.New("httpx: request blocked by SSRF protection (private/internal IP)")
)

// Client HTTP 客户端封装
type Client struct {
	client       *http.Client
	baseURL      string
	headers      map[string]string
	timeout      time.Duration
	retries      int
	retryWait    time.Duration
	ssrfProtect  bool     // SSRF 防护开关
	allowedHosts []string // SSRF 防护：允许的主机白名单（为空则检查所有）
	maxBodySize  int64    // 最大响应体大小
}

// Option 客户端配置选项
type Option func(*Client)

// NewClient 创建新的 HTTP 客户端
func NewClient(opts ...Option) *Client {
	c := &Client{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		headers:     make(map[string]string),
		timeout:     30 * time.Second,
		retries:     0,
		retryWait:   time.Second,
		ssrfProtect: false,             // 默认不启用（向后兼容）
		maxBodySize: 100 * 1024 * 1024, // 默认 100MB
	}

	for _, opt := range opts {
		opt(c)
	}

	// 如果启用了 SSRF 防护，使用自定义 Transport 在连接时检查 IP
	// 这可以防止 DNS Rebinding 攻击
	if c.ssrfProtect && c.client.Transport == nil {
		c.client.Transport = newSSRFSafeTransport(
			http.DefaultTransport.(*http.Transport),
			c.allowedHosts,
		)
	}

	return c
}

// WithTimeout 设置超时时间
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.timeout = timeout
		c.client.Timeout = timeout
	}
}

// WithBaseURL 设置基础 URL
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(baseURL, "/")
	}
}

// WithHeader 设置默认请求头
func WithHeader(key, value string) Option {
	return func(c *Client) {
		c.headers[key] = value
	}
}

// WithHeaders 设置多个默认请求头
func WithHeaders(headers map[string]string) Option {
	return func(c *Client) {
		for k, v := range headers {
			c.headers[k] = v
		}
	}
}

// WithRetry 设置重试次数
func WithRetry(retries int, wait time.Duration) Option {
	return func(c *Client) {
		c.retries = retries
		c.retryWait = wait
	}
}

// WithTransport 设置自定义 Transport
func WithTransport(transport http.RoundTripper) Option {
	return func(c *Client) {
		c.client.Transport = transport
	}
}

// WithSSRFProtection 启用 SSRF 防护
//
// 启用后会阻止对私有/内网 IP 地址的请求，包括：
//   - 回环地址：127.0.0.0/8
//   - 私有地址：10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
//   - 链路本地地址：169.254.0.0/16
//   - 云服务商元数据服务：169.254.169.254 等
//
// 参数 allowedHosts 为可选的主机白名单，白名单中的主机不受限制。
func WithSSRFProtection(allowedHosts ...string) Option {
	return func(c *Client) {
		c.ssrfProtect = true
		c.allowedHosts = allowedHosts
	}
}

// WithMaxBodySize 设置最大响应体大小（默认 100MB）
func WithMaxBodySize(size int64) Option {
	return func(c *Client) {
		c.maxBodySize = size
	}
}

// Request 表示一个 HTTP 请求
type Request struct {
	client   *Client
	method   string
	url      string
	headers  map[string]string
	query    url.Values
	body     io.Reader
	bodyData []byte // 缓存的 body 数据，用于重试
	ctx      context.Context
	jsonErr  error // JSON 编码错误
}

// R 创建新请求
func (c *Client) R() *Request {
	return &Request{
		client:  c,
		headers: make(map[string]string),
		query:   make(url.Values),
		ctx:     context.Background(),
	}
}

// SetContext 设置请求上下文
func (r *Request) SetContext(ctx context.Context) *Request {
	r.ctx = ctx
	return r
}

// SetHeader 设置请求头
func (r *Request) SetHeader(key, value string) *Request {
	r.headers[key] = value
	return r
}

// SetHeaders 设置多个请求头
func (r *Request) SetHeaders(headers map[string]string) *Request {
	for k, v := range headers {
		r.headers[k] = v
	}
	return r
}

// SetQuery 设置查询参数
func (r *Request) SetQuery(key, value string) *Request {
	r.query.Set(key, value)
	return r
}

// SetQueries 设置多个查询参数
func (r *Request) SetQueries(params map[string]string) *Request {
	for k, v := range params {
		r.query.Set(k, v)
	}
	return r
}

// SetBody 设置请求体
// 注意：如果启用了重试（WithRetry），会将 body 内容全部读取并缓存到内存中。
// 对于大文件上传，请考虑以下方案：
//   - 禁用重试（不使用 WithRetry）
//   - 使用 SetBodyBytes 预先读取数据
//   - 实现应用层重试逻辑
func (r *Request) SetBody(body io.Reader) *Request {
	r.body = body
	// 如果有重试且 body 不为 nil，需要缓存 body 内容
	if r.client.retries > 0 && body != nil {
		data, err := io.ReadAll(body)
		if err == nil {
			r.bodyData = data
			r.body = bytes.NewReader(data)
		}
	}
	return r
}

// SetBodyBytes 设置字节数组作为请求体（推荐用于需要重试的请求）
func (r *Request) SetBodyBytes(data []byte) *Request {
	r.bodyData = data
	r.body = bytes.NewReader(data)
	return r
}

// SetJSONBody 设置 JSON 请求体
// 如果 JSON 编码失败，会设置 jsonErr 错误，在执行请求时返回
func (r *Request) SetJSONBody(v any) *Request {
	data, err := json.Marshal(v)
	if err != nil {
		r.jsonErr = err
		return r
	}
	r.bodyData = data
	r.body = bytes.NewReader(data)
	r.headers["Content-Type"] = "application/json"
	return r
}

// SetFormBody 设置表单请求体
func (r *Request) SetFormBody(data map[string]string) *Request {
	form := url.Values{}
	for k, v := range data {
		form.Set(k, v)
	}
	encoded := form.Encode()
	r.bodyData = []byte(encoded)
	r.body = strings.NewReader(encoded)
	r.headers["Content-Type"] = "application/x-www-form-urlencoded"
	return r
}

// Get 发送 GET 请求
func (r *Request) Get(url string) (*Response, error) {
	r.method = http.MethodGet
	r.url = url
	return r.execute()
}

// Post 发送 POST 请求
func (r *Request) Post(url string) (*Response, error) {
	r.method = http.MethodPost
	r.url = url
	return r.execute()
}

// Put 发送 PUT 请求
func (r *Request) Put(url string) (*Response, error) {
	r.method = http.MethodPut
	r.url = url
	return r.execute()
}

// Delete 发送 DELETE 请求
func (r *Request) Delete(url string) (*Response, error) {
	r.method = http.MethodDelete
	r.url = url
	return r.execute()
}

// Patch 发送 PATCH 请求
func (r *Request) Patch(url string) (*Response, error) {
	r.method = http.MethodPatch
	r.url = url
	return r.execute()
}

// Head 发送 HEAD 请求
func (r *Request) Head(url string) (*Response, error) {
	r.method = http.MethodHead
	r.url = url
	return r.execute()
}

// execute 执行请求
func (r *Request) execute() (*Response, error) {
	// 检查 JSON 编码错误
	if r.jsonErr != nil {
		return nil, r.jsonErr
	}

	fullURL := r.url
	if r.client.baseURL != "" && !strings.HasPrefix(r.url, "http") {
		fullURL = r.client.baseURL + "/" + strings.TrimLeft(r.url, "/")
	}

	if len(r.query) > 0 {
		if strings.Contains(fullURL, "?") {
			fullURL += "&" + r.query.Encode()
		} else {
			fullURL += "?" + r.query.Encode()
		}
	}

	var resp *Response
	var err error

	for attempt := 0; attempt <= r.client.retries; attempt++ {
		if attempt > 0 {
			// 等待重试间隔，同时监听 context 取消
			select {
			case <-time.After(r.client.retryWait):
				// 继续重试
			case <-r.ctx.Done():
				return nil, r.ctx.Err()
			}
			// 重试时重置 body reader
			if r.bodyData != nil {
				r.body = bytes.NewReader(r.bodyData)
			}
		}

		resp, err = r.doRequest(fullURL)
		if err == nil && resp.StatusCode < 500 {
			break
		}
		// 注意：Response.Body 是 []byte，已在 doRequest 中读取并关闭了原始 http.Response.Body
		// 所以这里不需要额外关闭操作
	}

	return resp, err
}

// doRequest 发送单次请求
func (r *Request) doRequest(fullURL string) (*Response, error) {
	// SSRF 防护检查
	if r.client.ssrfProtect {
		if err := r.client.checkSSRF(fullURL); err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequestWithContext(r.ctx, r.method, fullURL, r.body)
	if err != nil {
		return nil, err
	}

	// 设置默认请求头
	for k, v := range r.client.headers {
		req.Header.Set(k, v)
	}

	// 设置请求特定的请求头（覆盖默认）
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}

	httpResp, err := r.client.client.Do(req)
	if err != nil {
		return nil, err
	}

	// 限制响应体大小，防止内存溢出攻击
	limitedReader := io.LimitReader(httpResp.Body, r.client.maxBodySize)
	body, err := io.ReadAll(limitedReader)
	httpResp.Body.Close()
	if err != nil {
		return nil, err
	}

	return &Response{
		StatusCode: httpResp.StatusCode,
		Status:     httpResp.Status,
		Headers:    httpResp.Header,
		Body:       body,
	}, nil
}

// checkSSRF 检查 URL 是否存在 SSRF 风险
// 返回 ErrSSRFBlocked 表示请求被拦截
func (c *Client) checkSSRF(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	// 只允许 http 和 https
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrSSRFBlocked
	}

	host := u.Hostname()
	port := u.Port()

	// 检查是否在白名单中（支持通配符和端口）
	if c.isHostAllowed(host, port) {
		return nil
	}

	// 检查是否为私有/内网 IP
	if isPrivateOrInternalHost(host) {
		return ErrSSRFBlocked
	}

	return nil
}

// isHostAllowed 检查主机是否在白名单中
func (c *Client) isHostAllowed(host, port string) bool {
	return isHostInAllowedList(host, port, c.allowedHosts)
}

// isHostInAllowedList 检查主机是否在允许列表中
// 支持以下格式:
//   - "example.com" - 精确匹配主机名（任意端口）
//   - "example.com:8080" - 精确匹配主机名和端口
//   - "*.example.com" - 通配符匹配子域名
//   - "*.example.com:443" - 通配符匹配子域名和指定端口
//   - "::1" - IPv6 地址精确匹配
//   - "[::1]:8080" - IPv6 地址带端口匹配
func isHostInAllowedList(host, port string, allowedHosts []string) bool {
	// 规范化主机名（大小写不敏感）
	lowerHost := strings.ToLower(host)

	for _, allowed := range allowedHosts {
		allowed = strings.ToLower(allowed)

		allowedHost, allowedPort := splitHostPort(allowed)

		// 如果白名单指定了端口，必须匹配
		if allowedPort != "" && allowedPort != port {
			continue
		}

		// 检查通配符匹配
		if strings.HasPrefix(allowedHost, "*.") {
			// 通配符模式：*.example.com 匹配 foo.example.com, bar.example.com
			suffix := allowedHost[1:] // ".example.com"
			if strings.HasSuffix(lowerHost, suffix) && lowerHost != suffix[1:] {
				return true
			}
		} else if allowedHost == lowerHost {
			// 精确匹配
			return true
		}
	}
	return false
}

// splitHostPort 分离白名单条目中的主机和端口
// 正确处理 IPv6 地址：
//   - "example.com" -> ("example.com", "")
//   - "example.com:8080" -> ("example.com", "8080")
//   - "::1" -> ("::1", "")
//   - "[::1]:8080" -> ("::1", "8080")
//   - "2001:db8::1" -> ("2001:db8::1", "")
func splitHostPort(hostport string) (host, port string) {
	// 标准 [IPv6]:port 格式
	if strings.HasPrefix(hostport, "[") {
		if idx := strings.LastIndex(hostport, "]"); idx != -1 {
			host = hostport[1:idx]
			// "]" 后面可能有 ":port"
			rest := hostport[idx+1:]
			if strings.HasPrefix(rest, ":") {
				port = rest[1:]
			}
			return host, port
		}
	}

	// 如果包含多个冒号，视为纯 IPv6 地址（无端口）
	if strings.Count(hostport, ":") > 1 {
		return hostport, ""
	}

	// 普通 host 或 host:port
	if idx := strings.LastIndex(hostport, ":"); idx != -1 {
		return hostport[:idx], hostport[idx+1:]
	}
	return hostport, ""
}

// isPrivateOrInternalHost 检查主机是否为私有或内部地址
func isPrivateOrInternalHost(host string) bool {
	// 检查特殊主机名
	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".local") ||
		lowerHost == "metadata.google.internal" || // GCP 元数据服务
		strings.HasSuffix(lowerHost, ".internal") {
		return true
	}

	// 解析 IP 地址
	ip := net.ParseIP(host)
	if ip == nil {
		// 如果是域名，使用带超时的 DNS 查询
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		resolver := &net.Resolver{}
		ips, err := resolver.LookupIP(ctx, "ip", host)
		if err != nil || len(ips) == 0 {
			// DNS 解析失败时，出于安全考虑应阻止请求
			// 防止攻击者利用 DNS 解析失败绕过 SSRF 防护
			// 如果确实需要访问该域名，应将其加入白名单
			return true
		}
		// 检查所有解析到的 IP 地址
		for _, resolvedIP := range ips {
			if resolvedIP.IsLoopback() || resolvedIP.IsPrivate() || resolvedIP.IsLinkLocalUnicast() ||
				resolvedIP.IsLinkLocalMulticast() || isCloudMetadataIP(resolvedIP) {
				return true
			}
		}
		ip = ips[0]
	}

	// 检查是否为私有/保留地址
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || isCloudMetadataIP(ip)
}

// isCloudMetadataIP 检查是否为云服务商元数据服务 IP
func isCloudMetadataIP(ip net.IP) bool {
	// AWS/Azure/GCP 元数据服务地址
	metadataIPs := []string{
		"169.254.169.254", // AWS, Azure, GCP
		"169.254.170.2",   // AWS ECS
		"fd00:ec2::254",   // AWS IPv6
	}
	for _, metaIP := range metadataIPs {
		if ip.Equal(net.ParseIP(metaIP)) {
			return true
		}
	}
	return false
}

// ssrfSafeTransport 防止 DNS Rebinding 攻击的 Transport
// 在初始化时设置 DialContext，连接时检查解析后的 IP 地址
// DialContext 不捕获请求级状态，可安全复用 Transport 连接池
type ssrfSafeTransport struct {
	transport *http.Transport
}

// newSSRFSafeTransport 创建 SSRF 安全的 Transport
// 在初始化时一次性设置 DialContext，避免每次 RoundTrip 克隆 Transport
func newSSRFSafeTransport(base *http.Transport, allowedHosts []string) *ssrfSafeTransport {
	// 克隆一次，设置好 DialContext 后持续复用
	transport := base.Clone()
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		// 检查是否在白名单中（支持通配符和端口）
		if isHostInAllowedList(host, port, allowedHosts) {
			// 白名单主机，使用默认 Dialer
			return (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext(ctx, network, addr)
		}

		// 使用带超时的 context 进行 DNS 查询
		lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		// 解析 IP 地址
		ips, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
		if err != nil {
			return nil, err
		}

		if len(ips) == 0 {
			return nil, errors.New("httpx: no IP addresses found for host")
		}

		// 检查所有解析的 IP 是否安全
		for _, ipAddr := range ips {
			ip := ipAddr.IP
			if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
				ip.IsLinkLocalMulticast() || isCloudMetadataIP(ip) {
				return nil, ErrSSRFBlocked
			}
		}

		// 建立连接（使用原始地址保持 TLS SNI 正确）
		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		conn, err := dialer.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}

		// 连接建立后验证实际连接的 IP（防止 DNS Rebinding）
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			remoteAddr := tcpConn.RemoteAddr()
			if tcpAddr, ok := remoteAddr.(*net.TCPAddr); ok {
				ip := tcpAddr.IP
				if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
					ip.IsLinkLocalMulticast() || isCloudMetadataIP(ip) {
					conn.Close()
					return nil, ErrSSRFBlocked
				}
			}
		}

		return conn, nil
	}

	return &ssrfSafeTransport{transport: transport}
}

func (t *ssrfSafeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.transport.RoundTrip(req)
}

// Response HTTP 响应
type Response struct {
	StatusCode int
	Status     string
	Headers    http.Header
	Body       []byte
}

// String 返回响应体字符串
func (r *Response) String() string {
	return string(r.Body)
}

// JSON 解析 JSON 响应体
func (r *Response) JSON(v any) error {
	return json.Unmarshal(r.Body, v)
}

// IsSuccess 判断是否成功（2xx）
func (r *Response) IsSuccess() bool {
	return r.StatusCode >= 200 && r.StatusCode < 300
}

// IsError 判断是否错误（4xx 或 5xx）
func (r *Response) IsError() bool {
	return r.StatusCode >= 400
}

// 便捷方法

// 包级单例 Client，避免每次创建新实例
var (
	defaultClient     *Client
	defaultClientOnce sync.Once
)

// getDefaultClient 获取包级单例 Client
func getDefaultClient() *Client {
	defaultClientOnce.Do(func() {
		defaultClient = NewClient()
	})
	return defaultClient
}

// Get 发送 GET 请求
func Get(url string) (*Response, error) {
	return getDefaultClient().R().Get(url)
}

// GetWithContext 发送带上下文的 GET 请求
func GetWithContext(ctx context.Context, url string) (*Response, error) {
	return getDefaultClient().R().SetContext(ctx).Get(url)
}

// Post 发送 POST 请求
func Post(url string, body any) (*Response, error) {
	return getDefaultClient().R().SetJSONBody(body).Post(url)
}

// PostWithContext 发送带上下文的 POST 请求
func PostWithContext(ctx context.Context, url string, body any) (*Response, error) {
	return getDefaultClient().R().SetContext(ctx).SetJSONBody(body).Post(url)
}

// PostForm 发送表单 POST 请求
func PostForm(url string, data map[string]string) (*Response, error) {
	return getDefaultClient().R().SetFormBody(data).Post(url)
}

// Put 发送 PUT 请求
func Put(url string, body any) (*Response, error) {
	return getDefaultClient().R().SetJSONBody(body).Put(url)
}

// Delete 发送 DELETE 请求
func Delete(url string) (*Response, error) {
	return getDefaultClient().R().Delete(url)
}
