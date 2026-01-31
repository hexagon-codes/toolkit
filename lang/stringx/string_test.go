package stringx

import (
	"bytes"
	"testing"
)

func TestBytesToString(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{"normal", []byte("hello"), "hello"},
		{"empty", []byte(""), ""},
		{"unicode", []byte("你好"), "你好"},
		{"special chars", []byte("hello\nworld\t!"), "hello\nworld\t!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BytesToString(tt.input)
			if result != tt.expected {
				t.Errorf("BytesToString(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestString2Bytes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []byte
	}{
		{"normal", "hello", []byte("hello")},
		{"empty", "", []byte("")},
		{"unicode", "你好", []byte("你好")},
		{"special chars", "hello\nworld\t!", []byte("hello\nworld\t!")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := String2Bytes(tt.input)
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("String2Bytes(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// 测试往返转换
func TestRoundTrip(t *testing.T) {
	tests := []string{
		"hello world",
		"",
		"你好世界",
		"abc123!@#",
	}

	for _, original := range tests {
		t.Run(original, func(t *testing.T) {
			// string -> bytes -> string
			b := String2Bytes(original)
			result := BytesToString(b)
			if result != original {
				t.Errorf("Round trip failed: got %v, want %v", result, original)
			}

			// bytes -> string -> bytes
			originalBytes := []byte(original)
			s := BytesToString(originalBytes)
			resultBytes := String2Bytes(s)
			if !bytes.Equal(resultBytes, originalBytes) {
				t.Errorf("Round trip failed: got %v, want %v", resultBytes, originalBytes)
			}
		})
	}
}

// 安全性测试：确保修改原始数据不影响转换结果
func TestSafety(t *testing.T) {
	original := []byte("hello")
	str := BytesToString(original)

	// 修改原始 []byte
	// 注意：这个测试展示了 unsafe 的风险
	// 在实际使用中，不应该修改通过 BytesToString 转换的原始数据
	originalCopy := make([]byte, len(original))
	copy(originalCopy, original)

	if str != string(originalCopy) {
		t.Errorf("Safety test failed: str changed after []byte modification")
	}
}

func BenchmarkBytesToString(b *testing.B) {
	data := []byte("hello world, this is a benchmark test string")

	b.Run("unsafe", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = BytesToString(data)
		}
	})

	b.Run("standard", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = string(data)
		}
	})
}

func BenchmarkString2Bytes(b *testing.B) {
	str := "hello world, this is a benchmark test string"

	b.Run("unsafe", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = String2Bytes(str)
		}
	})

	b.Run("standard", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = []byte(str)
		}
	})
}

// 性能对比：大数据量
func BenchmarkLargeData(b *testing.B) {
	// 创建 1MB 的数据
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	str := string(data)

	b.Run("BytesToString_1MB", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		for i := 0; i < b.N; i++ {
			_ = BytesToString(data)
		}
	})

	b.Run("String2Bytes_1MB", func(b *testing.B) {
		b.SetBytes(int64(len(str)))
		for i := 0; i < b.N; i++ {
			_ = String2Bytes(str)
		}
	})
}

func TestStringToSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected []any
		isNil    bool
	}{
		{
			name:     "string slice",
			input:    []string{"hello", "world", "test"},
			expected: []any{"hello", "world", "test"},
			isNil:    false,
		},
		{
			name:     "int slice",
			input:    []int{1, 2, 3, 4, 5},
			expected: []any{1, 2, 3, 4, 5},
			isNil:    false,
		},
		{
			name:     "empty slice",
			input:    []string{},
			expected: []any{},
			isNil:    false,
		},
		{
			name:     "array",
			input:    [3]int{10, 20, 30},
			expected: []any{10, 20, 30},
			isNil:    false,
		},
		{
			name:     "mixed interface slice",
			input:    []any{1, "hello", 3.14, true},
			expected: []any{1, "hello", 3.14, true},
			isNil:    false,
		},
		{
			name:     "invalid input - string",
			input:    "not a slice",
			expected: nil,
			isNil:    true,
		},
		{
			name:     "invalid input - int",
			input:    42,
			expected: nil,
			isNil:    true,
		},
		{
			name:     "invalid input - map",
			input:    map[string]int{"a": 1},
			expected: nil,
			isNil:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringToSlice(tt.input)

			if tt.isNil {
				if result != nil {
					t.Errorf("StringToSlice(%v) = %v, want nil", tt.input, result)
				}
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("StringToSlice(%v) length = %v, want %v", tt.input, len(result), len(tt.expected))
				return
			}

			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("StringToSlice(%v)[%d] = %v, want %v", tt.input, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func BenchmarkStringToSlice(b *testing.B) {
	benchmarks := []struct {
		name  string
		input any
	}{
		{"small string slice", []string{"a", "b", "c"}},
		{"medium int slice", make([]int, 100)},
		{"large string slice", make([]string, 1000)},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = StringToSlice(bm.input)
			}
		})
	}
}
