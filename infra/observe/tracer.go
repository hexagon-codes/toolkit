// Package observe 提供可观测性通用接口定义
//
// 本包定义了追踪器（Tracer）和指标（Metrics）的通用接口，
// 可以被 OpenTelemetry、Jaeger、Zipkin 等具体实现使用。
package observe

import (
	"context"
	"time"
)

// Tracer 追踪器接口
//
// Tracer 负责创建和管理 Span，支持分布式追踪。
// 典型的实现包括 OpenTelemetry、Jaeger、Zipkin 等。
type Tracer interface {
	// StartSpan 开始新的 Span
	//
	// 参数:
	//   - ctx: 上下文，用于传递父 Span 信息
	//   - name: Span 名称
	//   - opts: 可选的 Span 配置
	//
	// 返回:
	//   - 包含新 Span 的 context
	//   - 新创建的 Span
	StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, Span)

	// Shutdown 关闭追踪器
	//
	// 应在程序退出前调用，确保所有 Span 被正确导出
	Shutdown(ctx context.Context) error
}

// Span 追踪 Span 接口
//
// Span 表示一个操作的执行时间段，是分布式追踪的基本单元。
// 一个 Span 可以包含属性、事件、状态等信息。
type Span interface {
	// SpanID 返回 Span ID
	SpanID() string

	// TraceID 返回 Trace ID
	TraceID() string

	// SetName 设置 Span 名称
	SetName(name string)

	// SetInput 设置输入数据
	SetInput(input any)

	// SetOutput 设置输出数据
	SetOutput(output any)

	// SetTokenUsage 设置 Token 使用量（用于 LLM 追踪）
	SetTokenUsage(usage TokenUsage)

	// SetAttribute 设置单个属性
	SetAttribute(key string, value any)

	// SetAttributes 批量设置属性
	SetAttributes(attrs map[string]any)

	// AddEvent 添加事件
	//
	// 事件表示 Span 生命周期内发生的有意义的事情
	// attrs 参数格式: key1, value1, key2, value2, ...
	AddEvent(name string, attrs ...any)

	// RecordError 记录错误
	RecordError(err error)

	// SetStatus 设置状态
	SetStatus(code StatusCode, message string)

	// End 结束 Span
	End()

	// EndWithError 结束并记录错误
	EndWithError(err error)

	// IsRecording 返回是否正在记录
	IsRecording() bool
}

// SpanKind Span 类型
type SpanKind int

const (
	// SpanKindInternal 内部操作
	SpanKindInternal SpanKind = iota
	// SpanKindServer 服务端操作
	SpanKindServer
	// SpanKindClient 客户端操作
	SpanKindClient
	// SpanKindProducer 生产者操作
	SpanKindProducer
	// SpanKindConsumer 消费者操作
	SpanKindConsumer
)

// StatusCode 状态码
type StatusCode int

const (
	// StatusCodeUnset 未设置
	StatusCodeUnset StatusCode = iota
	// StatusCodeOK 成功
	StatusCodeOK
	// StatusCodeError 错误
	StatusCodeError
)

// TokenUsage Token 使用统计
type TokenUsage struct {
	// PromptTokens 提示词 Token 数
	PromptTokens int
	// CompletionTokens 补全 Token 数
	CompletionTokens int
	// TotalTokens 总 Token 数
	TotalTokens int
}

// SpanConfig Span 配置
type SpanConfig struct {
	// Kind Span 类型
	Kind SpanKind
	// Attributes 初始属性
	Attributes map[string]any
	// StartTime 开始时间（可选，默认为当前时间）
	StartTime time.Time
}

// SpanOption Span 配置选项
type SpanOption func(*SpanConfig)

// WithSpanKind 设置 Span 类型
func WithSpanKind(kind SpanKind) SpanOption {
	return func(c *SpanConfig) {
		c.Kind = kind
	}
}

// WithAttributes 设置初始属性
func WithAttributes(attrs map[string]any) SpanOption {
	return func(c *SpanConfig) {
		c.Attributes = attrs
	}
}

// WithStartTime 设置开始时间
func WithStartTime(t time.Time) SpanOption {
	return func(c *SpanConfig) {
		c.StartTime = t
	}
}

// 常用属性键
const (
	// 服务属性
	AttrServiceName    = "service.name"
	AttrServiceVersion = "service.version"

	// Agent 属性
	AttrAgentID   = "agent.id"
	AttrAgentName = "agent.name"
	AttrAgentType = "agent.type"

	// LLM 属性
	AttrLLMProvider         = "llm.provider"
	AttrLLMModel            = "llm.model"
	AttrLLMPromptTokens     = "llm.prompt_tokens"     // #nosec G101 -- 遥测属性名称，不是凭据。
	AttrLLMCompletionTokens = "llm.completion_tokens" // #nosec G101 -- 遥测属性名称，不是凭据。
	AttrLLMTotalTokens      = "llm.total_tokens"      // #nosec G101 -- 遥测属性名称，不是凭据。

	// Tool 属性
	AttrToolName   = "tool.name"
	AttrToolInput  = "tool.input"
	AttrToolOutput = "tool.output"

	// 错误属性
	AttrErrorType    = "error.type"
	AttrErrorMessage = "error.message"

	// 检索属性
	AttrRetrieverType = "retriever.type"
	AttrRetrieverTopK = "retriever.top_k"
	AttrDocumentCount = "retriever.doc_count"
)

// ============== Context 工具函数 ==============

type spanContextKey struct{}

// ContextWithSpan 将 Span 存入 context
func ContextWithSpan(ctx context.Context, span Span) context.Context {
	return context.WithValue(ctx, spanContextKey{}, span)
}

// SpanFromContext 从 context 获取 Span
func SpanFromContext(ctx context.Context) Span {
	if span, ok := ctx.Value(spanContextKey{}).(Span); ok {
		return span
	}
	return nil
}
