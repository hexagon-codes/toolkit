package main

import (
	"fmt"
	"reflect"
	"time"
	"unsafe"

	"github.com/everyday-items/toolkit/lang/stringx"
)

func main() {
	fmt.Println("=== lang/stringx 零拷贝字符串示例 ===")

	// 1. BytesToString - 零拷贝转换
	fmt.Println("1. BytesToString - []byte 转 string（零拷贝）")
	demonstrateBytesToString()

	// 2. String2Bytes - 零拷贝转换
	fmt.Println("\n2. String2Bytes - string 转 []byte（零拷贝）")
	demonstrateString2Bytes()

	// 3. StringToSlice - 切片转换
	fmt.Println("\n3. StringToSlice - 切片转 []any")
	demonstrateStringToSlice()

	// 4. 性能对比
	fmt.Println("\n4. 性能对比")
	benchmarkComparison()

	// 5. 实际应用场景
	fmt.Println("\n5. 实际应用场景")
	demonstratePracticalUseCases()

	// 6. 注意事项
	fmt.Println("\n6. 注意事项和最佳实践")
	demonstrateCautions()
}

// demonstrateBytesToString 演示 BytesToString
func demonstrateBytesToString() {
	// 基本使用
	fmt.Println("\n  [基本使用]")
	bytes := []byte("Hello, World!")
	str := stringx.BytesToString(bytes)
	fmt.Printf("  原始 []byte: %v\n", bytes)
	fmt.Printf("  ✓ 转换结果: %s\n", str)
	fmt.Printf("  ✓ 长度: %d\n", len(str))

	// 空切片
	fmt.Println("\n  [空切片]")
	emptyBytes := []byte{}
	emptyStr := stringx.BytesToString(emptyBytes)
	fmt.Printf("  空 []byte 转换: '%s' (长度: %d)\n", emptyStr, len(emptyStr))

	// 中文字符串
	fmt.Println("\n  [中文字符串]")
	chineseBytes := []byte("你好，世界！")
	chineseStr := stringx.BytesToString(chineseBytes)
	fmt.Printf("  中文 []byte: %v\n", chineseBytes)
	fmt.Printf("  ✓ 转换结果: %s\n", chineseStr)

	// 二进制数据
	fmt.Println("\n  [二进制数据]")
	binaryBytes := []byte{0x48, 0x65, 0x6c, 0x6c, 0x6f} // "Hello"
	binaryStr := stringx.BytesToString(binaryBytes)
	fmt.Printf("  二进制数据: %v\n", binaryBytes)
	fmt.Printf("  ✓ 转换结果: %s\n", binaryStr)
}

// demonstrateString2Bytes 演示 String2Bytes
func demonstrateString2Bytes() {
	// 基本使用
	fmt.Println("\n  [基本使用]")
	str := "Hello, Go!"
	bytes := stringx.String2Bytes(str)
	fmt.Printf("  原始 string: %s\n", str)
	fmt.Printf("  ✓ 转换结果: %v\n", bytes)
	fmt.Printf("  ✓ 长度: %d\n", len(bytes))

	// 空字符串
	fmt.Println("\n  [空字符串]")
	emptyStr := ""
	emptyBytes := stringx.String2Bytes(emptyStr)
	fmt.Printf("  空 string 转换: %v (长度: %d)\n", emptyBytes, len(emptyBytes))

	// 特殊字符
	fmt.Println("\n  [特殊字符]")
	specialStr := "Hello\nWorld\t!"
	specialBytes := stringx.String2Bytes(specialStr)
	fmt.Printf("  特殊字符 string: %q\n", specialStr)
	fmt.Printf("  ✓ 转换结果: %v\n", specialBytes)

	// 中文字符串
	fmt.Println("\n  [中文字符串]")
	chineseStr := "你好，世界！"
	chineseBytes := stringx.String2Bytes(chineseStr)
	fmt.Printf("  中文 string: %s\n", chineseStr)
	fmt.Printf("  ✓ 转换结果: %v (长度: %d)\n", chineseBytes, len(chineseBytes))
}

// demonstrateStringToSlice 演示 StringToSlice
func demonstrateStringToSlice() {
	// 整数切片
	fmt.Println("\n  [整数切片]")
	intSlice := []int{1, 2, 3, 4, 5}
	anySlice := stringx.StringToSlice(intSlice)
	fmt.Printf("  原始切片: %v\n", intSlice)
	fmt.Printf("  ✓ 转换结果: %v\n", anySlice)
	fmt.Printf("  ✓ 类型: %T\n", anySlice)

	// 字符串切片
	fmt.Println("\n  [字符串切片]")
	strSlice := []string{"apple", "banana", "cherry"}
	anySlice2 := stringx.StringToSlice(strSlice)
	fmt.Printf("  原始切片: %v\n", strSlice)
	fmt.Printf("  ✓ 转换结果: %v\n", anySlice2)

	// 结构体切片
	fmt.Println("\n  [结构体切片]")
	type Person struct {
		Name string
		Age  int
	}
	personSlice := []Person{
		{Name: "Alice", Age: 30},
		{Name: "Bob", Age: 25},
	}
	anySlice3 := stringx.StringToSlice(personSlice)
	fmt.Printf("  原始切片: %+v\n", personSlice)
	fmt.Printf("  ✓ 转换结果: %v\n", anySlice3)

	// 数组
	fmt.Println("\n  [数组]")
	arr := [3]string{"x", "y", "z"}
	anySlice4 := stringx.StringToSlice(arr)
	fmt.Printf("  原始数组: %v\n", arr)
	fmt.Printf("  ✓ 转换结果: %v\n", anySlice4)

	// 非切片类型
	fmt.Println("\n  [非切片类型]")
	notSlice := "not a slice"
	result := stringx.StringToSlice(notSlice)
	fmt.Printf("  非切片输入: %v\n", notSlice)
	fmt.Printf("  ✓ 转换结果: %v (返回 nil)\n", result)
}

// benchmarkComparison 性能对比
func benchmarkComparison() {
	testData := []byte("This is a test string for performance comparison. It contains some text to make it more realistic.")
	iterations := 1000000

	// 标准转换 (有拷贝)
	fmt.Println("\n  [标准转换] string([]byte) - 有内存拷贝")
	start := time.Now()
	for i := 0; i < iterations; i++ {
		_ = string(testData)
	}
	standardDuration := time.Since(start)
	fmt.Printf("  执行 %d 次: %v\n", iterations, standardDuration)

	// 零拷贝转换
	fmt.Println("\n  [零拷贝转换] BytesToString - 无内存拷贝")
	start = time.Now()
	for i := 0; i < iterations; i++ {
		_ = stringx.BytesToString(testData)
	}
	zeroCopyDuration := time.Since(start)
	fmt.Printf("  执行 %d 次: %v\n", iterations, zeroCopyDuration)

	// 性能提升
	improvement := float64(standardDuration) / float64(zeroCopyDuration)
	fmt.Printf("\n  ✓ 性能提升: %.2fx\n", improvement)
	fmt.Printf("  ✓ 节省时间: %v\n", standardDuration-zeroCopyDuration)

	// 内存分配对比
	fmt.Println("\n  [内存分配]")
	fmt.Printf("  标准转换: 每次分配 %d 字节\n", len(testData))
	fmt.Printf("  零拷贝: 0 字节分配\n")
}

// demonstratePracticalUseCases 实际应用场景
func demonstratePracticalUseCases() {
	// 场景1: HTTP 响应处理
	fmt.Println("\n  [场景1] HTTP 响应处理")
	responseBody := []byte(`{"status":"success","data":{"id":123,"name":"Alice"}}`)
	jsonString := stringx.BytesToString(responseBody)
	fmt.Printf("  HTTP 响应: %s\n", jsonString)
	fmt.Printf("  ✓ 零拷贝转换，避免重复分配\n")

	// 场景2: 文件读取
	fmt.Println("\n  [场景2] 文件读取")
	fileContent := []byte("File content from disk...")
	content := stringx.BytesToString(fileContent)
	fmt.Printf("  文件内容: %s\n", content)
	fmt.Printf("  ✓ 大文件读取时性能优势明显\n")

	// 场景3: 网络协议解析
	fmt.Println("\n  [场景3] 网络协议解析")
	packet := []byte("HEADER:DATA:CHECKSUM")
	packetStr := stringx.BytesToString(packet)
	fmt.Printf("  网络包: %s\n", packetStr)
	fmt.Printf("  ✓ 高频转换场景性能优化\n")

	// 场景4: 数据库查询结果
	fmt.Println("\n  [场景4] 数据库 []byte 字段处理")
	dbField := []byte("Database BLOB field content")
	fieldStr := stringx.BytesToString(dbField)
	fmt.Printf("  数据库字段: %s\n", fieldStr)
	fmt.Printf("  ✓ 避免不必要的内存拷贝\n")

	// 场景5: 日志处理
	fmt.Println("\n  [场景5] 日志消息处理")
	logMessage := []byte("[INFO] Application started successfully")
	log := stringx.BytesToString(logMessage)
	fmt.Printf("  日志: %s\n", log)
	fmt.Printf("  ✓ 高频日志场景性能提升\n")
}

// demonstrateCautions 注意事项
func demonstrateCautions() {
	// 注意1: 不要修改原始数据
	fmt.Println("\n  [注意1] BytesToString - 不要修改原始 []byte")
	originalBytes := []byte("Original")
	str := stringx.BytesToString(originalBytes)
	fmt.Printf("  转换前: []byte=%v, string=%s\n", originalBytes, str)

	// 修改原始 []byte（危险操作！）
	fmt.Println("  ⚠️  修改原始 []byte...")
	originalBytes[0] = 'M'
	fmt.Printf("  修改后: []byte=%v, string=%s\n", originalBytes, str)
	fmt.Printf("  ❌ string 内容也被修改了！这违反了 string 不可变性\n")

	// 注意2: 不要修改转换后的 []byte
	fmt.Println("\n  [注意2] String2Bytes - 不要修改返回的 []byte")
	originalStr := "Immutable"
	bytes := stringx.String2Bytes(originalStr)
	fmt.Printf("  转换前: string=%s, []byte=%v\n", originalStr, bytes)
	fmt.Printf("  ⚠️  修改返回的 []byte 会导致 panic（string 是不可变的）\n")
	// bytes[0] = 'X'  // 取消注释会 panic!

	// 注意3: 生命周期管理
	fmt.Println("\n  [注意3] 数据生命周期管理")
	fmt.Printf("  ✓ 确保原始数据在使用期间不被回收\n")
	fmt.Printf("  ✓ 不要在函数间传递零拷贝转换的数据（除非确保安全）\n")

	// 正确使用示例
	fmt.Println("\n  [最佳实践] 安全使用零拷贝")
	fmt.Println("  1. 只在性能关键路径使用")
	fmt.Println("  2. 确保数据只读")
	fmt.Println("  3. 数据生命周期在可控范围内")
	fmt.Println("  4. 需要修改时使用标准转换")

	// 内存布局对比
	fmt.Println("\n  [内存布局] 对比")
	testBytes := []byte("Test")

	standardStr := string(testBytes)
	zeroCopyStr := stringx.BytesToString(testBytes)

	fmt.Printf("  标准转换:\n")
	fmt.Printf("    []byte 地址: %p\n", &testBytes[0])
	fmt.Printf("    string 地址: 0x%x\n", (*reflect.StringHeader)(unsafe.Pointer(&standardStr)).Data)
	fmt.Printf("    ✓ 不同地址（有拷贝）\n")

	fmt.Printf("\n  零拷贝转换:\n")
	fmt.Printf("    []byte 地址: %p\n", &testBytes[0])
	fmt.Printf("    string 地址: 0x%x\n", (*reflect.StringHeader)(unsafe.Pointer(&zeroCopyStr)).Data)
	fmt.Printf("    ✓ 相同地址（零拷贝）\n")

	// 使用建议
	fmt.Println("\n  [使用建议]")
	fmt.Printf("  ✓ HTTP 请求/响应处理: 推荐使用\n")
	fmt.Printf("  ✓ 文件读取: 推荐使用\n")
	fmt.Printf("  ✓ 网络数据解析: 推荐使用\n")
	fmt.Printf("  ✓ 日志处理: 推荐使用\n")
	fmt.Printf("  ❌ 需要修改数据: 使用标准转换\n")
	fmt.Printf("  ❌ 跨 goroutine 传递: 谨慎使用\n")
	fmt.Printf("  ❌ 长期存储: 使用标准转换\n")
}
