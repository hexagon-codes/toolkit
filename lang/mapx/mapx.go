package mapx

// Keys 返回 map 的所有键
func Keys[K comparable, V any](m map[K]V) []K {
	if m == nil {
		return nil
	}
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Values 返回 map 的所有值
func Values[K comparable, V any](m map[K]V) []V {
	if m == nil {
		return nil
	}
	values := make([]V, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}

// Entries 返回 map 的所有键值对
func Entries[K comparable, V any](m map[K]V) []Entry[K, V] {
	if m == nil {
		return nil
	}
	entries := make([]Entry[K, V], 0, len(m))
	for k, v := range m {
		entries = append(entries, Entry[K, V]{Key: k, Value: v})
	}
	return entries
}

// Entry 键值对
type Entry[K comparable, V any] struct {
	Key   K
	Value V
}

// FromEntries 从键值对切片创建 map
func FromEntries[K comparable, V any](entries []Entry[K, V]) map[K]V {
	if entries == nil {
		return nil
	}
	m := make(map[K]V, len(entries))
	for _, e := range entries {
		m[e.Key] = e.Value
	}
	return m
}

// Filter 过滤 map
func Filter[K comparable, V any](m map[K]V, predicate func(K, V) bool) map[K]V {
	if m == nil {
		return nil
	}
	result := make(map[K]V)
	for k, v := range m {
		if predicate(k, v) {
			result[k] = v
		}
	}
	return result
}

// FilterKeys 根据键过滤 map
func FilterKeys[K comparable, V any](m map[K]V, predicate func(K) bool) map[K]V {
	if m == nil {
		return nil
	}
	result := make(map[K]V)
	for k, v := range m {
		if predicate(k) {
			result[k] = v
		}
	}
	return result
}

// FilterValues 根据值过滤 map
func FilterValues[K comparable, V any](m map[K]V, predicate func(V) bool) map[K]V {
	if m == nil {
		return nil
	}
	result := make(map[K]V)
	for k, v := range m {
		if predicate(v) {
			result[k] = v
		}
	}
	return result
}

// MapValues 转换 map 的值
func MapValues[K comparable, V any, R any](m map[K]V, transform func(V) R) map[K]R {
	if m == nil {
		return nil
	}
	result := make(map[K]R, len(m))
	for k, v := range m {
		result[k] = transform(v)
	}
	return result
}

// MapKeys 转换 map 的键
func MapKeys[K comparable, V any, R comparable](m map[K]V, transform func(K) R) map[R]V {
	if m == nil {
		return nil
	}
	result := make(map[R]V, len(m))
	for k, v := range m {
		result[transform(k)] = v
	}
	return result
}

// Merge 合并多个 map（后面的覆盖前面的）
func Merge[K comparable, V any](maps ...map[K]V) map[K]V {
	result := make(map[K]V)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

// MergeWith 使用自定义函数合并 map
func MergeWith[K comparable, V any](merge func(V, V) V, maps ...map[K]V) map[K]V {
	result := make(map[K]V)
	for _, m := range maps {
		for k, v := range m {
			if existing, ok := result[k]; ok {
				result[k] = merge(existing, v)
			} else {
				result[k] = v
			}
		}
	}
	return result
}

// Invert 反转 map 的键值
func Invert[K comparable, V comparable](m map[K]V) map[V]K {
	if m == nil {
		return nil
	}
	result := make(map[V]K, len(m))
	for k, v := range m {
		result[v] = k
	}
	return result
}

// Pick 选择指定的键
func Pick[K comparable, V any](m map[K]V, keys ...K) map[K]V {
	if m == nil {
		return nil
	}
	result := make(map[K]V)
	for _, k := range keys {
		if v, ok := m[k]; ok {
			result[k] = v
		}
	}
	return result
}

// Omit 排除指定的键
func Omit[K comparable, V any](m map[K]V, keys ...K) map[K]V {
	if m == nil {
		return nil
	}
	exclude := make(map[K]struct{}, len(keys))
	for _, k := range keys {
		exclude[k] = struct{}{}
	}
	result := make(map[K]V)
	for k, v := range m {
		if _, ok := exclude[k]; !ok {
			result[k] = v
		}
	}
	return result
}

// Contains 判断 map 是否包含指定键
func Contains[K comparable, V any](m map[K]V, key K) bool {
	_, ok := m[key]
	return ok
}

// ContainsAll 判断 map 是否包含所有指定键
func ContainsAll[K comparable, V any](m map[K]V, keys ...K) bool {
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}

// ContainsAny 判断 map 是否包含任意一个指定键
func ContainsAny[K comparable, V any](m map[K]V, keys ...K) bool {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

// GetOrDefault 获取值，不存在则返回默认值
func GetOrDefault[K comparable, V any](m map[K]V, key K, defaultVal V) V {
	if v, ok := m[key]; ok {
		return v
	}
	return defaultVal
}

// GetOrCompute 获取值，不存在则计算并存储
func GetOrCompute[K comparable, V any](m map[K]V, key K, compute func() V) V {
	if v, ok := m[key]; ok {
		return v
	}
	v := compute()
	m[key] = v
	return v
}

// Clone 浅拷贝 map
func Clone[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return nil
	}
	result := make(map[K]V, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// Equal 判断两个 map 是否相等（值需要是 comparable）
func Equal[K, V comparable](m1, m2 map[K]V) bool {
	if len(m1) != len(m2) {
		return false
	}
	for k, v1 := range m1 {
		if v2, ok := m2[k]; !ok || v1 != v2 {
			return false
		}
	}
	return true
}

// IsEmpty 判断 map 是否为空
func IsEmpty[K comparable, V any](m map[K]V) bool {
	return len(m) == 0
}

// ForEach 遍历 map
func ForEach[K comparable, V any](m map[K]V, fn func(K, V)) {
	for k, v := range m {
		fn(k, v)
	}
}

// Any 判断是否有任意元素满足条件
func Any[K comparable, V any](m map[K]V, predicate func(K, V) bool) bool {
	for k, v := range m {
		if predicate(k, v) {
			return true
		}
	}
	return false
}

// All 判断是否所有元素都满足条件
func All[K comparable, V any](m map[K]V, predicate func(K, V) bool) bool {
	for k, v := range m {
		if !predicate(k, v) {
			return false
		}
	}
	return true
}

// None 判断是否没有元素满足条件
func None[K comparable, V any](m map[K]V, predicate func(K, V) bool) bool {
	for k, v := range m {
		if predicate(k, v) {
			return false
		}
	}
	return true
}

// Count 统计满足条件的元素数量
func Count[K comparable, V any](m map[K]V, predicate func(K, V) bool) int {
	count := 0
	for k, v := range m {
		if predicate(k, v) {
			count++
		}
	}
	return count
}
