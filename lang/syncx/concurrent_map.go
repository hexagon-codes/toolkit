package syncx

import (
	"sync"
)

// ConcurrentMap 泛型并发安全 Map
//
// 基于 sync.Map 封装，提供类型安全的泛型接口
type ConcurrentMap[K comparable, V any] struct {
	m  sync.Map
	mu sync.Mutex // 保护 Update 操作的原子性
}

// NewConcurrentMap 创建一个新的 ConcurrentMap
//
// 返回:
//   - *ConcurrentMap[K, V]: 新的并发安全 Map
//
// 示例:
//
//	m := syncx.NewConcurrentMap[string, int]()
//	m.Store("count", 1)
func NewConcurrentMap[K comparable, V any]() *ConcurrentMap[K, V] {
	return &ConcurrentMap[K, V]{}
}

// Load 获取指定键的值
//
// 参数:
//   - key: 键
//
// 返回:
//   - V: 值
//   - bool: 键是否存在
//
// 示例:
//
//	value, ok := m.Load("key")
func (m *ConcurrentMap[K, V]) Load(key K) (V, bool) {
	value, ok := m.m.Load(key)
	if !ok {
		var zero V
		return zero, false
	}
	// 安全类型断言，防止类型不匹配导致 panic
	if typed, ok := value.(V); ok {
		return typed, true
	}
	var zero V
	return zero, false
}

// Store 存储键值对
//
// 参数:
//   - key: 键
//   - value: 值
//
// 示例:
//
//	m.Store("key", "value")
func (m *ConcurrentMap[K, V]) Store(key K, value V) {
	m.m.Store(key, value)
}

// Delete 删除指定键
//
// 参数:
//   - key: 要删除的键
//
// 示例:
//
//	m.Delete("key")
func (m *ConcurrentMap[K, V]) Delete(key K) {
	m.m.Delete(key)
}

// LoadOrStore 获取值，如果不存在则存储
//
// 参数:
//   - key: 键
//   - value: 如果键不存在时要存储的值
//
// 返回:
//   - V: 实际的值（已存在的值或新存储的值）
//   - bool: 如果值是已存在的返回 true，新存储的返回 false
//
// 示例:
//
//	actual, loaded := m.LoadOrStore("key", "default")
func (m *ConcurrentMap[K, V]) LoadOrStore(key K, value V) (V, bool) {
	actual, loaded := m.m.LoadOrStore(key, value)
	if typed, ok := actual.(V); ok {
		return typed, loaded
	}
	return value, false
}

// LoadAndDelete 获取并删除指定键
//
// 参数:
//   - key: 键
//
// 返回:
//   - V: 值
//   - bool: 键是否存在
//
// 示例:
//
//	value, ok := m.LoadAndDelete("key")
func (m *ConcurrentMap[K, V]) LoadAndDelete(key K) (V, bool) {
	value, loaded := m.m.LoadAndDelete(key)
	if !loaded {
		var zero V
		return zero, false
	}
	if typed, ok := value.(V); ok {
		return typed, true
	}
	var zero V
	return zero, false
}

// CompareAndSwap 比较并交换
//
// 参数:
//   - key: 键
//   - old: 期望的旧值
//   - new: 新值
//
// 返回:
//   - bool: 如果交换成功返回 true
func (m *ConcurrentMap[K, V]) CompareAndSwap(key K, old, new V) bool {
	return m.m.CompareAndSwap(key, old, new)
}

// CompareAndDelete 比较并删除
//
// 参数:
//   - key: 键
//   - old: 期望的值
//
// 返回:
//   - bool: 如果删除成功返回 true
func (m *ConcurrentMap[K, V]) CompareAndDelete(key K, old V) bool {
	return m.m.CompareAndDelete(key, old)
}

// Swap 存储值并返回旧值
//
// 参数:
//   - key: 键
//   - value: 新值
//
// 返回:
//   - V: 旧值
//   - bool: 键是否存在
func (m *ConcurrentMap[K, V]) Swap(key K, value V) (V, bool) {
	previous, loaded := m.m.Swap(key, value)
	if !loaded {
		var zero V
		return zero, false
	}
	if typed, ok := previous.(V); ok {
		return typed, true
	}
	var zero V
	return zero, false
}

// Range 遍历所有键值对
//
// 参数:
//   - fn: 遍历函数，返回 false 停止遍历
//
// 示例:
//
//	m.Range(func(key string, value int) bool {
//	    fmt.Printf("%s: %d\n", key, value)
//	    return true  // 继续遍历
//	})
func (m *ConcurrentMap[K, V]) Range(fn func(K, V) bool) {
	m.m.Range(func(key, value any) bool {
		k, kOk := key.(K)
		v, vOk := value.(V)
		if !kOk || !vOk {
			return true // 跳过类型不匹配的条目
		}
		return fn(k, v)
	})
}

// Len 返回元素数量
//
// 注意: 这需要遍历整个 Map，在高并发场景下可能不够精确
//
// 返回:
//   - int: 元素数量
func (m *ConcurrentMap[K, V]) Len() int {
	count := 0
	m.m.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// IsEmpty 检查 Map 是否为空
//
// 返回:
//   - bool: 如果为空返回 true
func (m *ConcurrentMap[K, V]) IsEmpty() bool {
	empty := true
	m.m.Range(func(_, _ any) bool {
		empty = false
		return false // 找到一个就停止
	})
	return empty
}

// Keys 返回所有键
//
// 注意: 在高并发场景下，返回的键可能与当前状态不完全一致
//
// 返回:
//   - []K: 所有键的切片
func (m *ConcurrentMap[K, V]) Keys() []K {
	var keys []K
	m.m.Range(func(key, _ any) bool {
		if k, ok := key.(K); ok {
			keys = append(keys, k)
		}
		return true
	})
	return keys
}

// Values 返回所有值
//
// 注意: 在高并发场景下，返回的值可能与当前状态不完全一致
//
// 返回:
//   - []V: 所有值的切片
func (m *ConcurrentMap[K, V]) Values() []V {
	var values []V
	m.m.Range(func(_, value any) bool {
		if v, ok := value.(V); ok {
			values = append(values, v)
		}
		return true
	})
	return values
}

// Clear 清空 Map
//
// 示例:
//
//	m.Clear()
func (m *ConcurrentMap[K, V]) Clear() {
	m.m.Range(func(key, _ any) bool {
		m.m.Delete(key)
		return true
	})
}

// GetOrCompute 获取值，如果不存在则计算并存储
//
// 参数:
//   - key: 键
//   - compute: 计算函数
//
// 返回:
//   - V: 实际的值
//
// 注意: compute 函数可能被多次调用，但只有一个结果会被存储
//
// 示例:
//
//	value := m.GetOrCompute("key", func() int {
//	    return expensiveComputation()
//	})
func (m *ConcurrentMap[K, V]) GetOrCompute(key K, compute func() V) V {
	if value, ok := m.Load(key); ok {
		return value
	}
	newValue := compute()
	actual, _ := m.LoadOrStore(key, newValue)
	return actual
}

// Update 更新指定键的值（原子操作）
//
// 参数:
//   - key: 键
//   - fn: 更新函数
//
// 返回:
//   - bool: 键是否存在
//
// 注意: 使用 mutex 保护 Load+Store 操作的原子性，支持不可比较的值类型（如 slice/map/function）
//
// 示例:
//
//	m.Update("count", func(v int) int { return v + 1 })
func (m *ConcurrentMap[K, V]) Update(key K, fn func(V) V) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.Load(key)
	if !ok {
		return false
	}
	newValue := fn(value)
	m.Store(key, newValue)
	return true
}

// ForEach 遍历所有键值对（Range 的别名，始终遍历完全）
//
// 参数:
//   - fn: 遍历函数
//
// 示例:
//
//	m.ForEach(func(key string, value int) {
//	    fmt.Printf("%s: %d\n", key, value)
//	})
func (m *ConcurrentMap[K, V]) ForEach(fn func(K, V)) {
	m.m.Range(func(key, value any) bool {
		k, kOk := key.(K)
		v, vOk := value.(V)
		if kOk && vOk {
			fn(k, v)
		}
		return true
	})
}

// Has 检查键是否存在
//
// 参数:
//   - key: 键
//
// 返回:
//   - bool: 键是否存在
//
// 示例:
//
//	if m.Has("key") {
//	    // ...
//	}
func (m *ConcurrentMap[K, V]) Has(key K) bool {
	_, ok := m.m.Load(key)
	return ok
}

// Get 获取值（Load 的别名）
//
// 参数:
//   - key: 键
//
// 返回:
//   - V: 值
//   - bool: 键是否存在
func (m *ConcurrentMap[K, V]) Get(key K) (V, bool) {
	return m.Load(key)
}

// Set 存储值（Store 的别名）
//
// 参数:
//   - key: 键
//   - value: 值
func (m *ConcurrentMap[K, V]) Set(key K, value V) {
	m.Store(key, value)
}

// SetIfAbsent 如果键不存在则设置
//
// 参数:
//   - key: 键
//   - value: 值
//
// 返回:
//   - bool: 如果设置成功返回 true（键不存在）
func (m *ConcurrentMap[K, V]) SetIfAbsent(key K, value V) bool {
	_, loaded := m.LoadOrStore(key, value)
	return !loaded
}

// ToMap 转换为普通 map（非线程安全）
//
// 返回:
//   - map[K]V: 普通 map
//
// 注意: 在高并发场景下，返回的 map 可能与当前状态不完全一致
func (m *ConcurrentMap[K, V]) ToMap() map[K]V {
	result := make(map[K]V)
	m.m.Range(func(key, value any) bool {
		k, kOk := key.(K)
		v, vOk := value.(V)
		if kOk && vOk {
			result[k] = v
		}
		return true
	})
	return result
}
