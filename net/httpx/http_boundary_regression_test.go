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
	"time"
)

func TestStreamCollectorsEnforceSafeDefaultEventLimit(t *testing.T) {
	tests := []struct {
		name string
		data string
		call func(*StreamResponse) (bool, error)
	}{
		{
			name: "data",
			data: strings.Repeat("data: x\n\n", DefaultMaxCollectionEvents+1),
			call: func(stream *StreamResponse) (bool, error) {
				result, err := stream.CollectData()
				return result == nil, err
			},
		},
		{
			name: "JSON",
			data: strings.Repeat("data: {}\n\n", DefaultMaxCollectionEvents+1),
			call: func(stream *StreamResponse) (bool, error) {
				result, err := stream.CollectJSON(func() any { return &map[string]any{} })
				return result == nil, err
			},
		},
		{
			name: "OpenAI content",
			data: strings.Repeat("data: {\"choices\":[{\"delta\":{\"content\":\"\"}}]}\n\n", DefaultMaxCollectionEvents+1),
			call: func(stream *StreamResponse) (bool, error) {
				result, err := stream.CollectOpenAIContent()
				return result == "", err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := io.NopCloser(strings.NewReader(test.data))
			stream := &StreamResponse{body: body, reader: bufio.NewReader(body)}
			empty, err := test.call(stream)
			if !errors.Is(err, ErrStreamCollectionLimit) {
				t.Fatalf("collector error = %v, want ErrStreamCollectionLimit", err)
			}
			if !empty {
				t.Fatal("collector returned partial data after reaching the default event limit")
			}
			if !stream.closed.Load() {
				t.Fatal("collector did not close the stream after reaching the default event limit")
			}
		})
	}
}

func TestStreamCollectorsEnforceConfiguredLimitsWithoutPartialResults(t *testing.T) {
	const openAIChunk = `{"choices":[{"delta":{"content":"x"}}]}`
	tests := []struct {
		name string
		data string
		call func(*StreamResponse, ...CollectionOption) (bool, error)
	}{
		{
			name: "data",
			data: "data: abc\n\ndata: de\n\n",
			call: func(stream *StreamResponse, options ...CollectionOption) (bool, error) {
				result, err := stream.CollectData(options...)
				return result == nil, err
			},
		},
		{
			name: "JSON",
			data: "data: {}\n\ndata: {}\n\n",
			call: func(stream *StreamResponse, options ...CollectionOption) (bool, error) {
				result, err := stream.CollectJSON(func() any { return &map[string]any{} }, options...)
				return result == nil, err
			},
		},
		{
			name: "OpenAI content",
			data: "data: " + openAIChunk + "\n\ndata: " + openAIChunk + "\n\n",
			call: func(stream *StreamResponse, options ...CollectionOption) (bool, error) {
				result, err := stream.CollectOpenAIContent(options...)
				return result == "", err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name+"/events", func(t *testing.T) {
			stream := newMemoryStream(test.data)
			empty, err := test.call(stream, WithMaxCollectionEvents(1))
			assertCollectionLimitFailure(t, stream, empty, err)
		})

		t.Run(test.name+"/bytes", func(t *testing.T) {
			stream := newMemoryStream(test.data)
			firstEvent, readErr := stream.ReadSSE()
			if readErr != nil {
				t.Fatal(readErr)
			}
			_ = stream.Close()

			stream = newMemoryStream(test.data)
			empty, err := test.call(stream, WithMaxCollectionBytes(int64(len(firstEvent.Data))))
			assertCollectionLimitFailure(t, stream, empty, err)
		})
	}
}

func TestStreamCollectorAcceptsExactConfiguredLimits(t *testing.T) {
	stream := newMemoryStream("data: abc\n\ndata: de\n\n")
	data, err := stream.CollectData(
		WithMaxCollectionBytes(5),
		WithMaxCollectionEvents(2),
	)
	if err != nil {
		t.Fatalf("CollectData() error = %v", err)
	}
	if strings.Join(data, "") != "abcde" {
		t.Fatalf("CollectData() = %v, want complete data", data)
	}
}

func TestStreamCollectorsRejectInvalidCollectionOptionsAndClose(t *testing.T) {
	tests := []struct {
		name   string
		option CollectionOption
	}{
		{name: "nil", option: nil},
		{name: "zero bytes", option: WithMaxCollectionBytes(0)},
		{name: "negative bytes", option: WithMaxCollectionBytes(-1)},
		{name: "zero events", option: WithMaxCollectionEvents(0)},
		{name: "negative events", option: WithMaxCollectionEvents(-1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stream := newMemoryStream("data: value\n\n")
			data, err := stream.CollectData(test.option)
			if data != nil {
				t.Fatalf("CollectData() = %v, want nil", data)
			}
			if !errors.Is(err, ErrInvalidClientConfig) {
				t.Fatalf("CollectData() error = %v, want ErrInvalidClientConfig", err)
			}
			if !stream.closed.Load() {
				t.Fatal("CollectData() did not close the stream after invalid configuration")
			}
		})
	}
}

func assertCollectionLimitFailure(t *testing.T, stream *StreamResponse, empty bool, err error) {
	t.Helper()
	if !errors.Is(err, ErrStreamCollectionLimit) {
		t.Fatalf("collector error = %v, want ErrStreamCollectionLimit", err)
	}
	if !empty {
		t.Fatal("collector returned partial data after reaching a configured limit")
	}
	if !stream.closed.Load() {
		t.Fatal("collector did not close the stream after reaching a configured limit")
	}
}

func newMemoryStream(data string) *StreamResponse {
	body := io.NopCloser(strings.NewReader(data))
	return &StreamResponse{body: body, reader: bufio.NewReader(body)}
}

func TestRequestsRequireExplicitContext(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = writer.Write([]byte("ok"))
	}))
	defer server.Close()

	client := MustNewClient()
	defer client.CloseIdleConnections()
	//nolint:staticcheck // 需要验证公开 API 对 nil context 的拒绝合同。
	response, err := client.R(nil).Get(server.URL)
	if response != nil {
		t.Fatalf("Get() response = %#v, want nil", response)
	}
	if !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("Get() error = %v, want ErrInvalidContext", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("network calls = %d, want 0", got)
	}
}

func TestZeroTimeoutCannotDisableSafetyBoundary(t *testing.T) {
	tests := []struct {
		name string
		call func() error
		want error
	}{
		{
			name: "wrapped client",
			call: func() error {
				client, err := NewClient(WithTimeout(0))
				if client != nil {
					client.CloseIdleConnections()
				}
				return err
			},
			want: ErrInvalidClientConfig,
		},
		{
			name: "raw client",
			call: func() error {
				client, err := NewRawClient(WithRawTimeout(0))
				if client != nil {
					client.CloseIdleConnections()
				}
				return err
			},
			want: ErrInvalidRawClientConfig,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, test.want) {
				t.Fatalf("configuration error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestRawClientHasFiniteDefaultTimeout(t *testing.T) {
	client := MustNewRawClient()
	defer client.CloseIdleConnections()
	if client.Timeout <= 0 {
		t.Fatalf("default raw client timeout = %v, want a positive duration", client.Timeout)
	}
}

func TestRawClientTimeoutBoundsRequestWithoutDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()

	client := MustNewRawClient(WithRawTimeout(50 * time.Millisecond))
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if response != nil {
		_ = response.Body.Close()
		t.Fatalf("Do() response = %#v, want nil", response)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Do() error = %v, want context deadline exceeded", err)
	}
}

func TestCallerShorterDeadlineIsPreserved(t *testing.T) {
	const clientTimeout = time.Minute
	deadlineObserved := make(chan time.Time, 1)
	client := MustNewClient(WithTimeout(clientTimeout))
	defer client.CloseIdleConnections()
	client.client.Transport = roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		deadline, ok := request.Context().Deadline()
		if !ok {
			return nil, errors.New("request context has no deadline")
		}
		deadlineObserved <- deadline
		return nil, context.Canceled
	})

	callerDeadline := time.Now().Add(5 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), callerDeadline)
	defer cancel()
	_, _ = client.R(ctx).Get("https://example.com")

	observed := <-deadlineObserved
	if delta := observed.Sub(callerDeadline); delta < -time.Millisecond || delta > time.Millisecond {
		t.Fatalf("request deadline = %v, want caller deadline %v", observed, callerDeadline)
	}
}

func TestRawClientPreservesCallerShorterDeadline(t *testing.T) {
	deadlineObserved := make(chan time.Time, 1)
	client := MustNewRawClient(
		WithRawTimeout(time.Minute),
		WithRawTransport(roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			deadline, ok := request.Context().Deadline()
			if !ok {
				return nil, errors.New("request context has no deadline")
			}
			deadlineObserved <- deadline
			return nil, context.Canceled
		})),
	)
	defer client.CloseIdleConnections()

	callerDeadline := time.Now().Add(5 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), callerDeadline)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	response, _ := client.Do(request)
	if response != nil {
		_ = response.Body.Close()
	}

	observed := <-deadlineObserved
	if delta := observed.Sub(callerDeadline); delta < -time.Millisecond || delta > time.Millisecond {
		t.Fatalf("request deadline = %v, want caller deadline %v", observed, callerDeadline)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
