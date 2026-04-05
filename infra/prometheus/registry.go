// Package prometheus 提供 Prometheus 指标导出
//
// 支持 Counter、Gauge、Histogram、Summary 等指标类型。
//
// 使用示例:
//
//	exporter := prometheus.NewExporter(
//	    prometheus.WithNamespace("myapp"),
//	)
//	http.Handle("/metrics", exporter.Handler())
//	http.ListenAndServe(":9090", nil)
package prometheus

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Registry 指标注册表
type Registry struct {
	counters   map[string]*PrometheusCounter
	gauges     map[string]*PrometheusGauge
	histograms map[string]*PrometheusHistogram
	summaries  map[string]*PrometheusSummary

	mu sync.RWMutex
}

// NewRegistry 创建注册表
func NewRegistry() *Registry {
	return &Registry{
		counters:   make(map[string]*PrometheusCounter),
		gauges:     make(map[string]*PrometheusGauge),
		histograms: make(map[string]*PrometheusHistogram),
		summaries:  make(map[string]*PrometheusSummary),
	}
}

// Counter 获取或创建 Counter
func (r *Registry) Counter(name, help string, labels ...string) *PrometheusCounter {
	r.mu.Lock()
	defer r.mu.Unlock()

	if c, ok := r.counters[name]; ok {
		return c
	}

	c := &PrometheusCounter{
		name:   name,
		help:   help,
		labels: labels,
		values: make(map[string]float64),
	}
	r.counters[name] = c
	return c
}

// Gauge 获取或创建 Gauge
func (r *Registry) Gauge(name, help string, labels ...string) *PrometheusGauge {
	r.mu.Lock()
	defer r.mu.Unlock()

	if g, ok := r.gauges[name]; ok {
		return g
	}

	g := &PrometheusGauge{
		name:   name,
		help:   help,
		labels: labels,
		values: make(map[string]float64),
	}
	r.gauges[name] = g
	return g
}

// Histogram 获取或创建 Histogram
func (r *Registry) Histogram(name, help string, buckets []float64, labels ...string) *PrometheusHistogram {
	r.mu.Lock()
	defer r.mu.Unlock()

	if h, ok := r.histograms[name]; ok {
		return h
	}

	if len(buckets) == 0 {
		buckets = DefaultBuckets
	}

	h := &PrometheusHistogram{
		name:    name,
		help:    help,
		labels:  labels,
		buckets: buckets,
		values:  make(map[string]*histogramValue),
	}
	r.histograms[name] = h
	return h
}

// Summary 获取或创建 Summary
func (r *Registry) Summary(name, help string, quantiles map[float64]float64, labels ...string) *PrometheusSummary {
	r.mu.Lock()
	defer r.mu.Unlock()

	if s, ok := r.summaries[name]; ok {
		return s
	}

	if quantiles == nil {
		quantiles = DefaultQuantiles
	}

	s := &PrometheusSummary{
		name:      name,
		help:      help,
		labels:    labels,
		quantiles: quantiles,
		values:    make(map[string]*summaryValue),
	}
	r.summaries[name] = s
	return s
}

// Gather 收集所有指标
func (r *Registry) Gather() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sb strings.Builder

	// Counters
	for _, c := range r.counters {
		sb.WriteString(c.String())
	}

	// Gauges
	for _, g := range r.gauges {
		sb.WriteString(g.String())
	}

	// Histograms
	for _, h := range r.histograms {
		sb.WriteString(h.String())
	}

	// Summaries
	for _, s := range r.summaries {
		sb.WriteString(s.String())
	}

	return sb.String()
}

// ============== Helper Functions ==============

func makeLabelKey(values []string) string {
	return strings.Join(values, "\x00")
}

func formatLabels(names []string, key string) string {
	if len(names) == 0 || key == "" {
		return ""
	}

	values := strings.Split(key, "\x00")
	if len(values) != len(names) {
		return ""
	}

	pairs := make([]string, len(names))
	for i, name := range names {
		pairs[i] = fmt.Sprintf(`%s="%s"`, name, values[i])
	}

	return "{" + strings.Join(pairs, ",") + "}"
}

func addLabel(existing, name, value string) string {
	if existing == "" {
		return fmt.Sprintf(`{%s="%s"}`, name, value)
	}

	// 去掉末尾的 }
	return existing[:len(existing)-1] + fmt.Sprintf(`,%s="%s"}`, name, value)
}

// ============== Prometheus Metrics ==============

// DefaultBuckets 默认桶
var DefaultBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

// DefaultQuantiles 默认分位数
var DefaultQuantiles = map[float64]float64{
	0.5:  0.05,
	0.9:  0.01,
	0.99: 0.001,
}

// PrometheusCounter Prometheus Counter
type PrometheusCounter struct {
	name   string
	help   string
	labels []string
	values map[string]float64
	mu     sync.RWMutex
}

// Inc 增加计数
func (c *PrometheusCounter) Inc(labelValues ...string) {
	c.Add(1, labelValues...)
}

// Add 增加指定值
func (c *PrometheusCounter) Add(v float64, labelValues ...string) {
	key := makeLabelKey(labelValues)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[key] += v
}

// String 返回 Prometheus 格式
func (c *PrometheusCounter) String() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# HELP %s %s\n", c.name, c.help))
	sb.WriteString(fmt.Sprintf("# TYPE %s counter\n", c.name))

	for key, value := range c.values {
		labels := formatLabels(c.labels, key)
		sb.WriteString(fmt.Sprintf("%s%s %v\n", c.name, labels, value))
	}

	return sb.String()
}

// PrometheusGauge Prometheus Gauge
type PrometheusGauge struct {
	name   string
	help   string
	labels []string
	values map[string]float64
	mu     sync.RWMutex
}

// Set 设置值
func (g *PrometheusGauge) Set(v float64, labelValues ...string) {
	key := makeLabelKey(labelValues)

	g.mu.Lock()
	defer g.mu.Unlock()
	g.values[key] = v
}

// Inc 增加
func (g *PrometheusGauge) Inc(labelValues ...string) {
	g.Add(1, labelValues...)
}

// Dec 减少
func (g *PrometheusGauge) Dec(labelValues ...string) {
	g.Add(-1, labelValues...)
}

// Add 增加指定值
func (g *PrometheusGauge) Add(v float64, labelValues ...string) {
	key := makeLabelKey(labelValues)

	g.mu.Lock()
	defer g.mu.Unlock()
	g.values[key] += v
}

// String 返回 Prometheus 格式
func (g *PrometheusGauge) String() string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# HELP %s %s\n", g.name, g.help))
	sb.WriteString(fmt.Sprintf("# TYPE %s gauge\n", g.name))

	for key, value := range g.values {
		labels := formatLabels(g.labels, key)
		sb.WriteString(fmt.Sprintf("%s%s %v\n", g.name, labels, value))
	}

	return sb.String()
}

// PrometheusHistogram Prometheus Histogram
type PrometheusHistogram struct {
	name    string
	help    string
	labels  []string
	buckets []float64
	values  map[string]*histogramValue
	mu      sync.RWMutex
}

type histogramValue struct {
	buckets map[float64]uint64
	sum     float64
	count   uint64
}

// Observe 观察值
func (h *PrometheusHistogram) Observe(v float64, labelValues ...string) {
	key := makeLabelKey(labelValues)

	h.mu.Lock()
	defer h.mu.Unlock()

	hv, ok := h.values[key]
	if !ok {
		hv = &histogramValue{
			buckets: make(map[float64]uint64),
		}
		for _, b := range h.buckets {
			hv.buckets[b] = 0
		}
		h.values[key] = hv
	}

	hv.sum += v
	hv.count++

	for _, b := range h.buckets {
		if v <= b {
			hv.buckets[b]++
		}
	}
}

// String 返回 Prometheus 格式
func (h *PrometheusHistogram) String() string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# HELP %s %s\n", h.name, h.help))
	sb.WriteString(fmt.Sprintf("# TYPE %s histogram\n", h.name))

	for key, hv := range h.values {
		labels := formatLabels(h.labels, key)

		// 排序桶
		sortedBuckets := make([]float64, 0, len(h.buckets))
		for b := range hv.buckets {
			sortedBuckets = append(sortedBuckets, b)
		}
		sort.Float64s(sortedBuckets)

		// 累积计数
		var cumulative uint64
		for _, b := range sortedBuckets {
			cumulative += hv.buckets[b]
			bucketLabels := addLabel(labels, "le", fmt.Sprintf("%v", b))
			sb.WriteString(fmt.Sprintf("%s_bucket%s %d\n", h.name, bucketLabels, cumulative))
		}

		// +Inf 桶
		infLabels := addLabel(labels, "le", "+Inf")
		sb.WriteString(fmt.Sprintf("%s_bucket%s %d\n", h.name, infLabels, hv.count))

		sb.WriteString(fmt.Sprintf("%s_sum%s %v\n", h.name, labels, hv.sum))
		sb.WriteString(fmt.Sprintf("%s_count%s %d\n", h.name, labels, hv.count))
	}

	return sb.String()
}

// PrometheusSummary Prometheus Summary
type PrometheusSummary struct {
	name      string
	help      string
	labels    []string
	quantiles map[float64]float64
	values    map[string]*summaryValue
	mu        sync.RWMutex
}

// maxObservations 环形缓冲区最大容量，防止无限增长导致 OOM
const maxObservations = 1000

type summaryValue struct {
	// observations 固定大小的环形缓冲区
	observations []float64
	// writePos 环形缓冲区写入位置
	writePos int
	// full 标记缓冲区是否已满（开始覆盖旧数据）
	full  bool
	sum   float64
	count uint64
}

// Observe 观察值
func (s *PrometheusSummary) Observe(v float64, labelValues ...string) {
	key := makeLabelKey(labelValues)

	s.mu.Lock()
	defer s.mu.Unlock()

	sv, ok := s.values[key]
	if !ok {
		sv = &summaryValue{
			observations: make([]float64, maxObservations),
		}
		s.values[key] = sv
	}

	// 环形缓冲区写入
	sv.observations[sv.writePos] = v
	sv.writePos++
	if sv.writePos >= maxObservations {
		sv.writePos = 0
		sv.full = true
	}
	sv.sum += v
	sv.count++
}

// String 返回 Prometheus 格式
func (s *PrometheusSummary) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# HELP %s %s\n", s.name, s.help))
	sb.WriteString(fmt.Sprintf("# TYPE %s summary\n", s.name))

	for key, sv := range s.values {
		labels := formatLabels(s.labels, key)

		// 计算分位数：从环形缓冲区中提取有效数据
		var obsLen int
		if sv.full {
			obsLen = maxObservations
		} else {
			obsLen = sv.writePos
		}
		if obsLen > 0 {
			sorted := make([]float64, obsLen)
			copy(sorted, sv.observations[:obsLen])
			sort.Float64s(sorted)

			for q := range s.quantiles {
				idx := int(float64(len(sorted)-1) * q)
				quantileLabels := addLabel(labels, "quantile", fmt.Sprintf("%v", q))
				sb.WriteString(fmt.Sprintf("%s%s %v\n", s.name, quantileLabels, sorted[idx]))
			}
		}

		sb.WriteString(fmt.Sprintf("%s_sum%s %v\n", s.name, labels, sv.sum))
		sb.WriteString(fmt.Sprintf("%s_count%s %d\n", s.name, labels, sv.count))
	}

	return sb.String()
}
