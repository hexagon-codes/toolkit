package timex

import (
	"strings"
	"testing"
	"time"
)

func TestMsecFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		contains []string // 检查结果是否包含这些子串
	}{
		{
			name:     "normal timestamp",
			input:    1706423456789, // 2024-01-28 15:04:16
			contains: []string{"2024", "01", "28"},
		},
		{
			name:     "zero timestamp",
			input:    0,
			contains: []string{"1970", "01", "01"},
		},
		{
			name:     "current time",
			input:    time.Now().UnixMilli(),
			contains: []string{"-", ":"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MsecFormat(tt.input)
			for _, substr := range tt.contains {
				if !strings.Contains(result, substr) {
					t.Errorf("MsecFormat(%v) = %v, want to contain %v", tt.input, result, substr)
				}
			}
			// 检查格式是否正确 (应该是 YYYY-MM-DD HH:MM:SS 格式)
			if len(result) != 19 {
				t.Errorf("MsecFormat(%v) length = %v, want 19", tt.input, len(result))
			}
		})
	}
}

func TestMsecFormatWithLayout(t *testing.T) {
	ms := int64(1706423456789) // 2024-01-28 15:04:16

	tests := []struct {
		name     string
		layout   string
		contains string
	}{
		{
			name:     "date only",
			layout:   "2006-01-02",
			contains: "2024-01-28",
		},
		{
			name:     "time only",
			layout:   "15:04:05",
			contains: ":",
		},
		{
			name:     "custom format",
			layout:   "2006/01/02",
			contains: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MsecFormatWithLayout(ms, tt.layout)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("MsecFormatWithLayout() = %v, want to contain %v", result, tt.contains)
			}
		})
	}
}

func TestSecFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		contains []string
	}{
		{
			name:     "normal timestamp",
			input:    1706423456, // 2024-01-28 15:04:16
			contains: []string{"2024", "01", "28"},
		},
		{
			name:     "zero timestamp",
			input:    0,
			contains: []string{"1970", "01", "01"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SecFormat(tt.input)
			for _, substr := range tt.contains {
				if !strings.Contains(result, substr) {
					t.Errorf("SecFormat(%v) = %v, want to contain %v", tt.input, result, substr)
				}
			}
		})
	}
}

func TestSecFormatWithLayout(t *testing.T) {
	sec := int64(1706423456) // 2024-01-28 15:04:16

	tests := []struct {
		name   string
		layout string
		want   string
	}{
		{
			name:   "date only",
			layout: "2006-01-02",
			want:   "2024-01-28",
		},
		{
			name:   "custom format",
			layout: "2006/01/02",
			want:   "2024/01/28",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SecFormatWithLayout(sec, tt.layout)
			if result != tt.want {
				t.Errorf("SecFormatWithLayout() = %v, want %v", result, tt.want)
			}
		})
	}
}

// 测试毫秒和秒的精度
func TestPrecision(t *testing.T) {
	now := time.Now()
	ms := now.UnixMilli()
	sec := now.Unix()

	msResult := MsecFormat(ms)
	secResult := SecFormat(sec)

	// 两者应该在同一秒内（忽略毫秒差异）
	if msResult[:17] != secResult[:17] { // 比较到分钟
		t.Errorf("Precision mismatch: MsecFormat=%v, SecFormat=%v", msResult, secResult)
	}
}

func BenchmarkMsecFormat(b *testing.B) {
	ms := time.Now().UnixMilli()
	for i := 0; i < b.N; i++ {
		_ = MsecFormat(ms)
	}
}

func BenchmarkMsecFormatWithLayout(b *testing.B) {
	ms := time.Now().UnixMilli()
	layout := "2006-01-02"
	for i := 0; i < b.N; i++ {
		_ = MsecFormatWithLayout(ms, layout)
	}
}

func BenchmarkSecFormat(b *testing.B) {
	sec := time.Now().Unix()
	for i := 0; i < b.N; i++ {
		_ = SecFormat(sec)
	}
}
