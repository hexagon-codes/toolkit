package rand

import (
	"crypto/rand"
	"math/big"
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

// Numeric 生成指定长度的随机数字字符串
func NumericString(length int) string {
	return StringFrom(Numeric, length)
}

// Alpha 生成指定长度的随机字母字符串
func AlphaString(length int) string {
	return StringFrom(Alpha, length)
}

// Lower 生成指定长度的小写字母字符串
func LowerString(length int) string {
	return StringFrom(AlphaLower, length)
}

// Upper 生成指定长度的大写字母字符串
func UpperString(length int) string {
	return StringFrom(AlphaUpper, length)
}

// StringFrom 从指定字符集生成随机字符串
// 使用 crypto/rand 生成加密安全的随机数
// 如果随机数生成失败会 panic（极少发生，通常表示系统熵源问题）
func StringFrom(charset string, length int) string {
	if length <= 0 {
		return ""
	}

	result := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))

	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			panic("crypto/rand.Int failed: " + err.Error())
		}
		result[i] = charset[num.Int64()]
	}

	return string(result)
}

// Int 生成指定范围的随机整数 [min, max)
// 使用 crypto/rand 生成加密安全的随机数
func Int(min, max int) int {
	if min >= max {
		return min
	}

	diff := max - min
	num, err := rand.Int(rand.Reader, big.NewInt(int64(diff)))
	if err != nil {
		panic("crypto/rand.Int failed: " + err.Error())
	}
	return int(num.Int64()) + min
}

// Int64 生成指定范围的随机 int64 [min, max)
// 使用 crypto/rand 生成加密安全的随机数
func Int64(min, max int64) int64 {
	if min >= max {
		return min
	}

	diff := max - min
	num, err := rand.Int(rand.Reader, big.NewInt(diff))
	if err != nil {
		panic("crypto/rand.Int failed: " + err.Error())
	}
	return num.Int64() + min
}

// Bytes 生成指定长度的随机字节数组
// 使用 crypto/rand 生成加密安全的随机字节
func Bytes(length int) []byte {
	if length <= 0 {
		return nil
	}

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		panic("crypto/rand.Read failed: " + err.Error())
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
