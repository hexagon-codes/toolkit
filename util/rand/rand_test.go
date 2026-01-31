package rand

import (
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	length := 16
	str := String(length)

	if len(str) != length {
		t.Errorf("expected length %d, got %d", length, len(str))
	}

	// 验证只包含字母和数字
	for _, char := range str {
		if !strings.ContainsRune(AlphaNumeric, char) {
			t.Errorf("unexpected character: %c", char)
		}
	}
}

func TestNumericString(t *testing.T) {
	length := 6
	str := NumericString(length)

	if len(str) != length {
		t.Errorf("expected length %d, got %d", length, len(str))
	}

	// 验证只包含数字
	for _, char := range str {
		if !strings.ContainsRune(Numeric, char) {
			t.Errorf("unexpected character: %c", char)
		}
	}
}

func TestAlphaString(t *testing.T) {
	length := 10
	str := AlphaString(length)

	if len(str) != length {
		t.Errorf("expected length %d, got %d", length, len(str))
	}

	// 验证只包含字母
	for _, char := range str {
		if !strings.ContainsRune(Alpha, char) {
			t.Errorf("unexpected character: %c", char)
		}
	}
}

func TestLowerString(t *testing.T) {
	length := 10
	str := LowerString(length)

	if len(str) != length {
		t.Errorf("expected length %d, got %d", length, len(str))
	}

	// 验证只包含小写字母
	for _, char := range str {
		if !strings.ContainsRune(AlphaLower, char) {
			t.Errorf("unexpected character: %c", char)
		}
	}
}

func TestUpperString(t *testing.T) {
	length := 10
	str := UpperString(length)

	if len(str) != length {
		t.Errorf("expected length %d, got %d", length, len(str))
	}

	// 验证只包含大写字母
	for _, char := range str {
		if !strings.ContainsRune(AlphaUpper, char) {
			t.Errorf("unexpected character: %c", char)
		}
	}
}

func TestStringFrom(t *testing.T) {
	charset := "ABC123"
	length := 20
	str := StringFrom(charset, length)

	if len(str) != length {
		t.Errorf("expected length %d, got %d", length, len(str))
	}

	// 验证只包含指定字符
	for _, char := range str {
		if !strings.ContainsRune(charset, char) {
			t.Errorf("unexpected character: %c (not in %s)", char, charset)
		}
	}
}

func TestStringFrom_ZeroLength(t *testing.T) {
	str := StringFrom("ABC", 0)

	if str != "" {
		t.Errorf("expected empty string, got %s", str)
	}
}

func TestInt(t *testing.T) {
	min, max := 1, 100
	count := 1000

	for i := 0; i < count; i++ {
		num := Int(min, max)

		if num < min || num >= max {
			t.Errorf("Int() = %d, expected range [%d, %d)", num, min, max)
		}
	}
}

func TestInt_SameMinMax(t *testing.T) {
	num := Int(5, 5)

	if num != 5 {
		t.Errorf("Int(5, 5) = %d, expected 5", num)
	}
}

func TestInt_MinGreaterThanMax(t *testing.T) {
	num := Int(10, 5)

	if num != 10 {
		t.Errorf("Int(10, 5) = %d, expected 10", num)
	}
}

func TestInt64(t *testing.T) {
	min, max := int64(1), int64(1000000)
	count := 100

	for i := 0; i < count; i++ {
		num := Int64(min, max)

		if num < min || num >= max {
			t.Errorf("Int64() = %d, expected range [%d, %d)", num, min, max)
		}
	}
}

func TestBytes(t *testing.T) {
	length := 32
	bytes := Bytes(length)

	if len(bytes) != length {
		t.Errorf("expected length %d, got %d", length, len(bytes))
	}
}

func TestBytes_ZeroLength(t *testing.T) {
	bytes := Bytes(0)

	if bytes != nil {
		t.Errorf("expected nil, got %v", bytes)
	}
}

func TestBool(t *testing.T) {
	trueCount := 0
	falseCount := 0
	iterations := 1000

	for i := 0; i < iterations; i++ {
		if Bool() {
			trueCount++
		} else {
			falseCount++
		}
	}

	// 验证 true 和 false 都有出现
	if trueCount == 0 || falseCount == 0 {
		t.Errorf("Bool() not random: true=%d, false=%d", trueCount, falseCount)
	}

	// 验证大致均匀分布（允许一定误差）
	ratio := float64(trueCount) / float64(falseCount)
	if ratio < 0.3 || ratio > 3.0 {
		t.Errorf("Bool() distribution unbalanced: true=%d, false=%d, ratio=%.2f", trueCount, falseCount, ratio)
	}
}

func TestCode(t *testing.T) {
	length := 6
	code := Code(length)

	if len(code) != length {
		t.Errorf("expected length %d, got %d", length, len(code))
	}

	// 验证只包含数字
	for _, char := range code {
		if !strings.ContainsRune(Numeric, char) {
			t.Errorf("unexpected character in code: %c", char)
		}
	}
}

func TestToken(t *testing.T) {
	length := 32
	token := Token(length)

	if len(token) != length {
		t.Errorf("expected length %d, got %d", length, len(token))
	}

	// 验证只包含字母和数字
	for _, char := range token {
		if !strings.ContainsRune(AlphaNumeric, char) {
			t.Errorf("unexpected character in token: %c", char)
		}
	}
}

func TestString_Uniqueness(t *testing.T) {
	generated := make(map[string]bool)
	count := 100

	for i := 0; i < count; i++ {
		str := String(16)
		if generated[str] {
			t.Errorf("duplicate string generated: %s", str)
		}
		generated[str] = true
	}

	if len(generated) != count {
		t.Errorf("expected %d unique strings, got %d", count, len(generated))
	}
}

// Benchmark 测试
func BenchmarkString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		String(16)
	}
}

func BenchmarkNumericString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NumericString(6)
	}
}

func BenchmarkInt(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Int(1, 100)
	}
}

func BenchmarkBytes(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Bytes(32)
	}
}
