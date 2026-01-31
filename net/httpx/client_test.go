package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c := NewClient()
	if c == nil {
		t.Error("NewClient should not return nil")
	}
}

func TestNewClientWithOptions(t *testing.T) {
	c := NewClient(
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

	c := NewClient()
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

	c := NewClient()
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

	c := NewClient(WithBaseURL(server.URL))
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

	c := NewClient()
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

	c := NewClient(WithRetry(3, 10*time.Millisecond))
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

	c := NewClient()
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

	c := NewClient()
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

	c := NewClient()
	_, err := c.R().SetContext(ctx).Get(server.URL)

	if err == nil {
		t.Error("expected error for cancelled context")
	}
}
