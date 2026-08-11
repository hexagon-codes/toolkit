// Package rand 提供密码学安全的随机值生成工具。
package rand

import (
	"crypto/rand"
)

const (
	// Numeric 数字字符
	Numeric = "0123456789"
	// Alpha 字母字符
	Alpha = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	// AlphaNumeric 字母+数字
	AlphaNumeric = Numeric + Alpha
	// AlphaLower 小写字母
	AlphaLower = "abcdefghijklmnopqrstuvwxyz"
	// AlphaUpper 大写字母
	AlphaUpper = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
)

// String 生成指定长度的随机字符串（字母+数字）
func String(length int) string {
	return StringFrom(AlphaNumeric, length)
}

// NumericString 生成指定长度的随机数字字符串。
func NumericString(length int) string {
	return StringFrom(Numeric, length)
}

// AlphaString 生成指定长度的随机字母字符串。
func AlphaString(length int) string {
	return StringFrom(Alpha, length)
}

// LowerString 生成指定长度的小写字母字符串。
func LowerString(length int) string {
	return StringFrom(AlphaLower, length)
}

// UpperString 生成指定长度的大写字母字符串。
func UpperString(length int) string {
	return StringFrom(AlphaUpper, length)
}

// StringFrom 从指定字符集生成随机字符串
// 使用 crypto/rand 生成加密安全的随机数
// 如果随机数生成失败会 panic（极少发生，通常表示系统熵源问题）
//
// 若需要在熵源失败时以 error 形式优雅传播而非 panic，请使用 TryStringFrom。
func StringFrom(charset string, length int) string {
	// 复用与 TryStringFrom 共享的核心实现 stringFrom，保持行为完全一致；
	// 唯一区别是本函数在出错时将 error 转为 panic（保持原有契约不变）。
	s, err := stringFrom(rand.Reader, charset, length)
	if err != nil {
		panic(err)
	}
	return s
}

// Int 生成指定范围的随机整数 [min, max)
// 使用 crypto/rand 生成加密安全的随机数
func Int(lower, upper int) int {
	num, err := TryInt(lower, upper)
	if err != nil {
		panic(err)
	}
	return num
}

// Int64 生成指定范围的随机 int64 [min, max)
// 使用 crypto/rand 生成加密安全的随机数
func Int64(lower, upper int64) int64 {
	num, err := TryInt64(lower, upper)
	if err != nil {
		panic(err)
	}
	return num
}

// Bytes 生成指定长度的随机字节数组
// 使用 crypto/rand 生成加密安全的随机字节
func Bytes(length int) []byte {
	bytes, err := TryBytes(length)
	if err != nil {
		panic(err)
	}
	return bytes
}

// Bool 生成随机布尔值
func Bool() bool {
	return Int(0, 2) == 1
}

// Code 生成指定长度的验证码（数字）
func Code(length int) string {
	return NumericString(length)
}

// Token 生成指定长度的 Token（字母+数字）
func Token(length int) string {
	return String(length)
}
