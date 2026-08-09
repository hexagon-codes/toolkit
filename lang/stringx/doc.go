// Package stringx 提供字符串转换和处理辅助函数。
//
// BytesToString 与 StringToBytes 使用普通的 Go 安全转换，返回值不会与可变输入
// 共享底层存储。StringToSlice 通过反射将切片或数组转换为 []any。
package stringx
