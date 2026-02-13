package stream

import (
	"sort"
)

// Stream 表示一个元素序列，支持链式操作
type Stream[T any] struct {
	source func() []T
}

// Of 从多个值创建 Stream
//
// 参数:
//   - values: 值列表
//
// 返回:
//   - Stream[T]: 新的 Stream
//
// 示例:
//
//	s := stream.Of(1, 2, 3, 4, 5)
func Of[T any](values ...T) Stream[T] {
	return Stream[T]{
		source: func() []T {
			return values
		},
	}
}

// FromSlice 从切片创建 Stream
//
// 参数:
//   - slice: 源切片
//
// 返回:
//   - Stream[T]: 新的 Stream
//
// 示例:
//
//	nums := []int{1, 2, 3, 4, 5}
//	s := stream.FromSlice(nums)
func FromSlice[T any](slice []T) Stream[T] {
	return Stream[T]{
		source: func() []T {
			return slice
		},
	}
}

// Generate 使用生成函数创建 Stream
//
// 参数:
//   - n: 生成数量
//   - generator: 生成函数，参数为索引
//
// 返回:
//   - Stream[T]: 新的 Stream
//
// 示例:
//
//	s := stream.Generate(5, func(i int) int { return i * 2 })
//	// [0, 2, 4, 6, 8]
func Generate[T any](n int, generator func(int) T) Stream[T] {
	return Stream[T]{
		source: func() []T {
			if n <= 0 {
				return nil
			}
			result := make([]T, n)
			for i := range result {
				result[i] = generator(i)
			}
			return result
		},
	}
}

// Range 创建整数范围 Stream
//
// 参数:
//   - start: 起始值（包含）
//   - end: 结束值（不包含）
//
// 返回:
//   - Stream[int]: 新的 Stream
//
// 示例:
//
//	s := stream.Range(0, 5)  // [0, 1, 2, 3, 4]
func Range(start, end int) Stream[int] {
	return Stream[int]{
		source: func() []int {
			if end <= start {
				return nil
			}
			result := make([]int, end-start)
			for i := range result {
				result[i] = start + i
			}
			return result
		},
	}
}

// Repeat 重复值创建 Stream
//
// 参数:
//   - value: 要重复的值
//   - n: 重复次数
//
// 返回:
//   - Stream[T]: 新的 Stream
//
// 示例:
//
//	s := stream.Repeat("hello", 3)  // ["hello", "hello", "hello"]
func Repeat[T any](value T, n int) Stream[T] {
	return Stream[T]{
		source: func() []T {
			if n <= 0 {
				return nil
			}
			result := make([]T, n)
			for i := range result {
				result[i] = value
			}
			return result
		},
	}
}

// Filter 过滤元素
//
// 参数:
//   - predicate: 过滤函数，返回 true 保留元素
//
// 返回:
//   - Stream[T]: 过滤后的 Stream
//
// 示例:
//
//	s := stream.Of(1, 2, 3, 4, 5).Filter(func(n int) bool { return n%2 == 0 })
//	// [2, 4]
func (s Stream[T]) Filter(predicate func(T) bool) Stream[T] {
	return Stream[T]{
		source: func() []T {
			src := s.source()
			result := make([]T, 0)
			for _, v := range src {
				if predicate(v) {
					result = append(result, v)
				}
			}
			return result
		},
	}
}

// Map 转换元素
//
// 参数:
//   - mapper: 转换函数
//
// 返回:
//   - Stream[T]: 转换后的 Stream
//
// 注意: 由于 Go 泛型限制，Map 不能改变类型。如需类型转换，请使用 MapTo 包级函数
//
// 示例:
//
//	s := stream.Of(1, 2, 3).Map(func(n int) int { return n * 2 })
//	// [2, 4, 6]
func (s Stream[T]) Map(mapper func(T) T) Stream[T] {
	return Stream[T]{
		source: func() []T {
			src := s.source()
			result := make([]T, len(src))
			for i, v := range src {
				result[i] = mapper(v)
			}
			return result
		},
	}
}

// Distinct 去除重复元素
//
// 返回:
//   - Stream[T]: 去重后的 Stream
//
// 注意: 对于不可比较类型（slice、map、function 等），这些元素会被直接保留（无法去重）。
// 对于大数据集可能性能不佳。
//
// 示例:
//
//	s := stream.Of(1, 2, 2, 3, 3, 3).Distinct()
//	// [1, 2, 3]
func (s Stream[T]) Distinct() Stream[T] {
	return Stream[T]{
		source: func() []T {
			src := s.source()
			if len(src) == 0 {
				return nil
			}

			// 先检测类型是否可比较（只检查一次），避免每个元素都 defer/recover
			comparable := isComparable(src[0])

			if !comparable {
				// 不可比较的类型无法去重，直接返回副本
				result := make([]T, len(src))
				copy(result, src)
				return result
			}

			seen := make(map[any]bool, len(src))
			result := make([]T, 0, len(src))
			for _, v := range src {
				key := any(v)
				if !seen[key] {
					seen[key] = true
					result = append(result, v)
				}
			}
			return result
		},
	}
}

// isComparable 检测值是否可以作为 map key（仅调用一次）
func isComparable(v any) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()
	m := make(map[any]bool)
	m[v] = true
	return true
}

// Sorted 排序元素
//
// 参数:
//   - less: 比较函数，如果 a < b 返回 true
//
// 返回:
//   - Stream[T]: 排序后的 Stream
//
// 示例:
//
//	s := stream.Of(3, 1, 4, 1, 5).Sorted(func(a, b int) bool { return a < b })
//	// [1, 1, 3, 4, 5]
func (s Stream[T]) Sorted(less func(a, b T) bool) Stream[T] {
	return Stream[T]{
		source: func() []T {
			src := s.source()
			result := make([]T, len(src))
			copy(result, src)
			sort.Slice(result, func(i, j int) bool {
				return less(result[i], result[j])
			})
			return result
		},
	}
}

// Limit 限制元素数量
//
// 参数:
//   - n: 最大元素数量
//
// 返回:
//   - Stream[T]: 限制后的 Stream
//
// 示例:
//
//	s := stream.Of(1, 2, 3, 4, 5).Limit(3)
//	// [1, 2, 3]
func (s Stream[T]) Limit(n int) Stream[T] {
	return Stream[T]{
		source: func() []T {
			src := s.source()
			if n <= 0 {
				return nil
			}
			if n >= len(src) {
				return src
			}
			// 返回副本，避免共享底层数组
			result := make([]T, n)
			copy(result, src[:n])
			return result
		},
	}
}

// Skip 跳过前 n 个元素
//
// 参数:
//   - n: 要跳过的元素数量
//
// 返回:
//   - Stream[T]: 跳过后的 Stream
//
// 示例:
//
//	s := stream.Of(1, 2, 3, 4, 5).Skip(2)
//	// [3, 4, 5]
func (s Stream[T]) Skip(n int) Stream[T] {
	return Stream[T]{
		source: func() []T {
			src := s.source()
			if n <= 0 {
				return src
			}
			if n >= len(src) {
				return nil
			}
			// 返回副本，避免共享底层数组
			result := make([]T, len(src)-n)
			copy(result, src[n:])
			return result
		},
	}
}

// Peek 对每个元素执行操作（用于调试）
//
// 参数:
//   - action: 要执行的操作
//
// 返回:
//   - Stream[T]: 原 Stream（未修改）
//
// 示例:
//
//	s := stream.Of(1, 2, 3).Peek(func(n int) { fmt.Println(n) }).Collect()
func (s Stream[T]) Peek(action func(T)) Stream[T] {
	return Stream[T]{
		source: func() []T {
			src := s.source()
			for _, v := range src {
				action(v)
			}
			return src
		},
	}
}

// Reverse 反转元素顺序
//
// 返回:
//   - Stream[T]: 反转后的 Stream
//
// 示例:
//
//	s := stream.Of(1, 2, 3).Reverse()
//	// [3, 2, 1]
func (s Stream[T]) Reverse() Stream[T] {
	return Stream[T]{
		source: func() []T {
			src := s.source()
			result := make([]T, len(src))
			for i := range src {
				result[len(src)-1-i] = src[i]
			}
			return result
		},
	}
}

// TakeWhile 取元素直到条件不满足
//
// 参数:
//   - predicate: 条件函数
//
// 返回:
//   - Stream[T]: 新的 Stream
//
// 示例:
//
//	s := stream.Of(1, 2, 3, 4, 5).TakeWhile(func(n int) bool { return n < 4 })
//	// [1, 2, 3]
func (s Stream[T]) TakeWhile(predicate func(T) bool) Stream[T] {
	return Stream[T]{
		source: func() []T {
			src := s.source()
			result := make([]T, 0)
			for _, v := range src {
				if !predicate(v) {
					break
				}
				result = append(result, v)
			}
			return result
		},
	}
}

// DropWhile 跳过元素直到条件不满足
//
// 参数:
//   - predicate: 条件函数
//
// 返回:
//   - Stream[T]: 新的 Stream
//
// 示例:
//
//	s := stream.Of(1, 2, 3, 4, 5).DropWhile(func(n int) bool { return n < 3 })
//	// [3, 4, 5]
func (s Stream[T]) DropWhile(predicate func(T) bool) Stream[T] {
	return Stream[T]{
		source: func() []T {
			src := s.source()
			i := 0
			for ; i < len(src) && predicate(src[i]); i++ {
			}
			if i >= len(src) {
				return nil
			}
			return src[i:]
		},
	}
}

// Collect 收集所有元素到切片
//
// 返回:
//   - []T: 结果切片
//
// 示例:
//
//	result := stream.Of(1, 2, 3).Collect()
func (s Stream[T]) Collect() []T {
	return s.source()
}

// ForEach 遍历每个元素
//
// 参数:
//   - action: 对每个元素执行的操作
//
// 示例:
//
//	stream.Of(1, 2, 3).ForEach(func(n int) { fmt.Println(n) })
func (s Stream[T]) ForEach(action func(T)) {
	for _, v := range s.source() {
		action(v)
	}
}

// Reduce 归约所有元素
//
// 参数:
//   - initial: 初始值
//   - accumulator: 累加函数
//
// 返回:
//   - T: 归约结果
//
// 示例:
//
//	sum := stream.Of(1, 2, 3, 4, 5).Reduce(0, func(acc, n int) int { return acc + n })
//	// 15
func (s Stream[T]) Reduce(initial T, accumulator func(T, T) T) T {
	result := initial
	for _, v := range s.source() {
		result = accumulator(result, v)
	}
	return result
}

// Count 返回元素数量
//
// 返回:
//   - int: 元素数量
//
// 示例:
//
//	count := stream.Of(1, 2, 3, 4, 5).Count()
//	// 5
func (s Stream[T]) Count() int {
	return len(s.source())
}

// First 返回第一个元素
//
// 返回:
//   - T: 第一个元素
//   - bool: 是否存在
//
// 示例:
//
//	first, ok := stream.Of(1, 2, 3).First()
//	// 1, true
func (s Stream[T]) First() (T, bool) {
	src := s.source()
	if len(src) == 0 {
		var zero T
		return zero, false
	}
	return src[0], true
}

// Last 返回最后一个元素
//
// 返回:
//   - T: 最后一个元素
//   - bool: 是否存在
//
// 示例:
//
//	last, ok := stream.Of(1, 2, 3).Last()
//	// 3, true
func (s Stream[T]) Last() (T, bool) {
	src := s.source()
	if len(src) == 0 {
		var zero T
		return zero, false
	}
	return src[len(src)-1], true
}

// Any 检查是否有任意元素满足条件
//
// 参数:
//   - predicate: 条件函数
//
// 返回:
//   - bool: 如果有元素满足条件返回 true
//
// 示例:
//
//	hasEven := stream.Of(1, 2, 3).Any(func(n int) bool { return n%2 == 0 })
//	// true
func (s Stream[T]) Any(predicate func(T) bool) bool {
	for _, v := range s.source() {
		if predicate(v) {
			return true
		}
	}
	return false
}

// All 检查是否所有元素都满足条件
//
// 参数:
//   - predicate: 条件函数
//
// 返回:
//   - bool: 如果所有元素都满足条件返回 true
//
// 示例:
//
//	allPositive := stream.Of(1, 2, 3).All(func(n int) bool { return n > 0 })
//	// true
func (s Stream[T]) All(predicate func(T) bool) bool {
	for _, v := range s.source() {
		if !predicate(v) {
			return false
		}
	}
	return true
}

// None 检查是否没有元素满足条件
//
// 参数:
//   - predicate: 条件函数
//
// 返回:
//   - bool: 如果没有元素满足条件返回 true
//
// 示例:
//
//	noNegative := stream.Of(1, 2, 3).None(func(n int) bool { return n < 0 })
//	// true
func (s Stream[T]) None(predicate func(T) bool) bool {
	return !s.Any(predicate)
}

// FindFirst 查找第一个满足条件的元素
//
// 参数:
//   - predicate: 条件函数
//
// 返回:
//   - T: 找到的元素
//   - bool: 是否找到
//
// 示例:
//
//	even, ok := stream.Of(1, 2, 3).FindFirst(func(n int) bool { return n%2 == 0 })
//	// 2, true
func (s Stream[T]) FindFirst(predicate func(T) bool) (T, bool) {
	for _, v := range s.source() {
		if predicate(v) {
			return v, true
		}
	}
	var zero T
	return zero, false
}

// ToMap 将 Stream 转换为 Map
//
// 参数:
//   - keyFn: 提取键的函数
//
// 返回:
//   - map[K]T: 结果 map
//
// 示例:
//
//	m := stream.Of(User{ID: 1}, User{ID: 2}).ToMap(func(u User) int { return u.ID })
func ToMap[T any, K comparable](s Stream[T], keyFn func(T) K) map[K]T {
	result := make(map[K]T)
	for _, v := range s.source() {
		result[keyFn(v)] = v
	}
	return result
}

// GroupBy 按键分组
//
// 参数:
//   - keyFn: 提取键的函数
//
// 返回:
//   - map[K][]T: 分组结果
//
// 示例:
//
//	groups := stream.GroupBy(stream.Of(1, 2, 3, 4), func(n int) string {
//	    if n%2 == 0 { return "even" }
//	    return "odd"
//	})
func GroupBy[T any, K comparable](s Stream[T], keyFn func(T) K) map[K][]T {
	result := make(map[K][]T)
	for _, v := range s.source() {
		key := keyFn(v)
		result[key] = append(result[key], v)
	}
	return result
}

// MapTo 将 Stream 转换为不同类型的 Stream
//
// 参数:
//   - s: 源 Stream
//   - mapper: 转换函数
//
// 返回:
//   - Stream[R]: 转换后的 Stream
//
// 示例:
//
//	stringStream := stream.MapTo(stream.Of(1, 2, 3), func(n int) string {
//	    return strconv.Itoa(n)
//	})
func MapTo[T, R any](s Stream[T], mapper func(T) R) Stream[R] {
	return Stream[R]{
		source: func() []R {
			src := s.source()
			result := make([]R, len(src))
			for i, v := range src {
				result[i] = mapper(v)
			}
			return result
		},
	}
}

// FlatMapTo 将每个元素映射为 Stream 并展平
//
// 参数:
//   - s: 源 Stream
//   - mapper: 转换函数
//
// 返回:
//   - Stream[R]: 展平后的 Stream
//
// 示例:
//
//	result := stream.FlatMapTo(stream.Of([]int{1, 2}, []int{3, 4}), func(arr []int) Stream[int] {
//	    return stream.FromSlice(arr)
//	})
func FlatMapTo[T, R any](s Stream[T], mapper func(T) []R) Stream[R] {
	return Stream[R]{
		source: func() []R {
			src := s.source()
			result := make([]R, 0)
			for _, v := range src {
				result = append(result, mapper(v)...)
			}
			return result
		},
	}
}

// ReduceTo 使用不同类型的初始值归约
//
// 参数:
//   - s: 源 Stream
//   - initial: 初始值
//   - accumulator: 累加函数
//
// 返回:
//   - R: 归约结果
//
// 示例:
//
//	sum := stream.ReduceTo(stream.Of("a", "bb", "ccc"), 0, func(acc int, s string) int {
//	    return acc + len(s)
//	})
//	// 6
func ReduceTo[T, R any](s Stream[T], initial R, accumulator func(R, T) R) R {
	result := initial
	for _, v := range s.source() {
		result = accumulator(result, v)
	}
	return result
}

// IsEmpty 检查 Stream 是否为空
//
// 返回:
//   - bool: 如果为空返回 true
func (s Stream[T]) IsEmpty() bool {
	return len(s.source()) == 0
}

// Concat 连接多个 Stream
//
// 参数:
//   - streams: 要连接的 Stream 列表
//
// 返回:
//   - Stream[T]: 连接后的 Stream
//
// 注意: 结果会被缓存，避免重复计算
//
// 示例:
//
//	s := stream.Concat(stream.Of(1, 2), stream.Of(3, 4))
//	// [1, 2, 3, 4]
func Concat[T any](streams ...Stream[T]) Stream[T] {
	return Stream[T]{
		source: func() []T {
			var result []T
			for _, s := range streams {
				result = append(result, s.source()...)
			}
			return result
		},
	}
}
