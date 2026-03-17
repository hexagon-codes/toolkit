// Package httpx 提供增强型 HTTP 客户端
//
// 支持重试、超时、熔断器和请求/响应日志记录。
//
// 基本用法:
//
//	client := httpx.New()
//	resp, err := client.Get(ctx, "https://api.example.com/data")
//
// 带选项:
//
//	client := httpx.New(
//	    httpx.WithTimeout(10*time.Second),
//	    httpx.WithRetry(3),
//	    httpx.WithBaseURL("https://api.example.com"),
//	)
//
// POST JSON:
//
//	resp, err := client.PostJSON(ctx, "/users", map[string]any{
//	    "name": "John",
//	})
//
// --- English ---
//
// Package httpx provides an enhanced HTTP client.
//
// Features retry, timeout, circuit breaker, and request/response logging.
//
// Basic usage:
//
//	client := httpx.New()
//	resp, err := client.Get(ctx, "https://api.example.com/data")
//
// With options:
//
//	client := httpx.New(
//	    httpx.WithTimeout(10*time.Second),
//	    httpx.WithRetry(3),
//	    httpx.WithBaseURL("https://api.example.com"),
//	)
//
// POST with JSON:
//
//	resp, err := client.PostJSON(ctx, "/users", map[string]any{
//	    "name": "John",
//	})
package httpx
