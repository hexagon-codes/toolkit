// Package prometheus 基于 client_golang 提供 Prometheus 指标能力。
package prometheus

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"

	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/expfmt"
)

var (
	// ErrNegativeCounterValue 表示计数器被传入负增量。
	ErrNegativeCounterValue = errors.New("prometheus: counter value must be non-negative")
	// ErrNilRegistry 表示构造指标组件时未提供注册表。
	ErrNilRegistry = errors.New("prometheus: registry must not be nil")
)

// DefaultBuckets 返回 client_golang 默认直方图桶的独立副本。
func DefaultBuckets() []float64 {
	return append([]float64(nil), prom.DefBuckets...)
}

// DefaultQuantiles 返回默认分位目标的独立副本。
func DefaultQuantiles() map[float64]float64 {
	return map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
}

// Registry 封装一个隔离的 client_golang 注册表。
type Registry struct {
	inner *prom.Registry
}

// NewRegistry 创建空的隔离注册表。
func NewRegistry() *Registry {
	return &Registry{inner: prom.NewRegistry()}
}

// Register 注册原生 client_golang 收集器。
func (r *Registry) Register(collector prom.Collector) error {
	return r.inner.Register(collector)
}

// Handler 返回当前注册表对应的 Prometheus HTTP 处理器。
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.inner, promhttp.HandlerOpts{EnableOpenMetrics: true})
}

// Gather 返回全部已注册指标的文本格式。
func (r *Registry) Gather() (string, error) {
	families, err := r.inner.Gather()
	if err != nil {
		return "", fmt.Errorf("gather prometheus metrics: %w", err)
	}

	var output bytes.Buffer
	for _, family := range families {
		if _, err := expfmt.MetricFamilyToText(&output, family); err != nil {
			return "", fmt.Errorf("encode prometheus metric %q: %w", family.GetName(), err)
		}
	}
	return output.String(), nil
}

// Counter 是校验标签数量的计数器向量。
type Counter struct {
	vec *prom.CounterVec
}

// Counter 获取或注册描述符完全一致的计数器。
func (r *Registry) Counter(name, help string, labels ...string) (*Counter, error) {
	vec := prom.NewCounterVec(prom.CounterOpts{Name: name, Help: help}, labels)
	collector, err := registerOrReuse(r.inner, vec)
	if err != nil {
		return nil, fmt.Errorf("register counter %q: %w", name, err)
	}
	existing, ok := collector.(*prom.CounterVec)
	if !ok {
		return nil, fmt.Errorf("register counter %q: metric name is already used by %T", name, collector)
	}
	return &Counter{vec: existing}, nil
}

// Inc 将指定标签的计数器增加一。
func (c *Counter) Inc(labelValues ...string) error {
	metric, err := c.vec.GetMetricWithLabelValues(labelValues...)
	if err != nil {
		return err
	}
	metric.Inc()
	return nil
}

// Add 将非负值增加到指定标签的计数器。
func (c *Counter) Add(value float64, labelValues ...string) error {
	if value < 0 {
		return ErrNegativeCounterValue
	}
	metric, err := c.vec.GetMetricWithLabelValues(labelValues...)
	if err != nil {
		return err
	}
	metric.Add(value)
	return nil
}

func (c *Counter) metric(labelValues ...string) (prom.Counter, error) {
	return c.vec.GetMetricWithLabelValues(labelValues...)
}

// Gauge 是校验标签数量的仪表向量。
type Gauge struct {
	vec *prom.GaugeVec
}

// Gauge 获取或注册描述符完全一致的仪表。
func (r *Registry) Gauge(name, help string, labels ...string) (*Gauge, error) {
	vec := prom.NewGaugeVec(prom.GaugeOpts{Name: name, Help: help}, labels)
	collector, err := registerOrReuse(r.inner, vec)
	if err != nil {
		return nil, fmt.Errorf("register gauge %q: %w", name, err)
	}
	existing, ok := collector.(*prom.GaugeVec)
	if !ok {
		return nil, fmt.Errorf("register gauge %q: metric name is already used by %T", name, collector)
	}
	return &Gauge{vec: existing}, nil
}

// Set 设置指定标签的仪表值。
func (g *Gauge) Set(value float64, labelValues ...string) error {
	metric, err := g.vec.GetMetricWithLabelValues(labelValues...)
	if err != nil {
		return err
	}
	metric.Set(value)
	return nil
}

// Inc 将指定标签的仪表增加一。
func (g *Gauge) Inc(labelValues ...string) error { return g.Add(1, labelValues...) }

// Dec 将指定标签的仪表减少一。
func (g *Gauge) Dec(labelValues ...string) error { return g.Add(-1, labelValues...) }

// Add 将给定值增加到指定标签的仪表。
func (g *Gauge) Add(value float64, labelValues ...string) error {
	metric, err := g.vec.GetMetricWithLabelValues(labelValues...)
	if err != nil {
		return err
	}
	metric.Add(value)
	return nil
}

func (g *Gauge) metric(labelValues ...string) (prom.Gauge, error) {
	return g.vec.GetMetricWithLabelValues(labelValues...)
}

// Histogram 是校验标签数量的直方图向量。
type Histogram struct {
	vec *prom.HistogramVec
}

// Histogram 获取或注册描述符完全一致的直方图。
func (r *Registry) Histogram(name, help string, buckets []float64, labels ...string) (*Histogram, error) {
	if len(buckets) == 0 {
		buckets = DefaultBuckets()
	}
	vec := prom.NewHistogramVec(prom.HistogramOpts{Name: name, Help: help, Buckets: buckets}, labels)
	collector, err := registerOrReuse(r.inner, vec)
	if err != nil {
		return nil, fmt.Errorf("register histogram %q: %w", name, err)
	}
	existing, ok := collector.(*prom.HistogramVec)
	if !ok {
		return nil, fmt.Errorf("register histogram %q: metric name is already used by %T", name, collector)
	}
	return &Histogram{vec: existing}, nil
}

// Observe 向指定标签的直方图记录观测值。
func (h *Histogram) Observe(value float64, labelValues ...string) error {
	metric, err := h.vec.GetMetricWithLabelValues(labelValues...)
	if err != nil {
		return err
	}
	metric.Observe(value)
	return nil
}

func (h *Histogram) metric(labelValues ...string) (prom.Observer, prom.Metric, error) {
	observer, err := h.vec.GetMetricWithLabelValues(labelValues...)
	if err != nil {
		return nil, nil, err
	}
	metric, ok := observer.(prom.Metric)
	if !ok {
		return nil, nil, fmt.Errorf("prometheus: histogram does not implement Metric")
	}
	return observer, metric, nil
}

// Summary 是校验标签数量的摘要向量。
type Summary struct {
	vec *prom.SummaryVec
}

// Summary 获取或注册描述符完全一致的摘要指标。
func (r *Registry) Summary(name, help string, quantiles map[float64]float64, labels ...string) (*Summary, error) {
	if quantiles == nil {
		quantiles = DefaultQuantiles()
	}
	vec := prom.NewSummaryVec(prom.SummaryOpts{Name: name, Help: help, Objectives: quantiles}, labels)
	collector, err := registerOrReuse(r.inner, vec)
	if err != nil {
		return nil, fmt.Errorf("register summary %q: %w", name, err)
	}
	existing, ok := collector.(*prom.SummaryVec)
	if !ok {
		return nil, fmt.Errorf("register summary %q: metric name is already used by %T", name, collector)
	}
	return &Summary{vec: existing}, nil
}

// Observe 向指定标签的摘要指标记录观测值。
func (s *Summary) Observe(value float64, labelValues ...string) error {
	metric, err := s.vec.GetMetricWithLabelValues(labelValues...)
	if err != nil {
		return err
	}
	metric.Observe(value)
	return nil
}

func registerOrReuse(registry *prom.Registry, collector prom.Collector) (prom.Collector, error) {
	if err := registry.Register(collector); err != nil {
		var alreadyRegistered prom.AlreadyRegisteredError
		if errors.As(err, &alreadyRegistered) {
			return alreadyRegistered.ExistingCollector, nil
		}
		return nil, err
	}
	return collector, nil
}
