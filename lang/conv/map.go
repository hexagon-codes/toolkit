package conv

import (
	"encoding/json"
)

// JSONToMap 将 JSON 字符串转换为 map[string]any
//
// 参数:
//   - jsonStr: JSON 字符串
//
// 返回:
//   - map 和 error
//
// 示例:
//
//	m, err := conv.JSONToMap(`{"name":"Alice","age":30}`)
//	// m = map[string]any{"name": "Alice", "age": 30}
func JSONToMap(jsonStr string) (map[string]any, error) {
	m := make(map[string]any)
	err := json.Unmarshal([]byte(jsonStr), &m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// MapToJSON 将 map[string]any 转换为 JSON 字符串
//
// 参数:
//   - m: 要转换的 map
//
// 返回:
//   - JSON 字符串和 error
//
// 示例:
//
//	json, err := conv.MapToJSON(map[string]any{"name": "Alice"})
//	// json = `{"name":"Alice"}`
func MapToJSON(m map[string]any) (string, error) {
	jsonByte, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(jsonByte), nil
}

// StringToMap 将字符串转换为 map[string]any (JSONToMap 的别名)
//
// 已废弃: 请使用 JSONToMap 以获得更清晰的语义
func StringToMap(content string) (map[string]any, error) {
	return JSONToMap(content)
}

// MergeMaps 合并多个 map 为一个
// 后面的 map 会覆盖前面的重复键
//
// 参数:
//   - maps: 可变数量的 map
//
// 返回:
//   - 合并后的 map
//
// 示例:
//
//	m1 := map[string]any{"a": 1, "b": 2}
//	m2 := map[string]any{"b": 3, "c": 4}
//	result := conv.MergeMaps(m1, m2)
//	// result = map[string]any{"a": 1, "b": 3, "c": 4}
func MergeMaps(maps ...map[string]any) map[string]any {
	result := make(map[string]any)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

// MapKeys 返回 map 的所有键
//
// 示例:
//
//	m := map[string]any{"name": "Alice", "age": 30}
//	keys := conv.MapKeys(m)
//	// keys = []string{"name", "age"} (顺序不保证)
func MapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// MapValues 返回 map 的所有值
//
// 示例:
//
//	m := map[string]any{"name": "Alice", "age": 30}
//	values := conv.MapValues(m)
//	// values = []any{"Alice", 30} (顺序不保证)
func MapValues(m map[string]any) []any {
	values := make([]any, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}
