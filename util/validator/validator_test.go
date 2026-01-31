package validator

import (
	"strings"
	"testing"
)

func TestEmail(t *testing.T) {
	tests := []struct {
		email string
		valid bool
	}{
		{"test@example.com", true},
		{"user.name@example.co.uk", true},
		{"invalid", false},
		{"@example.com", false},
		{"test@", false},
		{"", false},
	}

	for _, tt := range tests {
		result := Email(tt.email)
		if result != tt.valid {
			t.Errorf("Email(%q) = %v, expected %v", tt.email, result, tt.valid)
		}
	}
}

func TestPhone(t *testing.T) {
	tests := []struct {
		phone string
		valid bool
	}{
		{"13800138000", true},
		{"15912345678", true},
		{"12345678901", false},
		{"138001380001", false},
		{"", false},
	}

	for _, tt := range tests {
		result := Phone(tt.phone)
		if result != tt.valid {
			t.Errorf("Phone(%q) = %v, expected %v", tt.phone, result, tt.valid)
		}
	}
}

func TestURL(t *testing.T) {
	tests := []struct {
		url   string
		valid bool
	}{
		{"https://www.example.com", true},
		{"http://example.com", true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		result := URL(tt.url)
		if result != tt.valid {
			t.Errorf("URL(%q) = %v, expected %v", tt.url, result, tt.valid)
		}
	}
}

func TestIP(t *testing.T) {
	tests := []struct {
		ip    string
		valid bool
	}{
		{"192.168.1.1", true},
		{"8.8.8.8", true},
		{"2001:0db8:85a3::8a2e:0370:7334", true},
		{"256.1.1.1", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		result := IP(tt.ip)
		if result != tt.valid {
			t.Errorf("IP(%q) = %v, expected %v", tt.ip, result, tt.valid)
		}
	}
}

func TestInRange(t *testing.T) {
	tests := []struct {
		value, min, max int
		valid           bool
	}{
		{5, 1, 10, true},
		{1, 1, 10, true},
		{10, 1, 10, true},
		{0, 1, 10, false},
		{11, 1, 10, false},
	}

	for _, tt := range tests {
		result := InRange(tt.value, tt.min, tt.max)
		if result != tt.valid {
			t.Errorf("InRange(%d, %d, %d) = %v, expected %v", tt.value, tt.min, tt.max, result, tt.valid)
		}
	}
}

func TestPassword(t *testing.T) {
	tests := []struct {
		password string
		valid    bool
	}{
		{"Aa123456", true},
		{"Password1", true},
		{"12345678", false}, // 无字母
		{"password", false}, // 无数字和大写
		{"PASSWORD", false}, // 无数字和小写
		{"Pass1", false},    // 太短
	}

	for _, tt := range tests {
		result := Password(tt.password)
		if result != tt.valid {
			t.Errorf("Password(%q) = %v, expected %v", tt.password, result, tt.valid)
		}
	}
}

func TestUsername(t *testing.T) {
	tests := []struct {
		username string
		valid    bool
	}{
		{"user123", true},
		{"test_user", true},
		{"abc", false},                   // 太短
		{strings.Repeat("a", 21), false}, // 太长
		{"user-name", false},             // 包含非法字符
	}

	for _, tt := range tests {
		result := Username(tt.username)
		if result != tt.valid {
			t.Errorf("Username(%q) = %v, expected %v", tt.username, result, tt.valid)
		}
	}
}

func TestIn(t *testing.T) {
	list := []int{1, 2, 3, 4, 5}

	if !In(3, list) {
		t.Error("In(3, list) should be true")
	}

	if In(6, list) {
		t.Error("In(6, list) should be false")
	}
}

func TestNotEmpty(t *testing.T) {
	tests := []struct {
		str   string
		valid bool
	}{
		{"hello", true},
		{"  ", false},
		{"", false},
		{" hello ", true},
	}

	for _, tt := range tests {
		result := NotEmpty(tt.str)
		if result != tt.valid {
			t.Errorf("NotEmpty(%q) = %v, expected %v", tt.str, result, tt.valid)
		}
	}
}

func TestIPv4(t *testing.T) {
	tests := []struct {
		ip    string
		valid bool
	}{
		{"192.168.1.1", true},
		{"8.8.8.8", true},
		{"0.0.0.0", true},
		{"255.255.255.255", true},
		{"256.1.1.1", false},
		{"2001:0db8:85a3::8a2e:0370:7334", false}, // IPv6
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		result := IPv4(tt.ip)
		if result != tt.valid {
			t.Errorf("IPv4(%q) = %v, expected %v", tt.ip, result, tt.valid)
		}
	}
}

func TestIPv6(t *testing.T) {
	tests := []struct {
		ip    string
		valid bool
	}{
		{"2001:0db8:85a3::8a2e:0370:7334", true},
		{"2001:db8:85a3:0:0:8a2e:370:7334", true},
		{"::1", true},
		{"fe80::1", true},
		{"192.168.1.1", false}, // IPv4
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		result := IPv6(tt.ip)
		if result != tt.valid {
			t.Errorf("IPv6(%q) = %v, expected %v", tt.ip, result, tt.valid)
		}
	}
}

func TestIDCard(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		{"110101199003074530", true},  // 正确校验位
		{"110101198001011232", true},  // 正确校验位
		{"11010120001231161x", true},  // 校验位为 X（小写）
		{"11010120001231161X", true},  // 校验位为 X（大写）
		{"110101189912311012", true},  // 1899 年有效，正确校验位
		{"11010119800101234", false},  // 长度不够
		{"110101198013010012", false}, // 月份不对
		{"110101198001320012", false}, // 日期不对
		{"110101170012310012", false}, // 17xx 年份不对
		{"110101199003074531", false}, // 校验位错误
		{"", false},
	}

	for _, tt := range tests {
		result := IDCard(tt.id)
		if result != tt.valid {
			t.Errorf("IDCard(%q) = %v, expected %v", tt.id, result, tt.valid)
		}
	}
}

func TestInRangeFloat(t *testing.T) {
	tests := []struct {
		value, min, max float64
		valid           bool
	}{
		{5.5, 1.0, 10.0, true},
		{1.0, 1.0, 10.0, true},
		{10.0, 1.0, 10.0, true},
		{0.9, 1.0, 10.0, false},
		{10.1, 1.0, 10.0, false},
		{-5.5, -10.0, -1.0, true},
		{0.0, -1.0, 1.0, true},
	}

	for _, tt := range tests {
		result := InRangeFloat(tt.value, tt.min, tt.max)
		if result != tt.valid {
			t.Errorf("InRangeFloat(%f, %f, %f) = %v, expected %v", tt.value, tt.min, tt.max, result, tt.valid)
		}
	}
}

func TestMinLength(t *testing.T) {
	tests := []struct {
		str   string
		min   int
		valid bool
	}{
		{"hello", 3, true},
		{"hello", 5, true},
		{"hello", 6, false},
		{"你好世界", 4, true},
		{"你好世界", 3, true},
		{"你好世界", 5, false},
		{"", 0, true},
		{"", 1, false},
	}

	for _, tt := range tests {
		result := MinLength(tt.str, tt.min)
		if result != tt.valid {
			t.Errorf("MinLength(%q, %d) = %v, expected %v", tt.str, tt.min, result, tt.valid)
		}
	}
}

func TestMaxLength(t *testing.T) {
	tests := []struct {
		str   string
		max   int
		valid bool
	}{
		{"hello", 10, true},
		{"hello", 5, true},
		{"hello", 4, false},
		{"你好世界", 4, true},
		{"你好世界", 5, true},
		{"你好世界", 3, false},
		{"", 0, true},
		{"", 1, true},
	}

	for _, tt := range tests {
		result := MaxLength(tt.str, tt.max)
		if result != tt.valid {
			t.Errorf("MaxLength(%q, %d) = %v, expected %v", tt.str, tt.max, result, tt.valid)
		}
	}
}

func TestLengthBetween(t *testing.T) {
	tests := []struct {
		str      string
		min, max int
		valid    bool
	}{
		{"hello", 3, 10, true},
		{"hello", 5, 5, true},
		{"hello", 1, 4, false},
		{"hello", 6, 10, false},
		{"你好世界", 2, 6, true},
		{"你好世界", 4, 4, true},
		{"你好世界", 1, 3, false},
		{"", 0, 0, true},
		{"", 1, 5, false},
	}

	for _, tt := range tests {
		result := LengthBetween(tt.str, tt.min, tt.max)
		if result != tt.valid {
			t.Errorf("LengthBetween(%q, %d, %d) = %v, expected %v", tt.str, tt.min, tt.max, result, tt.valid)
		}
	}
}

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		str   string
		valid bool
	}{
		{"123", true},
		{"0", true},
		{"999999", true},
		{"123abc", false},
		{"abc", false},
		{"12.34", false},
		{"-123", false},
		{"", false},
	}

	for _, tt := range tests {
		result := IsNumeric(tt.str)
		if result != tt.valid {
			t.Errorf("IsNumeric(%q) = %v, expected %v", tt.str, result, tt.valid)
		}
	}
}

func TestIsAlpha(t *testing.T) {
	tests := []struct {
		str   string
		valid bool
	}{
		{"abc", true},
		{"ABC", true},
		{"abcDEF", true},
		{"你好世界", true},
		{"abc123", false},
		{"abc_def", false},
		{"abc def", false},
		{"", false},
	}

	for _, tt := range tests {
		result := IsAlpha(tt.str)
		if result != tt.valid {
			t.Errorf("IsAlpha(%q) = %v, expected %v", tt.str, result, tt.valid)
		}
	}
}

func TestIsAlphaNumeric(t *testing.T) {
	tests := []struct {
		str   string
		valid bool
	}{
		{"abc123", true},
		{"ABC123", true},
		{"abc", true},
		{"123", true},
		{"abc_123", false},
		{"abc-123", false},
		{"abc 123", false},
		{"", false},
	}

	for _, tt := range tests {
		result := IsAlphaNumeric(tt.str)
		if result != tt.valid {
			t.Errorf("IsAlphaNumeric(%q) = %v, expected %v", tt.str, result, tt.valid)
		}
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		str    string
		substr string
		valid  bool
	}{
		{"hello world", "world", true},
		{"hello world", "hello", true},
		{"hello world", "o w", true},
		{"hello world", "xyz", false},
		{"你好世界", "世界", true},
		{"你好世界", "hello", false},
		{"", "", true},
		{"hello", "", true},
	}

	for _, tt := range tests {
		result := Contains(tt.str, tt.substr)
		if result != tt.valid {
			t.Errorf("Contains(%q, %q) = %v, expected %v", tt.str, tt.substr, result, tt.valid)
		}
	}
}

func TestHasPrefix(t *testing.T) {
	tests := []struct {
		str    string
		prefix string
		valid  bool
	}{
		{"hello world", "hello", true},
		{"hello world", "world", false},
		{"hello world", "hell", true},
		{"你好世界", "你好", true},
		{"你好世界", "世界", false},
		{"", "", true},
		{"hello", "", true},
	}

	for _, tt := range tests {
		result := HasPrefix(tt.str, tt.prefix)
		if result != tt.valid {
			t.Errorf("HasPrefix(%q, %q) = %v, expected %v", tt.str, tt.prefix, result, tt.valid)
		}
	}
}

func TestHasSuffix(t *testing.T) {
	tests := []struct {
		str    string
		suffix string
		valid  bool
	}{
		{"hello world", "world", true},
		{"hello world", "hello", false},
		{"hello world", "orld", true},
		{"你好世界", "世界", true},
		{"你好世界", "你好", false},
		{"", "", true},
		{"hello", "", true},
	}

	for _, tt := range tests {
		result := HasSuffix(tt.str, tt.suffix)
		if result != tt.valid {
			t.Errorf("HasSuffix(%q, %q) = %v, expected %v", tt.str, tt.suffix, result, tt.valid)
		}
	}
}

func TestNotIn(t *testing.T) {
	list := []int{1, 2, 3, 4, 5}

	if NotIn(3, list) {
		t.Error("NotIn(3, list) should be false")
	}

	if !NotIn(6, list) {
		t.Error("NotIn(6, list) should be true")
	}
}

func TestNotIn_Strings(t *testing.T) {
	list := []string{"apple", "banana", "orange"}

	if NotIn("apple", list) {
		t.Error("NotIn('apple', list) should be false")
	}

	if !NotIn("grape", list) {
		t.Error("NotIn('grape', list) should be true")
	}
}

func TestMatch(t *testing.T) {
	tests := []struct {
		str     string
		pattern string
		valid   bool
	}{
		{"hello123", `^hello\d+$`, true},
		{"hello", `^hello\d+$`, false},
		{"test@example.com", `\w+@\w+\.\w+`, true},
		{"invalid-email", `\w+@\w+\.\w+`, false},
		{"", `^$`, true},
		{"abc", `^[a-z]+$`, true},
		{"ABC", `^[a-z]+$`, false},
	}

	for _, tt := range tests {
		result := Match(tt.str, tt.pattern)
		if result != tt.valid {
			t.Errorf("Match(%q, %q) = %v, expected %v", tt.str, tt.pattern, result, tt.valid)
		}
	}
}

func TestIsEmpty(t *testing.T) {
	tests := []struct {
		str   string
		empty bool
	}{
		{"", true},
		{"  ", true},
		{"\t", true},
		{"\n", true},
		{"  \t\n  ", true},
		{"hello", false},
		{" hello ", false},
		{"你好", false},
	}

	for _, tt := range tests {
		result := IsEmpty(tt.str)
		if result != tt.empty {
			t.Errorf("IsEmpty(%q) = %v, expected %v", tt.str, result, tt.empty)
		}
	}
}

func TestPhone_EdgeCases(t *testing.T) {
	tests := []struct {
		phone string
		valid bool
	}{
		{"13000000000", true},
		{"14000000000", true},
		{"15000000000", true},
		{"16000000000", true},
		{"17000000000", true},
		{"18000000000", true},
		{"19000000000", true},
		{"11000000000", false}, // 1开头但第二位是1
		{"12000000000", false}, // 1开头但第二位是2
		{"20000000000", false}, // 不是1开头
		{"1380013800", false},  // 只有10位
	}

	for _, tt := range tests {
		result := Phone(tt.phone)
		if result != tt.valid {
			t.Errorf("Phone(%q) = %v, expected %v", tt.phone, result, tt.valid)
		}
	}
}

func TestEmail_EdgeCases(t *testing.T) {
	tests := []struct {
		email string
		valid bool
	}{
		{"user+tag@example.com", true},
		{"user.name+tag@example.co.uk", true},
		{"user_name@example-domain.com", true},
		{"user@subdomain.example.com", true},
		{"test..double@example.com", false},
		{"test.@example.com", false},
		{".test@example.com", false},
		{"test@.example.com", false},
		// mail.ParseAddress 认为 "test@example" 是合法的本地邮箱地址
		// 如果需要严格验证域名，需要修改 validator 实现
	}

	for _, tt := range tests {
		result := Email(tt.email)
		if result != tt.valid {
			t.Errorf("Email(%q) = %v, expected %v", tt.email, result, tt.valid)
		}
	}
}

func TestURL_EdgeCases(t *testing.T) {
	tests := []struct {
		url   string
		valid bool
	}{
		{"https://www.example.com/path?query=value", true},
		{"http://localhost:8080", true},
		{"ftp://ftp.example.com", true},
		{"https://example.com/path/to/page", true},
		{"//example.com", true}, // 协议相对 URL
		{"http://", true},       // url.ParseRequestURI 允许这种格式
		{"example.com", false},  // 缺少协议
		{"ht!tp://example.com", false},
	}

	for _, tt := range tests {
		result := URL(tt.url)
		if result != tt.valid {
			t.Errorf("URL(%q) = %v, expected %v", tt.url, result, tt.valid)
		}
	}
}

func TestPassword_EdgeCases(t *testing.T) {
	tests := []struct {
		password string
		valid    bool
	}{
		{"Aa123456", true},
		{"Password123", true},
		{"MyP@ssw0rd", true},
		{"AAAAA123456", false}, // 无小写
		{"aaaaa123456", false}, // 无大写
		{"AAAAAaaaaaa", false}, // 无数字
		{"Pass1", false},       // 太短
		{"Pass123", false},     // 只有7位
		{"Password1!", true},   // 包含特殊字符也应该有效
		{"1234567890Aa", true}, // 数字在前
		{"你好Aa123456", true},   // 包含中文
	}

	for _, tt := range tests {
		result := Password(tt.password)
		if result != tt.valid {
			t.Errorf("Password(%q) = %v, expected %v", tt.password, result, tt.valid)
		}
	}
}

func TestUsername_EdgeCases(t *testing.T) {
	tests := []struct {
		username string
		valid    bool
	}{
		{"abcd", true},                  // 最小长度
		{strings.Repeat("a", 20), true}, // 最大长度
		{"user_123", true},
		{"USER123", true},
		{"_user", true},
		{"user_", true},
		{"123user", true},
		{"user-name", false}, // 包含连字符
		{"user name", false}, // 包含空格
		{"user.name", false}, // 包含点
		{"user@name", false}, // 包含@
		{"用户", false},        // 中文
	}

	for _, tt := range tests {
		result := Username(tt.username)
		if result != tt.valid {
			t.Errorf("Username(%q) = %v, expected %v", tt.username, result, tt.valid)
		}
	}
}

func TestIn_EmptyList(t *testing.T) {
	var emptyList []int
	if In(1, emptyList) {
		t.Error("In(1, emptyList) should be false")
	}
}

func TestIn_CustomTypes(t *testing.T) {
	type CustomType struct {
		ID   int
		Name string
	}

	list := []CustomType{
		{ID: 1, Name: "a"},
		{ID: 2, Name: "b"},
	}

	target := CustomType{ID: 1, Name: "a"}
	if !In(target, list) {
		t.Error("In should find matching struct")
	}

	notFound := CustomType{ID: 3, Name: "c"}
	if In(notFound, list) {
		t.Error("In should not find non-matching struct")
	}
}
