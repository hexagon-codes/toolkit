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
//	import "github.com/everyday-items/toolkit/lang/slicex"
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
package slicex
