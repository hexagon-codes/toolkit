// Package errorx 提供错误处理工具函数
//
// 包含错误包装、错误链和通用错误类型，
// 用于在 Go 应用中构建健壮的错误处理机制。
//
// 基本用法:
//
//	err := errorx.Wrap(originalErr, "上下文信息")
//	if errorx.Is(err, targetErr) {
//	    // 处理特定错误
//	}
//
// --- English ---
//
// Package errorx provides error handling utilities.
//
// It includes error wrapping, error chains, and common error types
// for building robust error handling in Go applications.
//
// Basic usage:
//
//	err := errorx.Wrap(originalErr, "context message")
//	if errorx.Is(err, targetErr) {
//	    // handle specific error
//	}
package errorx
