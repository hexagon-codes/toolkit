package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientRetriesTooManyRequests(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			writer.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := MustNewClient(WithRetry(1, time.Millisecond))
	response, err := client.R(context.Background()).Get(server.URL)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Get() status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
}

func TestNewPoolUsesEnvironmentProxyByDefault(t *testing.T) {
	p := NewDefaultPool()
	defer p.Close()

	if p.Transport().Proxy == nil {
		t.Fatal("NewDefaultPool() transport Proxy = nil, want environment proxy")
	}
	if got, want := reflect.ValueOf(p.Transport().Proxy).Pointer(), reflect.ValueOf(http.ProxyFromEnvironment).Pointer(); got != want {
		t.Fatal("NewDefaultPool() transport Proxy is not http.ProxyFromEnvironment")
	}
}
