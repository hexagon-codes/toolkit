package prometheus

import (
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	prom "github.com/prometheus/client_golang/prometheus"
)

type typedNilCollector struct{}

func (*typedNilCollector) Describe(chan<- *prom.Desc) {}
func (*typedNilCollector) Collect(chan<- prom.Metric) {}

type blockingDescribeCollector struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *blockingDescribeCollector) Describe(chan<- *prom.Desc) {
	c.once.Do(func() { close(c.entered) })
	<-c.release
}

func (*blockingDescribeCollector) Collect(chan<- prom.Metric) {}

type panicCollectCollector struct {
	desc     *prom.Desc
	panicErr error
}

func (c *panicCollectCollector) Describe(descriptions chan<- *prom.Desc) {
	descriptions <- c.desc
}

func (c *panicCollectCollector) Collect(chan<- prom.Metric) {
	panic(c.panicErr)
}

type panicDescribeCollector struct {
	panicErr error
}

func (c *panicDescribeCollector) Describe(chan<- *prom.Desc) {
	panic(c.panicErr)
}

func (*panicDescribeCollector) Collect(chan<- prom.Metric) {}

type uncheckedCollector struct{}

func (*uncheckedCollector) Describe(chan<- *prom.Desc) {}
func (*uncheckedCollector) Collect(chan<- prom.Metric) {}

type nonComparableCollector []int

func (nonComparableCollector) Describe(chan<- *prom.Desc) {}
func (nonComparableCollector) Collect(chan<- prom.Metric) {}

type closingDescribeCollector struct{}

func (*closingDescribeCollector) Describe(descriptions chan<- *prom.Desc) {
	close(descriptions)
}

func (*closingDescribeCollector) Collect(chan<- prom.Metric) {}

func TestRegistryRejectsTypedNilCollector(t *testing.T) {
	registry := NewRegistry()
	var collector *typedNilCollector
	if err := registry.Register(collector); !errors.Is(err, ErrNilCollector) {
		t.Fatalf("Register() error = %v, want ErrNilCollector", err)
	}
}

func TestRegistryRejectsCollectorWithoutStableIdentity(t *testing.T) {
	registry := NewRegistry()
	collector := nonComparableCollector{1}
	if err := registry.Register(collector); !errors.Is(err, ErrUntrackableCollector) {
		t.Fatalf("Register() error = %v, want ErrUntrackableCollector", err)
	}
}

func TestRegistryContainsDescribeChannelOwnershipViolation(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(&closingDescribeCollector{}); err == nil {
		t.Fatal("Register() error = nil, want channel ownership error")
	}
	probe := prom.NewGauge(prom.GaugeOpts{Name: "describe_channel_recovery", Help: "Recovery"})
	if err := registry.Register(probe); err != nil {
		t.Fatalf("Register() after channel ownership violation error = %v", err)
	}
}

func TestRegistryDoesNotHoldRegistryLockDuringCollectorDescribe(t *testing.T) {
	registry := NewRegistry()
	blocking := &blockingDescribeCollector{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	blockingResult := make(chan error, 1)
	go func() { blockingResult <- registry.Register(blocking) }()
	<-blocking.entered

	probe := prom.NewGauge(prom.GaugeOpts{Name: "describe_lock_probe", Help: "Probe"})
	probeResult := make(chan error, 1)
	go func() { probeResult <- registry.Register(probe) }()

	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	select {
	case err := <-probeResult:
		if err != nil {
			close(blocking.release)
			<-blockingResult
			t.Fatalf("Register(probe) error = %v", err)
		}
	case <-deadline.C:
		close(blocking.release)
		<-blockingResult
		<-probeResult
		t.Fatal("Register(probe) blocked behind an external Describe callback")
	}

	close(blocking.release)
	if err := <-blockingResult; err != nil {
		t.Fatalf("Register(blocking) error = %v", err)
	}
}

func TestRegistryConvertsCollectorPanicToGatherError(t *testing.T) {
	registry := NewRegistry()
	panicErr := errors.New("collector failed")
	collector := &panicCollectCollector{
		desc:     prom.NewDesc("panic_collect_metric", "Panic", nil, nil),
		panicErr: panicErr,
	}
	if err := registry.Register(collector); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Gather(); !errors.Is(err, panicErr) {
		t.Fatalf("Gather() error = %v, want collector panic error", err)
	}
}

func TestRegistryConvertsDescribePanicToRegistrationError(t *testing.T) {
	registry := NewRegistry()
	panicErr := errors.New("describe failed")
	if err := registry.Register(&panicDescribeCollector{panicErr: panicErr}); !errors.Is(err, panicErr) {
		t.Fatalf("Register() error = %v, want Describe panic error", err)
	}
	probe := prom.NewGauge(prom.GaugeOpts{Name: "describe_panic_recovery", Help: "Recovery"})
	if err := registry.Register(probe); err != nil {
		t.Fatalf("Register() after panic error = %v", err)
	}
}

func TestRegistryDoesNotHoldRegistryLockDuringCollectorCollect(t *testing.T) {
	registry := NewRegistry()
	collector := &blockingCollectCollector{
		desc:    prom.NewDesc("collect_lock_metric", "Blocking metric", nil, nil),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	if err := registry.Register(collector); err != nil {
		t.Fatal(err)
	}
	gatherResult := make(chan error, 1)
	go func() {
		_, err := registry.Gather()
		gatherResult <- err
	}()
	<-collector.entered

	probe := prom.NewGauge(prom.GaugeOpts{Name: "collect_lock_probe", Help: "Probe"})
	probeResult := make(chan error, 1)
	go func() { probeResult <- registry.Register(probe) }()
	select {
	case err := <-probeResult:
		if err != nil {
			close(collector.release)
			<-gatherResult
			t.Fatalf("Register(probe) error = %v", err)
		}
	case <-time.After(10 * time.Second):
		close(collector.release)
		<-gatherResult
		t.Fatal("Register(probe) blocked behind an external Collect callback")
	}

	close(collector.release)
	if err := <-gatherResult; err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
}

func TestRegistryRejectsMetricDefinitionDrift(t *testing.T) {
	tests := []struct {
		name     string
		register func(*Registry) error
	}{
		{
			name: "label order",
			register: func(registry *Registry) error {
				if _, err := registry.Counter("definition_requests_total", "Requests", "method", "status"); err != nil {
					return err
				}
				_, err := registry.Counter("definition_requests_total", "Requests", "status", "method")
				return err
			},
		},
		{
			name: "histogram buckets",
			register: func(registry *Registry) error {
				if _, err := registry.Histogram("definition_duration_seconds", "Duration", []float64{0.1, 1}); err != nil {
					return err
				}
				_, err := registry.Histogram("definition_duration_seconds", "Duration", []float64{0.2, 2})
				return err
			},
		},
		{
			name: "summary objectives",
			register: func(registry *Registry) error {
				if _, err := registry.Summary("definition_payload_bytes", "Payload", map[float64]float64{0.5: 0.05}); err != nil {
					return err
				}
				_, err := registry.Summary("definition_payload_bytes", "Payload", map[float64]float64{0.9: 0.01})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.register(NewRegistry()); err == nil {
				t.Fatal("registration error = nil, want definition drift rejection")
			}
		})
	}
}

func TestRegistryRejectsReservedAndInvalidMetricConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		register func(*Registry) error
	}{
		{
			name: "histogram reserved label",
			register: func(registry *Registry) error {
				_, err := registry.Histogram("invalid_histogram", "Histogram", []float64{1}, "le")
				return err
			},
		},
		{
			name: "summary reserved label",
			register: func(registry *Registry) error {
				_, err := registry.Summary("invalid_summary", "Summary", nil, "quantile")
				return err
			},
		},
		{
			name: "unordered buckets",
			register: func(registry *Registry) error {
				_, err := registry.Histogram("invalid_buckets", "Histogram", []float64{2, 1})
				return err
			},
		},
		{
			name: "NaN bucket",
			register: func(registry *Registry) error {
				_, err := registry.Histogram("nan_bucket", "Histogram", []float64{math.NaN()})
				return err
			},
		},
		{
			name: "invalid quantile",
			register: func(registry *Registry) error {
				_, err := registry.Summary("invalid_quantile", "Summary", map[float64]float64{1: 0.01})
				return err
			},
		},
		{
			name: "invalid quantile error",
			register: func(registry *Registry) error {
				_, err := registry.Summary("invalid_quantile_error", "Summary", map[float64]float64{0.5: 0})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("registration panicked: %v", recovered)
				}
			}()
			if err := test.register(NewRegistry()); err == nil {
				t.Fatal("registration error = nil, want validation error")
			}
		})
	}
}

func TestRegistryCopiesHistogramAndSummaryConfiguration(t *testing.T) {
	registry := NewRegistry()
	buckets := []float64{1}
	histogram, err := registry.Histogram("copy_duration_seconds", "Duration", buckets)
	if err != nil {
		t.Fatal(err)
	}
	buckets[0] = 100
	if observeErr := histogram.Observe(50); observeErr != nil {
		t.Fatal(observeErr)
	}

	quantiles := map[float64]float64{0.5: 0.05}
	summary, err := registry.Summary("copy_payload_bytes", "Payload", quantiles)
	if err != nil {
		t.Fatal(err)
	}
	delete(quantiles, 0.5)
	quantiles[0.9] = 0.01
	if err := summary.Observe(1); err != nil {
		t.Fatal(err)
	}

	output := gather(t, registry)
	if !strings.Contains(output, `copy_duration_seconds_bucket{le="1"} 0`) {
		t.Fatalf("histogram buckets changed through caller-owned slice:\n%s", output)
	}
	if !strings.Contains(output, `copy_payload_bytes{quantile="0.5"}`) {
		t.Fatalf("summary objectives changed through caller-owned map:\n%s", output)
	}
	if strings.Contains(output, `copy_payload_bytes{quantile="0.9"}`) {
		t.Fatalf("summary exposed a caller mutation:\n%s", output)
	}
}

func TestRegistryCopiesLabelNames(t *testing.T) {
	registry := NewRegistry()
	labels := []string{"method"}
	counter, err := registry.Counter("copy_labels_total", "Labels", labels...)
	if err != nil {
		t.Fatal(err)
	}
	labels[0] = "status"
	if err := counter.Inc("GET"); err != nil {
		t.Fatal(err)
	}

	output := gather(t, registry)
	if !strings.Contains(output, `copy_labels_total{method="GET"} 1`) {
		t.Fatalf("metric label names changed through caller-owned slice:\n%s", output)
	}
}

func TestRegistryRejectsGeneratedSeriesNameCollision(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.Histogram("collision_duration_seconds", "Duration", []float64{1}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Counter("collision_duration_seconds_count", "Count"); err == nil {
		t.Fatal("Counter() error = nil, want generated _count series collision rejection")
	}
}

func TestRegistryPreservesAlreadyRegisteredErrorChain(t *testing.T) {
	registry := NewRegistry()
	collector := prom.NewGauge(prom.GaugeOpts{Name: "duplicate_collector", Help: "Duplicate"})
	if err := registry.Register(collector); err != nil {
		t.Fatal(err)
	}
	err := registry.Register(collector)
	var alreadyRegistered prom.AlreadyRegisteredError
	if !errors.As(err, &alreadyRegistered) {
		t.Fatalf("Register() error = %v, want AlreadyRegisteredError", err)
	}
}

func TestRegistryRejectsDuplicateUncheckedCollector(t *testing.T) {
	registry := NewRegistry()
	collector := &uncheckedCollector{}
	if err := registry.Register(collector); err != nil {
		t.Fatal(err)
	}
	err := registry.Register(collector)
	var alreadyRegistered prom.AlreadyRegisteredError
	if !errors.As(err, &alreadyRegistered) {
		t.Fatalf("Register() error = %v, want AlreadyRegisteredError", err)
	}
	if alreadyRegistered.ExistingCollector != collector {
		t.Fatalf("ExistingCollector = %T, want original collector", alreadyRegistered.ExistingCollector)
	}
}

func TestRegistryUnregistersOriginalCollectorAndAllowsReregistration(t *testing.T) {
	registry := NewRegistry()
	collector := prom.NewGauge(prom.GaugeOpts{Name: "unregister_probe", Help: "Probe"})
	collector.Set(1)
	if err := registry.Register(collector); err != nil {
		t.Fatal(err)
	}
	if !registry.Unregister(collector) {
		t.Fatal("Unregister() = false, want true")
	}
	if registry.Unregister(collector) {
		t.Fatal("second Unregister() = true, want false")
	}
	if output := gather(t, registry); strings.Contains(output, "unregister_probe") {
		t.Fatalf("unregistered metric is still gathered:\n%s", output)
	}
	if err := registry.Register(collector); err != nil {
		t.Fatalf("Register() after Unregister() error = %v", err)
	}
}

func TestRegistryConcurrentRegisterUnregisterAndGather(t *testing.T) {
	registry := NewRegistry()
	collector := prom.NewGauge(prom.GaugeOpts{Name: "concurrent_registration_probe", Help: "Probe"})
	const workers = 24
	const iterations = 100
	errorsChannel := make(chan error, workers)
	start := make(chan struct{})
	var waitGroup sync.WaitGroup
	for worker := range workers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			for range iterations {
				switch worker % 3 {
				case 0:
					err := registry.Register(collector)
					var alreadyRegistered prom.AlreadyRegisteredError
					if err != nil && !errors.As(err, &alreadyRegistered) {
						errorsChannel <- err
						return
					}
				case 1:
					registry.Unregister(collector)
				default:
					if _, err := registry.Gather(); err != nil {
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
		t.Fatalf("concurrent registry operation error = %v", err)
	}
}
