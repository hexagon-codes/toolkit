package slicex

// Filter 过滤切片，返回满足条件的元素
//
// 参数:
//   - slice: 要过滤的切片
//   - fn: 过滤函数，返回 true 的元素会被保留
//
// 返回:
//   - []T: 过滤后的新切片
//
// 示例:
//
//	even := slicex.Filter([]int{1, 2, 3, 4}, func(n int) bool {
//	    return n%2 == 0  // [2, 4]
//	})
//
//	activeUsers := slicex.Filter(users, func(u User) bool {
//	    return u.Status == "active"
//	})
func Filter[T any](slice []T, fn func(T) bool) []T {
	result := make([]T, 0, len(slice))
	for _, item := range slice {
		if fn(item) {
			result = append(result, item)
		}
	}
	return result
}

// Reject 过滤切片，返回不满足条件的元素（Filter 的反向操作）
//
// 参数:
//   - slice: 要过滤的切片
//   - fn: 过滤函数，返回 true 的元素会被排除
//
// 返回:
//   - []T: 过滤后的新切片
//
// 示例:
//
//	odd := slicex.Reject([]int{1, 2, 3, 4}, func(n int) bool {
//	    return n%2 == 0  // [1, 3]
//	})
func Reject[T any](slice []T, fn func(T) bool) []T {
	result := make([]T, 0, len(slice))
	for _, item := range slice {
		if !fn(item) {
			result = append(result, item)
		}
	}
	return result
}
