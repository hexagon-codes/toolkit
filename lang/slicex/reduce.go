package slicex

// Reduce 聚合切片元素为单个值
//
// 参数:
//   - slice: 要聚合的切片
//   - initial: 初始值
//   - fn: 聚合函数
//
// 返回:
//   - R: 聚合后的结果
//
// 示例:
//
//	sum := slicex.Reduce([]int{1, 2, 3, 4}, 0, func(acc, n int) int {
//	    return acc + n  // 10
//	})
//
//	concat := slicex.Reduce([]string{"a", "b", "c"}, "", func(acc, s string) string {
//	    return acc + s  // "abc"
//	})
func Reduce[T any, R any](slice []T, initial R, fn func(R, T) R) R {
	result := initial
	for _, item := range slice {
		result = fn(result, item)
	}
	return result
}

// Some 检查是否至少有一个元素满足条件
//
// 参数:
//   - slice: 要检查的切片
//   - fn: 检查函数
//
// 返回:
//   - bool: 如果至少有一个元素满足条件返回 true
//
// 示例:
//
//	hasEven := slicex.Some([]int{1, 2, 3}, func(n int) bool {
//	    return n%2 == 0  // true (有 2)
//	})
func Some[T any](slice []T, fn func(T) bool) bool {
	for _, item := range slice {
		if fn(item) {
			return true
		}
	}
	return false
}

// Every 检查是否所有元素都满足条件
//
// 参数:
//   - slice: 要检查的切片
//   - fn: 检查函数
//
// 返回:
//   - bool: 如果所有元素都满足条件返回 true
//
// 示例:
//
//	allPositive := slicex.Every([]int{1, 2, 3}, func(n int) bool {
//	    return n > 0  // true
//	})
func Every[T any](slice []T, fn func(T) bool) bool {
	for _, item := range slice {
		if !fn(item) {
			return false
		}
	}
	return true
}

// Count 统计满足条件的元素数量
//
// 参数:
//   - slice: 要统计的切片
//   - fn: 检查函数
//
// 返回:
//   - int: 满足条件的元素数量
//
// 示例:
//
//	evenCount := slicex.Count([]int{1, 2, 3, 4}, func(n int) bool {
//	    return n%2 == 0  // 2
//	})
func Count[T any](slice []T, fn func(T) bool) int {
	count := 0
	for _, item := range slice {
		if fn(item) {
			count++
		}
	}
	return count
}

// GroupBy 按照 key 分组
//
// 参数:
//   - slice: 要分组的切片
//   - keyFn: 提取 key 的函数
//
// 返回:
//   - map[K][]T: 分组后的 map
//
// 示例:
//
//	type User struct { City string; Name string }
//	users := []User{{"Beijing", "Alice"}, {"Shanghai", "Bob"}, {"Beijing", "Charlie"}}
//	groups := slicex.GroupBy(users, func(u User) string {
//	    return u.City
//	})
//	// map[string][]User{
//	//     "Beijing": [{"Beijing", "Alice"}, {"Beijing", "Charlie"}],
//	//     "Shanghai": [{"Shanghai", "Bob"}],
//	// }
func GroupBy[T any, K comparable](slice []T, keyFn func(T) K) map[K][]T {
	result := make(map[K][]T)
	for _, item := range slice {
		key := keyFn(item)
		result[key] = append(result[key], item)
	}
	return result
}
