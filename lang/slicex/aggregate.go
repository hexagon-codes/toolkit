package slicex

import "github.com/hexagon-codes/toolkit/lang/mathx"

// Number 数字类型约束
type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~float32 | ~float64
}

// Min 返回切片中的最小值
//
// 参数:
//   - slice: 要查找的切片
//
// 返回:
//   - T: 最小值（空切片返回零值）
//
// 示例:
//
//	min := slicex.Min([]int{3, 1, 4, 1, 5})  // 1
func Min[T mathx.Ordered](slice []T) T {
	if len(slice) == 0 {
		var zero T
		return zero
	}
	minimum := slice[0]
	for _, v := range slice[1:] {
		if v < minimum {
			minimum = v
		}
	}
	return minimum
}

// Max 返回切片中的最大值
//
// 参数:
//   - slice: 要查找的切片
//
// 返回:
//   - T: 最大值（空切片返回零值）
//
// 示例:
//
//	max := slicex.Max([]int{3, 1, 4, 1, 5})  // 5
func Max[T mathx.Ordered](slice []T) T {
	if len(slice) == 0 {
		var zero T
		return zero
	}
	maximum := slice[0]
	for _, v := range slice[1:] {
		if v > maximum {
			maximum = v
		}
	}
	return maximum
}

// MinMax 同时返回切片中的最小值和最大值
//
// 参数:
//   - slice: 要查找的切片
//
// 返回:
//   - T: 最小值
//   - T: 最大值
//
// 示例:
//
//	min, max := slicex.MinMax([]int{3, 1, 4, 1, 5})  // 1, 5
func MinMax[T mathx.Ordered](slice []T) (minimum, maximum T) {
	if len(slice) == 0 {
		var zero T
		return zero, zero
	}
	minimum, maximum = slice[0], slice[0]
	for _, v := range slice[1:] {
		if v < minimum {
			minimum = v
		}
		if v > maximum {
			maximum = v
		}
	}
	return minimum, maximum
}

// MinBy 使用自定义比较函数返回最小元素
//
// 参数:
//   - slice: 要查找的切片
//   - less: 比较函数，如果 a < b 返回 true
//
// 返回:
//   - T: 最小元素
//   - bool: 是否找到
//
// 示例:
//
//	user, _ := slicex.MinBy(users, func(a, b User) bool {
//	    return a.Age < b.Age
//	})
func MinBy[T any](slice []T, less func(a, b T) bool) (T, bool) {
	if len(slice) == 0 {
		var zero T
		return zero, false
	}
	minimum := slice[0]
	for _, v := range slice[1:] {
		if less(v, minimum) {
			minimum = v
		}
	}
	return minimum, true
}

// MaxBy 使用自定义比较函数返回最大元素
//
// 参数:
//   - slice: 要查找的切片
//   - less: 比较函数，如果 a < b 返回 true
//
// 返回:
//   - T: 最大元素
//   - bool: 是否找到
//
// 示例:
//
//	user, _ := slicex.MaxBy(users, func(a, b User) bool {
//	    return a.Age < b.Age
//	})
func MaxBy[T any](slice []T, less func(a, b T) bool) (T, bool) {
	if len(slice) == 0 {
		var zero T
		return zero, false
	}
	maximum := slice[0]
	for _, v := range slice[1:] {
		if less(maximum, v) {
			maximum = v
		}
	}
	return maximum, true
}

// MinByKey 根据提取的键返回最小元素
//
// 参数:
//   - slice: 要查找的切片
//   - keyFn: 提取比较键的函数
//
// 返回:
//   - T: 键最小的元素
//   - bool: 是否找到
//
// 示例:
//
//	youngest, _ := slicex.MinByKey(users, func(u User) int {
//	    return u.Age
//	})
func MinByKey[T any, K mathx.Ordered](slice []T, keyFn func(T) K) (T, bool) {
	if len(slice) == 0 {
		var zero T
		return zero, false
	}
	minimum := slice[0]
	minKey := keyFn(minimum)
	for _, v := range slice[1:] {
		if key := keyFn(v); key < minKey {
			minimum = v
			minKey = key
		}
	}
	return minimum, true
}

// MaxByKey 根据提取的键返回最大元素
//
// 参数:
//   - slice: 要查找的切片
//   - keyFn: 提取比较键的函数
//
// 返回:
//   - T: 键最大的元素
//   - bool: 是否找到
//
// 示例:
//
//	oldest, _ := slicex.MaxByKey(users, func(u User) int {
//	    return u.Age
//	})
func MaxByKey[T any, K mathx.Ordered](slice []T, keyFn func(T) K) (T, bool) {
	if len(slice) == 0 {
		var zero T
		return zero, false
	}
	maximum := slice[0]
	maxKey := keyFn(maximum)
	for _, v := range slice[1:] {
		if key := keyFn(v); key > maxKey {
			maximum = v
			maxKey = key
		}
	}
	return maximum, true
}

// Sum 计算切片元素之和
//
// 参数:
//   - slice: 要求和的切片
//
// 返回:
//   - T: 所有元素之和
//
// 示例:
//
//	sum := slicex.Sum([]int{1, 2, 3, 4, 5})  // 15
func Sum[T Number](slice []T) T {
	var sum T
	for _, v := range slice {
		sum += v
	}
	return sum
}

// SumBy 使用提取函数计算和
//
// 参数:
//   - slice: 输入切片
//   - fn: 提取数值的函数
//
// 返回:
//   - R: 所有提取值之和
//
// 示例:
//
//	totalAge := slicex.SumBy(users, func(u User) int {
//	    return u.Age
//	})
func SumBy[T any, R Number](slice []T, fn func(T) R) R {
	var sum R
	for _, v := range slice {
		sum += fn(v)
	}
	return sum
}

// Average 计算切片元素的平均值
//
// 参数:
//   - slice: 要计算平均值的切片
//
// 返回:
//   - float64: 平均值（空切片返回 0）
//
// 示例:
//
//	avg := slicex.Average([]int{1, 2, 3, 4, 5})  // 3.0
func Average[T Number](slice []T) float64 {
	if len(slice) == 0 {
		return 0
	}
	var sum T
	for _, v := range slice {
		sum += v
	}
	return float64(sum) / float64(len(slice))
}

// AverageBy 使用提取函数计算平均值
//
// 参数:
//   - slice: 输入切片
//   - fn: 提取数值的函数
//
// 返回:
//   - float64: 平均值
//
// 示例:
//
//	avgAge := slicex.AverageBy(users, func(u User) int {
//	    return u.Age
//	})
func AverageBy[T any, R Number](slice []T, fn func(T) R) float64 {
	if len(slice) == 0 {
		return 0
	}
	var sum R
	for _, v := range slice {
		sum += fn(v)
	}
	return float64(sum) / float64(len(slice))
}

// Product 计算切片元素之积
//
// 参数:
//   - slice: 要计算乘积的切片
//
// 返回:
//   - T: 所有元素之积（空切片返回 1）
//
// 示例:
//
//	product := slicex.Product([]int{1, 2, 3, 4})  // 24
func Product[T Number](slice []T) T {
	if len(slice) == 0 {
		return T(1)
	}
	product := T(1)
	for _, v := range slice {
		product *= v
	}
	return product
}

// CountValue 统计指定值出现的次数
//
// 参数:
//   - slice: 要统计的切片
//   - value: 要统计的值
//
// 返回:
//   - int: 值出现的次数
//
// 示例:
//
//	count := slicex.CountValue([]int{1, 2, 2, 3, 2}, 2)  // 3
func CountValue[T comparable](slice []T, value T) int {
	count := 0
	for _, v := range slice {
		if v == value {
			count++
		}
	}
	return count
}
