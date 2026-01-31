package slicex

// Map 映射切片，将每个元素转换为新类型
//
// 参数:
//   - slice: 要映射的切片
//   - fn: 转换函数
//
// 返回:
//   - []R: 映射后的新切片
//
// 示例:
//
//	doubled := slicex.Map([]int{1, 2, 3}, func(n int) int {
//	    return n * 2  // [2, 4, 6]
//	})
//
//	names := slicex.Map(users, func(u User) string {
//	    return u.Name  // 提取所有用户名
//	})
func Map[T any, R any](slice []T, fn func(T) R) []R {
	result := make([]R, len(slice))
	for i, item := range slice {
		result[i] = fn(item)
	}
	return result
}

// MapWithIndex 映射切片，转换函数可以访问索引
//
// 参数:
//   - slice: 要映射的切片
//   - fn: 转换函数，接收索引和元素
//
// 返回:
//   - []R: 映射后的新切片
//
// 示例:
//
//	indexed := slicex.MapWithIndex([]string{"a", "b", "c"}, func(i int, s string) string {
//	    return fmt.Sprintf("%d:%s", i, s)  // ["0:a", "1:b", "2:c"]
//	})
func MapWithIndex[T any, R any](slice []T, fn func(int, T) R) []R {
	result := make([]R, len(slice))
	for i, item := range slice {
		result[i] = fn(i, item)
	}
	return result
}

// FlatMap 映射后展平结果（每个元素可以映射为多个元素）
//
// 参数:
//   - slice: 要映射的切片
//   - fn: 转换函数，返回切片
//
// 返回:
//   - []R: 展平后的新切片
//
// 示例:
//
//	result := slicex.FlatMap([]int{1, 2, 3}, func(n int) []int {
//	    return []int{n, n * 10}  // [1, 10, 2, 20, 3, 30]
//	})
func FlatMap[T any, R any](slice []T, fn func(T) []R) []R {
	result := make([]R, 0, len(slice)*2)
	for _, item := range slice {
		result = append(result, fn(item)...)
	}
	return result
}
