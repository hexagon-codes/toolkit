package httpx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hexagon-codes/toolkit/util/circuit"
)

func TestRetryPool_RejectsNonReplayableBody(t *testing.T) {
	pool := NewDefaultPool()
	defer pool.Close()
	config := DefaultRetryConfig()
	config.MaxRetries = 1
	retryPool := mustNewRetryPool(t, pool, config)
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPut,
		"http://example.invalid",
		io.NopCloser(strings.NewReader("payload")),
	)
	if err != nil {
		t.Fatal(err)
	}

	response, err := retryPool.Do(req)
	if response != nil {
		_ = response.Body.Close()
	}
	if !errors.Is(err, ErrRequestBodyNotReplayable) {
		t.Fatalf("expected ErrRequestBodyNotReplayable, got %v", err)
	}
}

func TestNewRetryPoolRejectsInvalidConfigurations(t *testing.T) {
	pool := NewDefaultPool()
	defer pool.Close()
	valid := DefaultRetryConfig()
	tests := []struct {
		name   string
		pool   *Pool
		config RetryConfig
	}{
		{name: "nil pool", pool: nil, config: valid},
		{name: "negative retries", pool: pool, config: RetryConfig{MaxRetries: -1, RetryCondition: valid.RetryCondition}},
		{name: "negative wait", pool: pool, config: RetryConfig{RetryWait: -time.Second, RetryCondition: valid.RetryCondition}},
		{name: "maximum wait too short", pool: pool, config: RetryConfig{RetryWait: time.Second, MaxRetryWait: time.Millisecond, RetryCondition: valid.RetryCondition}},
		{name: "nil condition", pool: pool, config: RetryConfig{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewRetryPool(test.pool, test.config); !errors.Is(err, ErrInvalidRetryConfig) {
				t.Fatalf("NewRetryPool() error = %v, want ErrInvalidRetryConfig", err)
			}
		})
	}
}

func TestPool_ResponseTimeIsArithmeticMean(t *testing.T) {
	pool := NewDefaultPool()
	defer pool.Close()

	pool.updateResponseTime(10 * time.Nanosecond)
	pool.updateResponseTime(20 * time.Nanosecond)

	stats := pool.GetStats()
	if stats.AvgResponseTime != 15*time.Nanosecond {
		t.Fatalf("expected 15ns average, got %s", stats.AvgResponseTime)
	}
	if stats.MaxResponseTime != 20*time.Nanosecond {
		t.Fatalf("expected 20ns maximum, got %s", stats.MaxResponseTime)
	}
}

func TestRetryPool_BodyReplay(t *testing.T) {
	// 记录每次请求收到的 body
	var attempt atomic.Int32
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		n := attempt.Add(1)
		if n <= 2 {
			// 前两次返回 500 触发重试
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// 第三次成功
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	pool := NewDefaultPool()
	defer pool.Close()

	retryPool := mustNewRetryPool(t, pool, RetryConfig{
		MaxRetries:   3,
		RetryWait:    time.Millisecond,
		MaxRetryWait: 10 * time.Millisecond,
		RetryCondition: func(resp *http.Response, err error) bool {
			if err != nil {
				return true
			}
			return resp.StatusCode >= 500
		},
	})

	// 创建带 Body 的 POST 请求
	reqBody := `{"key":"value"}`
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, strings.NewReader(reqBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "request-1")

	resp, err := retryPool.Do(req)
	if err != nil {
		t.Fatalf("expected success after retries, got error: %v", err)
	}
	defer resp.Body.Close()

	// 验证每次重试都收到了完整的 body
	if len(bodies) != 3 {
		t.Fatalf("expected 3 attempts, got %d", len(bodies))
	}
	for i, body := range bodies {
		if body != reqBody {
			t.Errorf("attempt %d: expected body %q, got %q", i+1, reqBody, body)
		}
	}
}

func TestRetryPool_NoBody(t *testing.T) {
	var attempt atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempt.Add(1)
		if n <= 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	pool := NewDefaultPool()
	defer pool.Close()

	retryPool := mustNewRetryPool(t, pool, RetryConfig{
		MaxRetries:   2,
		RetryWait:    time.Millisecond,
		MaxRetryWait: 10 * time.Millisecond,
		RetryCondition: func(resp *http.Response, err error) bool {
			if err != nil {
				return true
			}
			return resp.StatusCode >= 500
		},
	})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	resp, err := retryPool.Do(req)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("expected body 'ok', got %q", string(body))
	}
}

func TestRetryPool_AllRetriesFail_NoClosedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error"))
	}))
	defer server.Close()

	pool := NewDefaultPool()
	defer pool.Close()

	retryPool := mustNewRetryPool(t, pool, RetryConfig{
		MaxRetries:   2,
		RetryWait:    time.Millisecond,
		MaxRetryWait: 10 * time.Millisecond,
		RetryCondition: func(resp *http.Response, err error) bool {
			if err != nil {
				return true
			}
			return resp.StatusCode >= 500
		},
	})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	resp, err := retryPool.Do(req)
	// 所有重试失败后，应返回 nil Response 和 error
	if err == nil {
		t.Fatal("expected error after all retries exhausted")
	}
	if resp != nil {
		_ = resp.Body.Close()
		t.Error("expected nil response when all retries fail, got non-nil (would have closed body)")
	}
	if !strings.Contains(err.Error(), "max retries exceeded") {
		t.Errorf("expected 'max retries exceeded' error, got: %v", err)
	}
}

func TestRetryPool_LargeBody(t *testing.T) {
	// 验证大 body 也能正确重放
	largeBody := bytes.Repeat([]byte("A"), 1024*64) // 64KB
	var attempt atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if len(body) != len(largeBody) {
			t.Errorf("attempt %d: body size mismatch: got %d, want %d", attempt.Load()+1, len(body), len(largeBody))
		}
		n := attempt.Add(1)
		if n <= 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pool := NewDefaultPool()
	defer pool.Close()

	retryPool := mustNewRetryPool(t, pool, RetryConfig{
		MaxRetries:   2,
		RetryWait:    time.Millisecond,
		MaxRetryWait: 10 * time.Millisecond,
		RetryCondition: func(resp *http.Response, err error) bool {
			if err != nil {
				return true
			}
			return resp.StatusCode >= 500
		},
	})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, bytes.NewReader(largeBody))
	req.Header.Set("Idempotency-Key", "request-2")
	resp, err := retryPool.Do(req)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	defer resp.Body.Close()
	if got := attempt.Load(); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
}

func mustNewRetryPool(t *testing.T, pool *Pool, config RetryConfig) *RetryPool {
	t.Helper()
	retryPool, err := NewRetryPool(pool, config)
	if err != nil {
		t.Fatalf("NewRetryPool() error = %v", err)
	}
	return retryPool
}

func TestPool_BasicDo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	pool := NewDefaultPool()
	defer pool.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	resp, err := pool.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "hello" {
		t.Errorf("expected 'hello', got %q", string(body))
	}

	stats := pool.GetStats()
	if stats.TotalRequests != 1 {
		t.Errorf("expected 1 total request, got %d", stats.TotalRequests)
	}
}

func TestPool_ClosedPoolReturnsError(t *testing.T) {
	pool := NewDefaultPool()
	pool.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://localhost", nil)
	resp, err := pool.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Error("expected error from closed pool")
	}
}

func TestRateLimitedPoolRejectsInvalidRate(t *testing.T) {
	pool := NewDefaultPool()
	defer pool.Close()
	if _, err := NewRateLimitedPool(pool, 0); err == nil {
		t.Fatal("expected invalid rate error")
	}
}

func TestRateLimitedPool_DoubleCloseNoPanic(t *testing.T) {
	pool := NewDefaultPool()
	rlp, err := NewRateLimitedPool(pool, 5)
	if err != nil {
		t.Fatal(err)
	}

	rlp.Close()
	rlp.Close()
}

func TestCircuitBreakerPoolUsesSharedBreakerStateMachine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	pool := NewDefaultPool()
	breakerPool, err := NewCircuitBreakerPool(
		pool,
		circuit.WithThreshold(1),
		circuit.WithTimeout(time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer breakerPool.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	resp, err := breakerPool.Do(req)
	if err != nil {
		t.Fatalf("first response must be returned unchanged, got %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("unexpected first status %d", resp.StatusCode)
	}

	req, _ = http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	secondResponse, err := breakerPool.Do(req)
	if secondResponse != nil {
		_ = secondResponse.Body.Close()
	}
	if !errors.Is(err, circuit.ErrCircuitOpen) {
		t.Fatalf("expected shared circuit open error, got %v", err)
	}
}

func TestCircuitBreakerPoolDoesNotHideConcurrentClose(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseResponse := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-releaseResponse
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	breakerPool, err := NewCircuitBreakerPool(NewDefaultPool())
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		response, requestErr := breakerPool.Do(request)
		if response != nil {
			_ = response.Body.Close()
		}
		result <- requestErr
	}()
	<-requestStarted
	breakerPool.breaker.Close()
	close(releaseResponse)
	if err := <-result; !errors.Is(err, circuit.ErrBreakerClosed) {
		t.Fatalf("Do() error = %v, want ErrBreakerClosed", err)
	}
	breakerPool.pool.Close()
}

func TestDefaultPoolConfigReturnsIndependentValues(t *testing.T) {
	first := DefaultPoolConfig()
	first.MaxIdleConns = 1
	second := DefaultPoolConfig()
	if first.MaxIdleConns == second.MaxIdleConns {
		t.Fatal("DefaultPoolConfig() returned shared mutable state")
	}
	if got := second.MaxIdleConns; got != 100 {
		t.Fatalf("DefaultPoolConfig().MaxIdleConns = %d, want 100", got)
	}
}
