// Package timex 提供时间工具函数
//
// 这个包提供了时间戳格式化的便捷工具，支持毫秒和秒级时间戳。
//
// # 主要功能
//
// 毫秒时间戳:
//   - MsecFormat: 毫秒时间戳转 "Y-m-d H:i:s" 格式
//   - MsecFormatWithLayout: 毫秒时间戳转自定义格式
//
// 秒级时间戳:
//   - SecFormat: 秒级时间戳转 "Y-m-d H:i:s" 格式
//   - SecFormatWithLayout: 秒级时间戳转自定义格式
//
// # 使用示例
//
//	import "github.com/everyday-items/toolkit/lang/timex"
//	import "time"
//
//	// 毫秒时间戳格式化
//	ms := time.Now().UnixMilli()
//	formatted := timex.MsecFormat(ms)
//	// Output: "2024-01-29 15:04:05"
//
//	// 自定义格式
//	custom := timex.MsecFormatWithLayout(ms, "2006/01/02")
//	// Output: "2024/01/29"
//
//	timeOnly := timex.MsecFormatWithLayout(ms, "15:04:05")
//	// Output: "15:04:05"
//
//	// 秒级时间戳格式化
//	sec := time.Now().Unix()
//	formatted := timex.SecFormat(sec)
//	// Output: "2024-01-29 15:04:05"
//
//	custom := timex.SecFormatWithLayout(sec, "2006-01-02")
//	// Output: "2024-01-29"
//
// # 时间格式说明
//
// Go 的时间格式使用参考时间：2006-01-02 15:04:05
//
// 常用格式:
//   - "2006-01-02 15:04:05" - 完整日期时间
//   - "2006-01-02" - 日期
//   - "15:04:05" - 时间
//   - "2006/01/02" - 斜杠分隔
//   - "02-Jan-2006" - 英文月份
//
// # 设计原则
//
// 1. 简单易用：提供常用格式的快捷函数
// 2. 灵活扩展：支持自定义格式
// 3. 零外部依赖：只使用 Go 标准库
//
// # 注意事项
//
// - 所有函数都使用本地时区
// - 时间戳为 0 会返回 "1970-01-01 08:00:00"（北京时间）
// - 所有函数都是并发安全的
package timex
