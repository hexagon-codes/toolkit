package conv

import (
	"encoding/binary"
	"math"
	"strconv"
)

// Float32 将任意类型转换为 float32
//
// 支持的类型:
//   - float32, float64: 直接转换
//   - []byte: 二进制解码 (小端序，4字节)
//   - iFloat32 接口: 调用 Float32() 方法
//   - 其他: 转为字符串后解析
//
// 转换失败时返回 0
//
// 示例:
//
//	conv.Float32("3.14")    // 3.14
//	conv.Float32([]byte{...}) // 从二进制解码
func Float32(any any) float32 {
	if any == nil {
		return 0
	}
	switch value := any.(type) {
	case float32:
		return value
	case float64:
		return float32(value)
	case []byte:
		return bytesToFloat32(value)
	default:
		if f, ok := value.(iFloat32); ok {
			return f.Float32()
		}
		v, _ := strconv.ParseFloat(String(any), 64)
		return float32(v)
	}
}

// Float64 将任意类型转换为 float64
//
// 支持的类型:
//   - float32, float64: 直接转换
//   - []byte: 二进制解码 (小端序，8字节)
//   - iFloat64 接口: 调用 Float64() 方法
//   - 其他: 转为字符串后解析
//
// 转换失败时返回 0
//
// 示例:
//
//	conv.Float64("3.14159")  // 3.14159
//	conv.Float64(3.14)       // 3.14
func Float64(any any) float64 {
	if any == nil {
		return 0
	}
	switch value := any.(type) {
	case float32:
		return float64(value)
	case float64:
		return value
	case []byte:
		return bytesToFloat64(value)
	default:
		if f, ok := value.(iFloat64); ok {
			return f.Float64()
		}
		v, _ := strconv.ParseFloat(String(any), 64)
		return v
	}
}

// bytesToFloat32 将 []byte 解码为 float32 (小端序)
// 注意：如果字节数不足 4 个，返回 0（符合包的设计原则：转换失败返回零值）
// 调用方如需区分"真正的 0"和"转换失败"，应先检查 len(b) >= 4
func bytesToFloat32(b []byte) float32 {
	if len(b) < 4 {
		return 0
	}
	bits := binary.LittleEndian.Uint32(b[:4])
	return math.Float32frombits(bits)
}

// bytesToFloat64 将 []byte 解码为 float64 (小端序)
// 注意：如果字节数不足 8 个，返回 0（符合包的设计原则：转换失败返回零值）
// 调用方如需区分"真正的 0"和"转换失败"，应先检查 len(b) >= 8
func bytesToFloat64(b []byte) float64 {
	if len(b) < 8 {
		return 0
	}
	bits := binary.LittleEndian.Uint64(b[:8])
	return math.Float64frombits(bits)
}
