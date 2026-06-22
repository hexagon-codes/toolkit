package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockRT 实现 http.RoundTripper，用于验证 WithRawTransport 注入
type mockRT struct {
	called int
	resp   *http.Response
	err    error
}

func (m *mockRT) RoundTrip(r *http.Request) (*http.Response, error) {
	m.called++
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func TestRawClient_DefaultsAreSane(t *testing.T) {
	c := RawClient()
	if c == nil {
		t.Fatal("RawClient returned nil")
	}
	if c.Timeout != 0 {
		t.Errorf("default Timeout should be 0 (ctx-controlled), got %v", c.Timeout)
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("default Transport should be *http.Transport, got %T", c.Transport)
	}
	if tr.ResponseHeaderTimeout != 120*time.Second {
		t.Errorf("ResponseHeaderTimeout want 120s, got %v", tr.ResponseHeaderTimeout)
	}
	if tr.MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns want 100, got %d", tr.MaxIdleConns)
	}
	if tr.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout want 90s, got %v", tr.IdleConnTimeout)
	}
	// 必须像 net/http.DefaultTransport 一样遵循 HTTP(S)_PROXY 环境变量，否则在用代理
	// 上网的宿主机上，基于 RawClient 的客户端（如 browser skill）会绕过代理无法访问。
	if tr.Proxy == nil {
		t.Error("default transport must set Proxy (ProxyFromEnvironment) to honor HTTP(S)_PROXY")
	}
}

func TestRawClient_WithResponseHeaderTimeout(t *testing.T) {
	c := RawClient(WithResponseHeaderTimeout(5 * time.Second))
	tr := c.Transport.(*http.Transport)
	if tr.ResponseHeaderTimeout != 5*time.Second {
		t.Errorf("want 5s, got %v", tr.ResponseHeaderTimeout)
	}
}

func TestRawClient_WithRawTimeout(t *testing.T) {
	c := RawClient(WithRawTimeout(10 * time.Second))
	if c.Timeout != 10*time.Second {
		t.Errorf("want 10s, got %v", c.Timeout)
	}
}

func TestRawClient_WithRawTransport_OverridesEverything(t *testing.T) {
	m := &mockRT{resp: &http.Response{StatusCode: 200, Body: http.NoBody}}
	c := RawClient(
		WithRawTransport(m),
		// 即使指定了 ResponseHeaderTimeout，也应被 mock transport 覆盖
		WithResponseHeaderTimeout(1*time.Second),
	)
	if c.Transport != m {
		t.Error("custom Transport should override defaults")
	}
	// 验证 mock 被调用
	resp, err := c.Get("http://example.com")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if m.called != 1 {
		t.Errorf("mock transport should be called once, got %d", m.called)
	}
}

// TestRawClient_ContractIsNativeHttpClient 验证 RawClient 返回的是标准 *http.Client，
// 保持与所有 .Do(req) 调用点、测试 mock 注入等生态兼容
func TestRawClient_ContractIsNativeHttpClient(t *testing.T) {
	c := RawClient()
	// 类型断言本身就是契约：必须是 *http.Client
	var _ *http.Client = c

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	req, err := http.NewRequest("GET", ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Do(req) // ← 关键契约：支持 .Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestRawClient_WithMaxIdleConns(t *testing.T) {
	c := RawClient(WithMaxIdleConns(50), WithMaxIdleConnsPerHost(25), WithIdleConnTimeout(30*time.Second))
	tr := c.Transport.(*http.Transport)
	if tr.MaxIdleConns != 50 {
		t.Errorf("MaxIdleConns want 50, got %d", tr.MaxIdleConns)
	}
	if tr.MaxIdleConnsPerHost != 25 {
		t.Errorf("MaxIdleConnsPerHost want 25, got %d", tr.MaxIdleConnsPerHost)
	}
	if tr.IdleConnTimeout != 30*time.Second {
		t.Errorf("IdleConnTimeout want 30s, got %v", tr.IdleConnTimeout)
	}
}
