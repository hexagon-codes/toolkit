package httpx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestRetryPoolPreservesAttemptErrorWhenBodyRestoreFails(t *testing.T) {
	attemptErr := errors.New("attempt failed")
	restoreErr := errors.New("body restore failed")
	pool := NewDefaultPool()
	defer pool.Close()
	pool.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, attemptErr
	})

	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPut,
		"https://example.com/resource",
		io.NopCloser(strings.NewReader("payload")),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.GetBody = func() (io.ReadCloser, error) {
		return nil, restoreErr
	}

	retryPool := mustNewRetryPool(t, pool, RetryConfig{
		MaxRetries:   1,
		RetryWait:    0,
		MaxRetryWait: 0,
		RetryCondition: func(*http.Response, error) bool {
			return true
		},
	})
	response, err := retryPool.Do(request)
	if response != nil {
		_ = response.Body.Close()
		t.Fatalf("RetryPool.Do() response = %#v, want nil", response)
	}
	if !errors.Is(err, attemptErr) || !errors.Is(err, restoreErr) {
		t.Fatalf("RetryPool.Do() error = %v, want both attempt and restore errors", err)
	}
}

func TestRetryPoolPreservesAttemptErrorWhenContextCancelsBackoff(t *testing.T) {
	attemptErr := errors.New("attempt failed")
	ctx, cancel := context.WithCancel(context.Background())
	pool := NewDefaultPool()
	defer pool.Close()
	pool.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		cancel()
		return nil, attemptErr
	})

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com/resource", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	retryPool := mustNewRetryPool(t, pool, RetryConfig{
		MaxRetries:   1,
		RetryWait:    time.Hour,
		MaxRetryWait: time.Hour,
		RetryCondition: func(*http.Response, error) bool {
			return true
		},
	})
	response, err := retryPool.Do(request)
	if response != nil {
		_ = response.Body.Close()
		t.Fatalf("RetryPool.Do() response = %#v, want nil", response)
	}
	if !errors.Is(err, context.Canceled) || !errors.Is(err, attemptErr) {
		t.Fatalf("RetryPool.Do() error = %v, want both context cancellation and attempt error", err)
	}
}

func TestClientPreservesAttemptErrorWhenContextCancelsBackoff(t *testing.T) {
	attemptErr := errors.New("attempt failed")
	ctx, cancel := context.WithCancel(context.Background())
	client := MustNewClient(WithRetry(1, time.Hour))
	defer client.CloseIdleConnections()
	client.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		cancel()
		return nil, attemptErr
	})

	response, err := client.R(ctx).Get("https://example.com/resource")
	if response != nil {
		t.Fatalf("Client.Get() response = %#v, want nil", response)
	}
	if !errors.Is(err, context.Canceled) || !errors.Is(err, attemptErr) {
		t.Fatalf("Client.Get() error = %v, want both context cancellation and attempt error", err)
	}
}
