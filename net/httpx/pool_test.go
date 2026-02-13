package httpx

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

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

	pool := NewPool()
	defer pool.Close()

	retryPool := NewRetryPool(pool, RetryConfig{
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
	req, err := http.NewRequest("POST", server.URL, strings.NewReader(reqBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

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

	pool := NewPool()
	defer pool.Close()

	retryPool := NewRetryPool(pool, RetryConfig{
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

	req, _ := http.NewRequest("GET", server.URL, nil)
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

	pool := NewPool()
	defer pool.Close()

	retryPool := NewRetryPool(pool, RetryConfig{
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

	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := retryPool.Do(req)
	// 所有重试失败后，应返回 nil Response 和 error
	if err == nil {
		t.Fatal("expected error after all retries exhausted")
	}
	if resp != nil {
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

	pool := NewPool()
	defer pool.Close()

	retryPool := NewRetryPool(pool, RetryConfig{
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

	req, _ := http.NewRequest("POST", server.URL, bytes.NewReader(largeBody))
	resp, err := retryPool.Do(req)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	defer resp.Body.Close()
}

func TestPool_BasicDo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	pool := NewPool()
	defer pool.Close()

	req, _ := http.NewRequest("GET", server.URL, nil)
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
	pool := NewPool()
	pool.Close()

	req, _ := http.NewRequest("GET", "http://localhost", nil)
	_, err := pool.Do(req)
	if err == nil {
		t.Error("expected error from closed pool")
	}
}
