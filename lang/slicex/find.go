package slicex

// Find 查找第一个满足条件的元素
//
// 参数:
//   - slice: 要查找的切片
//   - fn: 查找函数，返回 true 表示找到
//
// 返回:
//   - T: 找到的元素
//   - bool: 是否找到
//
// 示例:
//
//	admin, found := slicex.Find(users, func(u User) bool {
//	    return u.Role == "admin"
//	})
//	if found {
//	    fmt.Println("Found admin:", admin.Name)
//	}
func Find[T any](slice []T, fn func(T) bool) (T, bool) {
	for _, item := range slice {
		if fn(item) {
			return item, true
		}
	}
	var zero T
	return zero, false
}

// FindIndex 查找第一个满足条件的元素的索引
//
// 参数:
//   - slice: 要查找的切片
//   - fn: 查找函数，返回 true 表示找到
//
// 返回:
//   - int: 找到的元素索引，未找到返回 -1
//
// 示例:
//
//	index := slicex.FindIndex([]int{1, 2, 3, 4}, func(n int) bool {
//	    return n > 2  // 2 (元素 3 的索引)
//	})
func FindIndex[T any](slice []T, fn func(T) bool) int {
	for i, item := range slice {
		if fn(item) {
			return i
		}
	}
	return -1
}

// FindLast 查找最后一个满足条件的元素
//
// 参数:
//   - slice: 要查找的切片
//   - fn: 查找函数，返回 true 表示找到
//
// 返回:
//   - T: 找到的元素
//   - bool: 是否找到
//
// 示例:
//
//	last, found := slicex.FindLast([]int{1, 2, 3, 2, 1}, func(n int) bool {
//	    return n == 2  // 返回索引 3 的元素
//	})
func FindLast[T any](slice []T, fn func(T) bool) (T, bool) {
	for i := len(slice) - 1; i >= 0; i-- {
		if fn(slice[i]) {
			return slice[i], true
		}
	}
	var zero T
	return zero, false
}

// IndexOf 查找元素在切片中的索引（使用 == 比较）
//
// 参数:
//   - slice: 要查找的切片
//   - item: 要查找的元素
//
// 返回:
//   - int: 元素索引，未找到返回 -1
//
// 示例:
//
//	index := slicex.IndexOf([]string{"a", "b", "c"}, "b")  // 1
//	index := slicex.IndexOf([]string{"a", "b", "c"}, "d")  // -1
func IndexOf[T comparable](slice []T, item T) int {
	for i, v := range slice {
		if v == item {
			return i
		}
	}
	return -1
}
