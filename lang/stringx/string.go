package stringx

import (
	"reflect"
)

// BytesToString 将 []byte 安全转换为独立的 string。
func BytesToString(b []byte) string {
	return string(b)
}

// StringToBytes 将 string 安全转换为可独立修改的 []byte。
func StringToBytes(s string) []byte {
	if s == "" {
		return nil
	}
	return []byte(s)
}

// String2Bytes 是 StringToBytes 的兼容别名。
//
// Deprecated: 请使用 StringToBytes。
func String2Bytes(s string) []byte {
	return StringToBytes(s)
}

// StringToSlice 将任意切片或数组转换为 []any
// 如果传入的不是切片或数组类型，返回 nil
func StringToSlice(arr any) []any {
	v := reflect.ValueOf(arr)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return nil
	}
	l := v.Len()
	ret := make([]any, l)
	for i := range l {
		ret[i] = v.Index(i).Interface()
	}
	return ret
}
