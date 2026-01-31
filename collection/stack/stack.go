package stack

import "sync"

// Stack 泛型栈（LIFO）
type Stack[T any] struct {
	items []T
}

// New 创建新的栈
func New[T any](items ...T) *Stack[T] {
	s := &Stack[T]{
		items: make([]T, 0, len(items)),
	}
	s.items = append(s.items, items...)
	return s
}

// NewWithCapacity 创建指定初始容量的栈
func NewWithCapacity[T any](capacity int) *Stack[T] {
	return &Stack[T]{
		items: make([]T, 0, capacity),
	}
}

// Push 入栈（添加到栈顶）
func (s *Stack[T]) Push(items ...T) *Stack[T] {
	s.items = append(s.items, items...)
	return s
}

// Pop 出栈（移除栈顶元素）
func (s *Stack[T]) Pop() (T, bool) {
	if len(s.items) == 0 {
		var zero T
		return zero, false
	}
	n := len(s.items) - 1
	item := s.items[n]
	var zero T
	s.items[n] = zero // 清除引用，帮助 GC 回收
	s.items = s.items[:n]
	return item, true
}

// Peek 查看栈顶元素（不移除）
func (s *Stack[T]) Peek() (T, bool) {
	if len(s.items) == 0 {
		var zero T
		return zero, false
	}
	return s.items[len(s.items)-1], true
}

// Top 查看栈顶元素（Peek 的别名）
func (s *Stack[T]) Top() (T, bool) {
	return s.Peek()
}

// Size 返回栈大小
func (s *Stack[T]) Size() int {
	return len(s.items)
}

// Len 返回栈大小（Size 的别名）
func (s *Stack[T]) Len() int {
	return len(s.items)
}

// IsEmpty 判断栈是否为空
func (s *Stack[T]) IsEmpty() bool {
	return len(s.items) == 0
}

// Clear 清空栈
func (s *Stack[T]) Clear() {
	// 清除引用，帮助 GC 回收
	var zero T
	for i := range s.items {
		s.items[i] = zero
	}
	s.items = s.items[:0]
}

// ToSlice 转换为切片（从栈底到栈顶）
func (s *Stack[T]) ToSlice() []T {
	result := make([]T, len(s.items))
	copy(result, s.items)
	return result
}

// ToSliceReverse 转换为切片（从栈顶到栈底）
func (s *Stack[T]) ToSliceReverse() []T {
	result := make([]T, len(s.items))
	for i, j := 0, len(s.items)-1; j >= 0; i, j = i+1, j-1 {
		result[i] = s.items[j]
	}
	return result
}

// Values 返回所有元素（ToSlice 的别名）
func (s *Stack[T]) Values() []T {
	return s.ToSlice()
}

// ForEach 从栈底到栈顶遍历所有元素
func (s *Stack[T]) ForEach(fn func(T)) {
	for _, item := range s.items {
		fn(item)
	}
}

// ForEachReverse 从栈顶到栈底遍历所有元素
func (s *Stack[T]) ForEachReverse(fn func(T)) {
	for i := len(s.items) - 1; i >= 0; i-- {
		fn(s.items[i])
	}
}

// Clone 克隆栈
func (s *Stack[T]) Clone() *Stack[T] {
	result := NewWithCapacity[T](len(s.items))
	result.items = append(result.items, s.items...)
	return result
}

// Contains 判断是否包含满足条件的元素
func (s *Stack[T]) Contains(predicate func(T) bool) bool {
	for _, item := range s.items {
		if predicate(item) {
			return true
		}
	}
	return false
}

// Filter 过滤元素，返回新栈
func (s *Stack[T]) Filter(predicate func(T) bool) *Stack[T] {
	result := NewWithCapacity[T](len(s.items))
	for _, item := range s.items {
		if predicate(item) {
			result.items = append(result.items, item)
		}
	}
	return result
}

// PopN 出栈 N 个元素
func (s *Stack[T]) PopN(n int) []T {
	if n <= 0 {
		return nil
	}
	if n > len(s.items) {
		n = len(s.items)
	}

	result := make([]T, n)
	var zero T
	for i := 0; i < n; i++ {
		idx := len(s.items) - 1 - i
		result[i] = s.items[idx]
		s.items[idx] = zero // 清除引用，帮助 GC 回收
	}
	s.items = s.items[:len(s.items)-n]
	return result
}

// PeekN 查看栈顶 N 个元素（不移除）
func (s *Stack[T]) PeekN(n int) []T {
	if n <= 0 {
		return nil
	}
	if n > len(s.items) {
		n = len(s.items)
	}

	result := make([]T, n)
	for i := 0; i < n; i++ {
		result[i] = s.items[len(s.items)-1-i]
	}
	return result
}

// Reverse 反转栈
func (s *Stack[T]) Reverse() {
	for i, j := 0, len(s.items)-1; i < j; i, j = i+1, j-1 {
		s.items[i], s.items[j] = s.items[j], s.items[i]
	}
}

// --- 线程安全版本 ---

// SyncStack 线程安全的栈
type SyncStack[T any] struct {
	s  *Stack[T]
	mu sync.RWMutex
}

// NewSyncStack 创建线程安全的栈
func NewSyncStack[T any]() *SyncStack[T] {
	return &SyncStack[T]{
		s: New[T](),
	}
}

// Push 入栈
func (ss *SyncStack[T]) Push(items ...T) {
	ss.mu.Lock()
	ss.s.Push(items...)
	ss.mu.Unlock()
}

// Pop 出栈
func (ss *SyncStack[T]) Pop() (T, bool) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.s.Pop()
}

// Peek 查看栈顶元素
func (ss *SyncStack[T]) Peek() (T, bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.s.Peek()
}

// Size 返回栈大小
func (ss *SyncStack[T]) Size() int {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.s.Size()
}

// IsEmpty 判断栈是否为空
func (ss *SyncStack[T]) IsEmpty() bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.s.IsEmpty()
}

// Clear 清空栈
func (ss *SyncStack[T]) Clear() {
	ss.mu.Lock()
	ss.s.Clear()
	ss.mu.Unlock()
}

// ToSlice 转换为切片
func (ss *SyncStack[T]) ToSlice() []T {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.s.ToSlice()
}

// PopN 出栈 N 个元素
func (ss *SyncStack[T]) PopN(n int) []T {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.s.PopN(n)
}
