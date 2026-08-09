// Package retry 提供可校验配置、指数退避、抖动和上下文取消能力。
//
// 重试耗尽时返回值同时包含 ErrMaxAttemptsReached 和最后一次执行错误，
// 调用方可以使用 errors.Is 或 errors.As 判断两者。
//
// 基本用法：
//
//	err := retry.Do(func() error {
//		return someOperation()
//	}, retry.Attempts(3), retry.Delay(time.Second))
//
// 带上下文：
//
//	err := retry.DoWithContext(ctx, func() error {
//		return someOperation()
//	}, retry.If(isRetryable))
package retry
