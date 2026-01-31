package conv

import (
	"fmt"
	"strconv"
)

// String 将任意类型转换为字符串
//
// 支持的类型:
//   - string: 直接返回
//   - []byte: 转换为字符串
//   - int/int8/int16/int32/int64: 格式化为十进制
//   - uint/uint8/uint16/uint32/uint64: 格式化为十进制
//   - float32/float64: 格式化为十进制（自动精度）
//   - bool: "true" 或 "false"
//   - iString 接口: 调用 String() 方法
//   - 其他: 使用 fmt.Sprintf("%v", value)
//
// 输入为 nil 时返回空字符串
//
// 示例:
//
//	conv.String(123)          // "123"
//	conv.String(45.67)        // "45.67"
//	conv.String(true)         // "true"
//	conv.String([]byte("a"))  // "a"
func String(any any) string {
	if any == nil {
		return ""
	}
	switch value := any.(type) {
	case string:
		return value
	case []byte:
		return string(value)
	case int:
		return strconv.Itoa(value)
	case int8:
		return strconv.FormatInt(int64(value), 10)
	case int16:
		return strconv.FormatInt(int64(value), 10)
	case int32:
		return strconv.FormatInt(int64(value), 10)
	case int64:
		return strconv.FormatInt(value, 10)
	case uint:
		return strconv.FormatUint(uint64(value), 10)
	case uint8:
		return strconv.FormatUint(uint64(value), 10)
	case uint16:
		return strconv.FormatUint(uint64(value), 10)
	case uint32:
		return strconv.FormatUint(uint64(value), 10)
	case uint64:
		return strconv.FormatUint(value, 10)
	case float32:
		return strconv.FormatFloat(float64(value), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(value)
	default:
		// 尝试 iString 接口
		if s, ok := value.(iString); ok {
			return s.String()
		}
		// 降级到 fmt.Sprintf
		return fmt.Sprintf("%v", value)
	}
}
