// Package prometheus 基于 client_golang 提供 Prometheus 指标能力。
package prometheus

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"

	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

var (
	// ErrNegativeCounterValue 表示计数器被传入负增量。
	ErrNegativeCounterValue = errors.New("prometheus: counter value must be non-negative")
	// ErrNilRegistry 表示构造指标组件时未提供注册表。
	ErrNilRegistry = errors.New("prometheus: registry must not be nil")
	// ErrNilCollector 表示注册了 nil 或 typed-nil 收集器。
	ErrNilCollector = errors.New("prometheus: collector must not be nil")
	// ErrUntrackableCollector 表示收集器动态类型无法提供稳定的注册身份。
	ErrUntrackableCollector = errors.New("prometheus: collector type must be comparable")
	// ErrMetricDefinitionConflict 表示同一指标或派生序列被声明为不同定义。
	ErrMetricDefinitionConflict = errors.New("prometheus: metric definition conflicts with an existing registration")
	// ErrInvalidMetricConfiguration 表示指标名称、标签或聚合参数无效。
	ErrInvalidMetricConfiguration = errors.New("prometheus: invalid metric configuration")
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

	definitionsMu sync.Mutex
	definitions   map[string]metricDefinition
	seriesOwners  map[string]string

	collectorsMu sync.Mutex
	collectors   []collectorRegistration
}

// NewRegistry 创建空的隔离注册表。
func NewRegistry() *Registry {
	return &Registry{
		inner:        prom.NewRegistry(),
		definitions:  make(map[string]metricDefinition),
		seriesOwners: make(map[string]string),
	}
}

// Register 注册原生 client_golang 收集器。
func (r *Registry) Register(collector prom.Collector) error {
	if !r.ready() {
		return ErrNilRegistry
	}
	if isNilCollector(collector) {
		return ErrNilCollector
	}
	if !reflect.TypeOf(collector).Comparable() {
		return ErrUntrackableCollector
	}
	if existing, ok := r.registeredCollector(collector); ok {
		return duplicateCollectorError(existing.collector, collector)
	}
	descriptions, err := snapshotCollectorDescriptions(collector)
	if err != nil {
		return err
	}
	proxy := &describedCollector{collector: collector, descriptions: descriptions}
	r.collectorsMu.Lock()
	defer r.collectorsMu.Unlock()
	if existing, ok := r.registeredCollectorLocked(collector); ok {
		return duplicateCollectorError(existing.collector, collector)
	}
	if err := r.inner.Register(proxy); err != nil {
		var alreadyRegistered prom.AlreadyRegisteredError
		if errors.As(err, &alreadyRegistered) {
			existing := alreadyRegistered.ExistingCollector
			if existingProxy, ok := existing.(*describedCollector); ok {
				existing = existingProxy.collector
			}
			err = prom.AlreadyRegisteredError{
				ExistingCollector: existing,
				NewCollector:      collector,
			}
		}
		return fmt.Errorf("register prometheus collector: %w", err)
	}
	r.collectors = append(r.collectors, collectorRegistration{collector: collector, proxy: proxy})
	return nil
}

// Unregister 注销此前通过 Register 注册的原始收集器实例。
func (r *Registry) Unregister(collector prom.Collector) bool {
	if !r.ready() || isNilCollector(collector) {
		return false
	}
	r.collectorsMu.Lock()
	defer r.collectorsMu.Unlock()
	index := r.collectorIndexLocked(collector)
	if index < 0 {
		return false
	}
	registration := r.collectors[index]
	if !r.inner.Unregister(registration.proxy) {
		return false
	}
	r.collectors = slices.Delete(r.collectors, index, index+1)
	return true
}

// Handler 返回当前注册表对应的 Prometheus HTTP 处理器。
func (r *Registry) Handler() http.Handler {
	if !r.ready() {
		return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			http.Error(writer, "Prometheus registry is not initialized", http.StatusInternalServerError)
		})
	}
	return promhttp.HandlerFor(r.inner, promhttp.HandlerOpts{EnableOpenMetrics: true})
}

// Gather 返回全部已注册指标的文本格式。
func (r *Registry) Gather() (string, error) {
	if !r.ready() {
		return "", ErrNilRegistry
	}
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
	if !r.ready() {
		return nil, ErrNilRegistry
	}
	labelNames, err := normalizeVariableLabels(labels, "")
	if err != nil {
		return nil, fmt.Errorf("register counter %q: %w", name, err)
	}
	vec := prom.NewCounterVec(prom.CounterOpts{Name: name, Help: help}, labelNames)
	collector, err := r.registerMetric(name, metricDefinition{
		kind:      metricKindCounter,
		help:      help,
		labels:    labelNames,
		collector: vec,
	})
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
	if !r.ready() {
		return nil, ErrNilRegistry
	}
	labelNames, err := normalizeVariableLabels(labels, "")
	if err != nil {
		return nil, fmt.Errorf("register gauge %q: %w", name, err)
	}
	vec := prom.NewGaugeVec(prom.GaugeOpts{Name: name, Help: help}, labelNames)
	collector, err := r.registerMetric(name, metricDefinition{
		kind:      metricKindGauge,
		help:      help,
		labels:    labelNames,
		collector: vec,
	})
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
	if !r.ready() {
		return nil, ErrNilRegistry
	}
	labelNames, err := normalizeVariableLabels(labels, "le")
	if err != nil {
		return nil, fmt.Errorf("register histogram %q: %w", name, err)
	}
	if len(buckets) == 0 {
		buckets = DefaultBuckets()
	} else {
		buckets = append([]float64(nil), buckets...)
	}
	if validationErr := validateBuckets(buckets); validationErr != nil {
		return nil, fmt.Errorf("register histogram %q: %w", name, validationErr)
	}
	vec := prom.NewHistogramVec(prom.HistogramOpts{Name: name, Help: help, Buckets: buckets}, labelNames)
	collector, err := r.registerMetric(name, metricDefinition{
		kind:      metricKindHistogram,
		help:      help,
		labels:    labelNames,
		buckets:   buckets,
		collector: vec,
	})
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
	if !r.ready() {
		return nil, ErrNilRegistry
	}
	labelNames, err := normalizeVariableLabels(labels, "quantile")
	if err != nil {
		return nil, fmt.Errorf("register summary %q: %w", name, err)
	}
	if quantiles == nil {
		quantiles = DefaultQuantiles()
	} else {
		quantiles = maps.Clone(quantiles)
	}
	if validationErr := validateQuantiles(quantiles); validationErr != nil {
		return nil, fmt.Errorf("register summary %q: %w", name, validationErr)
	}
	vec := prom.NewSummaryVec(prom.SummaryOpts{Name: name, Help: help, Objectives: quantiles}, labelNames)
	collector, err := r.registerMetric(name, metricDefinition{
		kind:      metricKindSummary,
		help:      help,
		labels:    labelNames,
		quantiles: quantiles,
		collector: vec,
	})
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

type metricKind string

const (
	metricKindCounter   metricKind = "counter"
	metricKindGauge     metricKind = "gauge"
	metricKindHistogram metricKind = "histogram"
	metricKindSummary   metricKind = "summary"
)

type metricDefinition struct {
	kind      metricKind
	help      string
	labels    []string
	buckets   []float64
	quantiles map[float64]float64
	collector prom.Collector
}

func (d metricDefinition) equal(other metricDefinition) bool {
	return d.kind == other.kind &&
		d.help == other.help &&
		slices.Equal(d.labels, other.labels) &&
		slices.Equal(d.buckets, other.buckets) &&
		maps.Equal(d.quantiles, other.quantiles)
}

func (r *Registry) registerMetric(name string, definition metricDefinition) (prom.Collector, error) {
	r.definitionsMu.Lock()
	defer r.definitionsMu.Unlock()

	if existing, ok := r.definitions[name]; ok {
		if !existing.equal(definition) {
			return nil, fmt.Errorf(
				"%w: metric %q is already declared as %s",
				ErrMetricDefinitionConflict,
				name,
				existing.kind,
			)
		}
		return existing.collector, nil
	}
	for _, seriesName := range definition.seriesNames(name) {
		if owner, ok := r.seriesOwners[seriesName]; ok {
			return nil, fmt.Errorf(
				"%w: series %q is already owned by metric %q",
				ErrMetricDefinitionConflict,
				seriesName,
				owner,
			)
		}
	}
	if err := r.inner.Register(definition.collector); err != nil {
		return nil, err
	}
	definition.labels = append([]string(nil), definition.labels...)
	definition.buckets = append([]float64(nil), definition.buckets...)
	definition.quantiles = maps.Clone(definition.quantiles)
	r.definitions[name] = definition
	for _, seriesName := range definition.seriesNames(name) {
		r.seriesOwners[seriesName] = name
	}
	return definition.collector, nil
}

func (d metricDefinition) seriesNames(name string) []string {
	switch d.kind {
	case metricKindHistogram:
		return []string{name, name + "_bucket", name + "_sum", name + "_count"}
	case metricKindSummary:
		return []string{name, name + "_sum", name + "_count"}
	default:
		return []string{name}
	}
}

func normalizeVariableLabels(labels []string, forbidden string) ([]string, error) {
	result := append([]string(nil), labels...)
	seen := make(map[string]struct{}, len(result))
	for _, label := range result {
		if !model.UTF8Validation.IsValidLabelName(label) || strings.HasPrefix(label, "__") {
			return nil, fmt.Errorf("%w: label name %q is invalid", ErrInvalidMetricConfiguration, label)
		}
		if label == forbidden {
			return nil, fmt.Errorf("%w: label name %q is reserved", ErrInvalidMetricConfiguration, label)
		}
		if _, ok := seen[label]; ok {
			return nil, fmt.Errorf("%w: duplicate label name %q", ErrInvalidMetricConfiguration, label)
		}
		seen[label] = struct{}{}
	}
	return result, nil
}

func validateBuckets(buckets []float64) error {
	for index, bucket := range buckets {
		if math.IsNaN(bucket) {
			return fmt.Errorf("%w: histogram bucket must not be NaN", ErrInvalidMetricConfiguration)
		}
		if index > 0 && bucket <= buckets[index-1] {
			return fmt.Errorf(
				"%w: histogram buckets must be strictly increasing",
				ErrInvalidMetricConfiguration,
			)
		}
	}
	return nil
}

func validateQuantiles(quantiles map[float64]float64) error {
	for quantile, allowedError := range quantiles {
		if math.IsNaN(quantile) || math.IsInf(quantile, 0) || quantile <= 0 || quantile >= 1 {
			return fmt.Errorf(
				"%w: summary quantile must be between 0 and 1",
				ErrInvalidMetricConfiguration,
			)
		}
		if math.IsNaN(allowedError) || math.IsInf(allowedError, 0) || allowedError <= 0 {
			return fmt.Errorf(
				"%w: summary quantile error must be positive and finite",
				ErrInvalidMetricConfiguration,
			)
		}
	}
	return nil
}

func (r *Registry) ready() bool {
	return r != nil && r.inner != nil
}

func isNilCollector(collector prom.Collector) bool {
	return isNilValue(collector)
}

func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

type collectorRegistration struct {
	collector prom.Collector
	proxy     *describedCollector
}

func (r *Registry) registeredCollector(collector prom.Collector) (collectorRegistration, bool) {
	r.collectorsMu.Lock()
	defer r.collectorsMu.Unlock()
	return r.registeredCollectorLocked(collector)
}

func (r *Registry) registeredCollectorLocked(collector prom.Collector) (collectorRegistration, bool) {
	index := r.collectorIndexLocked(collector)
	if index < 0 {
		return collectorRegistration{}, false
	}
	return r.collectors[index], true
}

func (r *Registry) collectorIndexLocked(collector prom.Collector) int {
	for index := range r.collectors {
		if sameCollectorInstance(r.collectors[index].collector, collector) {
			return index
		}
	}
	return -1
}

func sameCollectorInstance(left, right prom.Collector) bool {
	leftType := reflect.TypeOf(left)
	if leftType == nil || leftType != reflect.TypeOf(right) || !leftType.Comparable() {
		return false
	}
	return left == right
}

func duplicateCollectorError(existing, collector prom.Collector) error {
	return fmt.Errorf("register prometheus collector: %w", prom.AlreadyRegisteredError{
		ExistingCollector: existing,
		NewCollector:      collector,
	})
}

type describedCollector struct {
	collector    prom.Collector
	descriptions []*prom.Desc
}

func (c *describedCollector) Describe(descriptions chan<- *prom.Desc) {
	for _, description := range c.descriptions {
		descriptions <- description
	}
}

func (c *describedCollector) Collect(metrics chan<- prom.Metric) {
	defer func() {
		if recovered := recover(); recovered != nil {
			var panicErr error
			switch recovered := recovered.(type) {
			case error:
				panicErr = fmt.Errorf("prometheus: collector Collect panicked: %w", recovered)
			default:
				panicErr = fmt.Errorf("prometheus: collector Collect panicked: %v", recovered)
			}
			metrics <- prom.NewInvalidMetric(prom.NewInvalidDesc(panicErr), panicErr)
		}
	}()
	c.collector.Collect(metrics)
}

func snapshotCollectorDescriptions(collector prom.Collector) ([]*prom.Desc, error) {
	descriptionChannel := make(chan *prom.Desc)
	result := make(chan error, 1)
	go func() {
		var resultErr error
		defer func() {
			if recovered := recover(); recovered != nil {
				switch recovered := recovered.(type) {
				case error:
					resultErr = fmt.Errorf("prometheus: collector Describe panicked: %w", recovered)
				default:
					resultErr = fmt.Errorf("prometheus: collector Describe panicked: %v", recovered)
				}
			}
			resultErr = errors.Join(resultErr, closeDescriptionChannel(descriptionChannel))
			result <- resultErr
		}()
		collector.Describe(descriptionChannel)
	}()

	descriptions := make([]*prom.Desc, 0)
	var descriptionErr error
	for description := range descriptionChannel {
		if description == nil {
			descriptionErr = errors.Join(
				descriptionErr,
				errors.New("prometheus: collector Describe returned a nil descriptor"),
			)
			continue
		}
		descriptions = append(descriptions, description)
	}
	if err := errors.Join(descriptionErr, <-result); err != nil {
		return nil, err
	}
	return descriptions, nil
}

// closeDescriptionChannel 隔离收集器越权关闭 Describe 通道造成的二次 panic。
func closeDescriptionChannel(descriptions chan *prom.Desc) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("prometheus: collector Describe closed the descriptor channel")
		}
	}()
	close(descriptions)
	return nil
}
