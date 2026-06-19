package otel

import (
	"context"
	"fmt"
	"strings"

	"github.com/hexagon-codes/toolkit/infra/observe"
)

// Propagator 传播器接口
type Propagator interface {
	// Inject 注入追踪信息到载体
	Inject(ctx context.Context, carrier Carrier)

	// Extract 从载体提取追踪信息
	Extract(ctx context.Context, carrier Carrier) context.Context
}

// Carrier 载体接口
type Carrier interface {
	Get(key string) string
	Set(key, value string)
	Keys() []string
}

// MapCarrier map 载体
type MapCarrier map[string]string

// Get 获取值
func (m MapCarrier) Get(key string) string {
	return m[key]
}

// Set 设置值
func (m MapCarrier) Set(key, value string) {
	m[key] = value
}

// Keys 返回所有键
func (m MapCarrier) Keys() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// W3CTraceContextPropagator W3C 标准传播器
type W3CTraceContextPropagator struct{}

// NewW3CTraceContextPropagator 创建 W3C 标准传播器
func NewW3CTraceContextPropagator() *W3CTraceContextPropagator {
	return &W3CTraceContextPropagator{}
}

// Inject 注入追踪信息
func (p *W3CTraceContextPropagator) Inject(ctx context.Context, carrier Carrier) {
	span := observe.SpanFromContext(ctx)
	if span == nil {
		return
	}

	// traceparent header: 00-{trace-id}-{span-id}-{flags}
	traceparent := fmt.Sprintf("00-%s-%s-01", span.TraceID(), span.SpanID())
	carrier.Set("traceparent", traceparent)
}

// Extract 提取追踪信息
func (p *W3CTraceContextPropagator) Extract(ctx context.Context, carrier Carrier) context.Context {
	traceparent := carrier.Get("traceparent")
	if traceparent == "" {
		return ctx
	}

	// 解析 traceparent
	// 格式: version-{trace-id}-{span-id}-{flags}，4 个字段以 '-' 分隔。
	//
	// 注意：不能用 fmt.Sscanf("%s") 解析，因为动词 %s 匹配"到空白符/EOF 为止
	// 的连续非空白串"，根本不把 '-' 当分隔符，会把 traceID 之后的全部内容
	// （含 spanID、flags）贪婪吞进第一个 %s，导致后续字段为空、解析失败，
	// W3C 分布式追踪链路彻底断开。改用 strings.Split 按 '-' 精确切分。
	parts := strings.Split(traceparent, "-")
	if len(parts) != 4 {
		// 字段数不符合 W3C traceparent 规范，视为无效，保持原 ctx 不变。
		return ctx
	}
	traceID := parts[1]
	if traceID == "" {
		return ctx
	}

	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// B3Propagator B3 传播器（Zipkin 兼容）
type B3Propagator struct{}

// NewB3Propagator 创建 B3 传播器
func NewB3Propagator() *B3Propagator {
	return &B3Propagator{}
}

// Inject 注入追踪信息
func (p *B3Propagator) Inject(ctx context.Context, carrier Carrier) {
	span := observe.SpanFromContext(ctx)
	if span == nil {
		return
	}

	carrier.Set("X-B3-TraceId", span.TraceID())
	carrier.Set("X-B3-SpanId", span.SpanID())
	carrier.Set("X-B3-Sampled", "1")
}

// Extract 提取追踪信息
func (p *B3Propagator) Extract(ctx context.Context, carrier Carrier) context.Context {
	traceID := carrier.Get("X-B3-TraceId")
	if traceID == "" {
		return ctx
	}

	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// CompositePropagator 组合传播器
type CompositePropagator struct {
	propagators []Propagator
}

// NewCompositePropagator 创建组合传播器
func NewCompositePropagator(propagators ...Propagator) *CompositePropagator {
	return &CompositePropagator{propagators: propagators}
}

// Inject 注入追踪信息
func (p *CompositePropagator) Inject(ctx context.Context, carrier Carrier) {
	for _, prop := range p.propagators {
		prop.Inject(ctx, carrier)
	}
}

// Extract 提取追踪信息
func (p *CompositePropagator) Extract(ctx context.Context, carrier Carrier) context.Context {
	for _, prop := range p.propagators {
		ctx = prop.Extract(ctx, carrier)
	}
	return ctx
}
