// Package stream 提供函数式流处理 API
//
// Stream 提供链式操作来处理数据集合，类似 Java 8 的 Stream API。
// 支持延迟求值和各种转换、过滤、聚合操作。
//
// 主要特性:
//   - 延迟求值：中间操作不会立即执行
//   - 链式调用：流畅的 API 设计
//   - 类型安全：使用泛型保证类型安全
//
// 创建 Stream:
//   - Of: 从值创建
//   - FromSlice: 从切片创建
//   - Generate: 使用生成函数创建
//   - Range: 创建数字范围
//
// 中间操作（返回新 Stream）:
//   - Filter: 过滤
//   - Map: 转换
//   - FlatMap: 展平转换
//   - Distinct: 去重
//   - Sorted: 排序
//   - Limit: 限制数量
//   - Skip: 跳过元素
//
// 终端操作（返回结果）:
//   - Collect: 收集到切片
//   - ForEach: 遍历
//   - Reduce: 归约
//   - Count: 计数
//   - First/Last: 获取首尾元素
//   - Any/All/None: 条件检查
//
// 示例:
//
//	// 过滤和转换
//	result := stream.FromSlice([]int{1, 2, 3, 4, 5}).
//	    Filter(func(n int) bool { return n%2 == 0 }).
//	    Map(func(n int) int { return n * 2 }).
//	    Collect()
//	// [4, 8]
//
//	// 归约操作
//	sum := stream.FromSlice([]int{1, 2, 3, 4, 5}).
//	    Reduce(0, func(acc, n int) int { return acc + n })
//	// 15
//
// --- English ---
//
// Package stream provides a functional stream processing API.
//
// Stream provides chainable operations for processing data collections,
// similar to Java 8 Stream API. Supports lazy evaluation and various
// transform, filter, and aggregate operations.
//
// Main features:
//   - Lazy evaluation: intermediate operations are not executed immediately
//   - Fluent chaining: clean and expressive API design
//   - Type-safe: uses generics to guarantee type safety
//
// Creating a Stream:
//   - Of: create from values
//   - FromSlice: create from a slice
//   - Generate: create using a generator function
//   - Range: create a numeric range
//
// Intermediate operations (return a new Stream):
//   - Filter: filter elements
//   - Map: transform elements
//   - FlatMap: flatten and transform
//   - Distinct: deduplicate elements
//   - Sorted: sort elements
//   - Limit: limit the number of elements
//   - Skip: skip elements
//
// Terminal operations (return a result):
//   - Collect: collect into a slice
//   - ForEach: iterate over elements
//   - Reduce: reduce to a single value
//   - Count: count elements
//   - First/Last: get the first/last element
//   - Any/All/None: conditional checks
//
// Examples:
//
//	// Filter and transform
//	result := stream.FromSlice([]int{1, 2, 3, 4, 5}).
//	    Filter(func(n int) bool { return n%2 == 0 }).
//	    Map(func(n int) int { return n * 2 }).
//	    Collect()
//	// [4, 8]
//
//	// Reduce operation
//	sum := stream.FromSlice([]int{1, 2, 3, 4, 5}).
//	    Reduce(0, func(acc, n int) int { return acc + n })
//	// 15
package stream
