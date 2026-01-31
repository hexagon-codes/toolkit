package validator

import (
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// 预编译正则表达式（避免每次调用重新编译）
var (
	phoneRegex   = regexp.MustCompile(`^1[3-9]\d{9}$`)
	idCardRegex  = regexp.MustCompile(`^[1-9]\d{5}(18|19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]$`)
	numericRegex = regexp.MustCompile(`^\d+$`)
)

// Email 验证邮箱格式
func Email(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

// Phone 验证手机号（中国大陆）
func Phone(phone string) bool {
	if len(phone) != 11 {
		return false
	}
	return phoneRegex.MatchString(phone)
}

// URL 验证 URL 格式
func URL(rawURL string) bool {
	_, err := url.ParseRequestURI(rawURL)
	return err == nil
}

// IP 验证 IP 地址
func IP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// IPv4 验证 IPv4 地址
func IPv4(ip string) bool {
	parsedIP := net.ParseIP(ip)
	return parsedIP != nil && parsedIP.To4() != nil
}

// IPv6 验证 IPv6 地址
func IPv6(ip string) bool {
	parsedIP := net.ParseIP(ip)
	return parsedIP != nil && parsedIP.To4() == nil
}

// IDCard 验证身份证号（中国大陆18位）
// 包含格式验证和校验位验证
func IDCard(id string) bool {
	if len(id) != 18 {
		return false
	}

	// 格式验证
	if !idCardRegex.MatchString(id) {
		return false
	}

	// 校验位验证
	// 权重因子
	weights := []int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	// 校验码对照表
	checkCodes := []byte{'1', '0', 'X', '9', '8', '7', '6', '5', '4', '3', '2'}

	sum := 0
	for i := 0; i < 17; i++ {
		digit := int(id[i] - '0')
		sum += digit * weights[i]
	}

	// 计算校验码
	checkCode := checkCodes[sum%11]
	lastChar := id[17]
	if lastChar == 'x' {
		lastChar = 'X'
	}

	return lastChar == checkCode
}

// InRange 验证数字是否在范围内 [min, max]
func InRange(value, min, max int) bool {
	return value >= min && value <= max
}

// InRangeFloat 验证浮点数是否在范围内 [min, max]
func InRangeFloat(value, min, max float64) bool {
	return value >= min && value <= max
}

// MinLength 验证字符串最小长度
func MinLength(str string, min int) bool {
	return len([]rune(str)) >= min
}

// MaxLength 验证字符串最大长度
func MaxLength(str string, max int) bool {
	return len([]rune(str)) <= max
}

// LengthBetween 验证字符串长度在范围内 [min, max]
func LengthBetween(str string, min, max int) bool {
	length := len([]rune(str))
	return length >= min && length <= max
}

// IsNumeric 验证是否为数字
func IsNumeric(str string) bool {
	return numericRegex.MatchString(str)
}

// IsAlpha 验证是否为字母
func IsAlpha(str string) bool {
	for _, r := range str {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return len(str) > 0
}

// IsAlphaNumeric 验证是否为字母或数字
func IsAlphaNumeric(str string) bool {
	for _, r := range str {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return len(str) > 0
}

// Contains 验证字符串是否包含子串
func Contains(str, substr string) bool {
	return strings.Contains(str, substr)
}

// HasPrefix 验证字符串是否以前缀开头
func HasPrefix(str, prefix string) bool {
	return strings.HasPrefix(str, prefix)
}

// HasSuffix 验证字符串是否以后缀结尾
func HasSuffix(str, suffix string) bool {
	return strings.HasSuffix(str, suffix)
}

// In 验证值是否在列表中
func In[T comparable](value T, list []T) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// NotIn 验证值是否不在列表中
func NotIn[T comparable](value T, list []T) bool {
	return !In(value, list)
}

// Match 验证字符串是否匹配正则表达式
func Match(str, pattern string) bool {
	matched, _ := regexp.MatchString(pattern, str)
	return matched
}

// IsEmpty 验证字符串是否为空
func IsEmpty(str string) bool {
	return strings.TrimSpace(str) == ""
}

// NotEmpty 验证字符串不为空
func NotEmpty(str string) bool {
	return !IsEmpty(str)
}

// Password 验证密码强度（至少8位，包含大小写字母和数字）
func Password(password string) bool {
	if len(password) < 8 {
		return false
	}

	var (
		hasUpper  = false
		hasLower  = false
		hasNumber = false
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasNumber = true
		}
	}

	return hasUpper && hasLower && hasNumber
}

// Username 验证用户名（4-20位字母、数字、下划线）
func Username(username string) bool {
	if len(username) < 4 || len(username) > 20 {
		return false
	}
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_]+$`, username)
	return matched
}
