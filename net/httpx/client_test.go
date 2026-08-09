package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c := MustNewClient()
	if c == nil {
		t.Error("NewClient should not return nil")
	}
}

func TestNewClientWithOptions(t *testing.T) {
	c := MustNewClient(
		WithTimeout(5*time.Second),
		WithBaseURL("https://example.com"),
		WithHeader("X-Custom", "value"),
		WithHeaders(map[string]string{"X-Another": "value2"}),
		WithRetry(3, time.Second),
	)

	if c.timeout != 5*time.Second {
		t.Error("Timeout not set correctly")
	}

	if c.baseURL != "https://example.com" {
		t.Error("BaseURL not set correctly")
	}

	if c.headers["X-Custom"] != "value" {
		t.Error("Header not set correctly")
	}

	if c.headers["X-Another"] != "value2" {
		t.Error("Headers not set correctly")
	}

	if c.retries != 3 {
		t.Error("Retries not set correctly")
	}
}

func TestRequest_ResponseBodyLimitIsEnforced(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("12345"))
	}))
	defer server.Close()

	_, err := MustNewClient(WithMaxBodySize(4)).R().Get(server.URL)
	if !errors.Is(err, ErrResponseBodyTooLarge) {
		t.Fatalf("expected ErrResponseBodyTooLarge, got %v", err)
	}
}

func TestRequest_ResponseBodyAtLimitSucceeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("1234"))
	}))
	defer server.Close()

	resp, err := MustNewClient(WithMaxBodySize(4)).R().Get(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(resp.Body) != "1234" {
		t.Fatalf("unexpected response body %q", resp.Body)
	}
}

func TestRequest_SetBodyRejectsNonReplayableReaderWhenRetrying(t *testing.T) {
	client := MustNewClient(WithRetry(1, time.Millisecond))
	reader := ioReaderOnly{Reader: strings.NewReader("payload")}

	_, err := client.R().SetBody(reader).Put("http://example.invalid")
	if !errors.Is(err, ErrRequestBodyNotReplayable) {
		t.Fatalf("expected ErrRequestBodyNotReplayable, got %v", err)
	}
}

func TestRequest_SetBodyReplaysKnownReader(t *testing.T) {
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := new(bytes.Buffer)
		_, _ = data.ReadFrom(r.Body)
		bodies = append(bodies, data.String())
		if len(bodies) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := MustNewClient(WithRetry(1, time.Millisecond)).R().
		SetHeader("Idempotency-Key", "request-1").
		SetBody(strings.NewReader("payload")).
		Post(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bodies) != 2 || bodies[0] != "payload" || bodies[1] != "payload" {
		t.Fatalf("unexpected replayed bodies: %#v", bodies)
	}
}

func TestRequest_SetBodySnapshotsMutableReaders(t *testing.T) {
	tests := []struct {
		name   string
		reader func() (io.Reader, func())
	}{
		{
			name: "bytes buffer",
			reader: func() (io.Reader, func()) {
				buffer := bytes.NewBufferString("original")
				return buffer, func() { buffer.Bytes()[0] = 'X' }
			},
		},
		{
			name: "bytes reader",
			reader: func() (io.Reader, func()) {
				data := []byte("original")
				return bytes.NewReader(data), func() { data[0] = 'X' }
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader, mutate := test.reader()
			request := MustNewClient(WithRetry(1, time.Millisecond)).R().SetBody(reader)
			mutate()

			var replay bytes.Buffer
			if _, err := replay.ReadFrom(request.bodyFactory()); err != nil {
				t.Fatal(err)
			}
			if replay.String() != "original" {
				t.Fatalf("replayed body = %q, want original", replay.String())
			}
		})
	}
}

func TestSSRFSafeTransportValidatesTargetWhenUsingProxy(t *testing.T) {
	proxyReached := false
	proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		proxyReached = true
		writer.WriteHeader(http.StatusOK)
	}))
	defer proxy.Close()
	proxyURL, err := neturl.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}

	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	baseTransport.Proxy = http.ProxyURL(proxyURL)
	client := &http.Client{
		Transport: newSSRFSafeTransport(baseTransport, []string{proxyURL.Host}),
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:1/private", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if response != nil {
		_ = response.Body.Close()
	}
	if !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("client.Get() error = %v, want ErrSSRFBlocked", err)
	}
	if proxyReached {
		t.Fatal("proxy received a request for a blocked private target")
	}
}

func TestSSRFProtectionBlocksSpecialPurposeAddresses(t *testing.T) {
	blocked := []string{
		"0.0.0.1",
		"192.0.2.1",
		"198.18.0.1",
		"198.51.100.1",
		"203.0.113.1",
		"240.0.0.1",
		"64:ff9b::7f00:1",
		"2001:db8::1",
		"2002:7f00:1::",
	}
	for _, address := range blocked {
		t.Run("blocked_"+address, func(t *testing.T) {
			if !isBlockedSSRFIP(net.ParseIP(address)) {
				t.Fatalf("isBlockedSSRFIP(%q) = false, want true", address)
			}
		})
	}

	for _, address := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		t.Run("public_"+address, func(t *testing.T) {
			if isBlockedSSRFIP(net.ParseIP(address)) {
				t.Fatalf("isBlockedSSRFIP(%q) = true, want false", address)
			}
		})
	}
}

type ioReaderOnly struct {
	*strings.Reader
}

func TestGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	resp, err := Get(server.URL)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	if resp.String() != "OK" {
		t.Errorf("expected OK, got %s", resp.String())
	}
}

func TestGetWithContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := GetWithContext(ctx, server.URL)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestPost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected JSON content type")
		}

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)

		if body["name"] != "test" {
			t.Error("body not parsed correctly")
		}

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	resp, err := Post(server.URL, map[string]string{"name": "test"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
}

func TestPostForm(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Error("expected form content type")
		}

		r.ParseForm()
		if r.Form.Get("name") != "test" {
			t.Error("form not parsed correctly")
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resp, err := PostForm(server.URL, map[string]string{"name": "test"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !resp.IsSuccess() {
		t.Error("expected success")
	}
}

func TestPut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resp, err := Put(server.URL, map[string]string{"name": "test"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestDelete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	resp, err := Delete(server.URL)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}

func TestRequestWithQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != "test" {
			t.Error("query param not received")
		}
		if r.URL.Query().Get("page") != "1" {
			t.Error("query param not received")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := MustNewClient()
	resp, err := c.R().
		SetQuery("name", "test").
		SetQueries(map[string]string{"page": "1"}).
		Get(server.URL)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRequestWithHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "value" {
			t.Error("custom header not received")
		}
		if r.Header.Get("X-Another") != "value2" {
			t.Error("another header not received")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := MustNewClient()
	resp, err := c.R().
		SetHeader("X-Custom", "value").
		SetHeaders(map[string]string{"X-Another": "value2"}).
		Get(server.URL)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestResponseJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"name": "test"})
	}))
	defer server.Close()

	resp, err := Get(server.URL)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	var result map[string]string
	err = resp.JSON(&result)
	if err != nil {
		t.Errorf("JSON decode error: %v", err)
	}

	if result["name"] != "test" {
		t.Error("JSON not parsed correctly")
	}
}

func TestResponseIsSuccess(t *testing.T) {
	tests := []struct {
		code      int
		isSuccess bool
	}{
		{200, true},
		{201, true},
		{204, true},
		{299, true},
		{300, false},
		{400, false},
		{500, false},
	}

	for _, tt := range tests {
		resp := &Response{StatusCode: tt.code}
		if resp.IsSuccess() != tt.isSuccess {
			t.Errorf("IsSuccess for %d = %v, want %v", tt.code, resp.IsSuccess(), tt.isSuccess)
		}
	}
}

func TestResponseIsError(t *testing.T) {
	tests := []struct {
		code    int
		isError bool
	}{
		{200, false},
		{399, false},
		{400, true},
		{404, true},
		{500, true},
	}

	for _, tt := range tests {
		resp := &Response{StatusCode: tt.code}
		if resp.IsError() != tt.isError {
			t.Errorf("IsError for %d = %v, want %v", tt.code, resp.IsError(), tt.isError)
		}
	}
}

func TestBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users" {
			t.Errorf("expected /api/users, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := MustNewClient(WithBaseURL(server.URL))
	resp, err := c.R().Get("/api/users")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestQueryWithExistingQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("a") != "1" {
			t.Error("existing query param not preserved")
		}
		if r.URL.Query().Get("b") != "2" {
			t.Error("new query param not added")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := MustNewClient()
	_, err := c.R().
		SetQuery("b", "2").
		Get(server.URL + "?a=1")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := MustNewClient(WithRetry(3, 10*time.Millisecond))
	resp, err := c.R().Get(server.URL)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after retries, got %d", resp.StatusCode)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestPatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := MustNewClient()
	resp, err := c.R().Patch(server.URL)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("expected HEAD, got %s", r.Method)
		}
		w.Header().Set("X-Custom", "value")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := MustNewClient()
	resp, err := c.R().Head(server.URL)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if resp.Headers.Get("X-Custom") != "value" {
		t.Error("Header not received")
	}
}

func TestContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	c := MustNewClient()
	_, err := c.R().SetContext(ctx).Get(server.URL)

	if err == nil {
		t.Error("expected error for canceled context")
	}
}

func TestRequestBodyReplacementClearsPreviousJSONError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	request := MustNewClient().R().SetJSONBody(func() {})
	if _, err := request.Post(server.URL); err == nil {
		t.Fatal("Post() error = nil after an unsupported JSON value")
	}
	response, err := request.SetJSONBody(map[string]string{"status": "ok"}).Post(server.URL)
	if err != nil {
		t.Fatalf("Post() error after body replacement = %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Post() status = %d", response.StatusCode)
	}
}
