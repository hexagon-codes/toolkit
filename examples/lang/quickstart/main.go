package main

import (
	"fmt"

	"github.com/everyday-items/toolkit/lang/conv"
	"github.com/everyday-items/toolkit/lang/stringx"
	"github.com/everyday-items/toolkit/lang/timex"
)

func main() {
	fmt.Println("=== gopkg 快速示例 ===")

	// 1. 类型转换示例
	fmt.Println("📦 类型转换 (lang/conv):")

	// 各种类型转 string
	fmt.Printf("  Int to String: %s\n", conv.String(42))
	fmt.Printf("  Float to String: %s\n", conv.String(3.14))
	fmt.Printf("  Bool to String: %s\n", conv.String(true))

	// String 转 float
	fmt.Printf("  String to Float32: %.2f\n", conv.Float32("3.14159"))
	fmt.Printf("  String to Float64: %.6f\n", conv.Float64("2.718281828"))

	// JSON-Map 互转
	jsonStr := `{"name":"张三","age":30,"active":true}`
	m, _ := conv.JSONToMap(jsonStr)
	fmt.Printf("  JSON to Map: %+v\n", m)

	// Map 合并
	map1 := map[string]any{"a": 1, "b": 2}
	map2 := map[string]any{"c": 3, "b": 20}
	merged := conv.MergeMaps(map1, map2)
	fmt.Printf("  Merged Map: %+v\n", merged)

	fmt.Println("\n📝 字符串工具 (lang/stringx):")

	// 零拷贝转换（高性能）
	original := "Hello, 世界!"
	bytes := stringx.String2Bytes(original)
	backToString := stringx.BytesToString(bytes)
	fmt.Printf("  Original: %s\n", original)
	fmt.Printf("  To Bytes (zero-copy): %v\n", bytes[:10])
	fmt.Printf("  Back to String: %s\n", backToString)

	// 数组转切片
	intArray := []int{1, 2, 3, 4, 5}
	slice := stringx.StringToSlice(intArray)
	fmt.Printf("  Array to Slice: %v (type: %T)\n", slice, slice)

	fmt.Println("\n⏰ 时间工具 (lang/timex):")

	// 毫秒时间戳格式化
	msTimestamp := int64(1706423456789)
	formatted := timex.MsecFormat(msTimestamp)
	fmt.Printf("  Timestamp: %d\n", msTimestamp)
	fmt.Printf("  Formatted: %s\n", formatted)

	fmt.Println("\n✅ 示例完成!")
}
