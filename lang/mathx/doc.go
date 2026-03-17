// Package mathx 提供数学运算的工具函数
//
// 这个包提供了 Go 标准库 math 包的增强版本，主要特点是支持泛型。
//
// # 主要功能
//
// 比较和限制:
//   - Min/Max: 泛型版本的最小值/最大值
//   - MinMax: 同时返回最小值和最大值
//   - Clamp: 将值限制在指定范围内
//
// 绝对值:
//   - Abs: 泛型版本的绝对值
//   - AbsDiff: 两个数的差的绝对值
//
// 四舍五入:
//   - Round: 四舍五入到整数
//   - RoundTo: 四舍五入到指定小数位
//   - Ceil/Floor/Trunc: 取整函数
//
// # 使用示例
//
//	import "github.com/hexagon-codes/toolkit/lang/mathx"
//
//	// 泛型 Min/Max（支持 int, float64, string 等）
//	min := mathx.Min(3, 1, 4, 1, 5)           // 1 (int)
//	max := mathx.Max(3.14, 2.71, 1.41)        // 3.14 (float64)
//	minStr := mathx.Min("c", "a", "b")        // "a" (string)
//
//	// 限制值
//	clamped := mathx.Clamp(15, 0, 10)         // 10
//
//	// 绝对值（泛型）
//	abs := mathx.Abs(-5)                      // 5 (int)
//	absf := mathx.Abs(-3.14)                  // 3.14 (float64)
//
//	// 四舍五入
//	rounded := mathx.RoundTo(3.14159, 2)      // 3.14
//
// # 与标准库 math 的区别
//
// 1. 支持泛型：Min/Max/Abs 支持所有可比较类型
// 2. 更方便：Min/Max 支持可变参数
// 3. 零依赖：只使用 Go 标准库
//
// # 设计原则
//
// 1. 类型安全：使用泛型，编译时检查
// 2. 性能优先：函数尽可能内联
// 3. API 简洁：与标准库保持一致的命名
//
// # 注意事项
//
// - 所有函数都是并发安全的（纯函数，无状态）
// - 空参数会返回类型的零值
// - 浮点数运算遵循 IEEE 754 标准
//
// --- English ---
//
// Package mathx provides utility functions for mathematical operations.
//
// This package provides an enhanced version of the Go standard library math
// package, with generic support as its primary feature.
//
// # Main Features
//
// Comparison and clamping:
//   - Min/Max: generic minimum/maximum value
//   - MinMax: return both minimum and maximum at once
//   - Clamp: constrain a value within a specified range
//
// Absolute value:
//   - Abs: generic absolute value
//   - AbsDiff: absolute difference between two numbers
//
// Rounding:
//   - Round: round to nearest integer
//   - RoundTo: round to specified decimal places
//   - Ceil/Floor/Trunc: rounding functions
//
// # Usage Examples
//
//	import "github.com/hexagon-codes/toolkit/lang/mathx"
//
//	// Generic Min/Max (supports int, float64, string, etc.)
//	min := mathx.Min(3, 1, 4, 1, 5)           // 1 (int)
//	max := mathx.Max(3.14, 2.71, 1.41)        // 3.14 (float64)
//	minStr := mathx.Min("c", "a", "b")        // "a" (string)
//
//	// Clamp a value
//	clamped := mathx.Clamp(15, 0, 10)         // 10
//
//	// Absolute value (generic)
//	abs := mathx.Abs(-5)                      // 5 (int)
//	absf := mathx.Abs(-3.14)                  // 3.14 (float64)
//
//	// Rounding
//	rounded := mathx.RoundTo(3.14159, 2)      // 3.14
//
// # Differences from the Standard math Package
//
// 1. Generic support: Min/Max/Abs support all comparable types
// 2. More convenient: Min/Max accept variadic arguments
// 3. Zero dependencies: only uses Go standard library
//
// # Design Principles
//
// 1. Type-safe: uses generics for compile-time checks
// 2. Performance first: functions are inlined where possible
// 3. Clean API: naming is consistent with the standard library
//
// # Notes
//
// - All functions are concurrency-safe (pure functions, stateless)
// - Empty arguments return the type's zero value
// - Floating-point arithmetic follows the IEEE 754 standard
package mathx
