package stringx

import (
	"strings"
	"unicode/utf8"
)

// Reverse 反转字符串（支持 UTF-8）
func Reverse(s string) string {
	if s == "" {
		return ""
	}

	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}

	return string(runes)
}

// Truncate 截断字符串到指定长度，超出部分用 "..." 代替
func Truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}

	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}

	runes := []rune(s)
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}

	return string(runes[:maxLen-3]) + "..."
}

// TruncateWithSuffix 截断字符串到指定长度，使用自定义后缀
func TruncateWithSuffix(s string, maxLen int, suffix string) string {
	if maxLen <= 0 {
		return ""
	}

	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}

	suffixLen := utf8.RuneCountInString(suffix)
	if maxLen <= suffixLen {
		return string([]rune(s)[:maxLen])
	}

	runes := []rune(s)
	return string(runes[:maxLen-suffixLen]) + suffix
}

// PadLeft 左填充字符串到指定长度
func PadLeft(s string, length int, pad string) string {
	if pad == "" {
		pad = " "
	}

	sLen := utf8.RuneCountInString(s)
	if sLen >= length {
		return s
	}

	padLen := utf8.RuneCountInString(pad)
	repeatCount := (length - sLen + padLen - 1) / padLen

	padding := strings.Repeat(pad, repeatCount)
	paddingRunes := []rune(padding)
	needed := length - sLen

	return string(paddingRunes[:needed]) + s
}

// PadRight 右填充字符串到指定长度
func PadRight(s string, length int, pad string) string {
	if pad == "" {
		pad = " "
	}

	sLen := utf8.RuneCountInString(s)
	if sLen >= length {
		return s
	}

	padLen := utf8.RuneCountInString(pad)
	repeatCount := (length - sLen + padLen - 1) / padLen

	padding := strings.Repeat(pad, repeatCount)
	paddingRunes := []rune(padding)
	needed := length - sLen

	return s + string(paddingRunes[:needed])
}

// PadCenter 居中填充字符串到指定长度
func PadCenter(s string, length int, pad string) string {
	if pad == "" {
		pad = " "
	}

	sLen := utf8.RuneCountInString(s)
	if sLen >= length {
		return s
	}

	total := length - sLen
	left := total / 2
	right := total - left

	return PadLeft("", left, pad) + s + PadRight("", right, pad)
}

// RemovePrefix 移除字符串前缀
func RemovePrefix(s, prefix string) string {
	return strings.TrimPrefix(s, prefix)
}

// RemoveSuffix 移除字符串后缀
func RemoveSuffix(s, suffix string) string {
	return strings.TrimSuffix(s, suffix)
}

// IsEmpty 判断字符串是否为空
func IsEmpty(s string) bool {
	return s == ""
}

// IsBlank 判断字符串是否为空或只包含空白字符
func IsBlank(s string) bool {
	return strings.TrimSpace(s) == ""
}

// IsNotEmpty 判断字符串是否不为空
func IsNotEmpty(s string) bool {
	return s != ""
}

// IsNotBlank 判断字符串是否不为空且不只包含空白字符
func IsNotBlank(s string) bool {
	return strings.TrimSpace(s) != ""
}

// DefaultIfEmpty 如果字符串为空则返回默认值
func DefaultIfEmpty(s, defaultVal string) string {
	if s == "" {
		return defaultVal
	}
	return s
}

// DefaultIfBlank 如果字符串为空白则返回默认值
func DefaultIfBlank(s, defaultVal string) string {
	if strings.TrimSpace(s) == "" {
		return defaultVal
	}
	return s
}

// SubString 安全的子字符串（支持 UTF-8，不会 panic）
func SubString(s string, start, end int) string {
	runes := []rune(s)
	length := len(runes)

	if start < 0 {
		start = 0
	}
	if end > length {
		end = length
	}
	if start >= end || start >= length {
		return ""
	}

	return string(runes[start:end])
}

// ContainsAny 判断字符串是否包含任意一个子串
func ContainsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// ContainsAll 判断字符串是否包含所有子串
func ContainsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

// FirstNonEmpty 返回第一个非空字符串
func FirstNonEmpty(strs ...string) string {
	for _, s := range strs {
		if s != "" {
			return s
		}
	}
	return ""
}

// Repeat 重复字符串 n 次
func Repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(s, n)
}

// EnsurePrefix 确保字符串有指定前缀
func EnsurePrefix(s, prefix string) string {
	if strings.HasPrefix(s, prefix) {
		return s
	}
	return prefix + s
}

// EnsureSuffix 确保字符串有指定后缀
func EnsureSuffix(s, suffix string) string {
	if strings.HasSuffix(s, suffix) {
		return s
	}
	return s + suffix
}

// CountSubstring 统计子串出现次数
func CountSubstring(s, sub string) int {
	return strings.Count(s, sub)
}

// SplitAndTrim 分割字符串并去除每个部分的空白
func SplitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
