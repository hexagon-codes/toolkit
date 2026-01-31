package slicex

// Contains 检查切片中是否包含指定元素
//
// 参数:
//   - slice: 要检查的切片
//   - item: 要查找的元素
//
// 返回:
//   - bool: 如果找到返回 true，否则返回 false
//
// 示例:
//
//	found := slicex.Contains([]int{1, 2, 3}, 2)  // true
//	found := slicex.Contains([]string{"a", "b"}, "c")  // false
func Contains[T comparable](slice []T, item T) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

// ContainsFunc 使用自定义函数检查切片中是否包含满足条件的元素
//
// 参数:
//   - slice: 要检查的切片
//   - fn: 判断函数，返回 true 表示找到
//
// 返回:
//   - bool: 如果找到返回 true，否则返回 false
//
// 示例:
//
//	type User struct { Name string; Age int }
//	users := []User{{"Alice", 30}, {"Bob", 25}}
//	found := slicex.ContainsFunc(users, func(u User) bool {
//	    return u.Age > 28
//	})  // true
func ContainsFunc[T any](slice []T, fn func(T) bool) bool {
	for _, v := range slice {
		if fn(v) {
			return true
		}
	}
	return false
}
