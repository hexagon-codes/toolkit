package otel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hexagon-codes/toolkit/infra/observe"
)

// ============================================================
// W3C 传播器 round-trip 审计
// ============================================================

// auditError 测试用错误类型
type auditError struct{ msg string }

func (e *auditError) Error() string { return e.msg }

// startRecordingSpan 创建一个正在记录的 span 并放入 ctx，便于注入测试
func startRecordingSpan(t *testing.T, traceID, spanID string) context.Context {
	t.Helper()
	sp := &OTelSpan{
		traceID:   traceID,
		spanID:    spanID,
		recording: true,
	}
	return observe.ContextWithSpan(context.Background(), sp)
}

// TestW3CInjectExtractRoundTrip 验证 W3C 传播器注入后再提取能还原 traceID。
// 这是分布式追踪最核心的闭环：上游 Inject -> 下游 Extract 必须拿到同一个 traceID。
func TestW3CInjectExtractRoundTrip(t *testing.T) {
	prop := NewW3CTraceContextPropagator()

	const traceID = "trace123abc"
	const spanID = "span456def"

	// 上游注入
	carrier := MapCarrier{}
	ctx := startRecordingSpan(t, traceID, spanID)
	prop.Inject(ctx, carrier)

	tp := carrier.Get("traceparent")
	if tp == "" {
		t.Fatalf("Inject 未写入 traceparent header")
	}
	// 形如 00-traceID-spanID-01
	if !strings.HasPrefix(tp, "00-") {
		t.Errorf("traceparent 前缀错误: %q", tp)
	}

	// 下游提取
	newCtx := prop.Extract(context.Background(), carrier)
	got, ok := newCtx.Value(traceIDKey{}).(string)
	if !ok {
		t.Fatalf("Extract 未向 ctx 注入 traceID，traceparent=%q", tp)
	}
	if got != traceID {
		t.Errorf("round-trip traceID 不一致: 注入 %q, 提取 %q (traceparent=%q)", traceID, got, tp)
	}
}

// TestW3CExtractTable W3C Extract 表驱动：各类 traceparent 输入。
func TestW3CExtractTable(t *testing.T) {
	prop := NewW3CTraceContextPropagator()

	tests := []struct {
		name        string
		traceparent string
		wantTraceID string // 期望提取出的标准 traceID
		wantPresent bool   // 是否期望 ctx 中存在 traceID
	}{
		{"标准格式", "00-aaaabbbbccccdddd-1111222233334444-01", "aaaabbbbccccdddd", true},
		{"空header", "", "", false},
		{"仅版本", "00", "", false},
		{"缺字段", "00-onlytrace", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			carrier := MapCarrier{}
			if tt.traceparent != "" {
				carrier.Set("traceparent", tt.traceparent)
			}
			ctx := prop.Extract(context.Background(), carrier)
			got, ok := ctx.Value(traceIDKey{}).(string)
			if ok != tt.wantPresent {
				t.Errorf("traceID 存在性: got present=%v, want %v (提取值=%q)", ok, tt.wantPresent, got)
			}
			if tt.wantPresent && got != tt.wantTraceID {
				t.Errorf("提取 traceID = %q, want %q", got, tt.wantTraceID)
			}
		})
	}
}

// TestB3InjectExtractRoundTrip B3 传播器闭环验证。
func TestB3InjectExtractRoundTrip(t *testing.T) {
	prop := NewB3Propagator()
	const traceID = "b3trace999"
	const spanID = "b3span888"

	carrier := MapCarrier{}
	ctx := startRecordingSpan(t, traceID, spanID)
	prop.Inject(ctx, carrier)

	if carrier.Get("X-B3-TraceId") != traceID {
		t.Errorf("X-B3-TraceId = %q, want %q", carrier.Get("X-B3-TraceId"), traceID)
	}
	if carrier.Get("X-B3-SpanId") != spanID {
		t.Errorf("X-B3-SpanId = %q, want %q", carrier.Get("X-B3-SpanId"), spanID)
	}
	if carrier.Get("X-B3-Sampled") != "1" {
		t.Errorf("X-B3-Sampled = %q, want 1", carrier.Get("X-B3-Sampled"))
	}

	got := prop.Extract(context.Background(), carrier).Value(traceIDKey{})
	if got != traceID {
		t.Errorf("B3 round-trip traceID = %v, want %q", got, traceID)
	}
}

// TestB3ExtractEmpty 空 header 不应注入。
func TestB3ExtractEmpty(t *testing.T) {
	prop := NewB3Propagator()
	ctx := prop.Extract(context.Background(), MapCarrier{})
	if ctx.Value(traceIDKey{}) != nil {
		t.Error("B3 Extract 空 header 不应注入 traceID")
	}
}

// TestPropagatorInjectNilSpan span 为 nil 时 Inject 不应写入任何 header，也不应 panic。
func TestPropagatorInjectNilSpan(t *testing.T) {
	for _, prop := range []Propagator{NewW3CTraceContextPropagator(), NewB3Propagator()} {
		carrier := MapCarrier{}
		prop.Inject(context.Background(), carrier) // ctx 中无 span
		if len(carrier) != 0 {
			t.Errorf("%T: 无 span 时不应写入 header, got %v", prop, map[string]string(carrier))
		}
	}
}

// TestCompositePropagator 组合传播器同时注入 W3C + B3。
func TestCompositePropagator(t *testing.T) {
	comp := NewCompositePropagator(NewW3CTraceContextPropagator(), NewB3Propagator())
	const traceID = "comptrace"
	ctx := startRecordingSpan(t, traceID, "compspan")

	carrier := MapCarrier{}
	comp.Inject(ctx, carrier)

	if carrier.Get("traceparent") == "" {
		t.Error("组合传播器应注入 W3C traceparent")
	}
	if carrier.Get("X-B3-TraceId") != traceID {
		t.Error("组合传播器应注入 B3 header")
	}

	// 提取：至少 B3 能成功还原 traceID
	out := comp.Extract(context.Background(), carrier)
	if out.Value(traceIDKey{}) == nil {
		t.Error("组合传播器 Extract 应注入 traceID")
	}
}

// ============================================================
// 采样器审计
// ============================================================

// TestProbabilitySamplerClamp 边界裁剪。
func TestProbabilitySamplerClamp(t *testing.T) {
	tests := []struct {
		in   float64
		want float64
	}{
		{-100, 0},
		{-0.0001, 0},
		{0, 0},
		{0.5, 0.5},
		{1, 1},
		{1.0001, 1},
		{999, 1},
	}
	for _, tt := range tests {
		s := NewProbabilitySampler(tt.in)
		if s.rate != tt.want {
			t.Errorf("NewProbabilitySampler(%v).rate = %v, want %v", tt.in, s.rate, tt.want)
		}
	}
}

// TestProbabilitySamplerDeterministic 同一 traceID 多次调用结果必须一致。
func TestProbabilitySamplerDeterministic(t *testing.T) {
	s := NewProbabilitySampler(0.5)
	first := s.ShouldSample("deterministic-trace-id", "op")
	for i := 0; i < 100; i++ {
		if s.ShouldSample("deterministic-trace-id", "op") != first {
			t.Fatal("同一 traceID 采样结果不确定")
		}
	}
}

// TestProbabilitySamplerDistribution 大量随机 traceID 下采样率接近设定值。
func TestProbabilitySamplerDistribution(t *testing.T) {
	const rate = 0.3
	s := NewProbabilitySampler(rate)
	const N = 50000
	hit := 0
	for i := 0; i < N; i++ {
		if s.ShouldSample(fmt.Sprintf("trace-%d-distribution", i), "op") {
			hit++
		}
	}
	got := float64(hit) / float64(N)
	// 允许 ±5% 误差
	if got < rate-0.05 || got > rate+0.05 {
		t.Errorf("采样率偏离: got %.3f, want ~%.2f", got, rate)
	}
}

// TestProbabilitySamplerEmptyTraceID 空 traceID 不应 panic，且确定。
func TestProbabilitySamplerEmptyTraceID(t *testing.T) {
	s := NewProbabilitySampler(0.5)
	// 空字符串 hash=0, 0/10000=0 < 0.5 => true
	got := s.ShouldSample("", "op")
	if !got {
		t.Errorf("空 traceID hash=0, 期望被采样 (0 < 0.5)，got %v", got)
	}
}

// TestProbabilitySamplerUnicode Unicode traceID 不应 panic。
func TestProbabilitySamplerUnicode(t *testing.T) {
	s := NewProbabilitySampler(0.5)
	_ = s.ShouldSample("追踪-😀-标识符-🌍", "操作")
}

// TestRateLimitingSamplerBudget 初始预算允许若干请求后耗尽。
func TestRateLimitingSamplerBudget(t *testing.T) {
	// rate=5 => 初始 budget=5，连续调用应允许约 5 个后开始拒绝（同一瞬间几乎无补充）
	s := NewRateLimitingSampler(5)
	allowed := 0
	for i := 0; i < 20; i++ {
		if s.ShouldSample("t", "op") {
			allowed++
		}
	}
	// 同一瞬间补充几乎为 0，初始预算 5 => 允许约 5 个
	if allowed < 4 || allowed > 7 {
		t.Errorf("限流器瞬时允许数 = %d, 期望约等于初始 rate 5", allowed)
	}
}

// TestRateLimitingSamplerRefill 等待后预算补充。
func TestRateLimitingSamplerRefill(t *testing.T) {
	s := NewRateLimitingSampler(10)
	// 先耗尽
	for i := 0; i < 15; i++ {
		s.ShouldSample("t", "op")
	}
	// 等待补充
	time.Sleep(200 * time.Millisecond)
	if !s.ShouldSample("t", "op") {
		t.Error("等待后预算应已补充，期望可采样")
	}
}

// TestRateLimitingSamplerConcurrent 并发竞态：mutex 保护下不应 panic / data race。
func TestRateLimitingSamplerConcurrent(t *testing.T) {
	s := NewRateLimitingSampler(100)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				s.ShouldSample("concurrent-trace", "op")
			}
		}()
	}
	wg.Wait()
}

// TestRateLimitingSamplerZeroRate rate=0 时永不采样。
func TestRateLimitingSamplerZeroRate(t *testing.T) {
	s := NewRateLimitingSampler(0)
	for i := 0; i < 10; i++ {
		if s.ShouldSample("t", "op") {
			t.Error("rate=0 不应采样")
		}
	}
}

// TestRateLimitingSamplerNegativeRate rate 为负的行为审计（无裁剪保护）。
func TestRateLimitingSamplerNegativeRate(t *testing.T) {
	s := NewRateLimitingSampler(-5)
	// budget 初始为 -5，永远 < 1，不应采样；不应 panic
	for i := 0; i < 10; i++ {
		if s.ShouldSample("t", "op") {
			t.Error("rate<0 不应采样")
		}
	}
}

// TestAlwaysNeverSampler 平凡采样器。
func TestAlwaysNeverSampler(t *testing.T) {
	if !(&AlwaysSampler{}).ShouldSample("", "") {
		t.Error("AlwaysSampler 应恒为 true")
	}
	if (&NeverSampler{}).ShouldSample("any", "any") {
		t.Error("NeverSampler 应恒为 false")
	}
}

// ============================================================
// MapCarrier 审计
// ============================================================

// TestMapCarrierKeys 键集合正确性。
func TestMapCarrierKeys(t *testing.T) {
	c := MapCarrier{"a": "1", "b": "2", "c": "3"}
	keys := c.Keys()
	if len(keys) != 3 {
		t.Fatalf("Keys 长度 = %d, want 3", len(keys))
	}
	set := map[string]bool{}
	for _, k := range keys {
		set[k] = true
	}
	for _, want := range []string{"a", "b", "c"} {
		if !set[want] {
			t.Errorf("Keys 缺少 %q", want)
		}
	}
}

// TestMapCarrierEmptyAndMissing 空载体与缺失键。
func TestMapCarrierEmptyAndMissing(t *testing.T) {
	c := MapCarrier{}
	if c.Get("nope") != "" {
		t.Error("缺失键应返回空串")
	}
	if len(c.Keys()) != 0 {
		t.Error("空载体 Keys 应为空")
	}
	// 覆盖写
	c.Set("k", "v1")
	c.Set("k", "v2")
	if c.Get("k") != "v2" {
		t.Errorf("覆盖写后 = %q, want v2", c.Get("k"))
	}
}

// ============================================================
// Tracer / Span 审计
// ============================================================

// TestStartSpanParentChild 父子 span：子 span 应继承父 traceID 并记录 parentSpanID。
func TestStartSpanParentChild(t *testing.T) {
	tr := NewOTelTracer(WithServiceName("svc"))
	ctx := context.Background()

	ctx, parent := tr.StartSpan(ctx, "parent")
	parentTrace := parent.TraceID()
	parentSpanID := parent.SpanID()

	_, child := tr.StartSpan(ctx, "child")
	if child.TraceID() != parentTrace {
		t.Errorf("子 span traceID = %q, 应继承父 %q", child.TraceID(), parentTrace)
	}
	cs := child.(*OTelSpan)
	if cs.parentSpanID != parentSpanID {
		t.Errorf("子 span parentSpanID = %q, want %q", cs.parentSpanID, parentSpanID)
	}
	parent.End()
	child.End()
}

// TestStartSpanResourceAttributes StartSpan 应注入服务资源属性。
func TestStartSpanResourceAttributes(t *testing.T) {
	tr := NewOTelTracer(
		WithServiceName("my-svc"),
		WithServiceVersion("3.1.4"),
		WithEnvironment("staging"),
	)
	_, span := tr.StartSpan(context.Background(), "op")
	s := span.(*OTelSpan)
	if s.attributes["service.name"] != "my-svc" {
		t.Errorf("service.name = %v", s.attributes["service.name"])
	}
	if s.attributes["service.version"] != "3.1.4" {
		t.Errorf("service.version = %v", s.attributes["service.version"])
	}
	if s.attributes["deployment.environment"] != "staging" {
		t.Errorf("deployment.environment = %v", s.attributes["deployment.environment"])
	}
	span.End()
}

// TestStartSpanInitialAttributes WithAttributes 初始属性应进入 span。
func TestStartSpanInitialAttributes(t *testing.T) {
	tr := NewOTelTracer()
	_, span := tr.StartSpan(context.Background(), "op",
		observe.WithAttributes(map[string]any{"custom": "x"}),
		observe.WithSpanKind(observe.SpanKindServer),
	)
	s := span.(*OTelSpan)
	if s.attributes["custom"] != "x" {
		t.Error("初始属性 custom 丢失")
	}
	if s.kind != observe.SpanKindServer {
		t.Errorf("kind = %v, want Server", s.kind)
	}
	span.End()
}

// TestSpanStoreCleanupOnEnd span 结束后应从 tracer.spans 中移除。
func TestSpanStoreCleanupOnEnd(t *testing.T) {
	tr := NewOTelTracer()
	_, span := tr.StartSpan(context.Background(), "op")
	id := span.SpanID()

	if _, ok := tr.spans.Load(id); !ok {
		t.Fatal("StartSpan 后 span 应存于 tracer.spans")
	}
	span.End()
	if _, ok := tr.spans.Load(id); ok {
		t.Error("End 后 span 应从 tracer.spans 移除")
	}
}

// TestSpanEndIdempotent 重复 End 应幂等，不重复导出。
func TestSpanEndIdempotent(t *testing.T) {
	tr := NewOTelTracer(WithSamplingRate(1.0))
	exp := &countingExporter{}
	tr.SetExporter(exp)

	_, span := tr.StartSpan(context.Background(), "op")
	span.End()
	span.End()
	span.End()

	if n := atomic.LoadInt32(&exp.exportCalls); n != 1 {
		t.Errorf("重复 End 导出次数 = %d, want 1 (幂等)", n)
	}
}

// TestSpanNotRecordingNoExport 未采样的 span 结束时不应导出。
func TestSpanNotRecordingNoExport(t *testing.T) {
	tr := NewOTelTracer(WithSamplingRate(0.0)) // NeverSample via probability 0
	exp := &countingExporter{}
	tr.SetExporter(exp)

	_, span := tr.StartSpan(context.Background(), "op")
	if span.IsRecording() {
		t.Error("采样率 0 时 span 不应记录")
	}
	span.End()
	if n := atomic.LoadInt32(&exp.exportCalls); n != 0 {
		t.Errorf("未采样 span 导出次数 = %d, want 0", n)
	}
}

// TestSpanRecordErrorNil RecordError(nil) 不应记录任何东西。
func TestSpanRecordErrorNil(t *testing.T) {
	tr := NewOTelTracer()
	_, span := tr.StartSpan(context.Background(), "op")
	s := span.(*OTelSpan)
	before := len(s.events)
	span.RecordError(nil)
	if len(s.events) != before {
		t.Error("RecordError(nil) 不应添加事件")
	}
	span.End()
}

// TestSpanRecordErrorAttrs RecordError 应记录属性与 exception 事件。
func TestSpanRecordErrorAttrs(t *testing.T) {
	tr := NewOTelTracer()
	_, span := tr.StartSpan(context.Background(), "op")
	s := span.(*OTelSpan)
	span.RecordError(&auditError{"boom"})
	if s.attributes[observe.AttrErrorMessage] != "boom" {
		t.Errorf("error.message = %v", s.attributes[observe.AttrErrorMessage])
	}
	found := false
	for _, e := range s.events {
		if e.Name == "exception" {
			found = true
		}
	}
	if !found {
		t.Error("缺少 exception 事件")
	}
	span.End()
}

// TestSpanEndWithError EndWithError 应设置错误状态并结束。
func TestSpanEndWithError(t *testing.T) {
	tr := NewOTelTracer()
	_, span := tr.StartSpan(context.Background(), "op")
	s := span.(*OTelSpan)
	span.EndWithError(&auditError{"fatal"})
	if s.status != observe.StatusCodeError {
		t.Errorf("status = %v, want Error", s.status)
	}
	if !s.ended {
		t.Error("EndWithError 后应已结束")
	}
}

// TestSpanAddEventOddAttrs AddEvent 奇数个属性参数：最后一个无配对的应被丢弃，不应 panic / 越界。
func TestSpanAddEventOddAttrs(t *testing.T) {
	tr := NewOTelTracer()
	_, span := tr.StartSpan(context.Background(), "op")
	s := span.(*OTelSpan)
	// 3 个参数 = 1.5 对，循环 i<len-1=2，仅处理 i=0 => key0/val0
	span.AddEvent("ev", "key0", "val0", "dangling")
	if len(s.events) != 1 {
		t.Fatalf("事件数 = %d", len(s.events))
	}
	ev := s.events[0]
	if ev.Attributes["key0"] != "val0" {
		t.Errorf("key0 = %v", ev.Attributes["key0"])
	}
	if _, ok := ev.Attributes["dangling"]; ok {
		t.Error("悬空属性不应被记录")
	}
	span.End()
}

// TestSpanAddEventNonStringKey 非字符串 key 应被跳过。
func TestSpanAddEventNonStringKey(t *testing.T) {
	tr := NewOTelTracer()
	_, span := tr.StartSpan(context.Background(), "op")
	s := span.(*OTelSpan)
	span.AddEvent("ev", 123, "val") // key 不是 string
	if len(s.events[0].Attributes) != 0 {
		t.Error("非字符串 key 应被跳过")
	}
	span.End()
}

// TestSpanSetTokenUsage Token 用量应写入对应属性。
func TestSpanSetTokenUsage(t *testing.T) {
	tr := NewOTelTracer()
	_, span := tr.StartSpan(context.Background(), "op")
	s := span.(*OTelSpan)
	span.SetTokenUsage(observe.TokenUsage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30})
	if s.attributes[observe.AttrLLMTotalTokens] != 30 {
		t.Errorf("total tokens = %v", s.attributes[observe.AttrLLMTotalTokens])
	}
	span.End()
}

// TestSpanSetInputOutput SetInput/SetOutput 写入属性。
func TestSpanSetInputOutput(t *testing.T) {
	tr := NewOTelTracer()
	_, span := tr.StartSpan(context.Background(), "op")
	s := span.(*OTelSpan)
	span.SetInput("in")
	span.SetOutput("out")
	if s.attributes["input"] != "in" || s.attributes["output"] != "out" {
		t.Error("input/output 属性未写入")
	}
	span.End()
}

// TestSpanSetName SetName 修改名称。
func TestSpanSetName(t *testing.T) {
	tr := NewOTelTracer()
	_, span := tr.StartSpan(context.Background(), "old")
	span.SetName("new")
	if span.(*OTelSpan).name != "new" {
		t.Error("SetName 未生效")
	}
	span.End()
}

// TestSpanConcurrentMutation 并发设置属性/事件不应 data race。
func TestSpanConcurrentMutation(t *testing.T) {
	tr := NewOTelTracer()
	_, span := tr.StartSpan(context.Background(), "op")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			span.SetAttribute(fmt.Sprintf("k%d", n), n)
			span.AddEvent(fmt.Sprintf("ev%d", n), "x", n)
			span.SetStatus(observe.StatusCodeOK, "ok")
			_ = span.IsRecording()
		}(i)
	}
	wg.Wait()
	span.End()
}

// TestExtractInjectTraceID Tracer 的 ExtractTraceID/InjectTraceID 闭环。
func TestExtractInjectTraceID(t *testing.T) {
	tr := NewOTelTracer()
	ctx := tr.InjectTraceID(context.Background(), "tid-123")
	if tr.ExtractTraceID(ctx) != "tid-123" {
		t.Error("Inject/ExtractTraceID 不一致")
	}
	if tr.ExtractTraceID(context.Background()) != "" {
		t.Error("空 ctx 应返回空 traceID")
	}
}

// TestTracerShutdownExportsActiveSpans Shutdown 时应导出仍活跃（未 End）的 span。
func TestTracerShutdownExportsActiveSpans(t *testing.T) {
	tr := NewOTelTracer(WithSamplingRate(1.0))
	exp := &countingExporter{}
	tr.SetExporter(exp)

	// 启动但不结束
	tr.StartSpan(context.Background(), "leaked-1")
	tr.StartSpan(context.Background(), "leaked-2")

	if err := tr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown 失败: %v", err)
	}
	if atomic.LoadInt32(&exp.exportedSpans) != 2 {
		t.Errorf("Shutdown 导出 span 数 = %d, want 2", atomic.LoadInt32(&exp.exportedSpans))
	}
	if atomic.LoadInt32(&exp.shutdownCalls) != 1 {
		t.Errorf("exporter.Shutdown 调用次数 = %d, want 1", atomic.LoadInt32(&exp.shutdownCalls))
	}
}

// TestTracerShutdownNilExporter 无 exporter 时 Shutdown 不应 panic 并返回 nil。
func TestTracerShutdownNilExporter(t *testing.T) {
	tr := NewOTelTracer()
	if err := tr.Shutdown(context.Background()); err != nil {
		t.Errorf("nil exporter Shutdown 应返回 nil, got %v", err)
	}
}

// TestToSpanDataSnapshot toSpanData 返回的是深拷贝快照，后续修改不应影响已导出数据。
func TestToSpanDataSnapshot(t *testing.T) {
	tr := NewOTelTracer()
	_, span := tr.StartSpan(context.Background(), "op")
	s := span.(*OTelSpan)
	span.SetAttribute("a", 1)
	span.AddEvent("e1")

	data := s.toSpanData()
	// 修改原 span
	span.SetAttribute("a", 999)
	span.AddEvent("e2")

	if data.Attributes["a"] != 1 {
		t.Errorf("快照属性被后续修改污染: %v", data.Attributes["a"])
	}
	if len(data.Events) != 1 {
		t.Errorf("快照事件被后续修改污染: %d", len(data.Events))
	}
	span.End()
}

// ============================================================
// Exporter 审计
// ============================================================

// countingExporter 计数型 mock exporter（不 mock 被测逻辑，仅用于统计调用）
type countingExporter struct {
	exportCalls   int32
	exportedSpans int32
	shutdownCalls int32
}

func (e *countingExporter) ExportSpans(ctx context.Context, spans []*SpanData) error {
	atomic.AddInt32(&e.exportCalls, 1)
	atomic.AddInt32(&e.exportedSpans, int32(len(spans)))
	return nil
}

func (e *countingExporter) Shutdown(ctx context.Context) error {
	atomic.AddInt32(&e.shutdownCalls, 1)
	return nil
}

// failingExporter 总是返回错误的 exporter
type failingExporter struct{ err error }

func (e *failingExporter) ExportSpans(ctx context.Context, spans []*SpanData) error { return e.err }
func (e *failingExporter) Shutdown(ctx context.Context) error                       { return e.err }

// TestConsoleExporterPretty Pretty/非 Pretty 都应输出。
func TestConsoleExporterPretty(t *testing.T) {
	for _, pretty := range []bool{true, false} {
		var sb strings.Builder
		exp := &ConsoleExporter{Writer: &sb, Pretty: pretty}
		err := exp.ExportSpans(context.Background(), []*SpanData{{TraceID: "t", SpanID: "s", Name: "n"}})
		if err != nil {
			t.Fatalf("pretty=%v 导出失败: %v", pretty, err)
		}
		if sb.Len() == 0 {
			t.Errorf("pretty=%v 无输出", pretty)
		}
		// 非 pretty 不应含缩进的两个空格 + 引号换行模式（粗略校验单行）
		if !pretty && strings.Count(sb.String(), "\n") != 1 {
			t.Errorf("非 pretty 应单行输出, got %q", sb.String())
		}
	}
}

// TestConsoleExporterEmpty 空 span 列表导出应成功且无输出。
func TestConsoleExporterEmpty(t *testing.T) {
	var sb strings.Builder
	exp := NewConsoleExporter(&sb)
	if err := exp.ExportSpans(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if sb.Len() != 0 {
		t.Error("空列表不应有输出")
	}
}

// TestMultiExporterErrorAggregation 一个失败不阻止其他，返回最后错误。
func TestMultiExporterErrorAggregation(t *testing.T) {
	good := &countingExporter{}
	bad := &failingExporter{err: fmt.Errorf("boom")}
	multi := NewMultiExporter(bad, good)

	err := multi.ExportSpans(context.Background(), []*SpanData{{Name: "x"}})
	if err == nil {
		t.Error("应返回失败 exporter 的错误")
	}
	if atomic.LoadInt32(&good.exportCalls) != 1 {
		t.Error("好的 exporter 仍应被调用")
	}
}

// TestMultiExporterShutdown 多 exporter shutdown 聚合。
func TestMultiExporterShutdown(t *testing.T) {
	c1, c2 := &countingExporter{}, &countingExporter{}
	multi := NewMultiExporter(c1, c2)
	if err := multi.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c1.shutdownCalls != 1 || c2.shutdownCalls != 1 {
		t.Error("所有子 exporter 都应被 Shutdown")
	}
}

// TestOTLPExporterBatchFlushBySize 缓冲达到 batchSize 时自动 flush。
func TestOTLPExporterBatchFlushBySize(t *testing.T) {
	var got int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&got, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exp := NewOTLPExporter(srv.URL, WithOTLPBatchSize(2))
	defer exp.Shutdown(context.Background())

	// 第一次：1 个，未达 batchSize=2，不 flush
	if err := exp.ExportSpans(context.Background(), []*SpanData{mkSpan("a")}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&got) != 0 {
		t.Errorf("未达批量阈值不应发送, got %d", atomic.LoadInt32(&got))
	}
	// 第二次：总计 2 == batchSize，触发 flush
	if err := exp.ExportSpans(context.Background(), []*SpanData{mkSpan("b")}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&got) != 1 {
		t.Errorf("达批量阈值应发送一次, got %d", atomic.LoadInt32(&got))
	}
}

// TestOTLPExporterFlushOnShutdown Shutdown 应 flush 残留缓冲。
func TestOTLPExporterFlushOnShutdown(t *testing.T) {
	var got int32
	var bodyLen int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		atomic.AddInt32(&got, 1)
		atomic.StoreInt32(&bodyLen, int32(len(b)))
		// 校验路径
		if r.URL.Path != "/v1/traces" {
			t.Errorf("OTLP 路径错误: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exp := NewOTLPExporter(srv.URL, WithOTLPBatchSize(1000))
	// 放入少量，不触发 size flush
	exp.ExportSpans(context.Background(), []*SpanData{mkSpan("x")})
	if atomic.LoadInt32(&got) != 0 {
		t.Fatal("不应在 Shutdown 前发送")
	}
	if err := exp.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown 失败: %v", err)
	}
	if atomic.LoadInt32(&got) != 1 {
		t.Errorf("Shutdown 应 flush 残留, 发送次数 = %d", atomic.LoadInt32(&got))
	}
	if atomic.LoadInt32(&bodyLen) == 0 {
		t.Error("发送 body 不应为空")
	}
}

// TestOTLPExporterShutdownIdempotent 重复 Shutdown 不应 panic（close channel 二次会 panic 若无 Once）。
func TestOTLPExporterShutdownIdempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exp := NewOTLPExporter(srv.URL)
	if err := exp.Shutdown(context.Background()); err != nil {
		t.Fatalf("首次 Shutdown 失败: %v", err)
	}
	// 第二次不应 panic
	if err := exp.Shutdown(context.Background()); err != nil {
		t.Fatalf("二次 Shutdown 失败: %v", err)
	}
}

// TestOTLPExporterServerError 服务端 4xx/5xx 应返回错误。
func TestOTLPExporterServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server boom"))
	}))
	defer srv.Close()

	exp := NewOTLPExporter(srv.URL, WithOTLPBatchSize(1))
	defer exp.Shutdown(context.Background())

	err := exp.ExportSpans(context.Background(), []*SpanData{mkSpan("x")})
	if err == nil {
		t.Error("服务端 500 应返回错误")
	}
	if err != nil && !strings.Contains(err.Error(), "500") {
		t.Errorf("错误应含状态码, got %v", err)
	}
}

// TestOTLPExporterHeaders WithOTLPHeaders 应附加自定义请求头。
func TestOTLPExporterHeaders(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Custom")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exp := NewOTLPExporter(srv.URL,
		WithOTLPBatchSize(1),
		WithOTLPHeaders(map[string]string{"X-Custom": "yes"}),
	)
	defer exp.Shutdown(context.Background())

	exp.ExportSpans(context.Background(), []*SpanData{mkSpan("x")})
	if gotHeader != "yes" {
		t.Errorf("自定义 header 未发送, got %q", gotHeader)
	}
}

// TestOTLPExporterEmptyFlush 空缓冲 flush 应直接返回 nil，不发请求。
func TestOTLPExporterEmptyFlush(t *testing.T) {
	var got int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&got, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exp := NewOTLPExporter(srv.URL)
	if err := exp.flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	exp.Shutdown(context.Background())
	if atomic.LoadInt32(&got) != 0 {
		t.Errorf("空缓冲不应发请求, got %d", atomic.LoadInt32(&got))
	}
}

// TestOTLPPayloadGrouping 按 service.name 分组到不同 resourceSpans。
func TestOTLPPayloadGrouping(t *testing.T) {
	exp := NewOTLPExporter("http://x")
	defer exp.Shutdown(context.Background())

	spans := []*SpanData{
		{TraceID: "t", SpanID: "s1", Name: "a", Attributes: map[string]any{"service.name": "svc-a"}},
		{TraceID: "t", SpanID: "s2", Name: "b", Attributes: map[string]any{"service.name": "svc-b"}},
		{TraceID: "t", SpanID: "s3", Name: "c", Attributes: map[string]any{}}, // unknown
	}
	payload := exp.toOTLPPayload(spans)
	rs, ok := payload["resourceSpans"].([]map[string]any)
	if !ok {
		t.Fatalf("resourceSpans 类型错误")
	}
	if len(rs) != 3 {
		t.Errorf("应有 3 个 resourceSpans (svc-a, svc-b, unknown), got %d", len(rs))
	}
}

// TestOTLPValueConversion valueToOTLP 类型映射。
func TestOTLPValueConversion(t *testing.T) {
	exp := NewOTLPExporter("http://x")
	defer exp.Shutdown(context.Background())

	tests := []struct {
		in      any
		wantKey string
	}{
		{"str", "stringValue"},
		{42, "intValue"},
		{int64(42), "intValue"},
		{3.14, "doubleValue"},
		{true, "boolValue"},
		{[]int{1, 2}, "stringValue"}, // fallback
	}
	for _, tt := range tests {
		got := exp.valueToOTLP(tt.in)
		if _, ok := got[tt.wantKey]; !ok {
			t.Errorf("valueToOTLP(%v) 缺少键 %s, got %v", tt.in, tt.wantKey, got)
		}
	}
}

// TestOTLPSpanKindOffset spanToOTLP 的 kind 应 +1（OTLP kind 从 1 开始）。
func TestOTLPSpanKindOffset(t *testing.T) {
	exp := NewOTLPExporter("http://x")
	defer exp.Shutdown(context.Background())

	span := mkSpan("x")
	span.Kind = observe.SpanKindInternal // 0
	otlp := exp.spanToOTLP(span)
	if otlp["kind"] != 1 {
		t.Errorf("Internal(0) -> OTLP kind 应为 1, got %v", otlp["kind"])
	}

	span.Kind = observe.SpanKindClient // 2
	otlp = exp.spanToOTLP(span)
	if otlp["kind"] != 3 {
		t.Errorf("Client(2) -> OTLP kind 应为 3, got %v", otlp["kind"])
	}
}

// TestJaegerExporterEmpty 空 span 列表直接返回 nil。
func TestJaegerExporterEmpty(t *testing.T) {
	exp := NewJaegerExporter("http://x")
	if err := exp.ExportSpans(context.Background(), nil); err != nil {
		t.Errorf("空列表应返回 nil, got %v", err)
	}
}

// TestJaegerExporterSendAndAuth Jaeger 导出含 BasicAuth + 路径校验。
func TestJaegerExporterSendAndAuth(t *testing.T) {
	var path, user, pass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		user, pass, _ = r.BasicAuth()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exp := NewJaegerExporter(srv.URL).WithAuth("u", "p")
	if err := exp.ExportSpans(context.Background(), []*SpanData{mkSpan("x")}); err != nil {
		t.Fatal(err)
	}
	if path != "/api/traces" {
		t.Errorf("Jaeger 路径 = %s", path)
	}
	if user != "u" || pass != "p" {
		t.Errorf("BasicAuth = %q/%q, want u/p", user, pass)
	}
}

// TestJaegerType getJaegerType 类型映射。
func TestJaegerType(t *testing.T) {
	exp := NewJaegerExporter("http://x")
	tests := []struct {
		in   any
		want string
	}{
		{"s", "string"},
		{1, "int64"},
		{int64(1), "int64"},
		{1.0, "float64"},
		{float32(1), "float64"},
		{true, "bool"},
		{[]byte{}, "string"},
	}
	for _, tt := range tests {
		if got := exp.getJaegerType(tt.in); got != tt.want {
			t.Errorf("getJaegerType(%T) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

// TestJaegerExporterServerError 服务端错误返回 err。
func TestJaegerExporterServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	exp := NewJaegerExporter(srv.URL)
	if err := exp.ExportSpans(context.Background(), []*SpanData{mkSpan("x")}); err == nil {
		t.Error("502 应返回错误")
	}
}

// TestZipkinExporterSend Zipkin 导出 + 路径 + payload 校验。
func TestZipkinExporterSend(t *testing.T) {
	var path string
	var payload []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&payload)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exp := NewZipkinExporter(srv.URL, "my-zipkin-svc")
	if err := exp.ExportSpans(context.Background(), []*SpanData{mkSpan("x")}); err != nil {
		t.Fatal(err)
	}
	if path != "/api/v2/spans" {
		t.Errorf("Zipkin 路径 = %s", path)
	}
	if len(payload) != 1 {
		t.Fatalf("payload span 数 = %d", len(payload))
	}
	if ep, ok := payload[0]["localEndpoint"].(map[string]any); ok {
		if ep["serviceName"] != "my-zipkin-svc" {
			t.Errorf("serviceName = %v", ep["serviceName"])
		}
	} else {
		t.Error("缺少 localEndpoint")
	}
}

// TestZipkinExporterEmptySpans 空 span 列表（Zipkin 无早退保护）应仍发送空数组。
func TestZipkinExporterEmptySpans(t *testing.T) {
	var got int32
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&got, 1)
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exp := NewZipkinExporter(srv.URL, "svc")
	if err := exp.ExportSpans(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	// 记录实际行为：Zipkin 无早退，会发送空数组 "[]"
	if atomic.LoadInt32(&got) != 1 {
		t.Errorf("Zipkin 空列表仍发请求 (无早退保护), got %d", atomic.LoadInt32(&got))
	}
	if strings.TrimSpace(body) != "[]" {
		t.Errorf("空列表 body = %q, 期望 []", body)
	}
}

// TestExporterEndToEndViaTracer span.End 自动经 exporter 导出（真实链路）。
func TestExporterEndToEndViaTracer(t *testing.T) {
	var sb strings.Builder
	var mu sync.Mutex
	exp := &ConsoleExporter{Writer: writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return sb.Write(p)
	}), Pretty: false}

	tr := NewOTelTracer(WithSamplingRate(1.0), WithServiceName("e2e"))
	tr.SetExporter(exp)

	_, span := tr.StartSpan(context.Background(), "e2e-op")
	span.SetAttribute("foo", "bar")
	span.End()

	mu.Lock()
	out := sb.String()
	mu.Unlock()
	if !strings.Contains(out, "e2e-op") {
		t.Errorf("导出内容应含 span 名, got %q", out)
	}
}

// writerFunc 适配 io.Writer
type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

// mkSpan 构造一个带时间/属性/事件的 SpanData
func mkSpan(name string) *SpanData {
	now := time.Now()
	return &SpanData{
		TraceID:      "trace-" + name,
		SpanID:       "span-" + name,
		ParentSpanID: "parent-" + name,
		Name:         name,
		Kind:         observe.SpanKindInternal,
		StartTime:    now,
		EndTime:      now.Add(time.Millisecond),
		Attributes:   map[string]any{"service.name": "svc", "n": 1, "b": true, "f": 1.5},
		Events: []SpanEvent{
			{Name: "ev", Timestamp: now, Attributes: map[string]any{"k": "v"}},
		},
		Status:    observe.StatusCodeOK,
		StatusMsg: "ok",
	}
}
