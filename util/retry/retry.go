package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

var (
	// ErrMaxAttemptsReached 达到最大重试次数
	ErrMaxAttemptsReached = errors.New("max retry attempts reached")
)

// Config 重试配置
type Config struct {
	MaxAttempts int                                       // 最大尝试次数
	Delay       time.Duration                             // 重试延迟
	MaxDelay    time.Duration                             // 最大延迟（用于退避算法）
	Multiplier  float64                                   // 延迟倍数（指数退避）
	OnRetry     func(n int, err error)                    // 重试回调
	RetryIf     func(err error) bool                      // 重试条件判断
	DelayFunc   func(n int, config *Config) time.Duration // 自定义延迟函数

	// 抖动配置
	JitterFactor float64    // 抖动因子 (0.0-1.0)，例如 0.3 表示 ±30%
	JitterType   JitterType // 抖动类型

	// HTTP 感知
	RetryAfterAware bool // 是否感知 Retry-After 头
	LastError       error // 最后一次错误（内部使用）
}

// Option 配置选项
type Option func(*Config)

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		MaxAttempts: 3,
		Delay:       time.Second,
		MaxDelay:    30 * time.Second,
		Multiplier:  2.0,
		RetryIf:     func(err error) bool { return err != nil },
	}
}

// Attempts 设置最大尝试次数
func Attempts(n int) Option {
	return func(c *Config) {
		c.MaxAttempts = n
	}
}

// Delay 设置重试延迟
func Delay(d time.Duration) Option {
	return func(c *Config) {
		c.Delay = d
	}
}

// MaxDelay 设置最大延迟
func MaxDelay(d time.Duration) Option {
	return func(c *Config) {
		c.MaxDelay = d
	}
}

// Multiplier 设置延迟倍数
func Multiplier(m float64) Option {
	return func(c *Config) {
		c.Multiplier = m
	}
}

// OnRetry 设置重试回调
func OnRetry(fn func(n int, err error)) Option {
	return func(c *Config) {
		c.OnRetry = fn
	}
}

// RetryIf 设置重试条件
func RetryIf(fn func(err error) bool) Option {
	return func(c *Config) {
		c.RetryIf = fn
	}
}

// DelayType 延迟类型
func DelayType(delayType DelayTypeFunc) Option {
	return func(c *Config) {
		c.DelayFunc = delayType
	}
}

// Do 执行带重试的函数
func Do(fn func() error, opts ...Option) error {
	config := DefaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	var lastErr error
	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err
		config.LastError = err

		// 判断是否需要重试
		if !config.RetryIf(err) {
			return err
		}

		// 最后一次尝试不需要延迟
		if attempt == config.MaxAttempts {
			break
		}

		// 回调
		if config.OnRetry != nil {
			config.OnRetry(attempt, err)
		}

		// 计算延迟
		delay := calculateDelay(attempt, config)

		time.Sleep(delay)
	}

	return fmt.Errorf("%w: %v", ErrMaxAttemptsReached, lastErr)
}

// calculateDelay 计算延迟时间（支持抖动和 Retry-After）
func calculateDelay(attempt int, config *Config) time.Duration {
	// 首先检查 Retry-After
	if config.RetryAfterAware && config.LastError != nil {
		if retryAfter := GetRetryAfterFromError(config.LastError); retryAfter > 0 {
			return retryAfter
		}
	}

	// 计算基础延迟
	var delay time.Duration
	if config.DelayFunc != nil {
		delay = config.DelayFunc(attempt, config)
	} else {
		delay = config.Delay
	}

	// 添加抖动
	delay = addJitter(delay, config)

	// 确保不超过最大延迟
	if delay > config.MaxDelay {
		delay = config.MaxDelay
	}

	return delay
}

// DoWithContext 带上下文的重试
func DoWithContext(ctx context.Context, fn func() error, opts ...Option) error {
	config := DefaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	var lastErr error
	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		// 检查上下文
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err
		config.LastError = err

		// 判断是否需要重试
		if !config.RetryIf(err) {
			return err
		}

		// 最后一次尝试不需要延迟
		if attempt == config.MaxAttempts {
			break
		}

		// 回调
		if config.OnRetry != nil {
			config.OnRetry(attempt, err)
		}

		// 计算延迟
		delay := calculateDelay(attempt, config)

		// 使用上下文控制延迟
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}

	return fmt.Errorf("%w: %v", ErrMaxAttemptsReached, lastErr)
}

// DelayTypeFunc 延迟函数类型
type DelayTypeFunc func(n int, config *Config) time.Duration

// FixedDelay 固定延迟
func FixedDelay(n int, config *Config) time.Duration {
	return config.Delay
}

// LinearBackoff 线性退避
func LinearBackoff(n int, config *Config) time.Duration {
	delay := config.Delay * time.Duration(n)
	if delay > config.MaxDelay {
		delay = config.MaxDelay
	}
	return delay
}

// ExponentialBackoff 指数退避
func ExponentialBackoff(n int, config *Config) time.Duration {
	multiplier := math.Pow(config.Multiplier, float64(n-1))
	// 检查溢出：如果乘数过大或无穷大，直接返回最大延迟
	if math.IsInf(multiplier, 0) || math.IsNaN(multiplier) ||
		multiplier > float64(config.MaxDelay)/float64(config.Delay) {
		return config.MaxDelay
	}
	delay := time.Duration(float64(config.Delay) * multiplier)
	// 检查是否溢出为负数或超过最大延迟
	if delay <= 0 || delay > config.MaxDelay {
		return config.MaxDelay
	}
	return delay
}
