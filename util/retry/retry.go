package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

var (
	// ErrMaxAttemptsReached 表示全部尝试均已失败。
	ErrMaxAttemptsReached = errors.New("max retry attempts reached")
	// ErrInvalidConfig 表示重试配置无效。
	ErrInvalidConfig = errors.New("invalid retry config")
)

// Config 保存一次重试调用的独立配置快照，也供 DelayTypeFunc 读取退避参数。
// Do 和 DoWithContext 应通过 Option 配置；重试条件只能通过 If 设置。
type Config struct {
	MaxAttempts int                                       // 最大尝试次数
	Delay       time.Duration                             // 重试延迟
	MaxDelay    time.Duration                             // 最大延迟（用于退避算法）
	Multiplier  float64                                   // 延迟倍数（指数退避）
	OnRetry     func(n int, err error)                    // 重试回调
	DelayFunc   func(n int, config *Config) time.Duration // 自定义延迟函数
	predicate   func(err error) bool

	// 抖动配置
	JitterFactor float64    // 抖动因子 (0.0-1.0)，例如 0.3 表示 ±30%
	JitterType   JitterType // 抖动类型

	// HTTP 感知
	RetryAfterAware bool  // 是否感知 Retry-After 头
	LastError       error // 最后一次错误（内部使用）

}

// Option 配置选项
type Option func(*Config)

// DefaultConfig 返回独立的默认配置快照。
// 调用方可修改公开字段后传给 FixedDelay、LinearBackoff 或 ExponentialBackoff
// 做独立退避计算；Do 和 DoWithContext 仍通过 Option 接收配置。
func DefaultConfig() *Config {
	return &Config{
		MaxAttempts: 3,
		Delay:       time.Second,
		MaxDelay:    30 * time.Second,
		Multiplier:  2.0,
		predicate:   func(err error) bool { return err != nil },
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

// Multiplier 设置指数退避使用的延迟倍数，但不选择退避策略。
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

// If 设置重试条件
func If(fn func(err error) bool) Option {
	return func(c *Config) {
		c.predicate = fn
	}
}

// DelayType 显式选择延迟策略。
func DelayType(delayType DelayTypeFunc) Option {
	return func(c *Config) {
		c.DelayFunc = delayType
	}
}

// Do 执行带重试的函数
func Do(fn func() error, opts ...Option) error {
	if fn == nil {
		return fmt.Errorf("%w: retry function must not be nil", ErrInvalidConfig)
	}
	config, err := newConfig(opts...)
	if err != nil {
		return err
	}

	var lastErr error
	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		runErr := fn()
		if runErr == nil {
			return nil
		}

		lastErr = runErr
		config.LastError = runErr

		// 判断是否需要重试
		if !config.predicate(runErr) {
			return runErr
		}

		// 最后一次尝试不需要延迟
		if attempt == config.MaxAttempts {
			break
		}

		if config.OnRetry != nil {
			config.OnRetry(attempt, runErr)
		}

		// 计算延迟
		delay := calculateDelay(attempt, config)

		time.Sleep(delay)
	}

	return fmt.Errorf("%w: %w", ErrMaxAttemptsReached, lastErr)
}

func newConfig(options ...Option) (*Config, error) {
	config := DefaultConfig()
	for index, option := range options {
		if option == nil {
			return nil, fmt.Errorf("%w: option %d must not be nil", ErrInvalidConfig, index)
		}
		option(config)
	}
	switch {
	case config.MaxAttempts <= 0:
		return nil, fmt.Errorf("%w: attempts must be positive", ErrInvalidConfig)
	case config.Delay < 0:
		return nil, fmt.Errorf("%w: delay must not be negative", ErrInvalidConfig)
	case config.MaxDelay <= 0:
		return nil, fmt.Errorf("%w: max delay must be positive", ErrInvalidConfig)
	case config.Multiplier <= 0 || math.IsNaN(config.Multiplier) || math.IsInf(config.Multiplier, 0):
		return nil, fmt.Errorf("%w: multiplier must be finite and positive", ErrInvalidConfig)
	case config.predicate == nil:
		return nil, fmt.Errorf("%w: retry predicate must not be nil", ErrInvalidConfig)
	case config.JitterFactor < 0 || config.JitterFactor > 1 || math.IsNaN(config.JitterFactor):
		return nil, fmt.Errorf("%w: jitter factor must be between zero and one", ErrInvalidConfig)
	case config.JitterType < NoJitter || config.JitterType > DecorrelatedJitter:
		return nil, fmt.Errorf("%w: jitter type is unknown", ErrInvalidConfig)
	}
	applyDefaultDelay(config)
	return config, nil
}

// calculateDelay 计算延迟时间（支持抖动和 Retry-After）
func calculateDelay(attempt int, config *Config) time.Duration {
	// 首先检查 Retry-After
	if config.RetryAfterAware && config.LastError != nil {
		if retryAfter := GetRetryAfterFromError(config.LastError); retryAfter > 0 {
			if retryAfter > config.MaxDelay {
				return config.MaxDelay
			}
			return retryAfter
		}
	}

	// 先计算基础退避，再统一应用抖动，避免策略组合时静默丢失抖动配置。
	var baseDelay time.Duration
	if config.DelayFunc != nil {
		baseDelay = config.DelayFunc(attempt, config)
	} else {
		baseDelay = config.Delay
	}
	delay := addJitter(baseDelay, config)

	// 确保不超过最大延迟
	if delay > config.MaxDelay {
		delay = config.MaxDelay
	}
	if delay < 0 {
		return 0
	}

	return delay
}

// DoWithContext 在尝试与等待边界响应上下文取消。
// fn 不接收上下文，因此本函数不会分离执行或强制中断正在运行的 fn；
// 需要取消单次操作时，调用方应在闭包中向下传递 ctx。
func DoWithContext(ctx context.Context, fn func() error, opts ...Option) error {
	if ctx == nil {
		return fmt.Errorf("%w: context must not be nil", ErrInvalidConfig)
	}
	if fn == nil {
		return fmt.Errorf("%w: retry function must not be nil", ErrInvalidConfig)
	}
	config, err := newConfig(opts...)
	if err != nil {
		return err
	}

	var lastErr error
	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		if contextErr := ctx.Err(); contextErr != nil {
			return combineContextError(contextErr, lastErr)
		}

		runErr := fn()
		if runErr == nil {
			return nil
		}

		lastErr = runErr
		config.LastError = runErr
		if contextErr := ctx.Err(); contextErr != nil {
			return combineContextError(contextErr, lastErr)
		}

		// 判断是否需要重试
		retryable := config.predicate(runErr)
		if contextErr := ctx.Err(); contextErr != nil {
			return combineContextError(contextErr, lastErr)
		}
		if !retryable {
			return runErr
		}

		// 最后一次尝试不需要延迟
		if attempt == config.MaxAttempts {
			break
		}

		if config.OnRetry != nil {
			config.OnRetry(attempt, runErr)
		}
		if contextErr := ctx.Err(); contextErr != nil {
			return combineContextError(contextErr, lastErr)
		}

		// 计算延迟
		delay := calculateDelay(attempt, config)

		if contextErr := waitForRetry(ctx, delay); contextErr != nil {
			return combineContextError(contextErr, lastErr)
		}
	}

	if contextErr := ctx.Err(); contextErr != nil {
		return combineContextError(contextErr, lastErr)
	}
	return fmt.Errorf("%w: %w", ErrMaxAttemptsReached, lastErr)
}

func combineContextError(contextErr, lastErr error) error {
	if lastErr == nil {
		return contextErr
	}
	if errors.Is(lastErr, contextErr) {
		return lastErr
	}
	return fmt.Errorf("%w: %w", contextErr, lastErr)
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	select {
	case <-ctx.Done():
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// applyDefaultDelay 未指定延迟策略时使用固定延迟。
func applyDefaultDelay(config *Config) {
	if config.DelayFunc == nil {
		config.DelayFunc = FixedDelay
	}
}

// DelayTypeFunc 延迟函数类型；n 使用一基重试序号，首次重试为 1。
type DelayTypeFunc func(n int, config *Config) time.Duration

// FixedDelay 固定延迟
func FixedDelay(n int, config *Config) time.Duration {
	return config.Delay
}

// LinearBackoff 按一基重试序号计算线性退避，非正序号返回 0。
func LinearBackoff(n int, config *Config) time.Duration {
	if n <= 0 || config.Delay <= 0 || config.MaxDelay <= 0 {
		return 0
	}
	// 在乘法前比较商，避免极大重试次数令 time.Duration 溢出为负数。
	if time.Duration(n) > config.MaxDelay/config.Delay {
		return config.MaxDelay
	}
	return config.Delay * time.Duration(n)
}

// ExponentialBackoff 按一基重试序号计算指数退避，非正序号返回 0。
func ExponentialBackoff(n int, config *Config) time.Duration {
	if n <= 0 || config.Delay <= 0 || config.MaxDelay <= 0 || config.Multiplier <= 0 ||
		math.IsNaN(config.Multiplier) || math.IsInf(config.Multiplier, 0) {
		return 0
	}
	multiplier := math.Pow(config.Multiplier, float64(n-1))
	if math.IsNaN(multiplier) || multiplier <= 0 {
		return 0
	}
	// 指数增长超过上限时直接封顶，避免浮点数转 duration 时溢出。
	if math.IsInf(multiplier, 1) ||
		multiplier >= float64(config.MaxDelay)/float64(config.Delay) {
		return config.MaxDelay
	}
	delay := durationFromFloat(float64(config.Delay) * multiplier)
	if delay > config.MaxDelay {
		return config.MaxDelay
	}
	return delay
}
