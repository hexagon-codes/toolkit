package stringx_test

import (
	"fmt"

	"github.com/everyday-items/toolkit/lang/stringx"
)

func ExampleBytesToString() {
	// 零拷贝转换 []byte 到 string
	b := []byte("hello world")
	s := stringx.BytesToString(b)

	fmt.Println(s)
	// Output:
	// hello world
}

func ExampleString2Bytes() {
	// 零拷贝转换 string 到 []byte
	s := "hello world"
	b := stringx.String2Bytes(s)

	fmt.Println(string(b))
	// Output:
	// hello world
}

// 注意：这个示例展示了往返转换
func ExampleBytesToString_roundtrip() {
	original := "Hello, 世界!"

	// string -> bytes -> string
	bytes := stringx.String2Bytes(original)
	result := stringx.BytesToString(bytes)

	fmt.Println(result)
	// Output:
	// Hello, 世界!
}

func ExampleStringToSlice() {
	// 字符串切片转通用切片
	strSlice := []string{"apple", "banana", "cherry"}
	result := stringx.StringToSlice(strSlice)

	fmt.Println(len(result))
	fmt.Println(result[0])
	fmt.Println(result[1])
	// Output:
	// 3
	// apple
	// banana
}

func ExampleStringToSlice_intSlice() {
	// 整数切片转通用切片
	intSlice := []int{1, 2, 3, 4, 5}
	result := stringx.StringToSlice(intSlice)

	fmt.Println(len(result))
	fmt.Println(result[0])
	// Output:
	// 5
	// 1
}

func ExampleStringToSlice_array() {
	// 数组也可以转换
	arr := [3]string{"red", "green", "blue"}
	result := stringx.StringToSlice(arr)

	fmt.Println(len(result))
	// Output:
	// 3
}
