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
//	import "github.com/everyday-items/toolkit/lang/stringx"
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
package stringx
