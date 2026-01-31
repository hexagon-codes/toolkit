// Package conv 提供通用类型转换工具
//
// 这个包提供了各种 Go 类型之间的转换功能，设计原则是转换失败时返回零值而不是 panic。
//
// # 主要功能
//
// 基础类型转换:
//   - String: 任意类型转字符串
//   - Int/Int32/Int64: 任意类型转整数
//   - Uint/Uint32/Uint64: 任意类型转无符号整数
//   - Float32/Float64: 任意类型转浮点数
//   - Bool: 任意类型转布尔值
//
// JSON/Map 操作:
//   - JSONToMap: JSON 字符串转 Map
//   - MapToJSON: Map 转 JSON 字符串
//
// Map 操作:
//   - MergeMaps: 合并多个 Map
//   - MapKeys: 提取所有 key
//   - MapValues: 提取所有 value
//
// # 使用示例
//
//	import "github.com/everyday-items/toolkit/lang/conv"
//
//	// 字符串转换
//	s := conv.String(123)           // "123"
//	s := conv.String(true)          // "true"
//
//	// 整数转换
//	i := conv.Int("456")            // 456
//	i := conv.Int("invalid")        // 0 (失败返回零值)
//
//	// 浮点数转换
//	f := conv.Float64("3.14")       // 3.14
//	f := conv.Float64(123)          // 123.0
//
//	// JSON/Map 互转
//	m, _ := conv.JSONToMap(`{"name":"Alice"}`)
//	json, _ := conv.MapToJSON(m)
//
//	// Map 操作
//	merged := conv.MergeMaps(m1, m2)
//	keys := conv.MapKeys(m)
//	values := conv.MapValues(m)
//
// # 设计原则
//
// 1. 失败不 panic：转换失败返回零值
// 2. 接口驱动：支持自定义类型实现转换接口
// 3. 智能推断：自动处理常见类型
// 4. 零外部依赖：只使用 Go 标准库
//
// # 转换规则
//
// 转换函数按照以下顺序尝试：
//  1. 检查 nil，返回零值
//  2. 类型断言处理常见 Go 类型
//  3. 检查是否实现了转换接口（如 iString, iFloat32）
//  4. 降级到标准库函数（strconv, fmt）
//  5. 转换失败返回零值
//
// # 注意事项
//
// - 所有转换函数都是并发安全的（纯函数，无状态）
// - 转换失败不会 panic，而是返回类型的零值
// - 字符串 "true"、"yes"、"1" 会被转换为 true
// - 浮点数转整数会截断小数部分
package conv
