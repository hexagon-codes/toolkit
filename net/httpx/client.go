package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/netip"
	"net/textproto"
	neturl "net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	// ErrSSRFBlocked SSRF 防护拦截错误
	// 当请求目标为私有/内网 IP 时返回此错误
	ErrSSRFBlocked = errors.New("httpx: request blocked by SSRF protection (private/internal IP)")
	// ErrResponseBodyTooLarge 表示响应体超过客户端配置的上限。
	ErrResponseBodyTooLarge = errors.New("httpx: response body exceeds configured limit")
	// ErrRequestBodyNotReplayable 表示启用重试时请求体无法安全重放。
	ErrRequestBodyNotReplayable = errors.New("httpx: request body is not replayable")
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

// Option 客户端配置选项。
type Option func(*Client) error

// NewClient 校验配置并创建 HTTP 客户端。
func NewClient(opts ...Option) (*Client, error) {
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("%w: default transport cannot be cloned", ErrInvalidClientConfig)
	}
	transport := defaultTransport.Clone()
	c := &Client{
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		headers:     make(map[string]string),
		timeout:     30 * time.Second,
		retries:     0,
		retryWait:   time.Second,
		ssrfProtect: false,             // 默认不启用（向后兼容）
		maxBodySize: 100 * 1024 * 1024, // 默认 100MB
	}

	for index, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("%w: option %d must not be nil", ErrInvalidClientConfig, index)
		}
		if err := opt(c); err != nil {
			return nil, fmt.Errorf("%w: option %d: %w", ErrInvalidClientConfig, index, err)
		}
	}

	// 如果启用了 SSRF 防护，使用自定义 Transport 在连接时检查 IP
	// 这可以防止 DNS Rebinding 攻击
	if c.ssrfProtect {
		c.client.Transport = newSSRFSafeTransport(
			transport,
			c.allowedHosts,
		)
	}

	return c, nil
}

// MustNewClient 创建 HTTP 客户端，配置无效时触发 panic。
// 仅应用于编译期已知且经过测试的静态配置。
func MustNewClient(opts ...Option) *Client {
	client, err := NewClient(opts...)
	if err != nil {
		panic(err)
	}
	return client
}

// WithTimeout 设置超时时间
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) error {
		if timeout <= 0 {
			return errors.New("timeout must be positive")
		}
		c.timeout = timeout
		c.client.Timeout = timeout
		return nil
	}
}

// WithBaseURL 设置基础 URL
func WithBaseURL(baseURL string) Option {
	return func(c *Client) error {
		normalized, err := normalizeBaseURL(baseURL)
		if err != nil {
			return err
		}
		c.baseURL = normalized
		return nil
	}
}

// WithHeader 设置默认请求头
func WithHeader(key, value string) Option {
	return func(c *Client) error {
		if err := validateHeader(key, value); err != nil {
			return err
		}
		c.headers[key] = value
		return nil
	}
}

// WithHeaders 设置多个默认请求头
func WithHeaders(headers map[string]string) Option {
	return func(c *Client) error {
		for key, value := range headers {
			if err := validateHeader(key, value); err != nil {
				return err
			}
		}
		for k, v := range headers {
			c.headers[k] = v
		}
		return nil
	}
}

// WithRetry 设置重试次数
func WithRetry(retries int, wait time.Duration) Option {
	return func(c *Client) error {
		if retries < 0 {
			return errors.New("retry count must not be negative")
		}
		if wait < 0 {
			return errors.New("retry wait must not be negative")
		}
		c.retries = retries
		c.retryWait = wait
		return nil
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
	return func(c *Client) error {
		for _, allowedHost := range allowedHosts {
			if err := validateAllowedHost(allowedHost); err != nil {
				return err
			}
		}
		c.ssrfProtect = true
		c.allowedHosts = append([]string(nil), allowedHosts...)
		return nil
	}
}

// WithMaxBodySize 设置最大响应体大小（默认 100MB）
func WithMaxBodySize(size int64) Option {
	return func(c *Client) error {
		if size <= 0 || size == math.MaxInt64 {
			return errors.New("maximum response body size must be positive and finite")
		}
		c.maxBodySize = size
		return nil
	}
}

func normalizeBaseURL(baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", nil
	}
	parsed, err := neturl.Parse(baseURL)
	if err != nil || parsed.Host == "" ||
		(!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return "", errors.New("base URL must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("base URL must not contain user information or a fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = strings.TrimRight(parsed.RawPath, "/")
	return parsed.String(), nil
}

func resolveRequestURL(baseURL, target string, query neturl.Values) (string, error) {
	if strings.TrimSpace(target) != target {
		return "", fmt.Errorf("%w: URL must not contain surrounding whitespace", ErrInvalidRequest)
	}
	targetURL, err := neturl.Parse(target)
	if err != nil {
		return "", fmt.Errorf("%w: parse URL: %w", ErrInvalidRequest, err)
	}

	var resolved *neturl.URL
	if targetURL.IsAbs() {
		resolved = targetURL
	} else {
		if targetURL.Scheme != "" || targetURL.Host != "" {
			return "", fmt.Errorf("%w: relative URL must not contain a scheme or host", ErrInvalidRequest)
		}
		if baseURL == "" {
			return "", fmt.Errorf("%w: URL must be absolute when no base URL is configured", ErrInvalidRequest)
		}
		base, parseErr := neturl.Parse(baseURL)
		if parseErr != nil {
			return "", fmt.Errorf("%w: parse base URL: %w", ErrInvalidRequest, parseErr)
		}
		baseCopy := *base
		resolved = &baseCopy
		if targetPath := targetURL.EscapedPath(); targetPath != "" {
			joined, joinErr := neturl.JoinPath(baseURL, targetPath)
			if joinErr != nil {
				return "", fmt.Errorf("%w: join URL path: %w", ErrInvalidRequest, joinErr)
			}
			resolved, parseErr = neturl.Parse(joined)
			if parseErr != nil {
				return "", fmt.Errorf("%w: parse joined URL: %w", ErrInvalidRequest, parseErr)
			}
		}
		mergeURLQuery(resolved, targetURL.Query())
		resolved.Fragment = targetURL.Fragment
	}

	if resolved.User != nil {
		return "", fmt.Errorf("%w: URL must not contain user information", ErrInvalidRequest)
	}
	if resolved.Host == "" ||
		(!strings.EqualFold(resolved.Scheme, "http") && !strings.EqualFold(resolved.Scheme, "https")) {
		return "", fmt.Errorf("%w: URL must be absolute HTTP or HTTPS", ErrInvalidRequest)
	}
	mergeURLQuery(resolved, query)
	return resolved.String(), nil
}

func mergeURLQuery(target *neturl.URL, values neturl.Values) {
	if len(values) == 0 {
		return
	}
	merged := target.Query()
	for key, entries := range values {
		merged.Del(key)
		for _, entry := range entries {
			merged.Add(key, entry)
		}
	}
	target.RawQuery = merged.Encode()
}

func validateHeader(key, value string) error {
	if key == "" || textproto.CanonicalMIMEHeaderKey(key) == "" {
		return errors.New("header name is invalid")
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character < ' ' && character != '\t') || character == 0x7f {
			return fmt.Errorf("header %q contains an invalid value", key)
		}
	}
	return nil
}

func validateAllowedHost(entry string) error {
	if entry == "" || strings.TrimSpace(entry) != entry || strings.ContainsAny(entry, "/?#") || strings.Contains(entry, "://") {
		return fmt.Errorf("SSRF allow-list entry %q is invalid", entry)
	}
	switch {
	case strings.HasPrefix(entry, "["):
		closingBracket := strings.IndexByte(entry, ']')
		if closingBracket < 0 || net.ParseIP(entry[1:closingBracket]) == nil {
			return fmt.Errorf("SSRF allow-list entry %q is invalid", entry)
		}
		remainder := entry[closingBracket+1:]
		if remainder != "" && (!strings.HasPrefix(remainder, ":") || len(remainder) == 1) {
			return fmt.Errorf("SSRF allow-list entry %q is invalid", entry)
		}
	case strings.ContainsAny(entry, "[]"):
		return fmt.Errorf("SSRF allow-list entry %q is invalid", entry)
	case strings.Count(entry, ":") > 1 && net.ParseIP(entry) == nil:
		return fmt.Errorf("SSRF allow-list entry %q is invalid", entry)
	case strings.Count(entry, ":") == 1 && strings.HasSuffix(entry, ":"):
		return fmt.Errorf("SSRF allow-list entry %q has an invalid port", entry)
	}
	host, port := splitHostPort(entry)
	host = strings.TrimPrefix(host, "*.")
	if host == "" || strings.Contains(host, "*") {
		return fmt.Errorf("SSRF allow-list entry %q has no host", entry)
	}
	if port == "" {
		return nil
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("SSRF allow-list entry %q has an invalid port", entry)
	}
	return nil
}

// Request 表示一个 HTTP 请求
type Request struct {
	client      *Client
	method      string
	url         string
	headers     map[string]string
	query       neturl.Values
	body        io.Reader
	bodyFactory func() io.Reader
	bodyErr     error
	ctx         context.Context
	jsonErr     error // JSON 编码错误
}

// R 使用调用方上下文创建新请求。
// ctx 必须非 nil；调用方设置的更短截止时间优先于客户端总超时。
func (c *Client) R(ctx context.Context) *Request {
	return &Request{
		client:  c,
		headers: make(map[string]string),
		query:   make(neturl.Values),
		ctx:     ctx,
	}
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

// SetBody 设置请求体。
// 启用重试时，只接受 bytes.Buffer、bytes.Reader 或 strings.Reader，避免为了重放
// 无界读取任意流。其他 Reader 应改用 SetBodyBytes，或关闭客户端重试。
func (r *Request) SetBody(body io.Reader) *Request {
	r.jsonErr = nil
	r.body = body
	r.bodyFactory = nil
	r.bodyErr = nil
	if r.client.retries <= 0 || body == nil {
		return r
	}

	switch body := body.(type) {
	case *bytes.Buffer:
		data := bytes.Clone(body.Bytes())
		r.bodyFactory = func() io.Reader { return bytes.NewReader(data) }
	case *bytes.Reader:
		snapshot := *body
		data := make([]byte, snapshot.Len())
		readBytes, readErr := snapshot.Read(data)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			r.bodyErr = fmt.Errorf("%w: snapshot bytes reader: %w", ErrRequestBodyNotReplayable, readErr)
			return r
		}
		if readBytes != len(data) {
			r.bodyErr = fmt.Errorf("%w: snapshot bytes reader was incomplete", ErrRequestBodyNotReplayable)
			return r
		}
		r.bodyFactory = func() io.Reader { return bytes.NewReader(data) }
	case *strings.Reader:
		snapshot := *body
		r.bodyFactory = func() io.Reader {
			clone := snapshot
			return &clone
		}
	default:
		r.bodyErr = fmt.Errorf("%w: use SetBodyBytes or a standard in-memory reader", ErrRequestBodyNotReplayable)
	}
	if r.bodyFactory != nil {
		r.body = r.bodyFactory()
	}
	return r
}

// SetBodyBytes 设置字节数组作为请求体（推荐用于需要重试的请求）
func (r *Request) SetBodyBytes(data []byte) *Request {
	r.jsonErr = nil
	data = bytes.Clone(data)
	r.bodyFactory = func() io.Reader { return bytes.NewReader(data) }
	r.body = r.bodyFactory()
	r.bodyErr = nil
	return r
}

// SetJSONBody 设置 JSON 请求体
// 如果 JSON 编码失败，会设置 jsonErr 错误，在执行请求时返回
func (r *Request) SetJSONBody(v any) *Request {
	data, err := json.Marshal(v)
	if err != nil {
		r.jsonErr = err
		r.body = nil
		r.bodyFactory = nil
		r.bodyErr = nil
		return r
	}
	r.jsonErr = nil
	r.bodyFactory = func() io.Reader { return bytes.NewReader(data) }
	r.body = r.bodyFactory()
	r.bodyErr = nil
	r.headers["Content-Type"] = "application/json"
	return r
}

// SetFormBody 设置表单请求体
func (r *Request) SetFormBody(data map[string]string) *Request {
	r.jsonErr = nil
	form := neturl.Values{}
	for k, v := range data {
		form.Set(k, v)
	}
	encoded := form.Encode()
	r.bodyFactory = func() io.Reader { return strings.NewReader(encoded) }
	r.body = r.bodyFactory()
	r.bodyErr = nil
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
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("%w: request client is missing", ErrInvalidRequest)
	}
	if r.ctx == nil {
		return nil, ErrInvalidContext
	}
	if r.method == "" {
		return nil, fmt.Errorf("%w: method is missing", ErrInvalidRequest)
	}
	if r.url == "" && r.client.baseURL == "" {
		return nil, fmt.Errorf("%w: URL is missing", ErrInvalidRequest)
	}
	// 检查 JSON 编码错误
	if r.jsonErr != nil {
		return nil, r.jsonErr
	}

	maxRetries := 0
	if r.client.retries > 0 && r.isRetrySafe() {
		maxRetries = r.client.retries
		if r.bodyErr != nil {
			return nil, r.bodyErr
		}
	}

	fullURL, err := resolveRequestURL(r.client.baseURL, r.url, r.query)
	if err != nil {
		return nil, err
	}

	var resp *Response
	var lastAttemptErr error
	err = nil

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if r.bodyFactory != nil {
			r.body = r.bodyFactory()
		}
		if attempt > 0 {
			// 等待重试间隔，同时监听 context 取消
			timer := time.NewTimer(r.client.retryWait)
			select {
			case <-timer.C:
				// 继续重试
			case <-r.ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return nil, errors.Join(
					lastAttemptErr,
					fmt.Errorf("httpx: retry backoff interrupted: %w", r.ctx.Err()),
				)
			}
		}

		resp, err = r.doRequest(fullURL)
		if err == nil && resp.StatusCode < 500 {
			break
		}
		lastAttemptErr = err
		if lastAttemptErr == nil && resp != nil {
			lastAttemptErr = fmt.Errorf("retryable HTTP status: %s", resp.Status)
		}
		// 注意：Response.Body 是 []byte，已在 doRequest 中读取并关闭了原始 http.Response.Body
		// 所以这里不需要额外关闭操作
	}

	return resp, err
}

func (r *Request) isRetrySafe() bool {
	headers := make(http.Header, len(r.client.headers)+len(r.headers))
	for key, value := range r.client.headers {
		headers.Set(key, value)
	}
	for key, value := range r.headers {
		headers.Set(key, value)
	}
	return isHTTPRetrySafe(r.method, headers)
}

func isHTTPRetrySafe(method string, headers http.Header) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return strings.TrimSpace(headers.Get("Idempotency-Key")) != ""
	}
}

// doRequest 发送单次请求
func (r *Request) doRequest(fullURL string) (*Response, error) {
	// SSRF 防护检查
	if r.client.ssrfProtect {
		if err := r.client.checkSSRF(r.ctx, fullURL); err != nil {
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
	limitedReader := io.LimitReader(httpResp.Body, r.client.maxBodySize+1)
	body, readErr := io.ReadAll(limitedReader)
	closeErr := httpResp.Body.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, err
	}
	if int64(len(body)) > r.client.maxBodySize {
		return nil, fmt.Errorf("%w: limit is %d bytes", ErrResponseBodyTooLarge, r.client.maxBodySize)
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
func (c *Client) checkSSRF(ctx context.Context, rawURL string) error {
	return validateSSRFTarget(ctx, rawURL, c.allowedHosts)
}

// validateSSRFTarget 校验一次请求的真实目标；代理和重定向请求同样必须经过此入口。
func validateSSRFTarget(ctx context.Context, rawURL string, allowedHosts []string) error {
	u, err := neturl.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: invalid URL: %w", ErrSSRFBlocked, err)
	}

	// 只允许 http 和 https
	if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("%w: unsupported URL scheme", ErrSSRFBlocked)
	}

	host := u.Hostname()
	port := u.Port()
	if host == "" {
		return fmt.Errorf("%w: URL host is empty", ErrSSRFBlocked)
	}

	// 检查是否在白名单中（支持通配符和端口）
	if isHostInAllowedList(host, port, allowedHosts) {
		return nil
	}

	if isInternalHostname(host) {
		return fmt.Errorf("%w: internal host %q is not allowed", ErrSSRFBlocked, host)
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedSSRFIP(ip) {
			return fmt.Errorf("%w: address %q is not publicly routable", ErrSSRFBlocked, host)
		}
		return nil
	}

	if ctx == nil {
		ctx = context.Background()
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
	if err != nil {
		return fmt.Errorf("%w: DNS lookup for %q failed: %w", ErrSSRFBlocked, host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("%w: DNS lookup for %q returned no addresses", ErrSSRFBlocked, host)
	}
	for _, address := range ips {
		if isBlockedSSRFIP(address.IP) {
			return fmt.Errorf("%w: host %q resolves to a non-public address", ErrSSRFBlocked, host)
		}
	}

	return nil
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

// isInternalHostname 检查显式内部主机名。
func isInternalHostname(host string) bool {
	lowerHost := strings.ToLower(host)
	return lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".local") ||
		lowerHost == "metadata.google.internal" || // GCP 元数据服务
		strings.HasSuffix(lowerHost, ".internal")
}

// isBlockedSSRFIP 判断地址是否属于非公网、共享或云元数据地址。
func isBlockedSSRFIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsLoopback() || address.IsPrivate() ||
		address.IsLinkLocalUnicast() || address.IsMulticast() || isCloudMetadataIP(ip) {
		return true
	}
	for _, prefix := range blockedSSRFSpecialPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

var blockedSSRFSpecialPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
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
	transport    *http.Transport
	allowedHosts []string
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

		// ControlContext 在 TCP connect 前校验拨号器最终选中的 IP，消除 DNS
		// 二次解析带来的检查与使用时间窗，同时保留标准库的双栈拨号策略。
		return newSSRFSafeDialer().DialContext(ctx, network, addr)
	}

	return &ssrfSafeTransport{
		transport:    transport,
		allowedHosts: append([]string(nil), allowedHosts...),
	}
}

// newSSRFSafeDialer 创建在实际 connect 前校验目标 IP 的拨号器。
func newSSRFSafeDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		ControlContext: func(_ context.Context, _, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("%w: invalid dial address %q: %w", ErrSSRFBlocked, address, err)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("%w: dial address %q is not an IP address", ErrSSRFBlocked, address)
			}
			if isBlockedSSRFIP(ip) {
				return fmt.Errorf("%w: dial address %q is not publicly routable", ErrSSRFBlocked, address)
			}
			return nil
		},
	}
}

func (t *ssrfSafeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("%w: request URL is missing", ErrSSRFBlocked)
	}
	if err := validateSSRFTarget(req.Context(), req.URL.String(), t.allowedHosts); err != nil {
		return nil, err
	}
	return t.transport.RoundTrip(req)
}

// CloseIdleConnections 释放包装 Transport 持有的空闲连接。
func (t *ssrfSafeTransport) CloseIdleConnections() {
	if t == nil || t.transport == nil {
		return
	}
	t.transport.CloseIdleConnections()
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

// CloseIdleConnections 关闭底层客户端当前持有的空闲连接。
func (c *Client) CloseIdleConnections() {
	if c == nil || c.client == nil {
		return
	}
	c.client.CloseIdleConnections()
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
		defaultClient = MustNewClient()
	})
	return defaultClient
}

// Get 使用调用方上下文发送 GET 请求。
func Get(ctx context.Context, url string) (*Response, error) {
	return getDefaultClient().R(ctx).Get(url)
}

// Post 使用调用方上下文发送 JSON POST 请求。
func Post(ctx context.Context, url string, body any) (*Response, error) {
	return getDefaultClient().R(ctx).SetJSONBody(body).Post(url)
}

// PostForm 使用调用方上下文发送表单 POST 请求。
func PostForm(ctx context.Context, url string, data map[string]string) (*Response, error) {
	return getDefaultClient().R(ctx).SetFormBody(data).Post(url)
}

// Put 使用调用方上下文发送 JSON PUT 请求。
func Put(ctx context.Context, url string, body any) (*Response, error) {
	return getDefaultClient().R(ctx).SetJSONBody(body).Put(url)
}

// Delete 使用调用方上下文发送 DELETE 请求。
func Delete(ctx context.Context, url string) (*Response, error) {
	return getDefaultClient().R(ctx).Delete(url)
}
