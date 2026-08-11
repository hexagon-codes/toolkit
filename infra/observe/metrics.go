package observe

import (
	"sync"
	"time"
)

// Metrics 指标收集器接口
//
// Metrics 提供了创建各种类型指标的统一接口，
// 支持 Counter、Gauge、Histogram、Timer 等指标类型。
type Metrics interface {
	// Counter 获取或创建计数器
	//
	// Counter 是单调递增的指标，适用于统计请求数、错误数等
	Counter(name string, tags ...Tag) (Counter, error)

	// Gauge 获取或创建仪表盘
	//
	// Gauge 可以增加或减少，适用于统计当前值如活跃连接数、内存使用等
	Gauge(name string, tags ...Tag) (Gauge, error)

	// Histogram 获取或创建直方图
	//
	// Histogram 用于统计数值的分布，如响应时间、请求大小等
	Histogram(name string, tags ...Tag) (Histogram, error)

	// Timer 获取或创建计时器
	//
	// Timer 用于测量操作的持续时间
	Timer(name string, tags ...Tag) (Timer, error)
}

// Tag 是类型化指标标签，用于避免畸形的键值参数列表。
type Tag struct {
	Name  string
	Value string
}

// Counter 计数器接口
//
// Counter 是单调递增的指标，只能增加不能减少。
// 适用于统计请求总数、错误总数等场景。
type Counter interface {
	// Inc 增加 1
	Inc()

	// Add 增加指定值（必须 >= 0）
	Add(v float64) error

	// Value 获取当前值
	Value() float64
}

// Gauge 仪表盘接口
//
// Gauge 可以任意增减，表示某个瞬时值。
// 适用于统计当前活跃连接数、内存使用量等场景。
type Gauge interface {
	// Set 设置值
	Set(v float64)

	// Inc 增加 1
	Inc()

	// Dec 减少 1
	Dec()

	// Add 增加指定值（可以为负数）
	Add(v float64)

	// Value 获取当前值
	Value() float64
}

// Histogram 直方图接口
//
// Histogram 用于统计数值的分布情况。
// 适用于统计响应时间分布、请求大小分布等场景。
type Histogram interface {
	// Observe 观察一个值
	Observe(v float64)

	// Count 获取观察次数
	Count() uint64

	// Sum 获取观察值总和
	Sum() float64
}

// Timer 计时器接口
//
// Timer 用于测量操作的持续时间。
// 内部通常使用 Histogram 实现。
type Timer interface {
	// ObserveDuration 记录一个持续时间
	ObserveDuration(d time.Duration)

	// Time 测量函数执行时间
	//
	// 使用示例:
	//   timer.Time(func() {
	//       // 需要测量的操作
	//   })
	Time(fn func())

	// NewTimer 创建新的计时上下文
	//
	// 使用示例:
	//   tc := timer.NewTimer()
	//   // 执行操作
	//   tc.Stop() // 记录持续时间
	NewTimer() *TimerContext
}

// TimerContext 计时上下文
//
// 用于手动控制计时的开始和结束
type TimerContext struct {
	timer     Timer
	startTime time.Time

	mu       sync.Mutex
	stopped  bool
	duration time.Duration
}

// NewTimerContext 创建计时上下文
func NewTimerContext(timer Timer) *TimerContext {
	if isNilInterface(timer) {
		timer = nil
	}
	return &TimerContext{
		timer:     timer,
		startTime: time.Now(),
	}
}

// Stop 停止计时并记录持续时间
//
// 返回从开始到停止的持续时间
func (tc *TimerContext) Stop() time.Duration {
	if tc == nil {
		return 0
	}
	tc.mu.Lock()
	if tc.stopped {
		duration := tc.duration
		tc.mu.Unlock()
		return duration
	}
	tc.stopped = true
	tc.duration = time.Since(tc.startTime)
	duration := tc.duration
	timer := tc.timer
	tc.mu.Unlock()

	if !isNilInterface(timer) {
		timer.ObserveDuration(duration)
	}
	return duration
}

// ============== 预定义指标名称 ==============

// Agent 相关指标
const (
	// MetricAgentRunsTotal Agent 运行总数
	MetricAgentRunsTotal = "agent_runs_total"
	// MetricAgentRunDuration Agent 运行时长
	MetricAgentRunDuration = "agent_run_duration_seconds"
	// MetricAgentRunErrors Agent 运行错误数
	MetricAgentRunErrors = "agent_run_errors_total"
	// MetricAgentActiveCount 活跃 Agent 数
	MetricAgentActiveCount = "agent_active_count"
)

// LLM 相关指标
const (
	// MetricLLMCallsTotal LLM 调用总数
	MetricLLMCallsTotal = "llm_calls_total"
	// MetricLLMCallDuration LLM 调用时长
	MetricLLMCallDuration = "llm_call_duration_seconds"
	// MetricLLMCallErrors LLM 调用错误数
	MetricLLMCallErrors = "llm_call_errors_total"
	// MetricLLMPromptTokens 提示词 Token 数
	MetricLLMPromptTokens = "llm_prompt_tokens_total" // #nosec G101 -- 指标名称，不是凭据。
	// MetricLLMCompletionTokens 补全 Token 数
	MetricLLMCompletionTokens = "llm_completion_tokens_total" // #nosec G101 -- 指标名称，不是凭据。
)

// Tool 相关指标
const (
	// MetricToolCallsTotal 工具调用总数
	MetricToolCallsTotal = "tool_calls_total"
	// MetricToolCallDuration 工具调用时长
	MetricToolCallDuration = "tool_call_duration_seconds"
	// MetricToolCallErrors 工具调用错误数
	MetricToolCallErrors = "tool_call_errors_total"
)

// Retrieval 相关指标
const (
	// MetricRetrievalTotal 检索总数
	MetricRetrievalTotal = "retrieval_total"
	// MetricRetrievalDuration 检索时长
	MetricRetrievalDuration = "retrieval_duration_seconds"
	// MetricRetrievalDocCount 检索文档数
	MetricRetrievalDocCount = "retrieval_doc_count"
)
