package prometheus

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hexagon-codes/toolkit/infra/observe"
)

func TestRegistryHistogramUsesCumulativeBucketsOnce(t *testing.T) {
	registry := NewRegistry()
	histogram, err := registry.Histogram("request_seconds", "Request duration", []float64{0.1, 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if err := histogram.Observe(0.1); err != nil {
		t.Fatal(err)
	}

	output := gather(t, registry)
	for _, expected := range []string{
		`request_seconds_bucket{le="0.1"} 1`,
		`request_seconds_bucket{le="0.5"} 1`,
		`request_seconds_bucket{le="+Inf"} 1`,
		`request_seconds_count 1`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in exposition:\n%s", expected, output)
		}
	}
}

func TestRegistryRejectsDescriptorDrift(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.Counter("requests_total", "Requests", "method"); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Counter("requests_total", "Different help", "method"); err == nil {
		t.Fatal("expected inconsistent help to be rejected")
	}
	if _, err := registry.Gauge("requests_total", "Requests", "method"); err == nil {
		t.Fatal("expected metric type collision to be rejected")
	}
}

func TestCounterRejectsNegativeAndWrongLabelArity(t *testing.T) {
	registry := NewRegistry()
	counter, err := registry.Counter("requests_total", "Requests", "method")
	if err != nil {
		t.Fatal(err)
	}
	if err := counter.Add(-1, "GET"); !errors.Is(err, ErrNegativeCounterValue) {
		t.Fatalf("expected negative counter error, got %v", err)
	}
	if err := counter.Inc(); err == nil {
		t.Fatal("expected label arity error")
	}
	if err := counter.Add(2, "GET"); err != nil {
		t.Fatal(err)
	}
}

func TestFactoryPrefixesMetrics(t *testing.T) {
	registry := NewRegistry()
	factory, err := NewFactory(registry, "app", "api")
	if err != nil {
		t.Fatal(err)
	}
	counter, err := factory.Counter("requests_total", "Requests")
	if err != nil {
		t.Fatal(err)
	}
	if err := counter.Inc(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gather(t, registry), "app_api_requests_total") {
		t.Fatal("expected namespace and subsystem prefix")
	}
}

func TestMetricsAdapterPreservesTypedTags(t *testing.T) {
	registry := NewRegistry()
	adapter := mustMetricsAdapter(t, registry, "test", "")
	counter, err := adapter.Counter("requests_total", observe.Tag{Name: "method", Value: "GET"})
	if err != nil {
		t.Fatal(err)
	}
	counter.Inc()
	if err := counter.Add(5); err != nil {
		t.Fatal(err)
	}
	if counter.Value() != 6 {
		t.Fatalf("expected counter value 6, got %f", counter.Value())
	}

	output := gather(t, registry)
	if !strings.Contains(output, `method="GET"`) {
		t.Fatalf("metric tag was lost:\n%s", output)
	}
}

func TestMetricsAdapterRejectsDuplicateTags(t *testing.T) {
	adapter := mustMetricsAdapter(t, NewRegistry(), "test", "")
	_, err := adapter.Counter(
		"requests_total",
		observe.Tag{Name: "method", Value: "GET"},
		observe.Tag{Name: "method", Value: "POST"},
	)
	if err == nil {
		t.Fatal("expected duplicate tag error")
	}
}

func TestHistogramAdapterConcurrentObserve(t *testing.T) {
	adapter := mustMetricsAdapter(t, NewRegistry(), "test", "")
	histogram, err := adapter.Histogram("duration")
	if err != nil {
		t.Fatal(err)
	}

	const workers = 100
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			histogram.Observe(1)
		}()
	}
	wg.Wait()

	if histogram.Count() != workers {
		t.Fatalf("expected %d observations, got %d", workers, histogram.Count())
	}
	if histogram.Sum() != workers {
		t.Fatalf("expected sum %d, got %f", workers, histogram.Sum())
	}
}

func TestGaugeAndTimerAdapters(t *testing.T) {
	adapter := mustMetricsAdapter(t, NewRegistry(), "test", "")
	gauge, err := adapter.Gauge("connections")
	if err != nil {
		t.Fatal(err)
	}
	gauge.Set(10)
	gauge.Inc()
	gauge.Dec()
	if gauge.Value() != 10 {
		t.Fatalf("expected gauge value 10, got %f", gauge.Value())
	}

	timer, err := adapter.Timer("latency")
	if err != nil {
		t.Fatal(err)
	}
	timer.ObserveDuration(time.Millisecond)
	timer.Time(func() {})
	if timer.NewTimer().Stop() < 0 {
		t.Fatal("timer returned a negative duration")
	}
}

func TestExporterHandlerAndIdempotentShutdown(t *testing.T) {
	exporter, err := NewExporter(nil, WithNamespace("myapp"), WithSubsystem("api"))
	if err != nil {
		t.Fatal(err)
	}
	if exporter.Factory() == nil || exporter.Registry() == nil {
		t.Fatal("expected exporter dependencies")
	}

	recorder := httptest.NewRecorder()
	exporter.Handler().ServeHTTP(
		recorder,
		httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected handler status %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "go_goroutines") {
		t.Fatal("official Go runtime metrics were not registered")
	}

	if err := exporter.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := exporter.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestConstructorsRejectNilRegistry(t *testing.T) {
	if factory, err := NewFactory(nil, "app", ""); factory != nil || !errors.Is(err, ErrNilRegistry) {
		t.Fatalf("NewFactory(nil) = (%v, %v), want nil and ErrNilRegistry", factory, err)
	}
	if adapter, err := NewMetricsAdapter(nil, "app", ""); adapter != nil || !errors.Is(err, ErrNilRegistry) {
		t.Fatalf("NewMetricsAdapter(nil) = (%v, %v), want nil and ErrNilRegistry", adapter, err)
	}
}

func TestExporterCanStartAfterBindFailure(t *testing.T) {
	var listenConfig net.ListenConfig
	occupied, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := occupied.Addr().String()

	exporter, err := NewExporter()
	if err != nil {
		t.Fatal(err)
	}
	if err := exporter.ListenAndServe(address); err == nil {
		t.Fatal("ListenAndServe() succeeded on an occupied address")
	}
	if err := occupied.Close(); err != nil {
		t.Fatal(err)
	}

	serveResult := make(chan error, 1)
	go func() { serveResult <- exporter.ListenAndServe(address) }()
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(time.Second)
	for {
		request, requestErr := http.NewRequestWithContext(
			t.Context(),
			http.MethodGet,
			"http://"+address+"/metrics",
			http.NoBody,
		)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		response, requestErr := client.Do(request)
		if requestErr == nil {
			_ = response.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("exporter did not start after bind failure: %v", requestErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := exporter.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-serveResult:
		if err != nil {
			t.Fatalf("ListenAndServe() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ListenAndServe() did not return after Shutdown")
	}
}

func TestDefaultsAreConfigured(t *testing.T) {
	if len(DefaultBuckets()) == 0 || len(DefaultQuantiles()) == 0 {
		t.Fatal("expected default histogram and summary configuration")
	}
}

func TestDefaultsReturnIndependentSnapshots(t *testing.T) {
	buckets := DefaultBuckets()
	wantBucket := buckets[0]
	buckets[0] = -1
	if got := DefaultBuckets()[0]; got != wantBucket {
		t.Fatalf("DefaultBuckets()[0] = %v, want %v", got, wantBucket)
	}

	quantiles := DefaultQuantiles()
	wantQuantile := quantiles[0.5]
	quantiles[0.5] = -1
	if got := DefaultQuantiles()[0.5]; got != wantQuantile {
		t.Fatalf("DefaultQuantiles()[0.5] = %v, want %v", got, wantQuantile)
	}
}

func gather(t *testing.T, registry *Registry) string {
	t.Helper()
	output, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func mustMetricsAdapter(t *testing.T, registry *Registry, namespace, subsystem string) *MetricsAdapter {
	t.Helper()
	adapter, err := NewMetricsAdapter(registry, namespace, subsystem)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}
