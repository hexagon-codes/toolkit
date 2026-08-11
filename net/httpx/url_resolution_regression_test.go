package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestURLResolutionPreservesBasePathAndQueryPrecedence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/users" {
			t.Errorf("request path = %q, want /api/v1/users", request.URL.Path)
		}
		query := request.URL.Query()
		for key, want := range map[string]string{
			"tenant":   "base",
			"request":  "one",
			"extra":    "two",
			"override": "builder",
		} {
			if got := query.Get(key); got != want {
				t.Errorf("query %q = %q, want %q", key, got, want)
			}
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := MustNewClient(WithBaseURL(server.URL + "/api/v1?tenant=base&override=base"))
	defer client.CloseIdleConnections()
	response, err := client.R(context.Background()).
		SetQuery("extra", "two").
		SetQuery("override", "builder").
		Get("/users?request=one&override=request#client-fragment")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Get() status = %d, want 200", response.StatusCode)
	}
}

func TestStreamURLResolutionUsesSameRules(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/events" {
			t.Errorf("request path = %q, want /api/events", request.URL.Path)
		}
		query := request.URL.Query()
		if got := query.Get("tenant"); got != "base" {
			t.Errorf("tenant query = %q, want base", got)
		}
		if got := query.Get("source"); got != "builder" {
			t.Errorf("source query = %q, want builder", got)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("data: ok\n\n"))
	}))
	defer server.Close()

	client := MustNewClient(WithBaseURL(server.URL + "/api?tenant=base"))
	defer client.CloseIdleConnections()
	stream, err := client.R(context.Background()).
		SetQuery("source", "builder").
		GetStream("/events?source=request")
	if err != nil {
		t.Fatalf("GetStream() error = %v", err)
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
