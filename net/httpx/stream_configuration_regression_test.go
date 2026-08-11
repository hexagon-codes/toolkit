package httpx

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestStreamRejectsInvalidOptionsWithoutNetworkCall(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("data: unexpected\n\n"))
	}))
	defer server.Close()

	client := MustNewClient()
	defer client.CloseIdleConnections()
	tests := []struct {
		name   string
		option StreamOption
	}{
		{name: "nil option", option: nil},
		{name: "zero buffer", option: WithBufferSize(0)},
		{name: "negative buffer", option: WithBufferSize(-1)},
		{name: "zero event limit", option: WithMaxEventSize(0)},
		{name: "negative event limit", option: WithMaxEventSize(-1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("GetStream() panicked: %v", recovered)
				}
			}()
			stream, err := client.R(context.Background()).GetStream(server.URL, test.option)
			if stream != nil {
				_ = stream.Close()
				t.Fatalf("GetStream() stream = %#v, want nil", stream)
			}
			if !errors.Is(err, ErrInvalidClientConfig) {
				t.Fatalf("GetStream() error = %v, want ErrInvalidClientConfig", err)
			}
		})
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("network calls = %d, want 0", got)
	}
}

func TestStreamRejectsNilContext(t *testing.T) {
	client := MustNewClient()
	defer client.CloseIdleConnections()
	//nolint:staticcheck // 需要验证公开 API 对 nil context 的错误合同。
	stream, err := client.R(nil).GetStream("https://example.com/events")
	if stream != nil {
		_ = stream.Close()
		t.Fatalf("GetStream() stream = %#v, want nil", stream)
	}
	if !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("GetStream() error = %v, want ErrInvalidContext", err)
	}
}

func TestStreamConsumersRejectNilCallbacksWithoutPanic(t *testing.T) {
	tests := []struct {
		name string
		call func(*StreamResponse) error
	}{
		{name: "handler", call: func(stream *StreamResponse) error { return stream.OnData(nil) }},
		{name: "factory", call: func(stream *StreamResponse) error {
			_, err := stream.CollectJSON(nil)
			return err
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := io.NopCloser(strings.NewReader("data: {}\n\n"))
			stream := &StreamResponse{body: body, reader: bufio.NewReader(body)}
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("stream consumer panicked: %v", recovered)
				}
			}()
			if err := test.call(stream); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("stream consumer error = %v, want ErrInvalidRequest", err)
			}
			if !stream.closed.Load() {
				t.Fatal("stream consumer did not close the stream")
			}
		})
	}
}

func TestStreamRejectsOversizedSSEEventAndClosesBody(t *testing.T) {
	const maximumEventSize = 1 << 20
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("data: " + strings.Repeat("x", maximumEventSize+1) + "\n\n"))
	}))
	defer server.Close()

	client := MustNewClient()
	defer client.CloseIdleConnections()
	stream, err := client.R(context.Background()).GetStream(server.URL, WithBufferSize(256))
	if err != nil {
		t.Fatalf("GetStream() error = %v", err)
	}
	event, err := stream.ReadSSE()
	if event != nil {
		t.Fatalf("ReadSSE() event = %#v, want nil", event)
	}
	if !errors.Is(err, ErrInvalidSSE) {
		t.Fatalf("ReadSSE() error = %v, want ErrInvalidSSE", err)
	}
	if !errors.Is(err, ErrStreamEventTooLarge) {
		t.Fatalf("ReadSSE() error = %v, want ErrStreamEventTooLarge", err)
	}
	if !stream.closed.Load() {
		t.Fatal("oversized SSE event did not close the stream")
	}
}

func TestParseRetryRejectsEmptyAndOverflow(t *testing.T) {
	for _, value := range []string{"", strings.Repeat("9", 100)} {
		t.Run(value, func(t *testing.T) {
			if retry, err := parseRetry(value); retry != 0 || !errors.Is(err, ErrInvalidSSE) {
				t.Fatalf("parseRetry(%q) = (%d, %v), want 0 and ErrInvalidSSE", value, retry, err)
			}
		})
	}
}

func TestSSEFieldValuesFollowProtocolWhitespaceRules(t *testing.T) {
	body := io.NopCloser(strings.NewReader("id: invalid\x00id\nid: item \nevent: update \nretry: 1000 \ndata: hello\n\n"))
	stream := &StreamResponse{body: body, reader: bufio.NewReader(body)}
	defer stream.Close()

	event, err := stream.ReadSSE()
	if err != nil {
		t.Fatalf("ReadSSE() error = %v", err)
	}
	if event.ID != "item " {
		t.Fatalf("event ID = %q, want trailing space preserved", event.ID)
	}
	if event.Event != "update " {
		t.Fatalf("event type = %q, want trailing space preserved", event.Event)
	}
	if event.Retry != 0 {
		t.Fatalf("event retry = %d, want invalid value ignored", event.Retry)
	}
}
