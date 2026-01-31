package conv

import (
	"reflect"
	"testing"
)

func TestJSONToMap(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]any
		wantErr  bool
	}{
		{
			name:     "simple object",
			input:    `{"name":"Alice","age":30}`,
			expected: map[string]any{"name": "Alice", "age": float64(30)},
			wantErr:  false,
		},
		{
			name:     "nested object",
			input:    `{"user":{"name":"Bob"}}`,
			expected: map[string]any{"user": map[string]any{"name": "Bob"}},
			wantErr:  false,
		},
		{
			name:     "empty object",
			input:    `{}`,
			expected: map[string]any{},
			wantErr:  false,
		},
		{
			name:     "invalid json",
			input:    `{invalid}`,
			expected: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := JSONToMap(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("JSONToMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("JSONToMap() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestStringToMap(t *testing.T) {
	// StringToMap 是 JSONToMap 的别名，测试它是否正常工作
	tests := []struct {
		name     string
		input    string
		expected map[string]any
		wantErr  bool
	}{
		{
			name:     "valid json",
			input:    `{"key":"value"}`,
			expected: map[string]any{"key": "value"},
			wantErr:  false,
		},
		{
			name:     "invalid json",
			input:    `invalid`,
			expected: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := StringToMap(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("StringToMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("StringToMap() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMapToJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		contains string
		wantErr  bool
	}{
		{
			name:     "simple map",
			input:    map[string]any{"name": "Alice", "age": 30},
			contains: "Alice",
			wantErr:  false,
		},
		{
			name:     "empty map",
			input:    map[string]any{},
			contains: "{}",
			wantErr:  false,
		},
		{
			name:     "nested map",
			input:    map[string]any{"user": map[string]any{"name": "Bob"}},
			contains: "Bob",
			wantErr:  false,
		},
		{
			name:     "invalid value - channel",
			input:    map[string]any{"ch": make(chan int)},
			contains: "",
			wantErr:  true, // channel 无法序列化为 JSON
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := MapToJSON(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("MapToJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != "" {
				// 检查结果包含预期内容
				m, err := JSONToMap(result)
				if err != nil {
					t.Errorf("MapToJSON() result is not valid JSON: %v", err)
				}
				if len(m) != len(tt.input) {
					t.Errorf("MapToJSON() result length = %v, want %v", len(m), len(tt.input))
				}
			}
		})
	}
}

func TestMergeMaps(t *testing.T) {
	tests := []struct {
		name     string
		inputs   []map[string]any
		expected map[string]any
	}{
		{
			name: "merge two maps",
			inputs: []map[string]any{
				{"a": 1, "b": 2},
				{"c": 3, "d": 4},
			},
			expected: map[string]any{"a": 1, "b": 2, "c": 3, "d": 4},
		},
		{
			name: "merge with override",
			inputs: []map[string]any{
				{"a": 1, "b": 2},
				{"b": 3, "c": 4},
			},
			expected: map[string]any{"a": 1, "b": 3, "c": 4},
		},
		{
			name:     "merge empty maps",
			inputs:   []map[string]any{{}, {}},
			expected: map[string]any{},
		},
		{
			name:     "merge single map",
			inputs:   []map[string]any{{"a": 1}},
			expected: map[string]any{"a": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeMaps(tt.inputs...)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("MergeMaps() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMapKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected int // 只检查长度，因为顺序不保证
	}{
		{
			name:     "normal map",
			input:    map[string]any{"a": 1, "b": 2, "c": 3},
			expected: 3,
		},
		{
			name:     "empty map",
			input:    map[string]any{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapKeys(tt.input)
			if len(result) != tt.expected {
				t.Errorf("MapKeys() length = %v, want %v", len(result), tt.expected)
			}
			// 验证所有键都存在
			for _, key := range result {
				if _, ok := tt.input[key]; !ok {
					t.Errorf("MapKeys() returned invalid key: %v", key)
				}
			}
		})
	}
}

func TestMapValues(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected int // 只检查长度
	}{
		{
			name:     "normal map",
			input:    map[string]any{"a": 1, "b": 2, "c": 3},
			expected: 3,
		},
		{
			name:     "empty map",
			input:    map[string]any{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapValues(tt.input)
			if len(result) != tt.expected {
				t.Errorf("MapValues() length = %v, want %v", len(result), tt.expected)
			}
		})
	}
}

func BenchmarkJSONToMap(b *testing.B) {
	jsonStr := `{"name":"Alice","age":30,"active":true}`
	for i := 0; i < b.N; i++ {
		_, _ = JSONToMap(jsonStr)
	}
}

func BenchmarkMapToJSON(b *testing.B) {
	m := map[string]any{"name": "Alice", "age": 30, "active": true}
	for i := 0; i < b.N; i++ {
		_, _ = MapToJSON(m)
	}
}

func BenchmarkMergeMaps(b *testing.B) {
	m1 := map[string]any{"a": 1, "b": 2}
	m2 := map[string]any{"c": 3, "d": 4}
	for i := 0; i < b.N; i++ {
		_ = MergeMaps(m1, m2)
	}
}
