package httpx

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientDoesNotRetryNonIdempotentRequestWithoutKey(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		attempts.Add(1)
		_, _ = io.Copy(io.Discard, request.Body)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	response, err := MustNewClient(WithRetry(3, time.Millisecond)).R().
		SetBody(ioReaderOnly{Reader: strings.NewReader("payload")}).
		Post(server.URL)
	if err != nil {
		t.Fatalf("Post() error = %v, want nil", err)
	}
	if response == nil || response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("Post() response = %#v, want status 500", response)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("Post() attempts = %d, want 1", got)
	}
}

func TestRetryPoolDoesNotRetryNonIdempotentRequestWithoutKey(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		attempts.Add(1)
		_, _ = io.Copy(io.Discard, request.Body)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	pool := NewDefaultPool()
	defer pool.Close()
	retryPool := mustNewRetryPool(t, pool, RetryConfig{
		MaxRetries:   3,
		RetryWait:    time.Millisecond,
		MaxRetryWait: 10 * time.Millisecond,
		RetryCondition: func(response *http.Response, err error) bool {
			return err != nil || response.StatusCode >= http.StatusInternalServerError
		},
	})
	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		server.URL,
		io.NopCloser(strings.NewReader("payload")),
	)
	if err != nil {
		t.Fatal(err)
	}

	response, err := retryPool.Do(request)
	if err != nil {
		t.Fatalf("RetryPool.Do() error = %v, want nil", err)
	}
	if response == nil || response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("RetryPool.Do() response = %#v, want status 500", response)
	}
	defer response.Body.Close()
	if got := attempts.Load(); got != 1 {
		t.Fatalf("RetryPool.Do() attempts = %d, want 1", got)
	}
}

func TestStreamAllowsSingleUseBodyWhenClientRetryIsConfigured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("ReadAll() error = %v", err)
		}
		if string(body) != "payload" {
			t.Errorf("request body = %q, want payload", body)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("data: ok\n\n"))
	}))
	defer server.Close()

	stream, err := MustNewClient(WithRetry(3, time.Millisecond)).R().
		SetBody(ioReaderOnly{Reader: strings.NewReader("payload")}).
		PostStream(server.URL)
	if err != nil {
		t.Fatalf("PostStream() error = %v", err)
	}
	defer stream.Close()
	event, err := stream.ReadSSE()
	if err != nil {
		t.Fatalf("ReadSSE() error = %v", err)
	}
	if event.Data != "ok" {
		t.Fatalf("event data = %q, want ok", event.Data)
	}
}
