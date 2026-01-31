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
		ssrfProtect: false,                // 默认不启用（向后兼容）
		maxBodySize: 100 * 1024 * 1024,    // 默认 100MB
	}

	for _, opt := range opts {
		opt(c)
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
	bodyData []byte        // 缓存的 body 数据，用于重试
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
			time.Sleep(r.client.retryWait)
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

	// 检查是否在白名单中
	for _, allowed := range c.allowedHosts {
		if host == allowed {
			return nil
		}
	}

	// 检查是否为私有/内网 IP
	if isPrivateOrInternalHost(host) {
		return ErrSSRFBlocked
	}

	return nil
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
		// 如果是域名，尝试解析
		ips, err := net.LookupIP(host)
		if err != nil || len(ips) == 0 {
			return false // 无法解析，允许请求（由网络层处理）
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

// Get 发送 GET 请求
func Get(url string) (*Response, error) {
	return NewClient().R().Get(url)
}

// GetWithContext 发送带上下文的 GET 请求
func GetWithContext(ctx context.Context, url string) (*Response, error) {
	return NewClient().R().SetContext(ctx).Get(url)
}

// Post 发送 POST 请求
func Post(url string, body any) (*Response, error) {
	return NewClient().R().SetJSONBody(body).Post(url)
}

// PostWithContext 发送带上下文的 POST 请求
func PostWithContext(ctx context.Context, url string, body any) (*Response, error) {
	return NewClient().R().SetContext(ctx).SetJSONBody(body).Post(url)
}

// PostForm 发送表单 POST 请求
func PostForm(url string, data map[string]string) (*Response, error) {
	return NewClient().R().SetFormBody(data).Post(url)
}

// Put 发送 PUT 请求
func Put(url string, body any) (*Response, error) {
	return NewClient().R().SetJSONBody(body).Put(url)
}

// Delete 发送 DELETE 请求
func Delete(url string) (*Response, error) {
	return NewClient().R().Delete(url)
}
