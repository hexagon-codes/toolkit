package queue

import (
	"container/heap"
	"sync"
)

// Queue 泛型 FIFO 队列
// 使用环形缓冲区实现，避免 Dequeue 时的内存泄漏
type Queue[T any] struct {
	items []T
	head  int // 队首索引
	tail  int // 队尾索引（下一个插入位置）
	size  int // 当前元素数量
}

// New 创建新的队列
func New[T any](items ...T) *Queue[T] {
	capacity := len(items)
	if capacity < 8 {
		capacity = 8
	}
	q := &Queue[T]{
		items: make([]T, capacity),
		head:  0,
		tail:  0,
		size:  0,
	}
	for _, item := range items {
		q.items[q.tail] = item
		q.tail = (q.tail + 1) % len(q.items)
		q.size++
	}
	return q
}

// NewWithCapacity 创建指定初始容量的队列
func NewWithCapacity[T any](capacity int) *Queue[T] {
	if capacity < 8 {
		capacity = 8
	}
	return &Queue[T]{
		items: make([]T, capacity),
		head:  0,
		tail:  0,
		size:  0,
	}
}

// Enqueue 入队（添加到队尾）
func (q *Queue[T]) Enqueue(items ...T) *Queue[T] {
	for _, item := range items {
		// 检查是否需要扩容
		if q.size == len(q.items) {
			q.grow()
		}
		q.items[q.tail] = item
		q.tail = (q.tail + 1) % len(q.items)
		q.size++
	}
	return q
}

// grow 扩容队列
func (q *Queue[T]) grow() {
	newCap := len(q.items) * 2
	newItems := make([]T, newCap)
	// 复制元素到新数组
	for i := 0; i < q.size; i++ {
		newItems[i] = q.items[(q.head+i)%len(q.items)]
	}
	q.items = newItems
	q.head = 0
	q.tail = q.size
}

// Push 入队（Enqueue 的别名）
func (q *Queue[T]) Push(items ...T) *Queue[T] {
	return q.Enqueue(items...)
}

// Dequeue 出队（移除队首元素）
func (q *Queue[T]) Dequeue() (T, bool) {
	if q.size == 0 {
		var zero T
		return zero, false
	}
	item := q.items[q.head]
	var zero T
	q.items[q.head] = zero // 清除引用，帮助 GC 回收
	q.head = (q.head + 1) % len(q.items)
	q.size--
	return item, true
}

// Pop 出队（Dequeue 的别名）
func (q *Queue[T]) Pop() (T, bool) {
	return q.Dequeue()
}

// Peek 查看队首元素（不移除）
func (q *Queue[T]) Peek() (T, bool) {
	if q.size == 0 {
		var zero T
		return zero, false
	}
	return q.items[q.head], true
}

// Front 查看队首元素（Peek 的别名）
func (q *Queue[T]) Front() (T, bool) {
	return q.Peek()
}

// Back 查看队尾元素
func (q *Queue[T]) Back() (T, bool) {
	if q.size == 0 {
		var zero T
		return zero, false
	}
	// tail 指向下一个插入位置，所以尾元素在 tail-1
	idx := (q.tail - 1 + len(q.items)) % len(q.items)
	return q.items[idx], true
}

// Size 返回队列长度
func (q *Queue[T]) Size() int {
	return q.size
}

// Len 返回队列长度（Size 的别名）
func (q *Queue[T]) Len() int {
	return q.size
}

// IsEmpty 判断队列是否为空
func (q *Queue[T]) IsEmpty() bool {
	return q.size == 0
}

// Clear 清空队列
func (q *Queue[T]) Clear() {
	var zero T
	for i := 0; i < q.size; i++ {
		q.items[(q.head+i)%len(q.items)] = zero
	}
	q.head = 0
	q.tail = 0
	q.size = 0
}

// ToSlice 转换为切片
func (q *Queue[T]) ToSlice() []T {
	result := make([]T, q.size)
	for i := 0; i < q.size; i++ {
		result[i] = q.items[(q.head+i)%len(q.items)]
	}
	return result
}

// Values 返回所有元素（ToSlice 的别名）
func (q *Queue[T]) Values() []T {
	return q.ToSlice()
}

// ForEach 遍历所有元素
func (q *Queue[T]) ForEach(fn func(T)) {
	for i := 0; i < q.size; i++ {
		fn(q.items[(q.head+i)%len(q.items)])
	}
}

// Filter 过滤元素
func (q *Queue[T]) Filter(predicate func(T) bool) *Queue[T] {
	result := NewWithCapacity[T](q.size)
	for i := 0; i < q.size; i++ {
		item := q.items[(q.head+i)%len(q.items)]
		if predicate(item) {
			result.Enqueue(item)
		}
	}
	return result
}

// Clone 克隆队列
func (q *Queue[T]) Clone() *Queue[T] {
	result := NewWithCapacity[T](q.size)
	for i := 0; i < q.size; i++ {
		result.Enqueue(q.items[(q.head+i)%len(q.items)])
	}
	return result
}

// --- Deque 双端队列 ---

// Deque 泛型双端队列
// 使用环形缓冲区实现，PushFront 和 PushBack 都是 O(1)
type Deque[T any] struct {
	items []T
	head  int // 队首索引
	tail  int // 队尾索引（下一个插入位置）
	size  int // 当前元素数量
}

// NewDeque 创建新的双端队列
func NewDeque[T any](items ...T) *Deque[T] {
	capacity := len(items)
	if capacity < 8 {
		capacity = 8
	}
	d := &Deque[T]{
		items: make([]T, capacity),
		head:  0,
		tail:  0,
		size:  0,
	}
	for _, item := range items {
		d.items[d.tail] = item
		d.tail = (d.tail + 1) % len(d.items)
		d.size++
	}
	return d
}

// NewDequeWithCapacity 创建指定初始容量的双端队列
func NewDequeWithCapacity[T any](capacity int) *Deque[T] {
	if capacity < 8 {
		capacity = 8
	}
	return &Deque[T]{
		items: make([]T, capacity),
		head:  0,
		tail:  0,
		size:  0,
	}
}

// growDeque 扩容双端队列
func (d *Deque[T]) growDeque() {
	newCap := len(d.items) * 2
	newItems := make([]T, newCap)
	// 复制元素到新数组
	for i := 0; i < d.size; i++ {
		newItems[i] = d.items[(d.head+i)%len(d.items)]
	}
	d.items = newItems
	d.head = 0
	d.tail = d.size
}

// PushBack 从队尾添加元素（O(1) 均摊）
func (d *Deque[T]) PushBack(items ...T) *Deque[T] {
	for _, item := range items {
		if d.size == len(d.items) {
			d.growDeque()
		}
		d.items[d.tail] = item
		d.tail = (d.tail + 1) % len(d.items)
		d.size++
	}
	return d
}

// PushFront 从队首添加元素（O(1) 均摊）
func (d *Deque[T]) PushFront(items ...T) *Deque[T] {
	// 逆序添加以保持顺序
	for i := len(items) - 1; i >= 0; i-- {
		if d.size == len(d.items) {
			d.growDeque()
		}
		d.head = (d.head - 1 + len(d.items)) % len(d.items)
		d.items[d.head] = items[i]
		d.size++
	}
	return d
}

// PopBack 从队尾移除元素
func (d *Deque[T]) PopBack() (T, bool) {
	if d.size == 0 {
		var zero T
		return zero, false
	}
	d.tail = (d.tail - 1 + len(d.items)) % len(d.items)
	item := d.items[d.tail]
	var zero T
	d.items[d.tail] = zero // 清除引用，帮助 GC 回收
	d.size--
	return item, true
}

// PopFront 从队首移除元素
func (d *Deque[T]) PopFront() (T, bool) {
	if d.size == 0 {
		var zero T
		return zero, false
	}
	item := d.items[d.head]
	var zero T
	d.items[d.head] = zero // 清除引用，帮助 GC 回收
	d.head = (d.head + 1) % len(d.items)
	d.size--
	return item, true
}

// Front 查看队首元素
func (d *Deque[T]) Front() (T, bool) {
	if d.size == 0 {
		var zero T
		return zero, false
	}
	return d.items[d.head], true
}

// Back 查看队尾元素
func (d *Deque[T]) Back() (T, bool) {
	if d.size == 0 {
		var zero T
		return zero, false
	}
	idx := (d.tail - 1 + len(d.items)) % len(d.items)
	return d.items[idx], true
}

// Size 返回队列长度
func (d *Deque[T]) Size() int {
	return d.size
}

// Len 返回队列长度（Size 的别名）
func (d *Deque[T]) Len() int {
	return d.size
}

// IsEmpty 判断队列是否为空
func (d *Deque[T]) IsEmpty() bool {
	return d.size == 0
}

// Clear 清空队列
func (d *Deque[T]) Clear() {
	var zero T
	for i := 0; i < d.size; i++ {
		d.items[(d.head+i)%len(d.items)] = zero
	}
	d.head = 0
	d.tail = 0
	d.size = 0
}

// ToSlice 转换为切片
func (d *Deque[T]) ToSlice() []T {
	result := make([]T, d.size)
	for i := 0; i < d.size; i++ {
		result[i] = d.items[(d.head+i)%len(d.items)]
	}
	return result
}

// Clone 克隆双端队列
func (d *Deque[T]) Clone() *Deque[T] {
	result := NewDequeWithCapacity[T](d.size)
	for i := 0; i < d.size; i++ {
		result.PushBack(d.items[(d.head+i)%len(d.items)])
	}
	return result
}

// --- PriorityQueue 优先级队列 ---

// PriorityQueue 泛型优先级队列
type PriorityQueue[T any] struct {
	heap *priorityHeap[T]
	less func(a, b T) bool
}

// priorityHeap 内部堆实现
type priorityHeap[T any] struct {
	items []T
	less  func(a, b T) bool
}

func (h *priorityHeap[T]) Len() int           { return len(h.items) }
func (h *priorityHeap[T]) Less(i, j int) bool { return h.less(h.items[i], h.items[j]) }
func (h *priorityHeap[T]) Swap(i, j int)      { h.items[i], h.items[j] = h.items[j], h.items[i] }

func (h *priorityHeap[T]) Push(x any) {
	h.items = append(h.items, x.(T))
}

func (h *priorityHeap[T]) Pop() any {
	old := h.items
	n := len(old)
	item := old[n-1]
	h.items = old[0 : n-1]
	return item
}

// NewPriorityQueue 创建新的优先级队列
// less 函数定义优先级：返回 true 表示 a 优先级高于 b
func NewPriorityQueue[T any](less func(a, b T) bool) *PriorityQueue[T] {
	pq := &PriorityQueue[T]{
		heap: &priorityHeap[T]{
			items: make([]T, 0),
			less:  less,
		},
		less: less,
	}
	heap.Init(pq.heap)
	return pq
}

// NewMinHeap 创建最小堆（最小元素优先）
func NewMinHeap[T Ordered]() *PriorityQueue[T] {
	return NewPriorityQueue[T](func(a, b T) bool {
		return a < b
	})
}

// NewMaxHeap 创建最大堆（最大元素优先）
func NewMaxHeap[T Ordered]() *PriorityQueue[T] {
	return NewPriorityQueue[T](func(a, b T) bool {
		return a > b
	})
}

// Ordered 可排序类型约束
type Ordered interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64 | ~string
}

// Push 添加元素
func (pq *PriorityQueue[T]) Push(items ...T) *PriorityQueue[T] {
	for _, item := range items {
		heap.Push(pq.heap, item)
	}
	return pq
}

// Pop 移除并返回优先级最高的元素
func (pq *PriorityQueue[T]) Pop() (T, bool) {
	if pq.heap.Len() == 0 {
		var zero T
		return zero, false
	}
	item := heap.Pop(pq.heap).(T)
	return item, true
}

// Peek 查看优先级最高的元素（不移除）
func (pq *PriorityQueue[T]) Peek() (T, bool) {
	if pq.heap.Len() == 0 {
		var zero T
		return zero, false
	}
	return pq.heap.items[0], true
}

// Size 返回队列长度
func (pq *PriorityQueue[T]) Size() int {
	return pq.heap.Len()
}

// Len 返回队列长度（Size 的别名）
func (pq *PriorityQueue[T]) Len() int {
	return pq.heap.Len()
}

// IsEmpty 判断队列是否为空
func (pq *PriorityQueue[T]) IsEmpty() bool {
	return pq.heap.Len() == 0
}

// Clear 清空队列
func (pq *PriorityQueue[T]) Clear() {
	pq.heap.items = pq.heap.items[:0]
}

// ToSlice 转换为切片（按优先级顺序）
func (pq *PriorityQueue[T]) ToSlice() []T {
	// 克隆堆，然后逐个弹出
	clone := &priorityHeap[T]{
		items: make([]T, len(pq.heap.items)),
		less:  pq.less,
	}
	copy(clone.items, pq.heap.items)
	heap.Init(clone)

	result := make([]T, 0, len(clone.items))
	for clone.Len() > 0 {
		result = append(result, heap.Pop(clone).(T))
	}
	return result
}

// --- 线程安全版本 ---

// SyncQueue 线程安全的队列
type SyncQueue[T any] struct {
	q  *Queue[T]
	mu sync.RWMutex
}

// NewSyncQueue 创建线程安全的队列
func NewSyncQueue[T any]() *SyncQueue[T] {
	return &SyncQueue[T]{
		q: New[T](),
	}
}

// Enqueue 入队
func (sq *SyncQueue[T]) Enqueue(items ...T) {
	sq.mu.Lock()
	sq.q.Enqueue(items...)
	sq.mu.Unlock()
}

// Dequeue 出队
func (sq *SyncQueue[T]) Dequeue() (T, bool) {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	return sq.q.Dequeue()
}

// Peek 查看队首元素
func (sq *SyncQueue[T]) Peek() (T, bool) {
	sq.mu.RLock()
	defer sq.mu.RUnlock()
	return sq.q.Peek()
}

// Size 返回队列长度
func (sq *SyncQueue[T]) Size() int {
	sq.mu.RLock()
	defer sq.mu.RUnlock()
	return sq.q.Size()
}

// IsEmpty 判断队列是否为空
func (sq *SyncQueue[T]) IsEmpty() bool {
	sq.mu.RLock()
	defer sq.mu.RUnlock()
	return sq.q.IsEmpty()
}

// Clear 清空队列
func (sq *SyncQueue[T]) Clear() {
	sq.mu.Lock()
	sq.q.Clear()
	sq.mu.Unlock()
}

// SyncDeque 线程安全的双端队列
type SyncDeque[T any] struct {
	d  *Deque[T]
	mu sync.RWMutex
}

// NewSyncDeque 创建线程安全的双端队列
func NewSyncDeque[T any]() *SyncDeque[T] {
	return &SyncDeque[T]{
		d: NewDeque[T](),
	}
}

// PushBack 从队尾添加元素
func (sd *SyncDeque[T]) PushBack(items ...T) {
	sd.mu.Lock()
	sd.d.PushBack(items...)
	sd.mu.Unlock()
}

// PushFront 从队首添加元素
func (sd *SyncDeque[T]) PushFront(items ...T) {
	sd.mu.Lock()
	sd.d.PushFront(items...)
	sd.mu.Unlock()
}

// PopBack 从队尾移除元素
func (sd *SyncDeque[T]) PopBack() (T, bool) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	return sd.d.PopBack()
}

// PopFront 从队首移除元素
func (sd *SyncDeque[T]) PopFront() (T, bool) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	return sd.d.PopFront()
}

// Front 查看队首元素
func (sd *SyncDeque[T]) Front() (T, bool) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	return sd.d.Front()
}

// Back 查看队尾元素
func (sd *SyncDeque[T]) Back() (T, bool) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	return sd.d.Back()
}

// Size 返回队列长度
func (sd *SyncDeque[T]) Size() int {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	return sd.d.Size()
}

// IsEmpty 判断队列是否为空
func (sd *SyncDeque[T]) IsEmpty() bool {
	sd.mu.RLock()
	defer sd.mu.RUnlock()
	return sd.d.IsEmpty()
}

// Clear 清空队列
func (sd *SyncDeque[T]) Clear() {
	sd.mu.Lock()
	sd.d.Clear()
	sd.mu.Unlock()
}

// --- 线程安全优先级队列 ---

// SyncPriorityQueue 线程安全的优先级队列
type SyncPriorityQueue[T any] struct {
	pq *PriorityQueue[T]
	mu sync.RWMutex
}

// NewSyncPriorityQueue 创建线程安全的优先级队列
// less 函数定义优先级：返回 true 表示 a 优先级高于 b
func NewSyncPriorityQueue[T any](less func(a, b T) bool) *SyncPriorityQueue[T] {
	return &SyncPriorityQueue[T]{
		pq: NewPriorityQueue[T](less),
	}
}

// NewSyncMinHeap 创建线程安全的最小堆
func NewSyncMinHeap[T Ordered]() *SyncPriorityQueue[T] {
	return &SyncPriorityQueue[T]{
		pq: NewMinHeap[T](),
	}
}

// NewSyncMaxHeap 创建线程安全的最大堆
func NewSyncMaxHeap[T Ordered]() *SyncPriorityQueue[T] {
	return &SyncPriorityQueue[T]{
		pq: NewMaxHeap[T](),
	}
}

// Push 添加元素
func (spq *SyncPriorityQueue[T]) Push(items ...T) {
	spq.mu.Lock()
	spq.pq.Push(items...)
	spq.mu.Unlock()
}

// Pop 移除并返回优先级最高的元素
func (spq *SyncPriorityQueue[T]) Pop() (T, bool) {
	spq.mu.Lock()
	defer spq.mu.Unlock()
	return spq.pq.Pop()
}

// Peek 查看优先级最高的元素（不移除）
func (spq *SyncPriorityQueue[T]) Peek() (T, bool) {
	spq.mu.RLock()
	defer spq.mu.RUnlock()
	return spq.pq.Peek()
}

// Size 返回队列长度
func (spq *SyncPriorityQueue[T]) Size() int {
	spq.mu.RLock()
	defer spq.mu.RUnlock()
	return spq.pq.Size()
}

// Len 返回队列长度（Size 的别名）
func (spq *SyncPriorityQueue[T]) Len() int {
	return spq.Size()
}

// IsEmpty 判断队列是否为空
func (spq *SyncPriorityQueue[T]) IsEmpty() bool {
	spq.mu.RLock()
	defer spq.mu.RUnlock()
	return spq.pq.IsEmpty()
}

// Clear 清空队列
func (spq *SyncPriorityQueue[T]) Clear() {
	spq.mu.Lock()
	spq.pq.Clear()
	spq.mu.Unlock()
}

// ToSlice 转换为切片（按优先级顺序）
func (spq *SyncPriorityQueue[T]) ToSlice() []T {
	spq.mu.RLock()
	defer spq.mu.RUnlock()
	return spq.pq.ToSlice()
}
