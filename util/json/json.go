package json

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Marshal JSON序列化（忽略错误）
//
// 注意：错误会被忽略并返回空字符串。
// 无法区分「序列化失败」和「成功序列化为空字符串」。
// 如需错误处理请使用 MarshalE。
//
// 建议：在生产代码中优先使用 MarshalE 以正确处理错误。
func Marshal(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}

// MarshalE JSON序列化（返回错误）
func MarshalE(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// MustMarshal JSON序列化，失败时panic
//
// 警告：仅用于已验证的内部数据，不要用于处理不可信的外部输入
func MustMarshal(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("json marshal failed: %v", err))
	}
	return string(data)
}

// MarshalIndent JSON序列化（美化输出）
func MarshalIndent(v any) string {
	data, _ := json.MarshalIndent(v, "", "  ")
	return string(data)
}

// MustMarshalIndent JSON序列化（美化输出），失败时panic
//
// 警告：仅用于已验证的内部数据，不要用于处理不可信的外部输入
func MustMarshalIndent(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(fmt.Sprintf("json marshal indent failed: %v", err))
	}
	return string(data)
}

// Unmarshal JSON反序列化
func Unmarshal(data string, v any) error {
	return json.Unmarshal([]byte(data), v)
}

// MustUnmarshal JSON反序列化，失败时panic
//
// 警告：仅用于已验证的内部数据，不要用于处理不可信的外部输入
// 处理外部 JSON 请使用 Unmarshal 并检查返回的 error
func MustUnmarshal(data string, v any) {
	if err := json.Unmarshal([]byte(data), v); err != nil {
		panic(fmt.Sprintf("json unmarshal failed: %v", err))
	}
}

// UnmarshalBytes JSON反序列化（字节数组）
func UnmarshalBytes(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// Valid 验证JSON是否合法
func Valid(data string) bool {
	return json.Valid([]byte(data))
}

// ValidBytes 验证JSON是否合法（字节数组）
func ValidBytes(data []byte) bool {
	return json.Valid(data)
}

// Pretty 美化JSON字符串
func Pretty(data string) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(data), "", "  "); err != nil {
		return data
	}
	return buf.String()
}

// PrettyBytes 美化JSON字节数组
func PrettyBytes(data []byte) []byte {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return data
	}
	return buf.Bytes()
}

// Compact 压缩JSON字符串
func Compact(data string) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(data)); err != nil {
		return data
	}
	return buf.String()
}

// CompactBytes 压缩JSON字节数组
func CompactBytes(data []byte) []byte {
	var buf bytes.Buffer
	if err := json.Compact(&buf, data); err != nil {
		return data
	}
	return buf.Bytes()
}

// ToMap JSON字符串转Map
func ToMap(data string) (map[string]any, error) {
	var m map[string]any
	err := json.Unmarshal([]byte(data), &m)
	return m, err
}

// MustToMap JSON字符串转Map，失败时panic
//
// 警告：仅用于已验证的内部数据，不要用于处理不可信的外部输入
func MustToMap(data string) map[string]any {
	m, err := ToMap(data)
	if err != nil {
		panic(fmt.Sprintf("json to map failed: %v", err))
	}
	return m
}

// ToSlice JSON字符串转Slice
func ToSlice(data string) ([]any, error) {
	var s []any
	err := json.Unmarshal([]byte(data), &s)
	return s, err
}

// MustToSlice JSON字符串转Slice，失败时panic
//
// 警告：仅用于已验证的内部数据，不要用于处理不可信的外部输入
func MustToSlice(data string) []any {
	s, err := ToSlice(data)
	if err != nil {
		panic(fmt.Sprintf("json to slice failed: %v", err))
	}
	return s
}

// Print 打印JSON（美化输出）
func Print(v any) {
	fmt.Println(MarshalIndent(v))
}
