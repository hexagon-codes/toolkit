package main

import (
	"fmt"

	"github.com/everyday-items/toolkit/lang/conv"
)

func main() {
	fmt.Println("=== lang/conv 类型转换示例 ===")

	// 1. 基本类型转换
	fmt.Println("1. 基本类型转换")
	demonstrateBasicConversion()

	// 2. JSON-Map 互转
	fmt.Println("\n2. JSON-Map 互转")
	demonstrateJSONMapConversion()

	// 3. Map 操作
	fmt.Println("\n3. Map 操作")
	demonstrateMapOperations()

	// 4. 边界情况处理
	fmt.Println("\n4. 边界情况处理")
	demonstrateEdgeCases()

	// 5. 自定义类型转换
	fmt.Println("\n5. 自定义类型转换")
	demonstrateCustomConversion()
}

// demonstrateBasicConversion 演示基本类型转换
func demonstrateBasicConversion() {
	// String 转换
	fmt.Println("\n  [String] 转换为字符串")
	fmt.Printf("  conv.String(123)        = %s\n", conv.String(123))
	fmt.Printf("  conv.String(45.67)      = %s\n", conv.String(45.67))
	fmt.Printf("  conv.String(true)       = %s\n", conv.String(true))
	fmt.Printf("  conv.String([]byte)     = %s\n", conv.String([]byte("hello")))
	fmt.Printf("  conv.String(nil)        = '%s'\n", conv.String(nil))

	// Int 转换
	fmt.Println("\n  [Int] 转换为整数")
	fmt.Printf("  conv.Int(\"123\")        = %d\n", conv.Int("123"))
	fmt.Printf("  conv.Int(45.67)         = %d\n", conv.Int(45.67))
	fmt.Printf("  conv.Int(true)          = %d\n", conv.Int(true))
	fmt.Printf("  conv.Int(\"invalid\")    = %d (默认0)\n", conv.Int("invalid"))
	fmt.Printf("  conv.Int(nil)           = %d\n", conv.Int(nil))

	// Int64 转换
	fmt.Println("\n  [Int64] 转换为 int64")
	fmt.Printf("  conv.Int64(\"123\")      = %d\n", conv.Int64("123"))
	fmt.Printf("  conv.Int64(45.67)       = %d\n", conv.Int64(45.67))

	// Uint 转换
	fmt.Println("\n  [Uint] 转换为无符号整数")
	fmt.Printf("  conv.Uint(\"123\")       = %d\n", conv.Uint("123"))
	fmt.Printf("  conv.Uint(45.67)        = %d\n", conv.Uint(45.67))
	fmt.Printf("  conv.Uint(\"-123\")      = %d (负数转0)\n", conv.Uint("-123"))

	// Float32 转换
	fmt.Println("\n  [Float32] 转换为 float32")
	fmt.Printf("  conv.Float32(\"45.67\")  = %.2f\n", conv.Float32("45.67"))
	fmt.Printf("  conv.Float32(123)       = %.2f\n", conv.Float32(123))

	// Float64 转换
	fmt.Println("\n  [Float64] 转换为 float64")
	fmt.Printf("  conv.Float64(\"45.67\")  = %.2f\n", conv.Float64("45.67"))
	fmt.Printf("  conv.Float64(123)       = %.2f\n", conv.Float64(123))

	// Bool 转换
	fmt.Println("\n  [Bool] 转换为布尔值")
	fmt.Printf("  conv.Bool(\"true\")      = %v\n", conv.Bool("true"))
	fmt.Printf("  conv.Bool(1)            = %v\n", conv.Bool(1))
	fmt.Printf("  conv.Bool(\"yes\")       = %v\n", conv.Bool("yes"))
	fmt.Printf("  conv.Bool(\"on\")        = %v\n", conv.Bool("on"))
	fmt.Printf("  conv.Bool(0)            = %v\n", conv.Bool(0))
	fmt.Printf("  conv.Bool(\"false\")     = %v\n", conv.Bool("false"))
}

// demonstrateJSONMapConversion 演示 JSON-Map 互转
func demonstrateJSONMapConversion() {
	// JSON 转 Map
	fmt.Println("\n  [JSONToMap] JSON 字符串转 Map")
	jsonStr := `{"name":"Alice","age":30,"active":true}`
	dataMap, err := conv.JSONToMap(jsonStr)
	if err != nil {
		fmt.Printf("  ✗ 转换失败: %v\n", err)
	} else {
		fmt.Printf("  ✓ JSON 转 Map 成功:\n")
		for k, v := range dataMap {
			fmt.Printf("    %s: %v (%T)\n", k, v, v)
		}
	}

	// Map 转 JSON
	fmt.Println("\n  [MapToJSON] Map 转 JSON 字符串")
	userMap := map[string]any{
		"id":       1001,
		"username": "bob",
		"email":    "bob@example.com",
		"age":      25,
		"premium":  true,
	}
	jsonResult, err := conv.MapToJSON(userMap)
	if err != nil {
		fmt.Printf("  ✗ 转换失败: %v\n", err)
	} else {
		fmt.Printf("  ✓ Map 转 JSON 成功:\n")
		fmt.Printf("  %s\n", jsonResult)
	}

	// 嵌套 JSON 转换
	fmt.Println("\n  [嵌套 JSON] 复杂 JSON 转换")
	nestedJSON := `{
		"user": {
			"name": "Charlie",
			"contact": {
				"email": "charlie@example.com",
				"phone": "123-456-7890"
			}
		},
		"tags": ["golang", "redis", "mysql"]
	}`
	nestedMap, err := conv.JSONToMap(nestedJSON)
	if err != nil {
		fmt.Printf("  ✗ 转换失败: %v\n", err)
	} else {
		fmt.Printf("  ✓ 嵌套 JSON 转换成功:\n")
		fmt.Printf("  %+v\n", nestedMap)
	}
}

// demonstrateMapOperations 演示 Map 操作
func demonstrateMapOperations() {
	// MergeMaps - 合并多个 Map
	fmt.Println("\n  [MergeMaps] 合并多个 Map")
	map1 := map[string]any{"a": 1, "b": 2}
	map2 := map[string]any{"b": 3, "c": 4}
	map3 := map[string]any{"d": 5}

	merged := conv.MergeMaps(map1, map2, map3)
	fmt.Printf("  Map1: %v\n", map1)
	fmt.Printf("  Map2: %v\n", map2)
	fmt.Printf("  Map3: %v\n", map3)
	fmt.Printf("  ✓ 合并结果: %v\n", merged)

	// MapKeys - 提取所有键
	fmt.Println("\n  [MapKeys] 提取 Map 的所有键")
	dataMap := map[string]any{
		"name":   "David",
		"age":    35,
		"city":   "Beijing",
		"active": true,
	}
	keys := conv.MapKeys(dataMap)
	fmt.Printf("  Map: %v\n", dataMap)
	fmt.Printf("  ✓ 键列表: %v\n", keys)

	// MapValues - 提取所有值
	fmt.Println("\n  [MapValues] 提取 Map 的所有值")
	values := conv.MapValues(dataMap)
	fmt.Printf("  ✓ 值列表: %v\n", values)

	// 实用场景：构建查询条件
	fmt.Println("\n  [实用场景] 动态构建查询条件")
	conditions := map[string]any{
		"status":  "active",
		"age_min": 18,
		"age_max": 60,
		"city":    "Shanghai",
	}
	condKeys := conv.MapKeys(conditions)
	fmt.Printf("  查询条件: %v\n", conditions)
	fmt.Printf("  ✓ 需要过滤的字段: %v\n", condKeys)
}

// demonstrateEdgeCases 演示边界情况处理
func demonstrateEdgeCases() {
	fmt.Println("\n  [nil 处理] conv 函数的 nil 安全性")
	fmt.Printf("  conv.String(nil)  = '%s'\n", conv.String(nil))
	fmt.Printf("  conv.Int(nil)     = %d\n", conv.Int(nil))
	fmt.Printf("  conv.Float64(nil) = %.2f\n", conv.Float64(nil))
	fmt.Printf("  conv.Bool(nil)    = %v\n", conv.Bool(nil))

	fmt.Println("\n  [无效输入] 转换失败返回零值")
	fmt.Printf("  conv.Int(\"abc\")     = %d (默认0)\n", conv.Int("abc"))
	fmt.Printf("  conv.Float64(\"xyz\") = %.2f (默认0)\n", conv.Float64("xyz"))
	fmt.Printf("  conv.Bool(\"maybe\")  = %v (默认false)\n", conv.Bool("maybe"))

	fmt.Println("\n  [溢出处理] 大数值转换")
	fmt.Printf("  conv.Int(\"999999999999999\") = %d\n", conv.Int("999999999999999"))
	fmt.Printf("  conv.Uint(\"-123\")           = %d (负数转0)\n", conv.Uint("-123"))

	fmt.Println("\n  [空 Map] 空 Map 操作")
	emptyMap := map[string]any{}
	fmt.Printf("  MapKeys(empty)   = %v\n", conv.MapKeys(emptyMap))
	fmt.Printf("  MapValues(empty) = %v\n", conv.MapValues(emptyMap))

	merged := conv.MergeMaps(emptyMap, nil)
	fmt.Printf("  MergeMaps(empty, nil) = %v\n", merged)
}

// CustomStringer 自定义类型实现 String() 方法
type CustomStringer struct {
	Name  string
	Value int
}

func (c CustomStringer) String() string {
	return fmt.Sprintf("CustomStringer{Name=%s, Value=%d}", c.Name, c.Value)
}

// demonstrateCustomConversion 演示自定义类型转换
func demonstrateCustomConversion() {
	fmt.Println("\n  [自定义类型] 实现 String() 接口")
	custom := CustomStringer{Name: "MyObject", Value: 42}
	fmt.Printf("  原始类型: %+v\n", custom)
	fmt.Printf("  ✓ conv.String() = %s\n", conv.String(custom))

	// 指针类型
	fmt.Println("\n  [指针类型] 转换指针")
	ptr := &CustomStringer{Name: "PointerObject", Value: 100}
	fmt.Printf("  ✓ conv.String(ptr) = %s\n", conv.String(ptr))

	// 切片转换
	fmt.Println("\n  [切片类型] 转换切片")
	numbers := []int{1, 2, 3, 4, 5}
	fmt.Printf("  原始切片: %v\n", numbers)
	fmt.Printf("  ✓ conv.String(slice) = %s\n", conv.String(numbers))

	// 实际应用场景
	fmt.Println("\n  [实际场景] HTTP 查询参数构建")
	params := map[string]any{
		"page":     1,
		"pageSize": 20,
		"sort":     "created_at",
		"order":    "desc",
		"active":   true,
	}

	// 构建查询字符串
	queryString := ""
	for k, v := range params {
		if queryString != "" {
			queryString += "&"
		}
		queryString += fmt.Sprintf("%s=%s", k, conv.String(v))
	}
	fmt.Printf("  ✓ 查询字符串: %s\n", queryString)

	// 表单数据验证
	fmt.Println("\n  [实际场景] 表单数据验证")
	formData := map[string]any{
		"age":     "25",
		"price":   "99.99",
		"enabled": "true",
	}

	age := conv.Int(formData["age"])
	price := conv.Float64(formData["price"])
	enabled := conv.Bool(formData["enabled"])

	fmt.Printf("  表单数据: %v\n", formData)
	fmt.Printf("  ✓ 转换后:\n")
	fmt.Printf("    age (int): %d\n", age)
	fmt.Printf("    price (float64): %.2f\n", price)
	fmt.Printf("    enabled (bool): %v\n", enabled)
}
