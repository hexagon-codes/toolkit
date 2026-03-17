// Package reflectx 提供反射相关的工具函数
//
// 主要功能:
//   - StructToMap: 将结构体转换为 map
//   - MapToStruct: 将 map 转换为结构体
//   - GetField: 获取结构体字段值
//   - SetField: 设置结构体字段值
//   - DeepCopy: 深度拷贝
//   - IsZero: 检查值是否为零值
//   - IsNil: 检查值是否为 nil
//
// 示例:
//
//	type User struct {
//	    Name string `json:"name"`
//	    Age  int    `json:"age"`
//	}
//
//	// 结构体转 map
//	user := User{Name: "Alice", Age: 20}
//	m := reflectx.StructToMap(user)
//	// map[string]any{"Name": "Alice", "Age": 20}
//
//	// map 转结构体
//	data := map[string]any{"Name": "Bob", "Age": 25}
//	var u User
//	reflectx.MapToStruct(data, &u)
//
//	// 获取字段
//	name, _ := reflectx.GetField(user, "Name")
//
//	// 设置字段
//	reflectx.SetField(&user, "Age", 21)
//
// --- English ---
//
// Package reflectx provides reflection-based utility functions.
//
// Main features:
//   - StructToMap: convert a struct to a map
//   - MapToStruct: convert a map to a struct
//   - GetField: get a struct field value
//   - SetField: set a struct field value
//   - DeepCopy: deep copy a value
//   - IsZero: check if a value is the zero value
//   - IsNil: check if a value is nil
//
// Examples:
//
//	type User struct {
//	    Name string `json:"name"`
//	    Age  int    `json:"age"`
//	}
//
//	// Struct to map
//	user := User{Name: "Alice", Age: 20}
//	m := reflectx.StructToMap(user)
//	// map[string]any{"Name": "Alice", "Age": 20}
//
//	// Map to struct
//	data := map[string]any{"Name": "Bob", "Age": 25}
//	var u User
//	reflectx.MapToStruct(data, &u)
//
//	// Get a field
//	name, _ := reflectx.GetField(user, "Name")
//
//	// Set a field
//	reflectx.SetField(&user, "Age", 21)
package reflectx
