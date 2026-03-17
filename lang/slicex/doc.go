// Package slicex 提供切片操作的工具函数
//
// 这个包提供了 Go 标准库缺失的常用切片操作，使用泛型实现类型安全。
//
// # 主要功能
//
// 查找和检查:
//   - Contains: 检查是否包含元素
//   - Find: 查找满足条件的元素
//   - IndexOf: 查找元素索引
//
// 转换和映射:
//   - Map: 映射转换
//   - Filter: 过滤
//   - Unique: 去重
//
// 聚合:
//   - Reduce: 聚合为单个值
//   - GroupBy: 分组
//   - Count: 统计数量
//
// 工具:
//   - Reverse: 反转
//   - Chunk: 分块
//   - Take/Drop: 取前/跳过
//
// # 使用示例
//
//	import "github.com/hexagon-codes/toolkit/lang/slicex"
//
//	// 包含检查
//	found := slicex.Contains([]int{1, 2, 3}, 2)  // true
//
//	// 过滤
//	even := slicex.Filter([]int{1, 2, 3, 4}, func(n int) bool {
//	    return n%2 == 0  // [2, 4]
//	})
//
//	// 映射
//	doubled := slicex.Map([]int{1, 2, 3}, func(n int) int {
//	    return n * 2  // [2, 4, 6]
//	})
//
//	// 去重
//	unique := slicex.Unique([]int{1, 2, 2, 3})  // [1, 2, 3]
//
//	// 聚合
//	sum := slicex.Reduce([]int{1, 2, 3}, 0, func(acc, n int) int {
//	    return acc + n  // 6
//	})
//
// # 设计原则
//
// 1. 零外部依赖：只使用 Go 标准库
// 2. 类型安全：使用泛型，编译时类型检查
// 3. 不修改原切片：所有函数（除了 *InPlace 后缀的）都返回新切片
// 4. 性能优先：预分配容量，减少内存分配
//
// # 注意事项
//
// - 所有函数都是并发不安全的，如需并发访问请自行加锁
// - *InPlace 后缀的函数会修改原切片
// - 空切片作为参数时，大部分函数返回空切片而不是 nil
//
// --- English ---
//
// Package slicex provides utility functions for slice operations.
//
// This package provides commonly used slice operations missing from the Go
// standard library, implemented with generics for type safety.
//
// # Main Features
//
// Search and check:
//   - Contains: check if a slice contains an element
//   - Find: find an element matching a condition
//   - IndexOf: find the index of an element
//
// Transform and map:
//   - Map: map/transform elements
//   - Filter: filter elements
//   - Unique: deduplicate elements
//
// Aggregate:
//   - Reduce: aggregate to a single value
//   - GroupBy: group elements
//   - Count: count matching elements
//
// Utilities:
//   - Reverse: reverse a slice
//   - Chunk: split into chunks
//   - Take/Drop: take first N / skip first N elements
//
// # Usage Examples
//
//	import "github.com/hexagon-codes/toolkit/lang/slicex"
//
//	// Contains check
//	found := slicex.Contains([]int{1, 2, 3}, 2)  // true
//
//	// Filter
//	even := slicex.Filter([]int{1, 2, 3, 4}, func(n int) bool {
//	    return n%2 == 0  // [2, 4]
//	})
//
//	// Map
//	doubled := slicex.Map([]int{1, 2, 3}, func(n int) int {
//	    return n * 2  // [2, 4, 6]
//	})
//
//	// Unique
//	unique := slicex.Unique([]int{1, 2, 2, 3})  // [1, 2, 3]
//
//	// Reduce
//	sum := slicex.Reduce([]int{1, 2, 3}, 0, func(acc, n int) int {
//	    return acc + n  // 6
//	})
//
// # Design Principles
//
// 1. Zero external dependencies: only uses Go standard library
// 2. Type-safe: uses generics for compile-time type checking
// 3. Non-mutating: all functions (except those with *InPlace suffix) return a new slice
// 4. Performance first: pre-allocate capacity to reduce memory allocations
//
// # Notes
//
// - All functions are not concurrency-safe; add your own locking for concurrent access
// - Functions with the *InPlace suffix modify the original slice
// - When an empty slice is passed, most functions return an empty slice rather than nil
package slicex
