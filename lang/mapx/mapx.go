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
	for _, entry := range Entries(m) {
		if predicate(entry.Key, entry.Value) {
			result[entry.Key] = entry.Value
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
	for _, entry := range Entries(m) {
		if predicate(entry.Key) {
			result[entry.Key] = entry.Value
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
	for _, entry := range Entries(m) {
		if predicate(entry.Value) {
			result[entry.Key] = entry.Value
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
	for _, entry := range Entries(m) {
		result[entry.Key] = transform(entry.Value)
	}
	return result
}

// MapKeys 转换 map 的键
func MapKeys[K comparable, V any, R comparable](m map[K]V, transform func(K) R) map[R]V {
	if m == nil {
		return nil
	}
	result := make(map[R]V, len(m))
	for _, entry := range Entries(m) {
		result[transform(entry.Key)] = entry.Value
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
		for _, entry := range Entries(m) {
			if existing, ok := result[entry.Key]; ok {
				result[entry.Key] = merge(existing, entry.Value)
			} else {
				result[entry.Key] = entry.Value
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
//
// 注意: 此函数不是线程安全的。如果需要并发安全，请使用 syncx.ConcurrentMap
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
	for _, entry := range Entries(m) {
		fn(entry.Key, entry.Value)
	}
}

// Any 判断是否有任意元素满足条件
func Any[K comparable, V any](m map[K]V, predicate func(K, V) bool) bool {
	for _, entry := range Entries(m) {
		if predicate(entry.Key, entry.Value) {
			return true
		}
	}
	return false
}

// All 判断是否所有元素都满足条件
func All[K comparable, V any](m map[K]V, predicate func(K, V) bool) bool {
	for _, entry := range Entries(m) {
		if !predicate(entry.Key, entry.Value) {
			return false
		}
	}
	return true
}

// None 判断是否没有元素满足条件
func None[K comparable, V any](m map[K]V, predicate func(K, V) bool) bool {
	for _, entry := range Entries(m) {
		if predicate(entry.Key, entry.Value) {
			return false
		}
	}
	return true
}

// Count 统计满足条件的元素数量
func Count[K comparable, V any](m map[K]V, predicate func(K, V) bool) int {
	count := 0
	for _, entry := range Entries(m) {
		if predicate(entry.Key, entry.Value) {
			count++
		}
	}
	return count
}

// Diff 返回 m1 中有但 m2 中没有的键值对
//
// 参数:
//   - m1: 第一个 map
//   - m2: 第二个 map
//
// 返回:
//   - map[K]V: 差集（仅比较键，不比较值）
//
// 示例:
//
//	m1 := map[string]int{"a": 1, "b": 2, "c": 3}
//	m2 := map[string]int{"b": 2, "c": 4}
//	diff := mapx.Diff(m1, m2)  // map[string]int{"a": 1}
func Diff[K comparable, V any](m1, m2 map[K]V) map[K]V {
	if m1 == nil {
		return nil
	}
	result := make(map[K]V)
	for k, v := range m1 {
		if _, ok := m2[k]; !ok {
			result[k] = v
		}
	}
	return result
}

// DiffValues 返回两个 map 中值不同的键值对（使用 m1 的值）
//
// 参数:
//   - m1: 第一个 map
//   - m2: 第二个 map
//
// 返回:
//   - map[K]V: m1 中与 m2 值不同的键值对
//
// 示例:
//
//	m1 := map[string]int{"a": 1, "b": 2}
//	m2 := map[string]int{"a": 1, "b": 3}
//	diff := mapx.DiffValues(m1, m2)  // map[string]int{"b": 2}
func DiffValues[K, V comparable](m1, m2 map[K]V) map[K]V {
	if m1 == nil {
		return nil
	}
	result := make(map[K]V)
	for k, v1 := range m1 {
		if v2, ok := m2[k]; !ok || v1 != v2 {
			result[k] = v1
		}
	}
	return result
}

// Intersection 返回两个 map 共有的键（使用 m1 的值）
//
// 参数:
//   - m1: 第一个 map
//   - m2: 第二个 map
//
// 返回:
//   - map[K]V: 交集
//
// 示例:
//
//	m1 := map[string]int{"a": 1, "b": 2, "c": 3}
//	m2 := map[string]int{"b": 20, "c": 30, "d": 40}
//	inter := mapx.Intersection(m1, m2)  // map[string]int{"b": 2, "c": 3}
func Intersection[K comparable, V any](m1, m2 map[K]V) map[K]V {
	if m1 == nil || m2 == nil {
		return nil
	}
	result := make(map[K]V)
	for k, v := range m1 {
		if _, ok := m2[k]; ok {
			result[k] = v
		}
	}
	return result
}

// IntersectionValues 返回两个 map 中键和值都相同的元素
//
// 参数:
//   - m1: 第一个 map
//   - m2: 第二个 map
//
// 返回:
//   - map[K]V: 键值都相同的交集
//
// 示例:
//
//	m1 := map[string]int{"a": 1, "b": 2}
//	m2 := map[string]int{"a": 1, "b": 3}
//	inter := mapx.IntersectionValues(m1, m2)  // map[string]int{"a": 1}
func IntersectionValues[K, V comparable](m1, m2 map[K]V) map[K]V {
	if m1 == nil || m2 == nil {
		return nil
	}
	result := make(map[K]V)
	for k, v := range m1 {
		if v2, ok := m2[k]; ok && v == v2 {
			result[k] = v
		}
	}
	return result
}

// Transform 同时转换 map 的键和值
//
// 参数:
//   - m: 输入 map
//   - fn: 转换函数，接收原 key 和 value，返回新 key 和 value
//
// 返回:
//   - map[R]S: 转换后的 map
//
// 示例:
//
//	m := map[int]string{1: "one", 2: "two"}
//	result := mapx.Transform(m, func(k int, v string) (string, int) {
//	    return v, k
//	})
//	// map[string]int{"one": 1, "two": 2}
func Transform[K comparable, V any, R comparable, S any](m map[K]V, fn func(K, V) (R, S)) map[R]S {
	if m == nil {
		return nil
	}
	result := make(map[R]S, len(m))
	for _, entry := range Entries(m) {
		newK, newV := fn(entry.Key, entry.Value)
		result[newK] = newV
	}
	return result
}

// Pop 移除并返回指定键的值
//
// 参数:
//   - m: 输入 map
//   - key: 要移除的键
//
// 返回:
//   - V: 被移除的值
//   - bool: 键是否存在
//
// 示例:
//
//	m := map[string]int{"a": 1, "b": 2}
//	v, ok := mapx.Pop(m, "a")  // v=1, ok=true, m={"b": 2}
func Pop[K comparable, V any](m map[K]V, key K) (V, bool) {
	v, ok := m[key]
	if ok {
		delete(m, key)
	}
	return v, ok
}

// PopOr 移除并返回指定键的值，不存在则返回默认值
//
// 参数:
//   - m: 输入 map
//   - key: 要移除的键
//   - defaultVal: 默认值
//
// 返回:
//   - V: 被移除的值或默认值
//
// 示例:
//
//	m := map[string]int{"a": 1}
//	v := mapx.PopOr(m, "b", 0)  // v=0, m 不变
func PopOr[K comparable, V any](m map[K]V, key K, defaultVal V) V {
	v, ok := Pop(m, key)
	if !ok {
		return defaultVal
	}
	return v
}

// Update 使用函数更新指定键的值
//
// 参数:
//   - m: 输入 map
//   - key: 要更新的键
//   - fn: 更新函数，接收旧值返回新值
//
// 返回:
//   - bool: 键是否存在
//
// 示例:
//
//	m := map[string]int{"count": 5}
//	mapx.Update(m, "count", func(v int) int { return v + 1 })
//	// m["count"] = 6
func Update[K comparable, V any](m map[K]V, key K, fn func(V) V) bool {
	v, ok := m[key]
	if ok {
		m[key] = fn(v)
	}
	return ok
}

// UpdateOrInsert 使用函数更新值，如果键不存在则插入默认值
//
// 参数:
//   - m: 输入 map
//   - key: 要更新的键
//   - defaultVal: 键不存在时的默认值
//   - fn: 更新函数
//
// 返回:
//   - V: 更新后的值
//
// 示例:
//
//	m := map[string]int{}
//	v := mapx.UpdateOrInsert(m, "count", 0, func(v int) int { return v + 1 })
//	// v=1, m["count"]=1
func UpdateOrInsert[K comparable, V any](m map[K]V, key K, defaultVal V, fn func(V) V) V {
	v, ok := m[key]
	if !ok {
		v = defaultVal
	}
	newV := fn(v)
	m[key] = newV
	return newV
}

// SymmetricDiff 返回两个 map 的对称差集（只在一个 map 中存在的键）
//
// 参数:
//   - m1: 第一个 map
//   - m2: 第二个 map
//
// 返回:
//   - map[K]V: 对称差集
//
// 示例:
//
//	m1 := map[string]int{"a": 1, "b": 2}
//	m2 := map[string]int{"b": 2, "c": 3}
//	diff := mapx.SymmetricDiff(m1, m2)  // {"a": 1, "c": 3}
func SymmetricDiff[K comparable, V any](m1, m2 map[K]V) map[K]V {
	result := make(map[K]V)
	for k, v := range m1 {
		if _, ok := m2[k]; !ok {
			result[k] = v
		}
	}
	for k, v := range m2 {
		if _, ok := m1[k]; !ok {
			result[k] = v
		}
	}
	return result
}

// Collect 从切片创建 map
//
// 参数:
//   - slice: 输入切片
//   - fn: 提取键值对的函数
//
// 返回:
//   - map[K]V: 收集后的 map
//
// 示例:
//
//	users := []User{{ID: 1, Name: "Alice"}, {ID: 2, Name: "Bob"}}
//	byID := mapx.Collect(users, func(u User) (int, string) {
//	    return u.ID, u.Name
//	})
//	// map[int]string{1: "Alice", 2: "Bob"}
func Collect[T any, K comparable, V any](slice []T, fn func(T) (K, V)) map[K]V {
	if slice == nil {
		return nil
	}
	result := make(map[K]V, len(slice))
	for _, item := range append([]T(nil), slice...) {
		k, v := fn(item)
		result[k] = v
	}
	return result
}

// CollectByKey 根据键函数从切片创建 map（值为元素本身）
//
// 参数:
//   - slice: 输入切片
//   - keyFn: 提取键的函数
//
// 返回:
//   - map[K]T: 收集后的 map
//
// 示例:
//
//	users := []User{{ID: 1, Name: "Alice"}, {ID: 2, Name: "Bob"}}
//	byID := mapx.CollectByKey(users, func(u User) int { return u.ID })
func CollectByKey[T any, K comparable](slice []T, keyFn func(T) K) map[K]T {
	if slice == nil {
		return nil
	}
	result := make(map[K]T, len(slice))
	for _, item := range append([]T(nil), slice...) {
		result[keyFn(item)] = item
	}
	return result
}
