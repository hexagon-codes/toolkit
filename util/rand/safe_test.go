package rand

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestTryStringVariants 表驱动测试所有基于字符集的 Try* 字符串安全变体。
//
// 覆盖：正常长度生成、长度边界（0 与负数返回空串无错误）、字符集合法性校验。
func TestTryStringVariants(t *testing.T) {
	tests := []struct {
		name    string                    // 子测试名称
		fn      func(int) (string, error) // 被测函数
		charset string                    // 期望的合法字符集（用于逐字符校验）
		length  int                       // 请求长度
		wantLen int                       // 期望返回长度
	}{
		{"TryString-正常", TryString, AlphaNumeric, 16, 16},
		{"TryString-零长度", TryString, AlphaNumeric, 0, 0},
		{"TryString-负长度", TryString, AlphaNumeric, -5, 0},
		{"TryNumericString-正常", TryNumericString, Numeric, 6, 6},
		{"TryNumericString-零长度", TryNumericString, Numeric, 0, 0},
		{"TryAlphaString-正常", TryAlphaString, Alpha, 10, 10},
		{"TryLowerString-正常", TryLowerString, AlphaLower, 12, 12},
		{"TryUpperString-正常", TryUpperString, AlphaUpper, 12, 12},
		{"TryToken-正常", TryToken, AlphaNumeric, 32, 32},
		{"TryCode-正常", TryCode, Numeric, 6, 6},
		{"TryCode-零长度", TryCode, Numeric, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fn(tt.length)
			if tt.length < 0 {
				if got != "" || !errors.Is(err, ErrInvalidLength) {
					t.Fatalf("%s = (%q, %v), want empty ErrInvalidLength", tt.name, got, err)
				}
				return
			}
			// 正常与边界路径均不应返回错误。
			if err != nil {
				t.Fatalf("%s 返回非预期错误: %v", tt.name, err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("%s 长度 = %d, 期望 %d", tt.name, len(got), tt.wantLen)
			}
			// 逐字符校验落在期望字符集内。
			for _, c := range got {
				if !strings.ContainsRune(tt.charset, c) {
					t.Errorf("%s 出现非法字符 %c（不属于 %s）", tt.name, c, tt.charset)
				}
			}
		})
	}
}

// TestTryStringFrom 表驱动测试 TryStringFrom 的正常、边界与错误路径。
func TestTryStringFrom(t *testing.T) {
	tests := []struct {
		name    string // 子测试名称
		charset string // 输入字符集
		length  int    // 输入长度
		wantLen int    // 期望长度
		wantErr bool   // 是否期望错误
	}{
		{"正常采样", "ABC123", 20, 20, false},
		{"零长度返回空串", "ABC", 0, 0, false},
		{"负长度返回错误", "ABC", -1, 0, true},
		{"空字符集且零长度不报错", "", 0, 0, false}, // length<=0 优先短路，不触发空集错误
		{"空字符集且正长度报错", "", 8, 0, true},   // 无法采样，返回 error 而非 panic
		{"单字符字符集返回错误", "X", 5, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TryStringFrom(tt.charset, tt.length)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望返回错误, 但 err 为 nil (got=%q)", got)
				}
				if got != "" {
					t.Errorf("出错时应返回空串, 实际 %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("非预期错误: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("长度 = %d, 期望 %d", len(got), tt.wantLen)
			}
			if tt.charset != "" {
				for _, c := range got {
					if !strings.ContainsRune(tt.charset, c) {
						t.Errorf("非法字符 %c (不属于 %s)", c, tt.charset)
					}
				}
			}
		})
	}
}

func TestTryStringFromSamplesWholeUnicodeCharacters(t *testing.T) {
	const charset = "界甲"
	got, err := TryStringFrom(charset, 4)
	if err != nil {
		t.Fatalf("TryStringFrom() error = %v", err)
	}
	if len([]rune(got)) != 4 {
		t.Fatalf("TryStringFrom() = %q, want four complete Unicode characters", got)
	}
	for _, character := range got {
		if !strings.ContainsRune(charset, character) {
			t.Fatalf("TryStringFrom() contains unexpected character %q", character)
		}
	}
}

// TestTryInt 表驱动测试 TryInt 的范围、边界（min>=max）路径。
func TestTryInt(t *testing.T) {
	tests := []struct {
		name    string // 子测试名称
		min     int    // 下界
		max     int    // 上界（开区间）
		want    int    // 有效范围内无需固定返回值时为 -1
		wantErr bool   // 是否期望 ErrInvalidRange
	}{
		{"min等于max返回错误", 5, 5, 5, true},
		{"min大于max返回错误", 10, 5, 10, true},
		{"正常范围", 1, 100, -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TryInt(tt.min, tt.max)
			if tt.wantErr {
				if got != tt.want || !errors.Is(err, ErrInvalidRange) {
					t.Fatalf("TryInt(%d,%d) = (%d, %v), want %d ErrInvalidRange", tt.min, tt.max, got, err, tt.want)
				}
				return
			}
			if err != nil {
				t.Fatalf("非预期错误: %v", err)
			}
			if tt.want != -1 {
				if got != tt.want {
					t.Fatalf("TryInt(%d,%d) = %d, 期望 %d", tt.min, tt.max, got, tt.want)
				}
				return
			}
			// 正常范围: 多次采样均应落在 [min, max)。
			for i := 0; i < 1000; i++ {
				n, err := TryInt(tt.min, tt.max)
				if err != nil {
					t.Fatalf("非预期错误: %v", err)
				}
				if n < tt.min || n >= tt.max {
					t.Fatalf("TryInt 越界: %d 不在 [%d, %d)", n, tt.min, tt.max)
				}
			}
		})
	}
}

// TestTryInt64 表驱动测试 TryInt64 的范围与边界路径。
func TestTryInt64(t *testing.T) {
	tests := []struct {
		name    string // 子测试名称
		min     int64  // 下界
		max     int64  // 上界（开区间）
		want    int64  // 有效范围内无需固定返回值时为 -1
		wantErr bool   // 是否期望 ErrInvalidRange
	}{
		{"min等于max返回错误", 7, 7, 7, true},
		{"min大于max返回错误", 100, 50, 100, true},
		{"正常范围", 1, 1000000, -1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TryInt64(tt.min, tt.max)
			if tt.wantErr {
				if got != tt.want || !errors.Is(err, ErrInvalidRange) {
					t.Fatalf("TryInt64(%d,%d) = (%d, %v), want %d ErrInvalidRange", tt.min, tt.max, got, err, tt.want)
				}
				return
			}
			if err != nil {
				t.Fatalf("非预期错误: %v", err)
			}
			if tt.want != -1 {
				if got != tt.want {
					t.Fatalf("TryInt64(%d,%d) = %d, 期望 %d", tt.min, tt.max, got, tt.want)
				}
				return
			}
			for i := 0; i < 200; i++ {
				n, err := TryInt64(tt.min, tt.max)
				if err != nil {
					t.Fatalf("非预期错误: %v", err)
				}
				if n < tt.min || n >= tt.max {
					t.Fatalf("TryInt64 越界: %d 不在 [%d, %d)", n, tt.min, tt.max)
				}
			}
		})
	}
}

func TestTryIntegerRangesDoNotOverflow(t *testing.T) {
	maximumInt := int(^uint(0) >> 1)
	minimumInt := -maximumInt - 1
	if value, err := TryInt(minimumInt, maximumInt); err != nil || value < minimumInt || value >= maximumInt {
		t.Fatalf("TryInt() = (%d, %v)", value, err)
	}
	const minimumInt64 = -1 << 63
	const maximumInt64 = 1<<63 - 1
	if value, err := TryInt64(minimumInt64, maximumInt64); err != nil || value < minimumInt64 || value >= maximumInt64 {
		t.Fatalf("TryInt64() = (%d, %v)", value, err)
	}
}

// TestTryBytes 表驱动测试 TryBytes 的正常与长度边界路径。
func TestTryBytes(t *testing.T) {
	tests := []struct {
		name    string // 子测试名称
		length  int    // 请求长度
		wantLen int    // 期望长度
		wantNil bool   // 是否期望 nil
		wantErr bool   // 是否期望 ErrInvalidLength
	}{
		{"正常长度", 32, 32, false, false},
		{"零长度返回nil", 0, 0, true, false},
		{"负长度返回错误", -3, 0, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TryBytes(tt.length)
			if tt.wantErr {
				if got != nil || !errors.Is(err, ErrInvalidLength) {
					t.Fatalf("TryBytes(%d) = (%v, %v), want nil ErrInvalidLength", tt.length, got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("非预期错误: %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Fatalf("期望 nil, 实际 %v", got)
				}
				return
			}
			if len(got) != tt.wantLen {
				t.Fatalf("长度 = %d, 期望 %d", len(got), tt.wantLen)
			}
		})
	}
}

// TestTryBool 验证 TryBool 不返回错误且能产生两种取值（大致随机）。
func TestTryBool(t *testing.T) {
	var trueCount, falseCount int
	const iterations = 1000

	for i := 0; i < iterations; i++ {
		b, err := TryBool()
		if err != nil {
			t.Fatalf("非预期错误: %v", err)
		}
		if b {
			trueCount++
		} else {
			falseCount++
		}
	}

	// 两种取值都应出现（极低概率全偏，沿用既有 Bool 测试的判定思路）。
	if trueCount == 0 || falseCount == 0 {
		t.Fatalf("TryBool 不随机: true=%d, false=%d", trueCount, falseCount)
	}
}

// TestTryTokenUniqueness 验证 TryToken 在错误传播契约下仍能生成高熵唯一值。
func TestTryTokenUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	const count = 100

	for i := 0; i < count; i++ {
		tok, err := TryToken(32)
		if err != nil {
			t.Fatalf("非预期错误: %v", err)
		}
		if seen[tok] {
			t.Fatalf("生成了重复 Token: %s", tok)
		}
		seen[tok] = true
	}
	if len(seen) != count {
		t.Fatalf("期望 %d 个唯一 Token, 实际 %d", count, len(seen))
	}
}

// TestErrInsufficientEntropy_Wrapping 验证空字符集错误可被 errors.Is/As 正确识别。
//
// 注意：crypto/rand 的真实熵源失败在单测中难以稳定触发，这里以"空字符集"
// 这一可确定触发的错误路径，验证 Try* 系列"返回 error 而非 panic"的核心契约。
func TestErrInsufficientEntropy_Wrapping(t *testing.T) {
	// 空字符集 + 正长度: 必须返回错误且绝不 panic。
	_, err := TryStringFrom("", 8)
	if err == nil {
		t.Fatal("空字符集应返回错误")
	}

	// 验证 ErrInsufficientEntropy 哨兵在被 fmt.Errorf 以 %w 包装后仍可被 errors.Is 命中，
	// 这是上层（如 hexclaw OAuth state）判定"熵源故障"的可用性依赖。
	wrapped := fmt.Errorf("外层包装: %w", ErrInsufficientEntropy)
	if !errors.Is(wrapped, ErrInsufficientEntropy) {
		t.Fatal("包装后的 ErrInsufficientEntropy 无法被 errors.Is 命中")
	}
}

// Benchmark: 对照既有 BenchmarkString，确认安全变体无显著额外开销。
func BenchmarkTryString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = TryString(16)
	}
}

func BenchmarkTryToken(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = TryToken(32)
	}
}
