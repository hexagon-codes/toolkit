package prometheus

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	prom "github.com/prometheus/client_golang/prometheus"
)

type blockingCollectCollector struct {
	desc        *prom.Desc
	entered     chan struct{}
	release     chan struct{}
	enteredOnce sync.Once
}

func (c *blockingCollectCollector) Describe(descriptions chan<- *prom.Desc) {
	descriptions <- c.desc
}

func (c *blockingCollectCollector) Collect(metrics chan<- prom.Metric) {
	c.enteredOnce.Do(func() { close(c.entered) })
	<-c.release
	metrics <- prom.MustNewConstMetric(c.desc, prom.GaugeValue, 1)
}

type observedContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *observedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}

func TestExporterShutdownBeforeServeIsTerminal(t *testing.T) {
	exporter, err := NewExporter()
	if err != nil {
		t.Fatal(err)
	}
	if err := exporter.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	serveResult := make(chan error, 1)
	go func() { serveResult <- exporter.ListenAndServe("127.0.0.1:0") }()

	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case err := <-serveResult:
			if !errors.Is(err, ErrExporterClosed) {
				t.Fatalf("ListenAndServe() error = %v, want closed exporter error", err)
			}
			return
		case <-ticker.C:
			exporter.mu.RLock()
			server := exporter.server
			exporter.mu.RUnlock()
			if server != nil {
				_ = exporter.Shutdown(context.Background())
				<-serveResult
				t.Fatal("ListenAndServe() started after Shutdown")
			}
		case <-deadline.C:
			t.Fatal("ListenAndServe() did not resolve after Shutdown")
		}
	}
}

func TestExporterShutdownCanRetryAfterContextCancellation(t *testing.T) {
	exporter, err := NewExporter()
	if err != nil {
		t.Fatal(err)
	}
	collector := &blockingCollectCollector{
		desc:    prom.NewDesc("shutdown_blocking_metric", "Blocking metric", nil, nil),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(collector.release) }) }
	t.Cleanup(release)
	if registerErr := exporter.Registry().Register(collector); registerErr != nil {
		t.Fatal(registerErr)
	}

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if closeErr := listener.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	serveResult := make(chan error, 1)
	go func() { serveResult <- exporter.ListenAndServe(address) }()
	waitForExporterServer(t, exporter, serveResult)

	requestContext, cancelRequest := context.WithCancel(t.Context())
	defer cancelRequest()
	request, err := http.NewRequestWithContext(
		requestContext,
		http.MethodGet,
		"http://"+address+"/metrics",
		http.NoBody,
	)
	if err != nil {
		t.Fatal(err)
	}
	requestResult := make(chan error, 1)
	go func() {
		response, requestErr := http.DefaultClient.Do(request)
		if response != nil {
			_ = response.Body.Close()
		}
		requestResult <- requestErr
	}()

	select {
	case <-collector.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("metrics collection did not start")
	}

	canceledContext, cancelShutdown := context.WithCancel(context.Background())
	cancelShutdown()
	if err := exporter.Shutdown(canceledContext); !errors.Is(err, context.Canceled) {
		t.Fatalf("first Shutdown() error = %v, want context.Canceled", err)
	}
	select {
	case err := <-serveResult:
		if err != nil {
			t.Fatalf("ListenAndServe() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ListenAndServe() did not stop accepting connections")
	}

	retryBase, cancelRetry := context.WithCancel(context.Background())
	defer cancelRetry()
	retryContext := &observedContext{Context: retryBase, observed: make(chan struct{})}
	retryResult := make(chan error, 1)
	go func() { retryResult <- exporter.Shutdown(retryContext) }()
	select {
	case <-retryContext.observed:
	case err := <-retryResult:
		release()
		<-requestResult
		t.Fatalf("retry Shutdown() returned before draining the active request: %v", err)
	case <-time.After(10 * time.Second):
		release()
		<-requestResult
		t.Fatal("retry Shutdown() did not inspect its context")
	}

	release()
	if err := <-requestResult; err != nil {
		t.Fatalf("metrics request error = %v", err)
	}
	select {
	case err := <-retryResult:
		if err != nil {
			t.Fatalf("retry Shutdown() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("retry Shutdown() did not finish after the request drained")
	}
}

func TestExporterUsesIsolatedRegistry(t *testing.T) {
	globalMetric := prom.NewGauge(prom.GaugeOpts{
		Name: "toolkit_global_registry_isolation_probe",
		Help: "Global registry isolation probe",
	})
	if err := prom.Register(globalMetric); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { prom.Unregister(globalMetric) })

	exporter, err := NewExporter()
	if err != nil {
		t.Fatal(err)
	}
	output, err := exporter.Registry().Gather()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, "toolkit_global_registry_isolation_probe") {
		t.Fatal("exporter registry contains a metric from the process-global registry")
	}
}

func TestExporterConcurrentGatherUpdateAndShutdown(t *testing.T) {
	exporter, err := NewExporter()
	if err != nil {
		t.Fatal(err)
	}
	counter, err := exporter.Factory().Counter("concurrent_operations_total", "Operations")
	if err != nil {
		t.Fatal(err)
	}

	const workers = 24
	const iterations = 100
	start := make(chan struct{})
	errorsChannel := make(chan error, workers)
	var waitGroup sync.WaitGroup
	for worker := range workers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			for range iterations {
				switch worker % 3 {
				case 0:
					if err := counter.Inc(); err != nil {
						errorsChannel <- err
						return
					}
				case 1:
					if _, err := exporter.Registry().Gather(); err != nil {
						errorsChannel <- err
						return
					}
				default:
					if err := exporter.Shutdown(context.Background()); err != nil {
						errorsChannel <- err
						return
					}
				}
			}
		}()
	}
	close(start)
	waitGroup.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		t.Fatalf("concurrent operation error = %v", err)
	}
}

func waitForExporterServer(t *testing.T, exporter *Exporter, serveResult <-chan error) {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case err := <-serveResult:
			t.Fatalf("ListenAndServe() exited before startup: %v", err)
		case <-ticker.C:
			exporter.mu.RLock()
			server := exporter.server
			exporter.mu.RUnlock()
			if server != nil {
				return
			}
		case <-deadline.C:
			t.Fatal("ListenAndServe() did not publish its server")
		}
	}
}
