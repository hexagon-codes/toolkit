package retry

import (
	"math/rand/v2"
	"time"
)

// JitterType 抖动类型
type JitterType int

const (
	// NoJitter 无抖动
	NoJitter JitterType = iota
	// FullJitter 全抖动: [0, delay]
	FullJitter
	// EqualJitter 均等抖动: [delay/2, delay]
	EqualJitter
	// DecorrelatedJitter 去相关抖动
	DecorrelatedJitter
)

// WithJitter 启用抖动（避免惊群效应）
// factor: 抖动因子，0.0-1.0，例如 0.3 表示延迟 ±30%
func WithJitter(factor float64) Option {
	return func(c *Config) {
		c.JitterFactor = factor
	}
}

// WithJitterType 设置抖动类型
func WithJitterType(jitterType JitterType) Option {
	return func(c *Config) {
		c.JitterType = jitterType
	}
}

// WithFullJitter 启用全抖动
// 延迟在 [0, 计算值] 之间随机
func WithFullJitter() Option {
	return func(c *Config) {
		c.JitterType = FullJitter
	}
}

// WithEqualJitter 启用均等抖动
// 延迟在 [计算值/2, 计算值] 之间随机
func WithEqualJitter() Option {
	return func(c *Config) {
		c.JitterType = EqualJitter
	}
}

// addJitter 添加抖动到延迟时间
func addJitter(delay time.Duration, config *Config) time.Duration {
	if config.JitterType == NoJitter && config.JitterFactor == 0 {
		return delay
	}

	switch config.JitterType {
	case FullJitter:
		// 全抖动: [0, delay]
		return time.Duration(rand.Float64() * float64(delay))

	case EqualJitter:
		// 均等抖动: [delay/2, delay]
		half := float64(delay) / 2
		return time.Duration(half + rand.Float64()*half)

	case DecorrelatedJitter:
		// 去相关抖动: [delay, delay * 3]
		return time.Duration(float64(delay) + rand.Float64()*float64(delay)*2)

	default:
		// 使用 factor 的比例抖动: delay * (1 ± factor)
		if config.JitterFactor > 0 {
			jitter := float64(delay) * config.JitterFactor * (rand.Float64()*2 - 1)
			result := float64(delay) + jitter
			if result < 0 {
				return 0
			}
			return time.Duration(result)
		}
		return delay
	}
}

// ExponentialBackoffWithJitter 带抖动的指数退避
func ExponentialBackoffWithJitter(n int, config *Config) time.Duration {
	delay := ExponentialBackoff(n, config)
	return addJitter(delay, config)
}

// LinearBackoffWithJitter 带抖动的线性退避
func LinearBackoffWithJitter(n int, config *Config) time.Duration {
	delay := LinearBackoff(n, config)
	return addJitter(delay, config)
}
