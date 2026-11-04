package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReplayGuardAcceptsPreClonedTransport(t *testing.T) {
	base := &http.Transport{}
	_ = base.Clone() // 模拟调用方先 Clone 再配置 transport 的常见用法。
	if len(base.TLSNextProto) == 0 {
		t.Skip("bundled HTTP/2 handlers are not installed in this environment")
	}

	if _, err := NewReplayGuardTransport(base); err != nil {
		t.Fatalf("NewReplayGuardTransport(pre-cloned base) error = %v", err)
	}
	if _, err := NewReplayGuardTransport(base); err != nil {
		t.Fatalf("NewReplayGuardTransport(same base, second call) error = %v", err)
	}
}

func TestReplayGuardAcceptsUsedTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	base := &http.Transport{}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response, err := base.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	if closeErr := response.Body.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if len(base.TLSNextProto) == 0 {
		t.Skip("bundled HTTP/2 handlers are not installed in this environment")
	}

	if _, err := NewReplayGuardTransport(base); err != nil {
		t.Fatalf("NewReplayGuardTransport(used base) error = %v", err)
	}
}
