package retry

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HTTPError HTTP 错误，包含状态码和响应
type HTTPError struct {
	StatusCode int
	Status     string
	Response   *http.Response
	Body       []byte // 可选的响应体
}

// Error 实现 error 接口
func (e *HTTPError) Error() string {
	if e.Status != "" {
		return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Status)
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

// NewHTTPError 从响应创建 HTTP 错误
func NewHTTPError(resp *http.Response) *HTTPError {
	if resp == nil {
		return nil
	}
	return &HTTPError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Response:   resp,
	}
}

// NewHTTPErrorWithBody 从响应创建 HTTP 错误（包含响应体）
func NewHTTPErrorWithBody(resp *http.Response, body []byte) *HTTPError {
	err := NewHTTPError(resp)
	if err != nil {
		err.Body = body
	}
	return err
}

// IsRetryableHTTPError 判断是否是可重试的 HTTP 错误
// 可重试的情况：
// - 429 Too Many Requests (速率限制)
// - 500 Internal Server Error
// - 502 Bad Gateway
// - 503 Service Unavailable
// - 504 Gateway Timeout
// - 408 Request Timeout
// - 网络错误（连接超时、DNS 错误等）
func IsRetryableHTTPError(err error) bool {
	if err == nil {
		return false
	}

	// 检查 HTTP 错误
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return isRetryableStatusCode(httpErr.StatusCode)
	}

	// 检查网络错误
	if isNetworkError(err) {
		return true
	}

	return false
}

// isRetryableStatusCode 判断状态码是否可重试
func isRetryableStatusCode(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests: // 429
		return true
	case http.StatusInternalServerError: // 500
		return true
	case http.StatusBadGateway: // 502
		return true
	case http.StatusServiceUnavailable: // 503
		return true
	case http.StatusGatewayTimeout: // 504
		return true
	case http.StatusRequestTimeout: // 408
		return true
	default:
		return false
	}
}

// isNetworkError 判断是否是网络错误
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// 检查网络操作错误
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}

	// 检查 DNS 错误
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	// 检查连接被拒绝等
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	// 检查常见的网络错误信息
	errStr := err.Error()
	networkErrors := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"i/o timeout",
		"network is unreachable",
		"TLS handshake timeout",
		"EOF",
	}
	for _, ne := range networkErrors {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(ne)) {
			return true
		}
	}

	return false
}

// IsRateLimitError 判断是否是速率限制错误 (429)
func IsRateLimitError(err error) bool {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode == http.StatusTooManyRequests
	}
	return false
}

// IsServerError 判断是否是服务器错误 (5xx)
func IsServerError(err error) bool {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= 500 && httpErr.StatusCode < 600
	}
	return false
}

// IsClientError 判断是否是客户端错误 (4xx，通常不应重试)
func IsClientError(err error) bool {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= 400 && httpErr.StatusCode < 500
	}
	return false
}

// GetRetryAfter 从 HTTP 响应获取 Retry-After 时间
// 支持两种格式：
// - 秒数: "120"
// - HTTP 日期: "Wed, 21 Oct 2025 07:28:00 GMT"
func GetRetryAfter(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}

	header := resp.Header.Get("Retry-After")
	if header == "" {
		return 0
	}

	// 尝试解析为秒数
	if seconds, err := strconv.Atoi(header); err == nil {
		return time.Duration(seconds) * time.Second
	}

	// 尝试解析为 HTTP 日期
	if t, err := http.ParseTime(header); err == nil {
		duration := time.Until(t)
		if duration > 0 {
			return duration
		}
	}

	return 0
}

// GetRetryAfterFromError 从错误中获取 Retry-After 时间
func GetRetryAfterFromError(err error) time.Duration {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) && httpErr.Response != nil {
		return GetRetryAfter(httpErr.Response)
	}
	return 0
}

// WithRetryAfterAware 启用 Retry-After 感知
// 当遇到 429 响应时，使用 Retry-After 头指定的等待时间
func WithRetryAfterAware() Option {
	return func(c *Config) {
		c.RetryAfterAware = true
	}
}

// RetryIfHTTP 创建 HTTP 错误重试条件
// 可以指定哪些状态码需要重试
func RetryIfHTTP(statusCodes ...int) func(error) bool {
	codeSet := make(map[int]bool)
	for _, code := range statusCodes {
		codeSet[code] = true
	}

	return func(err error) bool {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) {
			return codeSet[httpErr.StatusCode]
		}
		return false
	}
}

// RetryIfHTTPOrNetwork 创建 HTTP + 网络错误重试条件
func RetryIfHTTPOrNetwork(statusCodes ...int) func(error) bool {
	httpRetry := RetryIfHTTP(statusCodes...)

	return func(err error) bool {
		if httpRetry(err) {
			return true
		}
		return isNetworkError(err)
	}
}
