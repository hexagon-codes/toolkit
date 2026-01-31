package conv_test

import (
	"fmt"

	"github.com/everyday-items/toolkit/lang/conv"
)

func ExampleString() {
	// 各种类型转字符串
	fmt.Println(conv.String(123))
	fmt.Println(conv.String(3.14))
	fmt.Println(conv.String(true))
	fmt.Println(conv.String([]byte("hello")))

	// Output:
	// 123
	// 3.14
	// true
	// hello
}

func ExampleInt() {
	// 字符串转整数
	fmt.Println(conv.Int("123"))
	fmt.Println(conv.Int("456"))

	// 浮点数转整数（截断）
	fmt.Println(conv.Int(3.99))

	// 布尔值转整数
	fmt.Println(conv.Int(true))
	fmt.Println(conv.Int(false))

	// Output:
	// 123
	// 456
	// 3
	// 1
	// 0
}

func ExampleFloat64() {
	// 字符串转浮点数
	fmt.Println(conv.Float64("3.14"))

	// 整数转浮点数
	fmt.Println(conv.Float64(42))

	// Output:
	// 3.14
	// 42
}

func ExampleBool() {
	// 各种值转布尔
	fmt.Println(conv.Bool(1))
	fmt.Println(conv.Bool(0))
	fmt.Println(conv.Bool("true"))
	fmt.Println(conv.Bool("false"))

	// Output:
	// true
	// false
	// true
	// false
}

func ExampleJSONToMap() {
	m, err := conv.JSONToMap(`{"name":"Alice","age":30}`)
	if err != nil {
		panic(err)
	}

	fmt.Println(m["name"])
	fmt.Println(m["age"])

	// Output:
	// Alice
	// 30
}

func ExampleMapToJSON() {
	m := map[string]any{
		"name": "Bob",
		"age":  25,
	}

	json, err := conv.MapToJSON(m)
	if err != nil {
		panic(err)
	}

	fmt.Println(json)
	// Output:
	// {"age":25,"name":"Bob"}
}

func ExampleMergeMaps() {
	m1 := map[string]any{"a": 1, "b": 2}
	m2 := map[string]any{"b": 3, "c": 4}

	result := conv.MergeMaps(m1, m2)

	fmt.Println(result["a"])
	fmt.Println(result["b"]) // m2 覆盖 m1
	fmt.Println(result["c"])

	// Output:
	// 1
	// 3
	// 4
}

func ExampleMapKeys() {
	m := map[string]any{
		"name": "Alice",
		"age":  30,
	}

	keys := conv.MapKeys(m)
	fmt.Println(len(keys))

	// Output:
	// 2
}

func ExampleMapValues() {
	m := map[string]any{
		"a": 1,
		"b": 2,
	}

	values := conv.MapValues(m)
	fmt.Println(len(values))

	// Output:
	// 2
}
