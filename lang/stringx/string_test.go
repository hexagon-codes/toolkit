package stringx

import (
	"bytes"
	"strings"
	"testing"
)

var (
	benchmarkStringSink string
	benchmarkBytesSink  []byte
	benchmarkSliceSink  []any
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

func TestStringToBytes(t *testing.T) {
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
			result := StringToBytes(tt.input)
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("StringToBytes(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestStringToBytesEmptyPreservesNil(t *testing.T) {
	if converted := StringToBytes(""); converted != nil {
		t.Fatalf("StringToBytes(empty) = %#v, want nil", converted)
	}
}

func TestString2BytesCompatibility(t *testing.T) {
	source := strings.Repeat("legacy", 2)
	converted := String2Bytes(source)
	if !bytes.Equal(converted, []byte(source)) {
		t.Fatalf("String2Bytes() = %q, want %q", converted, source)
	}

	converted[0] = 'L'
	if source != "legacylegacy" {
		t.Fatalf("String2Bytes must not expose string storage, got %q", source)
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
			b := StringToBytes(original)
			result := BytesToString(b)
			if result != original {
				t.Errorf("Round trip failed: got %v, want %v", result, original)
			}

			// bytes -> string -> bytes
			originalBytes := []byte(original)
			s := BytesToString(originalBytes)
			resultBytes := StringToBytes(s)
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

	original[0] = 'H'
	if str != "hello" {
		t.Fatalf("BytesToString must not alias mutable input, got %q", str)
	}

	source := strings.Repeat("world", 2)
	converted := StringToBytes(source)
	converted[0] = 'W'
	if source != "worldworld" {
		t.Fatalf("StringToBytes must not expose string storage, got %q", source)
	}
}

func BenchmarkBytesToString(b *testing.B) {
	data := []byte("hello world, this is a benchmark test string")

	b.Run("toolkit_safe_copy", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			benchmarkStringSink = BytesToString(data)
		}
	})

	b.Run("standard", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			benchmarkStringSink = string(data)
		}
	})
}

func BenchmarkStringToBytes(b *testing.B) {
	str := "hello world, this is a benchmark test string"

	b.Run("toolkit_safe_copy", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			benchmarkBytesSink = StringToBytes(str)
		}
	})

	b.Run("standard", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			benchmarkBytesSink = []byte(str)
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
			benchmarkStringSink = BytesToString(data)
		}
	})

	b.Run("StringToBytes_1MB", func(b *testing.B) {
		b.SetBytes(int64(len(str)))
		for i := 0; i < b.N; i++ {
			benchmarkBytesSink = StringToBytes(str)
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
				benchmarkSliceSink = StringToSlice(bm.input)
			}
		})
	}
}
