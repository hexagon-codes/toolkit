package httpx

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"testing"
	"time"
)

type typedNilRoundTripper struct{}

func (*typedNilRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected round trip")
}

func TestNewClientRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		option Option
	}{
		{name: "nil option", option: nil},
		{name: "negative timeout", option: WithTimeout(-time.Second)},
		{name: "invalid base URL", option: WithBaseURL("://invalid")},
		{name: "empty header name", option: WithHeader("", "value")},
		{name: "unsafe header value", option: WithHeader("X-Test", "value\r\ninjected")},
		{name: "negative retries", option: WithRetry(-1, time.Second)},
		{name: "negative retry wait", option: WithRetry(1, -time.Second)},
		{name: "empty SSRF allow entry", option: WithSSRFProtection("")},
		{name: "SSRF allow entry with empty port", option: WithSSRFProtection("example.com:")},
		{name: "SSRF allow entry with unclosed IPv6 bracket", option: WithSSRFProtection("[::1")},
		{name: "zero body limit", option: WithMaxBodySize(0)},
		{name: "overflowing body limit", option: WithMaxBodySize(math.MaxInt64)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewClient(test.option)
			if client != nil {
				client.CloseIdleConnections()
			}
			if !errors.Is(err, ErrInvalidClientConfig) {
				t.Fatalf("NewClient() error = %v, want ErrInvalidClientConfig", err)
			}
		})
	}
}

func TestClientOptionsPreserveUnderlyingErrors(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("option failed")
	client, err := NewClient(func(*Client) error { return sentinel })
	if client != nil {
		client.CloseIdleConnections()
		t.Fatalf("NewClient() client = %#v, want nil", client)
	}
	if !errors.Is(err, ErrInvalidClientConfig) || !errors.Is(err, sentinel) {
		t.Fatalf("NewClient() error = %v, want configuration and option errors", err)
	}

	rawClient, err := NewRawClient(func(*rawConfig) error { return sentinel })
	if rawClient != nil {
		rawClient.CloseIdleConnections()
		t.Fatalf("NewRawClient() client = %#v, want nil", rawClient)
	}
	if !errors.Is(err, ErrInvalidRawClientConfig) || !errors.Is(err, sentinel) {
		t.Fatalf("NewRawClient() error = %v, want configuration and option errors", err)
	}
}

func TestClientRejectsNilContext(t *testing.T) {
	client := MustNewClient(WithRetry(1, time.Millisecond))
	defer client.CloseIdleConnections()

	//nolint:staticcheck // 需要验证公开 API 对 nil context 的错误合同。
	response, err := client.R().SetContext(nil).Get("http://example.invalid")
	if response != nil {
		t.Fatalf("Get() response = %#v, want nil", response)
	}
	if !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("Get() error = %v, want ErrInvalidContext", err)
	}
}

func TestNewRawClientRejectsInvalidConfiguration(t *testing.T) {
	var nilTransport *typedNilRoundTripper
	tests := []struct {
		name   string
		option RawOption
	}{
		{name: "nil option", option: nil},
		{name: "negative timeout", option: WithRawTimeout(-time.Second)},
		{name: "negative response header timeout", option: WithResponseHeaderTimeout(-time.Second)},
		{name: "negative maximum idle connections", option: WithMaxIdleConns(-1)},
		{name: "negative per-host idle connections", option: WithMaxIdleConnsPerHost(-1)},
		{name: "negative idle timeout", option: WithIdleConnTimeout(-time.Second)},
		{name: "nil transport", option: WithRawTransport(nil)},
		{name: "typed nil transport", option: WithRawTransport(nilTransport)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewRawClient(test.option)
			if client != nil {
				client.CloseIdleConnections()
			}
			if !errors.Is(err, ErrInvalidRawClientConfig) {
				t.Fatalf("NewRawClient() error = %v, want ErrInvalidRawClientConfig", err)
			}
		})
	}
}

func TestNewRawClientRejectsInconsistentConnectionLimits(t *testing.T) {
	client, err := NewRawClient(
		WithMaxIdleConns(4),
		WithMaxIdleConnsPerHost(5),
	)
	if client != nil {
		client.CloseIdleConnections()
		t.Fatalf("NewRawClient() client = %#v, want nil", client)
	}
	if !errors.Is(err, ErrInvalidRawClientConfig) {
		t.Fatalf("NewRawClient() error = %v, want ErrInvalidRawClientConfig", err)
	}
}

func TestNewPoolRejectsInvalidConfiguration(t *testing.T) {
	valid := DefaultPoolConfig()
	tests := []struct {
		name   string
		mutate func(*PoolConfig)
	}{
		{name: "negative maximum idle connections", mutate: func(config *PoolConfig) { config.MaxIdleConns = -1 }},
		{name: "negative maximum host connections", mutate: func(config *PoolConfig) { config.MaxConnsPerHost = -1 }},
		{name: "negative per-host idle connections", mutate: func(config *PoolConfig) { config.MaxIdleConnsPerHost = -1 }},
		{name: "per-host idle exceeds global", mutate: func(config *PoolConfig) { config.MaxIdleConnsPerHost = config.MaxIdleConns + 1 }},
		{name: "per-host idle exceeds host maximum", mutate: func(config *PoolConfig) { config.MaxIdleConnsPerHost = config.MaxConnsPerHost + 1 }},
		{name: "negative idle timeout", mutate: func(config *PoolConfig) { config.IdleConnTimeout = -time.Second }},
		{name: "negative connect timeout", mutate: func(config *PoolConfig) { config.ConnectTimeout = -time.Second }},
		{name: "negative response header timeout", mutate: func(config *PoolConfig) { config.ResponseHeaderTimeout = -time.Second }},
		{name: "negative TLS handshake timeout", mutate: func(config *PoolConfig) { config.TLSHandshakeTimeout = -time.Second }},
		{name: "negative expect-continue timeout", mutate: func(config *PoolConfig) { config.ExpectContinueTimeout = -time.Second }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			test.mutate(&config)
			pool, err := NewPool(config)
			if pool != nil {
				pool.Close()
			}
			if !errors.Is(err, ErrInvalidPoolConfig) {
				t.Fatalf("NewPool() error = %v, want ErrInvalidPoolConfig", err)
			}
		})
	}
}

func TestNewPoolOwnsTLSConfiguration(t *testing.T) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	config := DefaultPoolConfig()
	config.TLSConfig = tlsConfig
	pool, err := NewPool(config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	tlsConfig.MinVersion = tls.VersionTLS13
	if got := pool.Transport().TLSClientConfig.MinVersion; got != tls.VersionTLS12 {
		t.Fatalf("pool TLS minimum version = %d, want %d", got, tls.VersionTLS12)
	}
}

func TestDefaultTransportsAttemptHTTP2(t *testing.T) {
	pool := NewDefaultPool()
	defer pool.Close()
	if !pool.Transport().ForceAttemptHTTP2 {
		t.Fatal("Pool transport ForceAttemptHTTP2 = false, want true")
	}

	rawClient := MustNewRawClient()
	rawTransport, ok := rawClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("NewRawClient() transport = %T, want *http.Transport", rawClient.Transport)
	}
	defer rawTransport.CloseIdleConnections()
	if !rawTransport.ForceAttemptHTTP2 {
		t.Fatal("RawClient transport ForceAttemptHTTP2 = false, want true")
	}
}

func TestClientOwnsTransportLifecycle(t *testing.T) {
	first := MustNewClient()
	defer first.CloseIdleConnections()
	second := MustNewClient()
	defer second.CloseIdleConnections()

	if first.client.Transport == nil || first.client.Transport == http.DefaultTransport {
		t.Fatalf("first client transport = %#v, want an owned transport", first.client.Transport)
	}
	if second.client.Transport == nil || second.client.Transport == http.DefaultTransport {
		t.Fatalf("second client transport = %#v, want an owned transport", second.client.Transport)
	}
	if first.client.Transport == second.client.Transport {
		t.Fatal("independent clients unexpectedly share a transport")
	}

	ssrfClient := MustNewClient(WithSSRFProtection("example.com"))
	defer ssrfClient.CloseIdleConnections()
	if _, ok := ssrfClient.client.Transport.(interface{ CloseIdleConnections() }); !ok {
		t.Fatalf("SSRF transport %T does not expose CloseIdleConnections", ssrfClient.client.Transport)
	}
}

func TestPoolWrappersRejectNilRequest(t *testing.T) {
	pool := NewDefaultPool()
	defer pool.Close()

	retryPool, err := NewRetryPool(pool, DefaultRetryConfig())
	if err != nil {
		t.Fatal(err)
	}
	ratePool, err := NewRateLimitedPool(pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	breakerPool, err := NewCircuitBreakerPool(pool)
	if err != nil {
		t.Fatal(err)
	}
	defer breakerPool.breaker.Close()

	tests := []struct {
		name string
		do   func() (*http.Response, error)
	}{
		{name: "pool", do: func() (*http.Response, error) { return pool.Do(nil) }},
		{name: "retry pool", do: func() (*http.Response, error) { return retryPool.Do(nil) }},
		{name: "rate limited pool", do: func() (*http.Response, error) { return ratePool.Do(nil) }},
		{name: "circuit breaker pool", do: func() (*http.Response, error) { return breakerPool.Do(nil) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, doErr := test.do()
			if response != nil {
				_ = response.Body.Close()
			}
			if !errors.Is(doErr, ErrInvalidRequest) {
				t.Fatalf("Do(nil) error = %v, want ErrInvalidRequest", doErr)
			}
		})
	}
}

func TestPoolDoWithContextRejectsNilInputs(t *testing.T) {
	pool := NewDefaultPool()
	defer pool.Close()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.invalid", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	//nolint:staticcheck // 需要验证公开 API 对 nil context 的错误合同。
	response, callErr := pool.DoWithContext(nil, request)
	if response != nil {
		if response.Body != nil {
			_ = response.Body.Close()
		}
		t.Fatalf("DoWithContext(nil, request) response = %#v, want nil", response)
	}
	if !errors.Is(callErr, ErrInvalidContext) {
		t.Fatalf("DoWithContext(nil, request) error = %v, want ErrInvalidContext", callErr)
	}
	response, callErr = pool.DoWithContext(context.Background(), nil)
	if response != nil {
		if response.Body != nil {
			_ = response.Body.Close()
		}
		t.Fatalf("DoWithContext(context, nil) response = %#v, want nil", response)
	}
	if !errors.Is(callErr, ErrInvalidRequest) {
		t.Fatalf("DoWithContext(context, nil) error = %v, want ErrInvalidRequest", callErr)
	}
}

func TestPoolRejectsNonHTTPURL(t *testing.T) {
	pool := NewDefaultPool()
	defer pool.Close()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "ftp://example.com/file", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	response, err := pool.Do(request)
	if response != nil {
		_ = response.Body.Close()
		t.Fatalf("Pool.Do() response = %#v, want nil", response)
	}
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Pool.Do() error = %v, want ErrInvalidRequest", err)
	}
}

func TestPoolWrappersRejectClosedPool(t *testing.T) {
	pool := NewDefaultPool()
	pool.Close()

	retryPool, retryErr := NewRetryPool(pool, DefaultRetryConfig())
	if retryPool != nil {
		t.Fatalf("NewRetryPool() pool = %#v, want nil", retryPool)
	}
	if !errors.Is(retryErr, ErrPoolClosed) {
		t.Fatalf("NewRetryPool() error = %v, want ErrPoolClosed", retryErr)
	}

	ratePool, rateErr := NewRateLimitedPool(pool, 1)
	if ratePool != nil {
		t.Fatalf("NewRateLimitedPool() pool = %#v, want nil", ratePool)
	}
	if !errors.Is(rateErr, ErrPoolClosed) {
		t.Fatalf("NewRateLimitedPool() error = %v, want ErrPoolClosed", rateErr)
	}

	breakerPool, breakerErr := NewCircuitBreakerPool(pool)
	if breakerPool != nil {
		breakerPool.Close()
		t.Fatalf("NewCircuitBreakerPool() pool = %#v, want nil", breakerPool)
	}
	if !errors.Is(breakerErr, ErrPoolClosed) {
		t.Fatalf("NewCircuitBreakerPool() error = %v, want ErrPoolClosed", breakerErr)
	}
}

func TestPoolWrappersFailFastWhenUnderlyingPoolCloses(t *testing.T) {
	pool := NewDefaultPool()
	retryPool, err := NewRetryPool(pool, RetryConfig{
		MaxRetries:   1,
		RetryWait:    time.Hour,
		MaxRetryWait: time.Hour,
		RetryCondition: func(*http.Response, error) bool {
			return true
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ratePool, err := NewRateLimitedPool(pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		do   func(*http.Request) (*http.Response, error)
	}{
		{name: "retry pool", do: retryPool.Do},
		{name: "rate limited pool", do: ratePool.Do},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, doErr := test.do(request.Clone(ctx))
			if response != nil {
				_ = response.Body.Close()
				t.Fatalf("Do() response = %#v, want nil", response)
			}
			if !errors.Is(doErr, ErrPoolClosed) {
				t.Fatalf("Do() error = %v, want ErrPoolClosed", doErr)
			}
			if errors.Is(doErr, context.Canceled) {
				t.Fatalf("Do() error = %v, wrapper did not fail before waiting", doErr)
			}
		})
	}
}

func TestIsTimeoutRecognizesWrappedNetworkError(t *testing.T) {
	timeoutErr := &net.DNSError{Err: "timed out", IsTimeout: true}
	if !isTimeout(fmt.Errorf("request failed: %w", timeoutErr)) {
		t.Fatal("isTimeout() = false for a wrapped timeout error")
	}
}
