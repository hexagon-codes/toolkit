// Package stringx 提供高性能字符串操作工具
//
// 这个包提供了一些性能优化的字符串操作，包括零拷贝转换（使用 unsafe）。
//
// # 主要功能
//
// 零拷贝转换:
//   - BytesToString: []byte 转 string（零拷贝）
//   - String2Bytes: string 转 []byte（零拷贝）
//
// 通用转换:
//   - StringToSlice: 字符串转任意类型切片（使用反射）
//
// # 使用示例
//
//	import "github.com/hexagon-codes/toolkit/lang/stringx"
//
//	// 零拷贝转换（高性能）
//	bytes := []byte("hello world")
//	str := stringx.BytesToString(bytes)  // 零拷贝，极快
//
//	str := "hello world"
//	bytes := stringx.String2Bytes(str)   // 零拷贝，极快
//
//	// 字符串转切片（使用反射）
//	result := stringx.StringToSlice("1,2,3", ",")
//	// result = []int{1, 2, 3}
//
// # 性能考虑
//
// unsafe 操作（零拷贝）:
//   - BytesToString 和 String2Bytes 使用 unsafe 指针
//   - 避免了内存分配和拷贝，性能极高
//   - 但需要注意数据生命周期
//
// 使用场景:
//   - ✅ 只读操作
//   - ✅ 性能关键路径
//   - ❌ 需要修改数据
//   - ❌ 数据生命周期不确定
//
// # 安全警告
//
// ⚠️ 重要：BytesToString 和 String2Bytes 使用了 unsafe 包
//
// 不安全的用法:
//
//	bytes := []byte("hello")
//	str := stringx.BytesToString(bytes)
//	bytes[0] = 'H'  // ❌ 危险！会修改 str 的内容
//
// 安全的用法:
//
//	bytes := []byte("hello")
//	str := stringx.BytesToString(bytes)
//	// 只读取 str，不修改 bytes
//	fmt.Println(str)  // ✅ 安全
//
// # 设计原则
//
// 1. 性能优先：在安全的前提下追求极致性能
// 2. 零外部依赖：只使用 Go 标准库
// 3. 明确警告：文档中清楚说明 unsafe 的风险
//
// # 注意事项
//
// - BytesToString 和 String2Bytes 返回的数据共享底层内存
// - 修改其中一个会影响另一个
// - 仅在确保不会修改数据时使用
// - StringToSlice 使用反射，性能开销较大
//
// --- English ---
//
// Package stringx provides high-performance string operation utilities.
//
// This package provides performance-optimized string operations,
// including zero-copy conversions using the unsafe package.
//
// # Main Features
//
// Zero-copy conversions:
//   - BytesToString: convert []byte to string (zero-copy)
//   - String2Bytes: convert string to []byte (zero-copy)
//
// General conversions:
//   - StringToSlice: convert a string to a slice of any type (using reflection)
//
// # Usage Examples
//
//	import "github.com/hexagon-codes/toolkit/lang/stringx"
//
//	// Zero-copy conversion (high performance)
//	bytes := []byte("hello world")
//	str := stringx.BytesToString(bytes)  // zero-copy, extremely fast
//
//	str := "hello world"
//	bytes := stringx.String2Bytes(str)   // zero-copy, extremely fast
//
//	// String to slice (using reflection)
//	result := stringx.StringToSlice("1,2,3", ",")
//	// result = []int{1, 2, 3}
//
// # Performance Considerations
//
// Unsafe operations (zero-copy):
//   - BytesToString and String2Bytes use unsafe pointers
//   - Avoids memory allocation and copying for extremely high performance
//   - Be aware of data lifetime considerations
//
// Use cases:
//   - ✅ Read-only operations
//   - ✅ Performance-critical paths
//   - ❌ When data needs to be modified
//   - ❌ When data lifetime is uncertain
//
// # Safety Warning
//
// ⚠️ Important: BytesToString and String2Bytes use the unsafe package.
//
// Unsafe usage:
//
//	bytes := []byte("hello")
//	str := stringx.BytesToString(bytes)
//	bytes[0] = 'H'  // ❌ Dangerous! This modifies the content of str
//
// Safe usage:
//
//	bytes := []byte("hello")
//	str := stringx.BytesToString(bytes)
//	// Only read str, do not modify bytes
//	fmt.Println(str)  // ✅ Safe
//
// # Design Principles
//
// 1. Performance first: pursue maximum performance within safety bounds
// 2. Zero external dependencies: only uses Go standard library
// 3. Clear warnings: unsafe risks are clearly documented
//
// # Notes
//
// - Data returned by BytesToString and String2Bytes shares the same underlying memory
// - Modifying one will affect the other
// - Only use when you are certain the data will not be modified
// - StringToSlice uses reflection and has a higher performance overhead
package stringx
