package retry

import "time"

// ============== AI API 预设策略 ==============

// OpenAIStrategy OpenAI 推荐重试策略
// 参考: https://platform.openai.com/docs/guides/rate-limits/error-mitigation
// - 5 次尝试
// - 指数退避：1s -> 2s -> 4s -> 8s -> 16s
// - 最大延迟 60s
// - 30% 抖动
// - 感知 Retry-After 头
// - 重试 429、5xx 和网络错误
var OpenAIStrategy = []Option{
	Attempts(5),
	Delay(1 * time.Second),
	MaxDelay(60 * time.Second),
	Multiplier(2.0),
	DelayType(ExponentialBackoff),
	WithJitter(0.3),
	WithRetryAfterAware(),
	RetryIf(IsRetryableHTTPError),
}

// ClaudeStrategy Anthropic Claude 推荐重试策略
// - 4 次尝试
// - 指数退避：2s -> 4s -> 8s -> 16s
// - 最大延迟 30s
// - 25% 抖动
// - 重试 429、5xx 和网络错误
var ClaudeStrategy = []Option{
	Attempts(4),
	Delay(2 * time.Second),
	MaxDelay(30 * time.Second),
	Multiplier(2.0),
	DelayType(ExponentialBackoff),
	WithJitter(0.25),
	RetryIf(IsRetryableHTTPError),
}

// GeminiStrategy Google Gemini 推荐重试策略
// - 3 次尝试
// - 指数退避：1s -> 2s -> 4s
// - 最大延迟 30s
// - 全抖动
// - 重试 429、5xx 和网络错误
var GeminiStrategy = []Option{
	Attempts(3),
	Delay(1 * time.Second),
	MaxDelay(30 * time.Second),
	Multiplier(2.0),
	DelayType(ExponentialBackoff),
	WithFullJitter(),
	RetryIf(IsRetryableHTTPError),
}

// DeepSeekStrategy DeepSeek 推荐重试策略
// - 5 次尝试
// - 指数退避
// - 30% 抖动
var DeepSeekStrategy = []Option{
	Attempts(5),
	Delay(1 * time.Second),
	MaxDelay(60 * time.Second),
	Multiplier(2.0),
	DelayType(ExponentialBackoff),
	WithJitter(0.3),
	WithRetryAfterAware(),
	RetryIf(IsRetryableHTTPError),
}

// QwenStrategy 通义千问推荐重试策略
var QwenStrategy = []Option{
	Attempts(4),
	Delay(1 * time.Second),
	MaxDelay(30 * time.Second),
	Multiplier(2.0),
	DelayType(ExponentialBackoff),
	WithJitter(0.25),
	RetryIf(IsRetryableHTTPError),
}

// AIAPIStrategy 通用 AI API 重试策略
// 适用于大多数 AI API
var AIAPIStrategy = []Option{
	Attempts(5),
	Delay(1 * time.Second),
	MaxDelay(60 * time.Second),
	Multiplier(2.0),
	DelayType(ExponentialBackoff),
	WithJitter(0.3),
	WithRetryAfterAware(),
	RetryIf(IsRetryableHTTPError),
}

// ============== 通用预设策略 ==============

// AggressiveStrategy 激进重试策略
// 适用于对延迟敏感的场景
// - 快速重试，短延迟
var AggressiveStrategy = []Option{
	Attempts(3),
	Delay(100 * time.Millisecond),
	MaxDelay(1 * time.Second),
	Multiplier(2.0),
	DelayType(ExponentialBackoff),
	WithJitter(0.1),
}

// ConservativeStrategy 保守重试策略
// 适用于需要避免过度重试的场景
// - 较少重试，长延迟
var ConservativeStrategy = []Option{
	Attempts(3),
	Delay(5 * time.Second),
	MaxDelay(60 * time.Second),
	Multiplier(2.0),
	DelayType(ExponentialBackoff),
	WithJitter(0.5),
}

// NetworkErrorStrategy 网络错误重试策略
// 适用于处理不稳定的网络连接
var NetworkErrorStrategy = []Option{
	Attempts(5),
	Delay(1 * time.Second),
	MaxDelay(30 * time.Second),
	Multiplier(2.0),
	DelayType(ExponentialBackoff),
	WithEqualJitter(),
	RetryIf(func(err error) bool {
		return isNetworkError(err)
	}),
}

// DatabaseStrategy 数据库重试策略
// 适用于数据库连接和查询
var DatabaseStrategy = []Option{
	Attempts(3),
	Delay(100 * time.Millisecond),
	MaxDelay(5 * time.Second),
	Multiplier(2.0),
	DelayType(ExponentialBackoff),
	WithJitter(0.2),
}

// ============== 辅助函数 ==============

// MergeOptions 合并多个 Option 切片
func MergeOptions(base []Option, additional ...Option) []Option {
	result := make([]Option, 0, len(base)+len(additional))
	result = append(result, base...)
	result = append(result, additional...)
	return result
}

// WithCallback 添加回调到现有策略
func WithCallback(strategy []Option, callback func(n int, err error)) []Option {
	return MergeOptions(strategy, OnRetry(callback))
}
