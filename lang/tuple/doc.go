// Package tuple 提供泛型元组类型，用于组合多个不同类型的值
//
// 主要类型:
//   - Tuple2[A, B]: 二元组
//   - Tuple3[A, B, C]: 三元组
//   - Tuple4[A, B, C, D]: 四元组
//
// 主要功能:
//   - 构造函数: T2/T3/T4
//   - 解包: Unpack 方法
//   - 交换: Swap 方法（仅 Tuple2）
//   - Zip/Unzip: 切片配对/拆分
//
// 示例:
//
//	// 创建元组
//	t := tuple.T2("name", 18)
//	name, age := t.Unpack()
//
//	// Zip 两个切片
//	names := []string{"Alice", "Bob"}
//	ages := []int{20, 25}
//	pairs := tuple.Zip2(names, ages)  // []Tuple2[string, int]
//
//	// Unzip 元组切片
//	names, ages = tuple.Unzip2(pairs)
//
// --- English ---
//
// Package tuple provides generic tuple types for combining multiple values
// of different types.
//
// Main types:
//   - Tuple2[A, B]: a 2-element tuple
//   - Tuple3[A, B, C]: a 3-element tuple
//   - Tuple4[A, B, C, D]: a 4-element tuple
//
// Main features:
//   - Constructors: T2/T3/T4
//   - Unpack: Unpack method
//   - Swap: Swap method (Tuple2 only)
//   - Zip/Unzip: pair/split slices
//
// Examples:
//
//	// Create a tuple
//	t := tuple.T2("name", 18)
//	name, age := t.Unpack()
//
//	// Zip two slices
//	names := []string{"Alice", "Bob"}
//	ages := []int{20, 25}
//	pairs := tuple.Zip2(names, ages)  // []Tuple2[string, int]
//
//	// Unzip a tuple slice
//	names, ages = tuple.Unzip2(pairs)
package tuple
