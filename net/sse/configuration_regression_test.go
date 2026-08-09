package sse

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestReaderConstructorsRejectInvalidConfiguration(t *testing.T) {
	if reader, err := NewReader(nil); reader != nil || !errors.Is(err, ErrInvalidReaderConfig) {
		t.Fatalf("NewReader(nil) = (%v, %v), want (nil, ErrInvalidReaderConfig)", reader, err)
	}
	if reader, err := NewReaderWithSize(strings.NewReader(""), 0); reader != nil || !errors.Is(err, ErrInvalidReaderConfig) {
		t.Fatalf("NewReaderWithSize(..., 0) = (%v, %v), want (nil, ErrInvalidReaderConfig)", reader, err)
	}
	if reader, err := NewReaderWithOptions(strings.NewReader(""), nil); reader != nil || !errors.Is(err, ErrInvalidReaderConfig) {
		t.Fatalf("NewReaderWithOptions(..., nil) = (%v, %v), want (nil, ErrInvalidReaderConfig)", reader, err)
	}
	if reader, err := NewReaderWithOptions(strings.NewReader(""), WithMaxTotalBytes(-1)); reader != nil || !errors.Is(err, ErrInvalidReaderConfig) {
		t.Fatalf("NewReaderWithOptions(..., negative limit) = (%v, %v), want (nil, ErrInvalidReaderConfig)", reader, err)
	}
	if reader, err := NewReaderWithOptions(strings.NewReader(""), WithDoneFunc(nil)); reader != nil || !errors.Is(err, ErrInvalidReaderConfig) {
		t.Fatalf("NewReaderWithOptions(..., nil done function) = (%v, %v), want (nil, ErrInvalidReaderConfig)", reader, err)
	}
}

func TestOptionsPreserveUnderlyingErrors(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("option failed")
	reader, err := NewReaderWithOptions(strings.NewReader(""), func(*Reader) error { return sentinel })
	if reader != nil {
		t.Fatalf("NewReaderWithOptions() reader = %#v, want nil", reader)
	}
	if !errors.Is(err, ErrInvalidReaderConfig) || !errors.Is(err, sentinel) {
		t.Fatalf("NewReaderWithOptions() error = %v, want configuration and option errors", err)
	}

	client, err := NewClient("https://example.com/events", func(*ClientConfig) error { return sentinel })
	if client != nil {
		client.CloseIdleConnections()
		t.Fatalf("NewClient() client = %#v, want nil", client)
	}
	if !errors.Is(err, ErrInvalidClientConfig) || !errors.Is(err, sentinel) {
		t.Fatalf("NewClient() error = %v, want configuration and option errors", err)
	}
}

func TestReaderEachRejectsNilHandler(t *testing.T) {
	reader := mustReader(NewReader(strings.NewReader("data: ok\n\n")))
	if err := reader.Each(nil); !errors.Is(err, ErrInvalidHandler) {
		t.Fatalf("Each(nil) error = %v, want ErrInvalidHandler", err)
	}
}

func TestClientRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		option ClientOption
	}{
		{name: "empty URL", url: ""},
		{name: "relative URL", url: "/events"},
		{name: "unsupported scheme", url: "file:///tmp/events"},
		{name: "nil option", url: "https://example.com/events", option: nil},
		{name: "negative timeout", url: "https://example.com/events", option: WithTimeout(-time.Second)},
		{name: "nil HTTP client", url: "https://example.com/events", option: WithHTTPClient(nil)},
		{name: "unsafe header", url: "https://example.com/events", option: WithHeaders(map[string]string{"X-Test": "ok\r\ninjected"})},
		{name: "unsafe event ID", url: "https://example.com/events", option: WithLastEventID("one\ntwo")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var options []ClientOption
			if test.option != nil || test.name == "nil option" {
				options = append(options, test.option)
			}
			client, err := NewClient(test.url, options...)
			if client != nil {
				client.CloseIdleConnections()
			}
			if !errors.Is(err, ErrInvalidClientConfig) {
				t.Fatalf("NewClient() error = %v, want ErrInvalidClientConfig", err)
			}
		})
	}
}

func TestClientConnectRejectsNilContext(t *testing.T) {
	client := mustClient(NewClient("https://example.com/events"))
	defer client.CloseIdleConnections()
	//nolint:staticcheck // 需要验证公开 API 对 nil context 的错误合同。
	stream, err := client.Connect(nil)
	if stream != nil || !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("Connect(nil) = (%v, %v), want (nil, ErrInvalidContext)", stream, err)
	}
}

type typedNilContext struct{}

func (*typedNilContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*typedNilContext) Done() <-chan struct{}       { return nil }
func (*typedNilContext) Err() error                  { return nil }
func (*typedNilContext) Value(any) any               { return nil }

func TestClientConnectRejectsTypedNilContext(t *testing.T) {
	var called atomic.Bool
	client := mustClient(NewClient(
		"https://example.com/events",
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			called.Store(true)
			return nil, errors.New("transport must not be called")
		})}),
	))
	var ctx *typedNilContext
	stream, err := client.Connect(ctx)
	if stream != nil || !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("Connect(typed nil) = (%v, %v), want (nil, ErrInvalidContext)", stream, err)
	}
	if called.Load() {
		t.Fatal("Connect() called the transport for a typed-nil context")
	}
}

func TestClientOptionsCannotMutateConstructedConfiguration(t *testing.T) {
	requestHeaders := make(chan http.Header, 1)
	var retained *ClientConfig
	retainConfig := func(config *ClientConfig) error {
		config.Headers["X-Test"] = "original"
		config.LastEventID = "original-id"
		retained = config
		return nil
	}
	client := mustClient(NewClient(
		"https://example.com/events",
		retainConfig,
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requestHeaders <- request.Header.Clone()
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		})}),
	))
	retained.Headers["X-Test"] = "mutated"
	retained.LastEventID = "mutated-id"

	stream, err := client.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	headers := <-requestHeaders
	if got := headers.Get("X-Test"); got != "original" {
		t.Fatalf("X-Test header = %q, want original", got)
	}
	if got := headers.Get("Last-Event-ID"); got != "original-id" {
		t.Fatalf("Last-Event-ID header = %q, want original-id", got)
	}
}

func TestClientTimeoutIncludesResponseHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()

	client := mustClient(NewClient(server.URL, WithTimeout(25*time.Millisecond)))
	defer client.CloseIdleConnections()
	fallbackContext, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := client.Connect(fallbackContext)
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("Connect() error = nil, want response-header timeout")
	}
	if elapsed >= 150*time.Millisecond {
		t.Fatalf("Connect() returned after %v, want configured timeout to cover response headers", elapsed)
	}
	var timeoutError interface{ Timeout() bool }
	if !errors.As(err, &timeoutError) || !timeoutError.Timeout() {
		t.Fatalf("Connect() error = %v, want timeout error", err)
	}
}

func TestClientRejectsUnexpectedContentTypeAndClosesBody(t *testing.T) {
	body := &trackingReadCloser{Reader: strings.NewReader("not an SSE stream")}
	client := mustClient(NewClient(
		"https://example.com/events",
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       body,
			}, nil
		})}),
	))

	stream, err := client.Connect(context.Background())
	if stream != nil || !errors.Is(err, ErrUnexpectedContentType) {
		t.Fatalf("Connect() = (%v, %v), want (nil, ErrUnexpectedContentType)", stream, err)
	}
	if !body.closed {
		t.Fatal("Connect() did not close the rejected response body")
	}
}

type panicOnTypedNilBody struct {
	value byte
}

func (b *panicOnTypedNilBody) Read([]byte) (int, error) {
	_ = b.value
	return 0, io.EOF
}

func (b *panicOnTypedNilBody) Close() error {
	_ = b.value
	return nil
}

func TestClientRejectsTypedNilResponseBodyWithoutPanic(t *testing.T) {
	client := mustClient(NewClient(
		"https://example.com/events",
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			var body *panicOnTypedNilBody
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       body,
			}, nil
		})}),
	))

	stream, err := client.Connect(context.Background())
	if stream != nil || !errors.Is(err, ErrInvalidReaderConfig) {
		t.Fatalf("Connect() = (%v, %v), want (nil, ErrInvalidReaderConfig)", stream, err)
	}
}

func TestWriterAndFormatterRejectProtocolInjection(t *testing.T) {
	if writer, err := NewWriter(nil); writer != nil || !errors.Is(err, ErrInvalidWriter) {
		t.Fatalf("NewWriter(nil) = (%v, %v), want (nil, ErrInvalidWriter)", writer, err)
	}

	tests := []struct {
		name  string
		event *Event
	}{
		{name: "nil event", event: nil},
		{name: "ID newline", event: &Event{ID: "one\ntwo"}},
		{name: "ID null", event: &Event{ID: "one\x00two"}},
		{name: "event newline", event: &Event{Event: "one\r\ntwo"}},
		{name: "negative retry", event: &Event{Retry: -1}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writer := mustWriter(NewWriter(recorder))
			if err := writer.Write(test.event); !errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("Write() error = %v, want ErrInvalidEvent", err)
			}
			if recorder.Body.Len() != 0 {
				t.Fatalf("Write() emitted %q for an invalid event", recorder.Body.String())
			}
			if formatted, err := FormatEvent(test.event); formatted != "" || !errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("FormatEvent() = (%q, %v), want (empty, ErrInvalidEvent)", formatted, err)
			}
		})
	}
}

func TestWriterNormalizesDataLineEndings(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := mustWriter(NewWriter(recorder))
	if err := writer.WriteData("one\rtwo\r\nthree\nfour"); err != nil {
		t.Fatal(err)
	}
	const expected = "data: one\ndata: two\ndata: three\ndata: four\n\n"
	if got := recorder.Body.String(); got != expected {
		t.Fatalf("WriteData() output = %q, want %q", got, expected)
	}
}

func TestWriterRejectsCommentInjection(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := mustWriter(NewWriter(recorder))
	if err := writer.WriteComment("keep-alive\ndata: injected"); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("WriteComment() error = %v, want ErrInvalidEvent", err)
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("WriteComment() emitted %q", recorder.Body.String())
	}
}

type shortResponseWriter struct {
	header http.Header
}

func (w *shortResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (*shortResponseWriter) Write(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	return len(buffer) - 1, nil
}

func (*shortResponseWriter) WriteHeader(int) {}

func TestWriterRejectsSilentShortWrites(t *testing.T) {
	tests := []struct {
		name  string
		write func(*Writer) error
	}{
		{name: "event", write: func(writer *Writer) error { return writer.WriteData("payload") }},
		{name: "comment", write: func(writer *Writer) error { return writer.WriteComment("keep-alive") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writer := MustNewWriter(&shortResponseWriter{})
			if err := test.write(writer); !errors.Is(err, io.ErrShortWrite) {
				t.Fatalf("write error = %v, want io.ErrShortWrite", err)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (c *trackingReadCloser) Close() error {
	c.closed = true
	return nil
}

func mustReader(reader *Reader, err error) *Reader {
	if err != nil {
		panic(err)
	}
	return reader
}

func mustClient(client *Client, err error) *Client {
	if err != nil {
		panic(err)
	}
	return client
}

func mustWriter(writer *Writer, err error) *Writer {
	if err != nil {
		panic(err)
	}
	return writer
}
