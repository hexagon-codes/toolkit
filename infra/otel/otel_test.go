package otel

import (
	"bytes"
	"context"
	"testing"
)

func TestNewOTelTracer(t *testing.T) {
	tracer := NewOTelTracer()

	if tracer == nil {
		t.Fatal("expected non-nil tracer")
	}

	if tracer.serviceName != "default" {
		t.Errorf("expected service name 'default', got '%s'", tracer.serviceName)
	}
}

func TestNewOTelTracerWithOptions(t *testing.T) {
	tracer := NewOTelTracer(
		WithServiceName("test-service"),
		WithServiceVersion("2.0.0"),
		WithEnvironment("production"),
		WithSamplingRate(0.5),
	)

	if tracer.serviceName != "test-service" {
		t.Errorf("expected service name 'test-service', got '%s'", tracer.serviceName)
	}

	if tracer.config.ServiceVersion != "2.0.0" {
		t.Errorf("expected version '2.0.0', got '%s'", tracer.config.ServiceVersion)
	}

	if tracer.config.Environment != "production" {
		t.Errorf("expected environment 'production', got '%s'", tracer.config.Environment)
	}
}

func TestStartSpan(t *testing.T) {
	tracer := NewOTelTracer(WithServiceName("test"))
	ctx := context.Background()

	ctx, span := tracer.StartSpan(ctx, "test-operation")

	if span == nil {
		t.Fatal("expected non-nil span")
	}

	if span.TraceID() == "" {
		t.Error("expected non-empty trace ID")
	}

	if span.SpanID() == "" {
		t.Error("expected non-empty span ID")
	}

	span.End()
}

func TestSpanAttributes(t *testing.T) {
	tracer := NewOTelTracer()
	ctx := context.Background()

	_, span := tracer.StartSpan(ctx, "test")
	otelSpan := span.(*OTelSpan)

	span.SetAttribute("key", "value")
	if otelSpan.attributes["key"] != "value" {
		t.Error("expected attribute to be set")
	}

	span.SetAttributes(map[string]any{
		"key2": "value2",
		"key3": 123,
	})
	if otelSpan.attributes["key2"] != "value2" {
		t.Error("expected key2 attribute")
	}
	if otelSpan.attributes["key3"] != 123 {
		t.Error("expected key3 attribute")
	}

	span.End()
}

func TestSpanEvents(t *testing.T) {
	tracer := NewOTelTracer()
	ctx := context.Background()

	_, span := tracer.StartSpan(ctx, "test")
	otelSpan := span.(*OTelSpan)

	span.AddEvent("event1", "key", "value")

	if len(otelSpan.events) != 1 {
		t.Errorf("expected 1 event, got %d", len(otelSpan.events))
	}

	if otelSpan.events[0].Name != "event1" {
		t.Errorf("expected event name 'event1', got '%s'", otelSpan.events[0].Name)
	}

	span.End()
}

func TestSpanRecordError(t *testing.T) {
	tracer := NewOTelTracer()
	ctx := context.Background()

	_, span := tracer.StartSpan(ctx, "test")
	otelSpan := span.(*OTelSpan)

	err := &testError{msg: "test error"}
	span.RecordError(err)

	if otelSpan.attributes["error.message"] != "test error" {
		t.Error("expected error message to be recorded")
	}

	if len(otelSpan.events) != 1 {
		t.Error("expected exception event to be added")
	}

	span.End()
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestConsoleExporter(t *testing.T) {
	buf := &bytes.Buffer{}
	exporter := NewConsoleExporter(buf)

	spans := []*SpanData{
		{
			TraceID: "trace-1",
			SpanID:  "span-1",
			Name:    "test",
		},
	}

	err := exporter.ExportSpans(context.Background(), spans)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected output")
	}

	err = exporter.Shutdown(context.Background())
	if err != nil {
		t.Errorf("shutdown failed: %v", err)
	}
}

func TestProbabilitySampler(t *testing.T) {
	// 100% 采样
	sampler := NewProbabilitySampler(1.0)
	if !sampler.ShouldSample("trace-1", "span") {
		t.Error("expected 100% sampling to always sample")
	}

	// 0% 采样
	sampler = NewProbabilitySampler(0.0)
	if sampler.ShouldSample("trace-1", "span") {
		t.Error("expected 0% sampling to never sample")
	}

	// 边界值测试
	sampler = NewProbabilitySampler(-0.5)
	if sampler.rate != 0 {
		t.Error("expected negative rate to be clamped to 0")
	}

	sampler = NewProbabilitySampler(1.5)
	if sampler.rate != 1 {
		t.Error("expected rate > 1 to be clamped to 1")
	}
}

func TestAlwaysSampler(t *testing.T) {
	sampler := &AlwaysSampler{}
	if !sampler.ShouldSample("trace", "span") {
		t.Error("AlwaysSampler should always return true")
	}
}

func TestNeverSampler(t *testing.T) {
	sampler := &NeverSampler{}
	if sampler.ShouldSample("trace", "span") {
		t.Error("NeverSampler should always return false")
	}
}

func TestMapCarrier(t *testing.T) {
	carrier := MapCarrier{}

	carrier.Set("key", "value")
	if carrier.Get("key") != "value" {
		t.Error("expected value to be set")
	}

	keys := carrier.Keys()
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}
}

func TestW3CTraceContextPropagator(t *testing.T) {
	prop := NewW3CTraceContextPropagator()

	// 回归: 原断言只验"非 nil"这类无信息量弱不变量, 长期掩盖了 W3C Extract
	// 因 fmt.Sscanf %s 贪婪匹配而彻底失效的 bug。此处升级为验证 Extract 真正
	// 从 traceparent 中解析出 traceID 并注入 ctx, 锁死核心闭环, 不得弱化。
	carrier := MapCarrier{
		"traceparent": "00-trace123-span456-01",
	}
	ctx := prop.Extract(context.Background(), carrier)

	if ctx == nil {
		t.Fatal("expected non-nil context")
	}

	got, ok := ctx.Value(traceIDKey{}).(string)
	if !ok {
		t.Fatal("expected traceID to be injected into context from traceparent")
	}
	if got != "trace123" {
		t.Errorf("extracted traceID = %q, want %q", got, "trace123")
	}
}

func TestMultiExporter(t *testing.T) {
	buf1 := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}

	exp1 := NewConsoleExporter(buf1)
	exp2 := NewConsoleExporter(buf2)

	multi := NewMultiExporter(exp1, exp2)

	spans := []*SpanData{
		{TraceID: "t1", SpanID: "s1", Name: "test"},
	}

	err := multi.ExportSpans(context.Background(), spans)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	if buf1.Len() == 0 || buf2.Len() == 0 {
		t.Error("expected both exporters to receive data")
	}

	err = multi.Shutdown(context.Background())
	if err != nil {
		t.Errorf("shutdown failed: %v", err)
	}
}

func TestTracerShutdown(t *testing.T) {
	tracer := NewOTelTracer()

	err := tracer.Shutdown(context.Background())
	if err != nil {
		t.Errorf("shutdown failed: %v", err)
	}
}
