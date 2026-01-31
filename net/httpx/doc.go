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
