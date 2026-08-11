// Package otel 提供 OpenTelemetry 集成
//
// 支持将追踪数据导出到 Jaeger、Zipkin、OTLP 等后端。
//
// 使用示例:
//
//	tracer := otel.NewTracer(
//	    otel.WithServiceName("my-service"),
//	)
//	defer tracer.Shutdown(context.Background())
//
//	ctx, span := tracer.StartSpan(ctx, "operation")
//	defer span.End()
package otel

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/hexagon-codes/toolkit/infra/observe"
	"github.com/hexagon-codes/toolkit/util/idgen"
)

// Tracer OpenTelemetry 追踪器
type Tracer struct {
	// serviceName 服务名称
	serviceName string

	// generation 当前接收新导出的导出器代际
	generation *exporterGeneration

	// sampler 采样器
	sampler Sampler

	// spans 活跃的 Span
	spans sync.Map

	// config 配置
	config Config

	mu sync.RWMutex

	shutdownStarted  bool
	shutdownDone     chan struct{}
	shutdownErr      error
	retirements      map[*exporterGeneration]struct{}
	retirementErrors retirementErrorState

	errMu          sync.RWMutex
	firstExportErr error
	exportFailures uint64
}

// ErrTracerShutdown 表示追踪器已关闭，不能再接管新的导出器。
var ErrTracerShutdown = errors.New("tracer is shut down")

// Config OpenTelemetry 配置
type Config struct {
	// ServiceName 服务名称
	ServiceName string

	// ServiceVersion 服务版本
	ServiceVersion string

	// Environment 环境
	Environment string

	// SamplingRate 采样率（0-1）
	SamplingRate float64
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		ServiceName:    "default",
		ServiceVersion: "1.0.0",
		Environment:    "development",
		SamplingRate:   1.0,
	}
}

// Option 配置选项
type Option func(*Config)

// WithServiceName 设置服务名称
func WithServiceName(name string) Option {
	return func(c *Config) {
		c.ServiceName = name
	}
}

// WithServiceVersion 设置服务版本
func WithServiceVersion(version string) Option {
	return func(c *Config) {
		c.ServiceVersion = version
	}
}

// WithEnvironment 设置环境
func WithEnvironment(env string) Option {
	return func(c *Config) {
		c.Environment = env
	}
}

// WithSamplingRate 设置采样率
func WithSamplingRate(rate float64) Option {
	return func(c *Config) {
		c.SamplingRate = rate
	}
}

// NewTracer 创建 OpenTelemetry 追踪器
func NewTracer(opts ...Option) *Tracer {
	config := DefaultConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&config)
		}
	}

	t := &Tracer{
		serviceName:  config.ServiceName,
		sampler:      NewProbabilitySampler(config.SamplingRate),
		config:       config,
		shutdownDone: make(chan struct{}),
		retirements:  make(map[*exporterGeneration]struct{}),
	}

	return t
}

// SetExporter 设置并接管导出器所有权。
//
// 调用开始即转移 exporter 所有权，无论本方法返回 nil 还是 error，调用方都不得
// 继续使用或关闭 exporter。替换时新导出器立即生效，旧导出器会被关闭；旧导出器
// 的关闭错误会返回，但不会撤销替换。Tracer 已关闭时，传入的导出器也会被关闭，
// 避免其后台任务泄漏。ctx 只约束本次等待；超时或取消后，已接管导出器的退役仍会
// 在后台继续，调用方不得重新关闭它。
func (t *Tracer) SetExporter(ctx context.Context, exporter Exporter) error {
	if isNilExporter(exporter) {
		exporter = nil
	}
	t.mu.Lock()
	if t.shutdownStarted {
		t.mu.Unlock()
		return errors.Join(ErrTracerShutdown, retireRejectedExporter(ctx, exporter))
	}
	if sameExporter(exporterFromGeneration(t.generation), exporter) {
		t.mu.Unlock()
		return nil
	}
	previous := t.generation
	if exporter == nil {
		t.generation = nil
	} else {
		t.generation = newExporterGeneration(exporter)
	}
	if previous != nil {
		previous.beginDrain()
		if t.retirements == nil {
			t.retirements = make(map[*exporterGeneration]struct{})
		}
		t.retirements[previous] = struct{}{}
	}
	t.mu.Unlock()

	if previous == nil {
		return nil
	}
	previous.startRetirement(context.Background(), nil, t.reapRetirement)
	_, err := previous.waitRetirement(ctx)
	if err != nil {
		return fmt.Errorf("shutdown previous exporter: %w", err)
	}
	return nil
}

func exporterFromGeneration(generation *exporterGeneration) Exporter {
	if generation == nil {
		return nil
	}
	return generation.exporter
}

func retireRejectedExporter(ctx context.Context, exporter Exporter) error {
	if exporter == nil {
		return nil
	}
	generation := newExporterGeneration(exporter)
	generation.beginDrain()
	generation.startRetirement(context.Background(), nil, nil)
	_, err := generation.waitRetirement(ctx)
	if err != nil {
		return fmt.Errorf("shutdown rejected exporter: %w", err)
	}
	return nil
}

// reapRetirement 在代际完成后先摘除对象并记入有界错误历史，再允许等待者继续。
func (t *Tracer) reapRetirement(generation *exporterGeneration, err error) {
	t.mu.Lock()
	delete(t.retirements, generation)
	t.retirementErrors.add(err)
	t.mu.Unlock()
}

func sameExporter(first, second Exporter) bool {
	firstNil := isNilExporter(first)
	secondNil := isNilExporter(second)
	if firstNil || secondNil {
		return firstNil && secondNil
	}
	firstValue := reflect.ValueOf(first)
	secondValue := reflect.ValueOf(second)
	return firstValue.Type() == secondValue.Type() &&
		firstValue.Type().Comparable() &&
		firstValue.Interface() == secondValue.Interface()
}

func isNilExporter(exporter Exporter) bool {
	if exporter == nil {
		return true
	}
	value := reflect.ValueOf(exporter)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// StartSpan 开始新的 Span
func (t *Tracer) StartSpan(ctx context.Context, name string, opts ...observe.SpanOption) (context.Context, observe.Span) {
	// 应用选项
	cfg := &observe.SpanConfig{
		Attributes: make(map[string]any),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	// 生成 ID
	traceID := t.ExtractTraceID(ctx)
	if traceID == "" {
		traceID = idgen.NanoID()
	}
	spanID := idgen.NanoID()

	// 获取父 Span
	var parentSpanID string
	if parentSpan := observe.SpanFromContext(ctx); parentSpan != nil {
		parentSpanID = parentSpan.SpanID()
	}

	// 采样决策
	shouldSample := t.sampler.ShouldSample(traceID, name)
	startTime := cfg.StartTime
	if startTime.IsZero() {
		startTime = time.Now()
	}

	// 创建 Span
	span := &Span{
		tracer:       t,
		traceID:      traceID,
		spanID:       spanID,
		parentSpanID: parentSpanID,
		name:         name,
		kind:         cfg.Kind,
		startTime:    startTime,
		attributes:   make(map[string]any),
		events:       make([]SpanEvent, 0),
		status:       observe.StatusCodeUnset,
		recording:    shouldSample,
	}

	// 设置初始属性
	for k, v := range cfg.Attributes {
		span.attributes[k] = v
	}

	// 添加资源属性
	span.attributes["service.name"] = t.serviceName
	span.attributes["service.version"] = t.config.ServiceVersion
	span.attributes["deployment.environment"] = t.config.Environment

	// 关闭开始后不再接收新的记录型 Span，避免留下无法导出的活跃数据。
	t.mu.Lock()
	if t.shutdownStarted {
		span.recording = false
	} else {
		t.spans.Store(spanID, span)
	}
	t.mu.Unlock()

	// 更新 context
	ctx = observe.ContextWithSpan(ctx, span)
	ctx = t.InjectTraceID(ctx, traceID)

	return ctx, span
}

// ExtractTraceID 提取 Trace ID
func (t *Tracer) ExtractTraceID(ctx context.Context) string {
	if traceID, ok := ctx.Value(traceIDKey{}).(string); ok {
		return traceID
	}
	return ""
}

// InjectTraceID 注入 Trace ID
func (t *Tracer) InjectTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// Shutdown 关闭追踪器。
//
// 首次调用会原子终止全部活跃 Span 并启动所有导出器代际的退役。ctx 约束当前
// 调用的等待时间；代际排空并刷新最终 Span 后，导出器使用该 ctx 停止自有任务。
func (t *Tracer) Shutdown(ctx context.Context) error {
	t.mu.Lock()
	if t.shutdownDone == nil {
		t.shutdownDone = make(chan struct{})
	}
	if !t.shutdownStarted {
		t.shutdownStarted = true
		current := t.generation
		t.generation = nil
		if current != nil {
			current.beginDrain()
			if t.retirements == nil {
				t.retirements = make(map[*exporterGeneration]struct{})
			}
			t.retirements[current] = struct{}{}
		}

		shutdownTime := time.Now()
		spans := make([]*SpanData, 0)
		t.spans.Range(func(key, value any) bool {
			if span, ok := value.(*Span); ok {
				if data, finalized := span.finalize(shutdownTime); finalized && span.recording && current != nil {
					spans = append(spans, data)
				}
			}
			t.spans.Delete(key)
			return true
		})

		generations := make([]*exporterGeneration, 0, len(t.retirements))
		for generation := range t.retirements {
			generations = append(generations, generation)
		}
		t.mu.Unlock()

		for _, generation := range generations {
			if generation == current {
				generation.startRetirement(ctx, spans, t.reapRetirement)
			} else {
				generation.startRetirement(ctx, nil, t.reapRetirement)
			}
		}
		go t.completeShutdown(generations)
	} else {
		t.mu.Unlock()
	}

	return t.waitShutdown(ctx)
}

func (t *Tracer) completeShutdown(generations []*exporterGeneration) {
	for _, generation := range generations {
		if _, err := generation.waitRetirement(context.Background()); err != nil {
			continue
		}
	}
	exportErr := t.exportErrorSnapshot()

	t.mu.Lock()
	t.shutdownErr = errors.Join(exportErr, t.retirementErrors.snapshot())
	close(t.shutdownDone)
	t.mu.Unlock()
}

func (t *Tracer) waitShutdown(ctx context.Context) error {
	t.mu.RLock()
	done := t.shutdownDone
	t.mu.RUnlock()
	select {
	case <-done:
		return t.shutdownError()
	default:
	}
	select {
	case <-done:
		return t.shutdownError()
	case <-ctx.Done():
		return errors.Join(
			t.exportErrorSnapshot(),
			t.retirementErrorSnapshot(),
			fmt.Errorf("wait for tracer shutdown: %w", ctx.Err()),
		)
	}
}

func (t *Tracer) retirementErrorSnapshot() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.retirementErrors.snapshot()
}

func (t *Tracer) shutdownError() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.shutdownErr
}

type traceIDKey struct{}

// Span OpenTelemetry Span 实现
type Span struct {
	tracer       *Tracer
	traceID      string
	spanID       string
	parentSpanID string
	name         string
	kind         observe.SpanKind
	startTime    time.Time
	endTime      time.Time
	attributes   map[string]any
	events       []SpanEvent
	status       observe.StatusCode
	statusMsg    string
	input        any
	output       any
	tokenUsage   observe.TokenUsage
	recording    bool
	ended        bool

	mu sync.RWMutex
}

// SpanEvent Span 事件
type SpanEvent struct {
	Name       string         `json:"name"`
	Timestamp  time.Time      `json:"timestamp"`
	Attributes map[string]any `json:"attributes"`
}

// SpanID 返回 Span ID
func (s *Span) SpanID() string {
	return s.spanID
}

// TraceID 返回 Trace ID
func (s *Span) TraceID() string {
	return s.traceID
}

// SetName 设置名称
func (s *Span) SetName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.name = name
}

// SetInput 设置输入
func (s *Span) SetInput(input any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.input = input
	s.attributes["input"] = input
}

// SetOutput 设置输出
func (s *Span) SetOutput(output any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.output = output
	s.attributes["output"] = output
}

// SetTokenUsage 设置 Token 使用量
func (s *Span) SetTokenUsage(usage observe.TokenUsage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokenUsage = usage
	s.attributes[observe.AttrLLMPromptTokens] = usage.PromptTokens
	s.attributes[observe.AttrLLMCompletionTokens] = usage.CompletionTokens
	s.attributes[observe.AttrLLMTotalTokens] = usage.TotalTokens
}

// SetAttribute 设置属性
func (s *Span) SetAttribute(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attributes[key] = value
}

// SetAttributes 批量设置属性
func (s *Span) SetAttributes(attrs map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range attrs {
		s.attributes[k] = v
	}
}

// AddEvent 添加事件
func (s *Span) AddEvent(name string, attrs ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	event := SpanEvent{
		Name:       name,
		Timestamp:  time.Now(),
		Attributes: make(map[string]any),
	}

	// 解析属性
	for i := 0; i < len(attrs)-1; i += 2 {
		if key, ok := attrs[i].(string); ok {
			event.Attributes[key] = attrs[i+1]
		}
	}

	s.events = append(s.events, event)
}

// RecordError 记录错误
func (s *Span) RecordError(err error) {
	if err == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.attributes[observe.AttrErrorType] = fmt.Sprintf("%T", err)
	s.attributes[observe.AttrErrorMessage] = err.Error()

	s.events = append(s.events, SpanEvent{
		Name:      "exception",
		Timestamp: time.Now(),
		Attributes: map[string]any{
			"exception.type":    fmt.Sprintf("%T", err),
			"exception.message": err.Error(),
		},
	})
}

// SetStatus 设置状态
func (s *Span) SetStatus(code observe.StatusCode, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = code
	s.statusMsg = message
}

// End 结束 Span
func (s *Span) End() {
	s.tracer.mu.Lock()
	data, finalized := s.finalize(time.Now())
	if !finalized {
		s.tracer.mu.Unlock()
		return
	}
	s.tracer.spans.Delete(s.spanID)
	var lease *exporterLease
	if s.recording && s.tracer.generation != nil {
		lease = s.tracer.generation.acquire()
	}
	s.tracer.mu.Unlock()

	if lease == nil {
		return
	}
	defer lease.release()
	s.tracer.recordExportError(lease.generation.exporter.ExportSpans(context.Background(), []*SpanData{data}))
}

func (t *Tracer) recordExportError(err error) {
	if err == nil {
		return
	}
	t.errMu.Lock()
	defer t.errMu.Unlock()
	t.exportFailures++
	if t.firstExportErr == nil {
		t.firstExportErr = err
	}
}

func (t *Tracer) exportErrorSnapshot() error {
	t.errMu.RLock()
	defer t.errMu.RUnlock()
	if t.firstExportErr == nil {
		return nil
	}
	return fmt.Errorf("span export failed %d times: %w", t.exportFailures, t.firstExportErr)
}

// EndWithError 结束并记录错误
func (s *Span) EndWithError(err error) {
	s.RecordError(err)
	s.SetStatus(observe.StatusCodeError, err.Error())
	s.End()
}

// IsRecording 是否正在记录
func (s *Span) IsRecording() bool {
	return s.recording
}

// toSpanData 转换为导出数据
func (s *Span) toSpanData() *SpanData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.toSpanDataLocked()
}

func (s *Span) finalize(endTime time.Time) (*SpanData, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return nil, false
	}
	s.ended = true
	s.endTime = endTime
	return s.toSpanDataLocked(), true
}

func (s *Span) toSpanDataLocked() *SpanData {

	events := make([]SpanEvent, len(s.events))
	copy(events, s.events)

	attrs := make(map[string]any)
	for k, v := range s.attributes {
		attrs[k] = v
	}

	return &SpanData{
		TraceID:      s.traceID,
		SpanID:       s.spanID,
		ParentSpanID: s.parentSpanID,
		Name:         s.name,
		Kind:         s.kind,
		StartTime:    s.startTime,
		EndTime:      s.endTime,
		Attributes:   attrs,
		Events:       events,
		Status:       s.status,
		StatusMsg:    s.statusMsg,
	}
}

// SpanData 导出数据
type SpanData struct {
	TraceID      string             `json:"trace_id"`
	SpanID       string             `json:"span_id"`
	ParentSpanID string             `json:"parent_span_id,omitempty"`
	Name         string             `json:"name"`
	Kind         observe.SpanKind   `json:"kind"`
	StartTime    time.Time          `json:"start_time"`
	EndTime      time.Time          `json:"end_time"`
	Attributes   map[string]any     `json:"attributes"`
	Events       []SpanEvent        `json:"events"`
	Status       observe.StatusCode `json:"status"`
	StatusMsg    string             `json:"status_message,omitempty"`
}

// 确保实现了接口
var _ observe.Tracer = (*Tracer)(nil)
var _ observe.Span = (*Span)(nil)
