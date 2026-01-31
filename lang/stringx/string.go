package stringx

import (
	"reflect"
	"unsafe"
)

// BytesToString 将 []byte 零拷贝转换为 string
//
// ⚠️ 重要警告：
//   - 不要修改原始 []byte，否则会导致返回的 string 内容变化
//   - 此函数使用 unsafe 操作，仅用于性能关键路径
//   - 如果不确定是否安全，请使用标准的 string(b) 转换
func BytesToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}

// StringToBytes 将 string 零拷贝转换为 []byte
//
// ⚠️ 重要警告：
//   - 绝对不要修改返回的 []byte，否则会导致 panic 或未定义行为
//   - Go 中的 string 是不可变的，修改其底层数据违反语言规范
//   - 此函数使用 unsafe 操作，仅用于只读场景（如传递给只读 API）
//   - 如果需要可修改的 []byte，请使用标准的 []byte(s) 转换
func StringToBytes(s string) []byte {
	if s == "" {
		return []byte{}
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// String2Bytes 是 StringToBytes 的别名，为向后兼容保留
// Deprecated: 请使用 StringToBytes 替代
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
