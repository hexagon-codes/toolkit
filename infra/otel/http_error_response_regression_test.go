package otel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const expectedMaxExporterErrorBodyBytes = 64 << 10

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type trackingResponseBody struct {
	reader     io.Reader
	closeErr   error
	readBytes  atomic.Int64
	closeCalls atomic.Int32
}

func (b *trackingResponseBody) Read(buffer []byte) (int, error) {
	read, err := b.reader.Read(buffer)
	b.readBytes.Add(int64(read))
	return read, err
}

func (b *trackingResponseBody) Close() error {
	b.closeCalls.Add(1)
	return b.closeErr
}

func TestHTTPExportersBoundErrorResponseAndPreserveCloseError(t *testing.T) {
	for _, exporterName := range []string{"otlp", "jaeger", "zipkin"} {
		t.Run(exporterName, func(t *testing.T) {
			closeErr := errors.New("response body close failed")
			tail := "sensitive-tail-must-not-appear"
			payload := strings.Repeat("x", expectedMaxExporterErrorBodyBytes+1) + tail +
				strings.Repeat("y", expectedMaxExporterErrorBodyBytes)
			body := &trackingResponseBody{
				reader:   strings.NewReader(payload),
				closeErr: closeErr,
			}
			client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusBadGateway,
					Status:     http.StatusText(http.StatusBadGateway),
					Header:     make(http.Header),
					Body:       body,
					Request:    request,
				}, nil
			})}
			exporter := newHTTPExporterForTest(t, exporterName, client)

			err := exporter.ExportSpans(context.Background(), []*SpanData{mkSpan("bounded-error")})
			if err == nil {
				t.Fatal("ExportSpans() error = nil, want non-2xx response error")
			}
			if got := body.readBytes.Load(); got > expectedMaxExporterErrorBodyBytes+1 {
				t.Fatalf("error response bytes read = %d, want at most %d", got, expectedMaxExporterErrorBodyBytes+1)
			}
			if !strings.Contains(err.Error(), "truncated=true") {
				t.Fatalf("ExportSpans() error = %q, want explicit truncation diagnostic", err)
			}
			if strings.Contains(err.Error(), tail) {
				t.Fatalf("ExportSpans() error contains response bytes beyond the limit: %q", err)
			}
			if !errors.Is(err, closeErr) {
				t.Fatalf("ExportSpans() error = %v, want response body close error in chain", err)
			}
			if got := body.closeCalls.Load(); got != 1 {
				t.Fatalf("response body Close() calls = %d, want 1", got)
			}
		})
	}
}

func TestHTTPExportersRejectEveryNon2xxResponse(t *testing.T) {
	for _, statusCode := range []int{http.StatusEarlyHints, http.StatusMultipleChoices} {
		for _, exporterName := range []string{"otlp", "jaeger", "zipkin"} {
			t.Run(fmt.Sprintf("%s/status=%d", exporterName, statusCode), func(t *testing.T) {
				client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: statusCode,
						Status:     http.StatusText(statusCode),
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader("non-success")),
						Request:    request,
					}, nil
				})}
				exporter := newHTTPExporterForTest(t, exporterName, client)

				if err := exporter.ExportSpans(context.Background(), []*SpanData{mkSpan("non-success")}); err == nil {
					t.Fatalf("ExportSpans() error = nil for HTTP status %d", statusCode)
				}
			})
		}
	}
}

func newHTTPExporterForTest(t *testing.T, exporterName string, client *http.Client) Exporter {
	t.Helper()
	switch exporterName {
	case "otlp":
		exporter := mustNewOTLPExporter(
			t,
			"http://collector.example",
			WithOTLPBatchSize(1),
			WithOTLPBatchTimeout(time.Hour),
		)
		exporter.client = client
		t.Cleanup(func() {
			exporter.bufferMu.Lock()
			exporter.buffer = nil
			exporter.bufferMu.Unlock()
			_ = exporter.Shutdown(context.Background())
		})
		return exporter
	case "jaeger":
		exporter := NewJaegerExporter("http://collector.example")
		exporter.client = client
		return exporter
	case "zipkin":
		exporter := NewZipkinExporter("http://collector.example", "test-service")
		exporter.client = client
		return exporter
	default:
		t.Fatalf("unknown exporter %q", exporterName)
		return nil
	}
}
