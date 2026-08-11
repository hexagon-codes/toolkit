// Package slice 提供泛型切片处理工具。
package slice

import "math/rand/v2"

type integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

type floating interface {
	~float32 | ~float64
}

type number interface {
	integer | floating
}

// Unique 去重（保持顺序）
func Unique[T comparable](slice []T) []T {
	seen := make(map[T]struct{}, len(slice))
	result := make([]T, 0, len(slice))

	for _, item := range slice {
		if _, ok := seen[item]; !ok {
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}

	return compact(result)
}

// Contains 判断切片是否包含元素
func Contains[T comparable](slice []T, item T) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

// IndexOf 查找元素索引，不存在返回 -1
func IndexOf[T comparable](slice []T, item T) int {
	for i, v := range slice {
		if v == item {
			return i
		}
	}
	return -1
}

// LastIndexOf 查找元素最后一次出现的索引，不存在返回 -1
func LastIndexOf[T comparable](slice []T, item T) int {
	for i := len(slice) - 1; i >= 0; i-- {
		if slice[i] == item {
			return i
		}
	}
	return -1
}

// Remove 移除第一个匹配的元素（不修改原始切片）
func Remove[T comparable](slice []T, item T) []T {
	for i, v := range slice {
		if v == item {
			result := make([]T, 0, len(slice)-1)
			result = append(result, slice[:i]...)
			result = append(result, slice[i+1:]...)
			return result
		}
	}
	return clone(slice)
}

// RemoveAll 移除所有匹配的元素
func RemoveAll[T comparable](slice []T, item T) []T {
	result := make([]T, 0, len(slice))
	for _, v := range slice {
		if v != item {
			result = append(result, v)
		}
	}
	return compact(result)
}

// RemoveAt 移除指定索引的元素（不修改原始切片）
func RemoveAt[T any](slice []T, index int) []T {
	if index < 0 || index >= len(slice) {
		return clone(slice)
	}
	result := make([]T, 0, len(slice)-1)
	result = append(result, slice[:index]...)
	result = append(result, slice[index+1:]...)
	return result
}

// Reverse 反转切片
func Reverse[T any](slice []T) []T {
	result := make([]T, len(slice))
	for i, v := range slice {
		result[len(slice)-1-i] = v
	}
	return result
}

// Shuffle 打乱切片顺序（使用 Fisher-Yates 算法）
func Shuffle[T any](slice []T) []T {
	result := make([]T, len(slice))
	copy(result, slice)

	// 使用全局伪随机源；该随机性不适用于密钥、令牌等安全场景。
	for i := len(result) - 1; i > 0; i-- {
		j := rand.IntN(i + 1) // #nosec G404 -- 洗牌随机性不用于密钥、令牌或其他安全用途。
		result[i], result[j] = result[j], result[i]
	}

	return result
}

// Chunk 将切片分成多个子切片
func Chunk[T any](slice []T, size int) [][]T {
	if len(slice) == 0 {
		return nil
	}
	if size <= 0 {
		size = len(slice)
	}

	chunks := make([][]T, 0, (len(slice)-1)/size+1)
	for i := 0; i < len(slice); i += size {
		end := i + size
		if end > len(slice) {
			end = len(slice)
		}
		chunk := make([]T, end-i)
		copy(chunk, slice[i:end])
		chunks = append(chunks, chunk)
	}

	return chunks
}

// Flatten 扁平化二维切片
func Flatten[T any](slices [][]T) []T {
	var result []T
	for _, s := range slices {
		result = append(result, s...)
	}
	return compact(result)
}

// Union 并集
func Union[T comparable](slice1, slice2 []T) []T {
	set := make(map[T]struct{})
	result := make([]T, 0, len(slice1)+len(slice2))

	for _, item := range slice1 {
		if _, ok := set[item]; !ok {
			set[item] = struct{}{}
			result = append(result, item)
		}
	}

	for _, item := range slice2 {
		if _, ok := set[item]; !ok {
			set[item] = struct{}{}
			result = append(result, item)
		}
	}

	return compact(result)
}

// Intersect 交集
func Intersect[T comparable](slice1, slice2 []T) []T {
	set := make(map[T]struct{})
	for _, item := range slice1 {
		set[item] = struct{}{}
	}

	var result []T
	seen := make(map[T]struct{})

	for _, item := range slice2 {
		if _, ok := set[item]; ok {
			if _, exists := seen[item]; !exists {
				seen[item] = struct{}{}
				result = append(result, item)
			}
		}
	}

	return result
}

// Difference 差集（在 slice1 中但不在 slice2 中）
func Difference[T comparable](slice1, slice2 []T) []T {
	set := make(map[T]struct{})
	for _, item := range slice2 {
		set[item] = struct{}{}
	}

	var result []T
	for _, item := range slice1 {
		if _, ok := set[item]; !ok {
			result = append(result, item)
		}
	}

	return result
}

// Equal 判断两个切片是否相等
func Equal[T comparable](slice1, slice2 []T) bool {
	if len(slice1) != len(slice2) {
		return false
	}

	for i := range slice1 {
		if slice1[i] != slice2[i] {
			return false
		}
	}

	return true
}

// Sum 求和（整数）
func Sum[T integer](slice []T) T {
	var sum T
	for _, v := range slice {
		sum += v
	}
	return sum
}

// SumFloat 求和（浮点数）
func SumFloat[T floating](slice []T) T {
	var sum T
	for _, v := range slice {
		sum += v
	}
	return sum
}

// Max 获取最大值
func Max[T number](slice []T) T {
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

// Min 获取最小值
func Min[T number](slice []T) T {
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

// GroupBy 分组
func GroupBy[T any, K comparable](slice []T, keyFunc func(T) K) map[K][]T {
	groups := make(map[K][]T)
	for _, item := range slice {
		key := keyFunc(item)
		groups[key] = append(groups[key], item)
	}
	return groups
}

// CountBy 计数
func CountBy[T any, K comparable](slice []T, keyFunc func(T) K) map[K]int {
	counts := make(map[K]int)
	for _, item := range slice {
		key := keyFunc(item)
		counts[key]++
	}
	return counts
}

// Any 判断是否有元素满足条件
func Any[T any](slice []T, predicate func(T) bool) bool {
	for _, item := range slice {
		if predicate(item) {
			return true
		}
	}
	return false
}

// All 判断是否所有元素都满足条件
func All[T any](slice []T, predicate func(T) bool) bool {
	for _, item := range slice {
		if !predicate(item) {
			return false
		}
	}
	return true
}

// First 获取第一个元素
func First[T any](slice []T) (T, bool) {
	if len(slice) == 0 {
		var zero T
		return zero, false
	}
	return slice[0], true
}

// Last 获取最后一个元素
func Last[T any](slice []T) (T, bool) {
	if len(slice) == 0 {
		var zero T
		return zero, false
	}
	return slice[len(slice)-1], true
}

func clone[T any](slice []T) []T {
	if slice == nil {
		return nil
	}
	result := make([]T, len(slice))
	copy(result, slice)
	return result
}

func compact[T any](slice []T) []T {
	if cap(slice) == len(slice) {
		return slice
	}
	return clone(slice)
}
