package list

import "sync"

// Node 链表节点
type Node[T any] struct {
	Value T
	prev  *Node[T]
	next  *Node[T]
	list  *List[T]
}

// Next 返回下一个节点
func (n *Node[T]) Next() *Node[T] {
	if n.list == nil || n.next == n.list.root {
		return nil
	}
	return n.next
}

// Prev 返回上一个节点
func (n *Node[T]) Prev() *Node[T] {
	if n.list == nil || n.prev == n.list.root {
		return nil
	}
	return n.prev
}

// List 泛型双向链表
type List[T any] struct {
	root *Node[T] // 哨兵节点
	len  int
}

// New 创建新的链表
func New[T any](items ...T) *List[T] {
	l := &List[T]{}
	l.init()
	for _, item := range items {
		l.PushBack(item)
	}
	return l
}

// init 初始化链表
func (l *List[T]) init() {
	l.root = &Node[T]{}
	l.root.next = l.root
	l.root.prev = l.root
	l.root.list = l
	l.len = 0
}

// lazyInit 延迟初始化
func (l *List[T]) lazyInit() {
	if l.root == nil {
		l.init()
	}
}

// Len 返回链表长度
func (l *List[T]) Len() int {
	return l.len
}

// Size 返回链表长度（Len 的别名）
func (l *List[T]) Size() int {
	return l.len
}

// IsEmpty 判断链表是否为空
func (l *List[T]) IsEmpty() bool {
	return l.len == 0
}

// Front 返回链表头节点
func (l *List[T]) Front() *Node[T] {
	if l.len == 0 {
		return nil
	}
	return l.root.next
}

// Back 返回链表尾节点
func (l *List[T]) Back() *Node[T] {
	if l.len == 0 {
		return nil
	}
	return l.root.prev
}

// PushFront 在链表头部插入元素
func (l *List[T]) PushFront(v T) *Node[T] {
	l.lazyInit()
	return l.insertAfter(v, l.root)
}

// PushBack 在链表尾部插入元素
func (l *List[T]) PushBack(v T) *Node[T] {
	l.lazyInit()
	return l.insertAfter(v, l.root.prev)
}

// InsertBefore 在指定节点前插入元素
// 如果 mark 为 nil 或不属于该链表，返回 nil
func (l *List[T]) InsertBefore(v T, mark *Node[T]) *Node[T] {
	if mark == nil || mark.list != l {
		return nil
	}
	return l.insertAfter(v, mark.prev)
}

// InsertAfter 在指定节点后插入元素
// 如果 mark 为 nil 或不属于该链表，返回 nil
func (l *List[T]) InsertAfter(v T, mark *Node[T]) *Node[T] {
	if mark == nil || mark.list != l {
		return nil
	}
	return l.insertAfter(v, mark)
}

// insertAfter 在指定节点后插入新节点
func (l *List[T]) insertAfter(v T, at *Node[T]) *Node[T] {
	n := &Node[T]{Value: v, list: l}
	n.prev = at
	n.next = at.next
	n.prev.next = n
	n.next.prev = n
	l.len++
	return n
}

// Remove 移除指定节点
func (l *List[T]) Remove(n *Node[T]) T {
	if n.list == l {
		l.remove(n)
	}
	return n.Value
}

// remove 内部移除节点
func (l *List[T]) remove(n *Node[T]) {
	n.prev.next = n.next
	n.next.prev = n.prev
	n.next = nil
	n.prev = nil
	n.list = nil
	l.len--
}

// PopFront 移除并返回链表头元素
func (l *List[T]) PopFront() (T, bool) {
	if l.len == 0 {
		var zero T
		return zero, false
	}
	n := l.root.next
	l.remove(n)
	return n.Value, true
}

// PopBack 移除并返回链表尾元素
func (l *List[T]) PopBack() (T, bool) {
	if l.len == 0 {
		var zero T
		return zero, false
	}
	n := l.root.prev
	l.remove(n)
	return n.Value, true
}

// MoveToFront 将节点移动到链表头部
func (l *List[T]) MoveToFront(n *Node[T]) {
	if n.list != l || l.root.next == n {
		return
	}
	l.move(n, l.root)
}

// MoveToBack 将节点移动到链表尾部
func (l *List[T]) MoveToBack(n *Node[T]) {
	if n.list != l || l.root.prev == n {
		return
	}
	l.move(n, l.root.prev)
}

// MoveBefore 将节点移动到指定节点之前
// 如果 n 或 mark 为 nil，或不属于该链表，不执行任何操作
func (l *List[T]) MoveBefore(n, mark *Node[T]) {
	if n == nil || mark == nil || n.list != l || n == mark || mark.list != l {
		return
	}
	l.move(n, mark.prev)
}

// MoveAfter 将节点移动到指定节点之后
// 如果 n 或 mark 为 nil，或不属于该链表，不执行任何操作
func (l *List[T]) MoveAfter(n, mark *Node[T]) {
	if n == nil || mark == nil || n.list != l || n == mark || mark.list != l {
		return
	}
	l.move(n, mark)
}

// move 将节点移动到 at 之后
func (l *List[T]) move(n, at *Node[T]) {
	if n == at {
		return
	}
	n.prev.next = n.next
	n.next.prev = n.prev

	n.prev = at
	n.next = at.next
	n.prev.next = n
	n.next.prev = n
}

// Clear 清空链表
func (l *List[T]) Clear() {
	l.init()
}

// ToSlice 转换为切片
func (l *List[T]) ToSlice() []T {
	result := make([]T, 0, l.len)
	for n := l.Front(); n != nil; n = n.Next() {
		result = append(result, n.Value)
	}
	return result
}

// Values 返回所有元素（ToSlice 的别名）
func (l *List[T]) Values() []T {
	return l.ToSlice()
}

// ForEach 遍历所有元素
func (l *List[T]) ForEach(fn func(T)) {
	for n := l.Front(); n != nil; n = n.Next() {
		fn(n.Value)
	}
}

// ForEachNode 遍历所有节点
func (l *List[T]) ForEachNode(fn func(*Node[T])) {
	for n := l.Front(); n != nil; n = n.Next() {
		fn(n)
	}
}

// ForEachReverse 反向遍历所有元素
func (l *List[T]) ForEachReverse(fn func(T)) {
	for n := l.Back(); n != nil; n = n.Prev() {
		fn(n.Value)
	}
}

// Find 查找第一个满足条件的节点
func (l *List[T]) Find(predicate func(T) bool) *Node[T] {
	for n := l.Front(); n != nil; n = n.Next() {
		if predicate(n.Value) {
			return n
		}
	}
	return nil
}

// FindAll 查找所有满足条件的节点
func (l *List[T]) FindAll(predicate func(T) bool) []*Node[T] {
	result := make([]*Node[T], 0)
	for n := l.Front(); n != nil; n = n.Next() {
		if predicate(n.Value) {
			result = append(result, n)
		}
	}
	return result
}

// Contains 判断是否包含满足条件的元素
func (l *List[T]) Contains(predicate func(T) bool) bool {
	return l.Find(predicate) != nil
}

// Filter 过滤元素，返回新链表
func (l *List[T]) Filter(predicate func(T) bool) *List[T] {
	result := New[T]()
	for n := l.Front(); n != nil; n = n.Next() {
		if predicate(n.Value) {
			result.PushBack(n.Value)
		}
	}
	return result
}

// Clone 克隆链表
func (l *List[T]) Clone() *List[T] {
	result := New[T]()
	for n := l.Front(); n != nil; n = n.Next() {
		result.PushBack(n.Value)
	}
	return result
}

// Reverse 反转链表
func (l *List[T]) Reverse() {
	if l.len <= 1 {
		return
	}

	current := l.root.next
	for current != l.root {
		current.prev, current.next = current.next, current.prev
		current = current.prev // 因为已经交换，所以用 prev
	}
	l.root.prev, l.root.next = l.root.next, l.root.prev
}

// PushFrontList 将另一个链表的所有元素添加到头部
func (l *List[T]) PushFrontList(other *List[T]) {
	l.lazyInit()
	for i, n := other.Len(), other.Back(); i > 0; i, n = i-1, n.Prev() {
		l.PushFront(n.Value)
	}
}

// PushBackList 将另一个链表的所有元素添加到尾部
func (l *List[T]) PushBackList(other *List[T]) {
	l.lazyInit()
	for n := other.Front(); n != nil; n = n.Next() {
		l.PushBack(n.Value)
	}
}

// --- 线程安全版本 ---

// SyncList 线程安全的双向链表
type SyncList[T any] struct {
	l  *List[T]
	mu sync.RWMutex
}

// NewSyncList 创建线程安全的双向链表
func NewSyncList[T any]() *SyncList[T] {
	return &SyncList[T]{
		l: New[T](),
	}
}

// Len 返回链表长度
func (sl *SyncList[T]) Len() int {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return sl.l.Len()
}

// IsEmpty 判断链表是否为空
func (sl *SyncList[T]) IsEmpty() bool {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return sl.l.IsEmpty()
}

// PushFront 在链表头部插入元素
func (sl *SyncList[T]) PushFront(v T) {
	sl.mu.Lock()
	sl.l.PushFront(v)
	sl.mu.Unlock()
}

// PushBack 在链表尾部插入元素
func (sl *SyncList[T]) PushBack(v T) {
	sl.mu.Lock()
	sl.l.PushBack(v)
	sl.mu.Unlock()
}

// PopFront 移除并返回链表头元素
func (sl *SyncList[T]) PopFront() (T, bool) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return sl.l.PopFront()
}

// PopBack 移除并返回链表尾元素
func (sl *SyncList[T]) PopBack() (T, bool) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return sl.l.PopBack()
}

// Front 返回链表头元素值
func (sl *SyncList[T]) Front() (T, bool) {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	if n := sl.l.Front(); n != nil {
		return n.Value, true
	}
	var zero T
	return zero, false
}

// Back 返回链表尾元素值
func (sl *SyncList[T]) Back() (T, bool) {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	if n := sl.l.Back(); n != nil {
		return n.Value, true
	}
	var zero T
	return zero, false
}

// Clear 清空链表
func (sl *SyncList[T]) Clear() {
	sl.mu.Lock()
	sl.l.Clear()
	sl.mu.Unlock()
}

// ToSlice 转换为切片
func (sl *SyncList[T]) ToSlice() []T {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return sl.l.ToSlice()
}

// ForEach 遍历所有元素（线程安全）
func (sl *SyncList[T]) ForEach(fn func(T)) {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	sl.l.ForEach(fn)
}

// ForEachReverse 反向遍历所有元素（线程安全）
func (sl *SyncList[T]) ForEachReverse(fn func(T)) {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	sl.l.ForEachReverse(fn)
}

// Find 查找第一个满足条件的元素值（线程安全）
func (sl *SyncList[T]) Find(predicate func(T) bool) (T, bool) {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	if n := sl.l.Find(predicate); n != nil {
		return n.Value, true
	}
	var zero T
	return zero, false
}

// FindAll 查找所有满足条件的元素值（线程安全）
func (sl *SyncList[T]) FindAll(predicate func(T) bool) []T {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	nodes := sl.l.FindAll(predicate)
	result := make([]T, len(nodes))
	for i, n := range nodes {
		result[i] = n.Value
	}
	return result
}

// Contains 判断是否包含满足条件的元素（线程安全）
func (sl *SyncList[T]) Contains(predicate func(T) bool) bool {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return sl.l.Contains(predicate)
}

// Filter 过滤元素，返回新的线程安全链表
func (sl *SyncList[T]) Filter(predicate func(T) bool) *SyncList[T] {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	result := NewSyncList[T]()
	sl.l.ForEach(func(v T) {
		if predicate(v) {
			result.PushBack(v)
		}
	})
	return result
}

// Clone 克隆链表（线程安全）
func (sl *SyncList[T]) Clone() *SyncList[T] {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	result := NewSyncList[T]()
	sl.l.ForEach(func(v T) {
		result.l.PushBack(v) // 直接操作内部链表避免重复加锁
	})
	return result
}

// Reverse 反转链表（线程安全）
func (sl *SyncList[T]) Reverse() {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.l.Reverse()
}
