// Package retry 提供带指数退避的重试功能
//
// 支持可配置的重试次数、延迟策略和自定义重试条件。
//
// 基本用法:
//
//	err := retry.Do(func() error {
//	    return someOperation()
//	}, retry.WithAttempts(3), retry.WithDelay(time.Second))
//
// 带指数退避:
//
//	err := retry.Do(func() error {
//	    return httpCall()
//	}, retry.WithBackoff(retry.ExponentialBackoff{
//	    InitialInterval: time.Second,
//	    MaxInterval:     time.Minute,
//	    Multiplier:      2,
//	}))
//
// --- English ---
//
// Package retry provides retry functionality with exponential backoff.
//
// It supports configurable retry attempts, delays, and custom retry conditions.
//
// Basic usage:
//
//	err := retry.Do(func() error {
//	    return someOperation()
//	}, retry.WithAttempts(3), retry.WithDelay(time.Second))
//
// With exponential backoff:
//
//	err := retry.Do(func() error {
//	    return httpCall()
//	}, retry.WithBackoff(retry.ExponentialBackoff{
//	    InitialInterval: time.Second,
//	    MaxInterval:     time.Minute,
//	    Multiplier:      2,
//	}))
package retry
