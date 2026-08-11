package prometheus

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"

	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	dto "github.com/prometheus/client_model/go"

	"github.com/hexagon-codes/toolkit/infra/observe"
)

// ErrExporterAlreadyStarted 表示同一导出器已经启动。
var ErrExporterAlreadyStarted = errors.New("prometheus: exporter already started")

// ErrExporterClosed 表示导出器已经进入终止状态。
var ErrExporterClosed = errors.New("prometheus: exporter is closed")

type exporterState uint8

const (
	exporterStateReady exporterState = iota
	exporterStateStarting
	exporterStateServing
	exporterStateClosed
)

// Exporter 通过 HTTP 暴露隔离的指标注册表。
type Exporter struct {
	namespace string
	subsystem string
	registry  *Registry
	factory   *Factory

	mu     sync.RWMutex
	server *http.Server
	state  exporterState
}

// ExporterOption 配置 Exporter。
type ExporterOption func(*Exporter)

// NewExporter 创建导出器并注册官方 Go 运行时收集器。
func NewExporter(opts ...ExporterOption) (*Exporter, error) {
	exporter := &Exporter{namespace: "app", registry: NewRegistry()}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(exporter)
	}
	if err := exporter.registry.Register(collectors.NewGoCollector()); err != nil {
		return nil, fmt.Errorf("register Go runtime metrics: %w", err)
	}
	factory, err := NewFactory(exporter.registry, exporter.namespace, exporter.subsystem)
	if err != nil {
		return nil, err
	}
	exporter.factory = factory
	return exporter, nil
}

// WithNamespace 设置应用指标命名空间。
func WithNamespace(namespace string) ExporterOption {
	return func(exporter *Exporter) { exporter.namespace = namespace }
}

// WithSubsystem 设置应用指标子系统。
func WithSubsystem(subsystem string) ExporterOption {
	return func(exporter *Exporter) { exporter.subsystem = subsystem }
}

// Registry 返回隔离的指标注册表。
func (e *Exporter) Registry() *Registry { return e.registry }

// Factory 返回应用指标工厂。
func (e *Exporter) Factory() *Factory { return e.factory }

// Handler 返回注册表的 Prometheus 处理器。
func (e *Exporter) Handler() http.Handler { return e.registry.Handler() }

// ListenAndServe 提供 /metrics 服务，直到调用 Shutdown。
func (e *Exporter) ListenAndServe(addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", e.Handler())

	e.mu.Lock()
	switch e.state {
	case exporterStateClosed:
		e.mu.Unlock()
		return ErrExporterClosed
	case exporterStateStarting, exporterStateServing:
		e.mu.Unlock()
		return ErrExporterAlreadyStarted
	}
	e.state = exporterStateStarting
	e.mu.Unlock()

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(context.Background(), "tcp", addr)
	if err != nil {
		e.mu.Lock()
		closed := e.state == exporterStateClosed
		if e.state == exporterStateStarting {
			e.state = exporterStateReady
		}
		e.mu.Unlock()
		listenErr := fmt.Errorf("listen for prometheus exporter: %w", err)
		if closed {
			return errors.Join(ErrExporterClosed, listenErr)
		}
		return listenErr
	}
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	e.mu.Lock()
	if e.state == exporterStateClosed {
		e.mu.Unlock()
		if closeErr := listener.Close(); closeErr != nil {
			return errors.Join(ErrExporterClosed, fmt.Errorf("close prometheus listener: %w", closeErr))
		}
		return ErrExporterClosed
	}
	e.server = server
	e.state = exporterStateServing
	e.mu.Unlock()

	err = server.Serve(listener)
	e.mu.Lock()
	if e.server == server && e.state != exporterStateClosed {
		e.server = nil
		e.state = exporterStateReady
	}
	e.mu.Unlock()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("serve prometheus metrics: %w", err)
	}
	return nil
}

// Shutdown 优雅停止 HTTP 服务，多次调用是安全的。
func (e *Exporter) Shutdown(ctx context.Context) error {
	if isNilValue(ctx) {
		return errors.New("prometheus: shutdown context must not be nil")
	}
	e.mu.Lock()
	e.state = exporterStateClosed
	server := e.server
	e.mu.Unlock()
	if server == nil {
		return nil
	}
	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown prometheus exporter: %w", err)
	}
	e.mu.Lock()
	if e.server == server {
		e.server = nil
	}
	e.mu.Unlock()
	return nil
}

// MetricsAdapter 将 observe.Metrics 适配到 client_golang 指标。
type MetricsAdapter struct {
	registry  *Registry
	namespace string
	subsystem string
}

// NewMetricsAdapter 创建 observe.Metrics 适配器。
func NewMetricsAdapter(registry *Registry, namespace, subsystem string) (*MetricsAdapter, error) {
	if !registry.ready() {
		return nil, ErrNilRegistry
	}
	return &MetricsAdapter{registry: registry, namespace: namespace, subsystem: subsystem}, nil
}

// Counter 获取绑定给定标签的计数器。
func (a *MetricsAdapter) Counter(name string, tags ...observe.Tag) (observe.Counter, error) {
	labelNames, labelValues, err := normalizeTags(tags)
	if err != nil {
		return nil, err
	}
	counter, err := a.registry.Counter(a.fullName(name), name, labelNames...)
	if err != nil {
		return nil, err
	}
	metric, err := counter.metric(labelValues...)
	if err != nil {
		return nil, err
	}
	return &counterAdapter{counter: metric}, nil
}

// Histogram 获取绑定给定标签的直方图。
func (a *MetricsAdapter) Histogram(name string, tags ...observe.Tag) (observe.Histogram, error) {
	labelNames, labelValues, err := normalizeTags(tags)
	if err != nil {
		return nil, err
	}
	histogram, err := a.registry.Histogram(a.fullName(name), name, DefaultBuckets(), labelNames...)
	if err != nil {
		return nil, err
	}
	observer, metric, err := histogram.metric(labelValues...)
	if err != nil {
		return nil, err
	}
	return &histogramAdapter{observer: observer, metric: metric}, nil
}

// Gauge 获取绑定给定标签的仪表。
func (a *MetricsAdapter) Gauge(name string, tags ...observe.Tag) (observe.Gauge, error) {
	labelNames, labelValues, err := normalizeTags(tags)
	if err != nil {
		return nil, err
	}
	gauge, err := a.registry.Gauge(a.fullName(name), name, labelNames...)
	if err != nil {
		return nil, err
	}
	metric, err := gauge.metric(labelValues...)
	if err != nil {
		return nil, err
	}
	return &gaugeAdapter{gauge: metric}, nil
}

// Timer 获取绑定给定标签的计时器。
func (a *MetricsAdapter) Timer(name string, tags ...observe.Tag) (observe.Timer, error) {
	histogram, err := a.Histogram(name+"_seconds", tags...)
	if err != nil {
		return nil, err
	}
	return &timerAdapter{histogram: histogram}, nil
}

func (a *MetricsAdapter) fullName(name string) string {
	return metricName(a.namespace, a.subsystem, name)
}

func normalizeTags(tags []observe.Tag) (names, values []string, resultErr error) {
	sorted := append([]observe.Tag(nil), tags...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	names = make([]string, len(sorted))
	values = make([]string, len(sorted))
	for i, tag := range sorted {
		if tag.Name == "" {
			return nil, nil, errors.New("prometheus: tag name must not be empty")
		}
		if i > 0 && tag.Name == sorted[i-1].Name {
			return nil, nil, fmt.Errorf("prometheus: duplicate tag %q", tag.Name)
		}
		names[i] = tag.Name
		values[i] = tag.Value
	}
	return names, values, nil
}

var _ observe.Metrics = (*MetricsAdapter)(nil)

type counterAdapter struct{ counter prom.Counter }

func (c *counterAdapter) Inc() { c.counter.Inc() }
func (c *counterAdapter) Add(value float64) error {
	if value < 0 {
		return ErrNegativeCounterValue
	}
	c.counter.Add(value)
	return nil
}
func (c *counterAdapter) Value() float64 {
	var metric dto.Metric
	if err := c.counter.Write(&metric); err != nil {
		return 0
	}
	return metric.GetCounter().GetValue()
}

type histogramAdapter struct {
	observer prom.Observer
	metric   prom.Metric
}

func (h *histogramAdapter) Observe(value float64) { h.observer.Observe(value) }
func (h *histogramAdapter) Count() uint64 {
	var metric dto.Metric
	if err := h.metric.Write(&metric); err != nil {
		return 0
	}
	return metric.GetHistogram().GetSampleCount()
}
func (h *histogramAdapter) Sum() float64 {
	var metric dto.Metric
	if err := h.metric.Write(&metric); err != nil {
		return 0
	}
	return metric.GetHistogram().GetSampleSum()
}

type gaugeAdapter struct{ gauge prom.Gauge }

func (g *gaugeAdapter) Set(value float64) { g.gauge.Set(value) }
func (g *gaugeAdapter) Inc()              { g.gauge.Inc() }
func (g *gaugeAdapter) Dec()              { g.gauge.Dec() }
func (g *gaugeAdapter) Add(value float64) { g.gauge.Add(value) }
func (g *gaugeAdapter) Value() float64 {
	var metric dto.Metric
	if err := g.gauge.Write(&metric); err != nil {
		return 0
	}
	return metric.GetGauge().GetValue()
}

type timerAdapter struct{ histogram observe.Histogram }

func (t *timerAdapter) ObserveDuration(duration time.Duration) {
	t.histogram.Observe(duration.Seconds())
}
func (t *timerAdapter) Time(fn func()) {
	start := time.Now()
	fn()
	t.ObserveDuration(time.Since(start))
}
func (t *timerAdapter) NewTimer() *observe.TimerContext { return observe.NewTimerContext(t) }
