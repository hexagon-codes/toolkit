package set

import (
	"fmt"
	"strings"
	"sync"
	"unsafe"
)

// Set 泛型 HashSet 实现
type Set[T comparable] struct {
	m map[T]struct{}
}

// New 创建新的 Set
func New[T comparable](items ...T) *Set[T] {
	s := &Set[T]{
		m: make(map[T]struct{}, len(items)),
	}
	for _, item := range items {
		s.m[item] = struct{}{}
	}
	return s
}

// NewWithSize 创建指定初始容量的 Set
func NewWithSize[T comparable](size int) *Set[T] {
	return &Set[T]{
		m: make(map[T]struct{}, size),
	}
}

// FromSlice 从切片创建 Set
func FromSlice[T comparable](items []T) *Set[T] {
	return New(items...)
}

// Add 添加元素
func (s *Set[T]) Add(items ...T) *Set[T] {
	for _, item := range items {
		s.m[item] = struct{}{}
	}
	return s
}

// Remove 移除元素
func (s *Set[T]) Remove(items ...T) *Set[T] {
	for _, item := range items {
		delete(s.m, item)
	}
	return s
}

// Contains 判断是否包含元素
func (s *Set[T]) Contains(item T) bool {
	_, ok := s.m[item]
	return ok
}

// ContainsAll 判断是否包含所有元素
func (s *Set[T]) ContainsAll(items ...T) bool {
	for _, item := range items {
		if _, ok := s.m[item]; !ok {
			return false
		}
	}
	return true
}

// ContainsAny 判断是否包含任意一个元素
func (s *Set[T]) ContainsAny(items ...T) bool {
	for _, item := range items {
		if _, ok := s.m[item]; ok {
			return true
		}
	}
	return false
}

// Size 返回元素数量
func (s *Set[T]) Size() int {
	return len(s.m)
}

// Len 返回元素数量（Size 的别名）
func (s *Set[T]) Len() int {
	return len(s.m)
}

// IsEmpty 判断是否为空
func (s *Set[T]) IsEmpty() bool {
	return len(s.m) == 0
}

// Clear 清空所有元素
func (s *Set[T]) Clear() {
	s.m = make(map[T]struct{})
}

// ToSlice 转换为切片
func (s *Set[T]) ToSlice() []T {
	result := make([]T, 0, len(s.m))
	for item := range s.m {
		result = append(result, item)
	}
	return result
}

// Values 返回所有元素（ToSlice 的别名）
func (s *Set[T]) Values() []T {
	return s.ToSlice()
}

// Clone 克隆 Set
func (s *Set[T]) Clone() *Set[T] {
	newSet := NewWithSize[T](len(s.m))
	for item := range s.m {
		newSet.m[item] = struct{}{}
	}
	return newSet
}

// --- 集合运算 ---

// Union 并集
func (s *Set[T]) Union(other *Set[T]) *Set[T] {
	result := s.Clone()
	for item := range other.m {
		result.m[item] = struct{}{}
	}
	return result
}

// Intersection 交集
func (s *Set[T]) Intersection(other *Set[T]) *Set[T] {
	result := NewWithSize[T](min(len(s.m), len(other.m)))

	// 遍历较小的集合
	smaller, larger := s, other
	if len(s.m) > len(other.m) {
		smaller, larger = other, s
	}

	for item := range smaller.m {
		if _, ok := larger.m[item]; ok {
			result.m[item] = struct{}{}
		}
	}
	return result
}

// Difference 差集（s - other）
func (s *Set[T]) Difference(other *Set[T]) *Set[T] {
	result := NewWithSize[T](len(s.m))
	for item := range s.m {
		if _, ok := other.m[item]; !ok {
			result.m[item] = struct{}{}
		}
	}
	return result
}

// SymmetricDifference 对称差集
func (s *Set[T]) SymmetricDifference(other *Set[T]) *Set[T] {
	result := NewWithSize[T](len(s.m) + len(other.m))

	for item := range s.m {
		if _, ok := other.m[item]; !ok {
			result.m[item] = struct{}{}
		}
	}

	for item := range other.m {
		if _, ok := s.m[item]; !ok {
			result.m[item] = struct{}{}
		}
	}

	return result
}

// IsSubset 判断是否为子集
func (s *Set[T]) IsSubset(other *Set[T]) bool {
	if len(s.m) > len(other.m) {
		return false
	}
	for item := range s.m {
		if _, ok := other.m[item]; !ok {
			return false
		}
	}
	return true
}

// IsSuperset 判断是否为超集
func (s *Set[T]) IsSuperset(other *Set[T]) bool {
	return other.IsSubset(s)
}

// IsDisjoint 判断是否无交集
func (s *Set[T]) IsDisjoint(other *Set[T]) bool {
	smaller, larger := s, other
	if len(s.m) > len(other.m) {
		smaller, larger = other, s
	}

	for item := range smaller.m {
		if _, ok := larger.m[item]; ok {
			return false
		}
	}
	return true
}

// Equal 判断两个 Set 是否相等
func (s *Set[T]) Equal(other *Set[T]) bool {
	if len(s.m) != len(other.m) {
		return false
	}
	for item := range s.m {
		if _, ok := other.m[item]; !ok {
			return false
		}
	}
	return true
}

// --- 遍历 ---

// ForEach 遍历所有元素
func (s *Set[T]) ForEach(fn func(T)) {
	for item := range s.m {
		fn(item)
	}
}

// Filter 过滤元素
func (s *Set[T]) Filter(predicate func(T) bool) *Set[T] {
	result := NewWithSize[T](len(s.m))
	for item := range s.m {
		if predicate(item) {
			result.m[item] = struct{}{}
		}
	}
	return result
}

// Any 判断是否存在满足条件的元素
func (s *Set[T]) Any(predicate func(T) bool) bool {
	for item := range s.m {
		if predicate(item) {
			return true
		}
	}
	return false
}

// All 判断是否所有元素都满足条件
func (s *Set[T]) All(predicate func(T) bool) bool {
	for item := range s.m {
		if !predicate(item) {
			return false
		}
	}
	return true
}

// None 判断是否没有元素满足条件
func (s *Set[T]) None(predicate func(T) bool) bool {
	for item := range s.m {
		if predicate(item) {
			return false
		}
	}
	return true
}

// Count 统计满足条件的元素数量
func (s *Set[T]) Count(predicate func(T) bool) int {
	count := 0
	for item := range s.m {
		if predicate(item) {
			count++
		}
	}
	return count
}

// Pop 随机移除并返回一个元素
func (s *Set[T]) Pop() (T, bool) {
	for item := range s.m {
		delete(s.m, item)
		return item, true
	}
	var zero T
	return zero, false
}

// String 返回字符串表示
func (s *Set[T]) String() string {
	items := s.ToSlice()
	strs := make([]string, len(items))
	for i, item := range items {
		strs[i] = fmt.Sprintf("%v", item)
	}
	return "Set{" + strings.Join(strs, ", ") + "}"
}

// --- 包级别函数 ---

// Union 合并多个 Set
func Union[T comparable](sets ...*Set[T]) *Set[T] {
	if len(sets) == 0 {
		return New[T]()
	}

	totalSize := 0
	for _, set := range sets {
		totalSize += set.Size()
	}

	result := NewWithSize[T](totalSize)
	for _, set := range sets {
		for item := range set.m {
			result.m[item] = struct{}{}
		}
	}
	return result
}

// Intersection 求多个 Set 的交集
func Intersection[T comparable](sets ...*Set[T]) *Set[T] {
	if len(sets) == 0 {
		return New[T]()
	}
	if len(sets) == 1 {
		return sets[0].Clone()
	}

	// 找最小的 Set
	minSet := sets[0]
	for _, set := range sets[1:] {
		if set.Size() < minSet.Size() {
			minSet = set
		}
	}

	result := NewWithSize[T](minSet.Size())

	for item := range minSet.m {
		inAll := true
		for _, set := range sets {
			if set == minSet {
				continue
			}
			if _, ok := set.m[item]; !ok {
				inAll = false
				break
			}
		}
		if inAll {
			result.m[item] = struct{}{}
		}
	}

	return result
}

// Difference 求差集（第一个 Set 减去其他所有 Set）
func Difference[T comparable](sets ...*Set[T]) *Set[T] {
	if len(sets) == 0 {
		return New[T]()
	}
	if len(sets) == 1 {
		return sets[0].Clone()
	}

	result := sets[0].Clone()
	for _, set := range sets[1:] {
		for item := range set.m {
			delete(result.m, item)
		}
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --- 线程安全版本 ---

// SyncSet 线程安全的 HashSet
type SyncSet[T comparable] struct {
	s  *Set[T]
	mu sync.RWMutex
}

// NewSyncSet 创建线程安全的 Set
func NewSyncSet[T comparable](items ...T) *SyncSet[T] {
	return &SyncSet[T]{
		s: New(items...),
	}
}

// Add 添加元素（线程安全）
func (ss *SyncSet[T]) Add(items ...T) *SyncSet[T] {
	ss.mu.Lock()
	ss.s.Add(items...)
	ss.mu.Unlock()
	return ss
}

// Remove 移除元素（线程安全）
func (ss *SyncSet[T]) Remove(items ...T) *SyncSet[T] {
	ss.mu.Lock()
	ss.s.Remove(items...)
	ss.mu.Unlock()
	return ss
}

// Contains 判断是否包含元素（线程安全）
func (ss *SyncSet[T]) Contains(item T) bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.s.Contains(item)
}

// ContainsAll 判断是否包含所有元素（线程安全）
func (ss *SyncSet[T]) ContainsAll(items ...T) bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.s.ContainsAll(items...)
}

// ContainsAny 判断是否包含任意一个元素（线程安全）
func (ss *SyncSet[T]) ContainsAny(items ...T) bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.s.ContainsAny(items...)
}

// Size 返回元素数量（线程安全）
func (ss *SyncSet[T]) Size() int {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.s.Size()
}

// Len 返回元素数量（线程安全）
func (ss *SyncSet[T]) Len() int {
	return ss.Size()
}

// IsEmpty 判断是否为空（线程安全）
func (ss *SyncSet[T]) IsEmpty() bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.s.IsEmpty()
}

// Clear 清空所有元素（线程安全）
func (ss *SyncSet[T]) Clear() {
	ss.mu.Lock()
	ss.s.Clear()
	ss.mu.Unlock()
}

// ToSlice 转换为切片（线程安全）
func (ss *SyncSet[T]) ToSlice() []T {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.s.ToSlice()
}

// Values 返回所有元素（线程安全）
func (ss *SyncSet[T]) Values() []T {
	return ss.ToSlice()
}

// Clone 克隆 Set（线程安全）
func (ss *SyncSet[T]) Clone() *SyncSet[T] {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return &SyncSet[T]{
		s: ss.s.Clone(),
	}
}

// Pop 随机移除并返回一个元素（线程安全）
func (ss *SyncSet[T]) Pop() (T, bool) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.s.Pop()
}

// ForEach 遍历所有元素（线程安全）
func (ss *SyncSet[T]) ForEach(fn func(T)) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	ss.s.ForEach(fn)
}

// Filter 过滤元素（线程安全）
func (ss *SyncSet[T]) Filter(predicate func(T) bool) *SyncSet[T] {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return &SyncSet[T]{
		s: ss.s.Filter(predicate),
	}
}

// Any 判断是否存在满足条件的元素（线程安全）
func (ss *SyncSet[T]) Any(predicate func(T) bool) bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.s.Any(predicate)
}

// All 判断是否所有元素都满足条件（线程安全）
func (ss *SyncSet[T]) All(predicate func(T) bool) bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.s.All(predicate)
}

// Union 并集（线程安全）
// 使用地址排序确保固定的加锁顺序，防止 ABBA 死锁
func (ss *SyncSet[T]) Union(other *SyncSet[T]) *SyncSet[T] {
	// 按地址排序加锁，防止死锁
	first, second := orderByAddr(ss, other)
	first.mu.RLock()
	second.mu.RLock()
	defer first.mu.RUnlock()
	defer second.mu.RUnlock()
	return &SyncSet[T]{
		s: ss.s.Union(other.s),
	}
}

// Intersection 交集（线程安全）
// 使用地址排序确保固定的加锁顺序，防止 ABBA 死锁
func (ss *SyncSet[T]) Intersection(other *SyncSet[T]) *SyncSet[T] {
	first, second := orderByAddr(ss, other)
	first.mu.RLock()
	second.mu.RLock()
	defer first.mu.RUnlock()
	defer second.mu.RUnlock()
	return &SyncSet[T]{
		s: ss.s.Intersection(other.s),
	}
}

// Difference 差集（线程安全）
// 使用地址排序确保固定的加锁顺序，防止 ABBA 死锁
func (ss *SyncSet[T]) Difference(other *SyncSet[T]) *SyncSet[T] {
	first, second := orderByAddr(ss, other)
	first.mu.RLock()
	second.mu.RLock()
	defer first.mu.RUnlock()
	defer second.mu.RUnlock()
	return &SyncSet[T]{
		s: ss.s.Difference(other.s),
	}
}

// String 返回字符串表示（线程安全）
func (ss *SyncSet[T]) String() string {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return "Sync" + ss.s.String()
}

// orderByAddr 按地址排序两个 SyncSet，确保固定的加锁顺序
// 用于防止多个 goroutine 同时操作两个 SyncSet 时发生 ABBA 死锁
func orderByAddr[T comparable](a, b *SyncSet[T]) (*SyncSet[T], *SyncSet[T]) {
	// 使用 uintptr 比较地址，确保一致的排序
	if uintptr(unsafe.Pointer(a)) < uintptr(unsafe.Pointer(b)) {
		return a, b
	}
	return b, a
}
