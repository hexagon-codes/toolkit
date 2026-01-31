package poolx

import (
	"container/heap"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// Lock-Free Ring Buffer Queue (MPMC - Multi-Producer Multi-Consumer)
// ============================================================================

// ringBufferSize must be a power of 2 for efficient modulo operation
const defaultRingBufferSize = 1024

// ringNode represents a slot in the ring buffer
type ringNode[T any] struct {
	sequence atomic.Uint64
	value    T
}

// LockFreeQueue is a bounded MPMC lock-free queue based on ring buffer.
// Inspired by Dmitry Vyukov's bounded MPMC queue.
type LockFreeQueue[T any] struct {
	_        CacheLinePad
	buffer   []ringNode[T]
	mask     uint64
	_        CacheLinePad
	head     atomic.Uint64 // Consumer position
	_        CacheLinePad
	tail     atomic.Uint64 // Producer position
	_        CacheLinePad
	capacity uint64
}

// NewLockFreeQueue creates a new lock-free queue with the specified capacity.
// Capacity will be rounded up to the next power of 2.
func NewLockFreeQueue[T any](capacity int) *LockFreeQueue[T] {
	if capacity <= 0 {
		capacity = defaultRingBufferSize
	}

	// Round up to next power of 2
	cap64 := uint64(capacity)
	cap64--
	cap64 |= cap64 >> 1
	cap64 |= cap64 >> 2
	cap64 |= cap64 >> 4
	cap64 |= cap64 >> 8
	cap64 |= cap64 >> 16
	cap64 |= cap64 >> 32
	cap64++

	q := &LockFreeQueue[T]{
		buffer:   make([]ringNode[T], cap64),
		mask:     cap64 - 1,
		capacity: cap64,
	}

	// Initialize sequence numbers
	for i := uint64(0); i < cap64; i++ {
		q.buffer[i].sequence.Store(i)
	}

	return q
}

// Enqueue adds an item to the queue.
// Returns false if the queue is full.
func (q *LockFreeQueue[T]) Enqueue(item T) bool {
	var pos uint64
	var node *ringNode[T]

	for {
		pos = q.tail.Load()
		node = &q.buffer[pos&q.mask]
		seq := node.sequence.Load()
		diff := int64(seq) - int64(pos)

		if diff == 0 {
			// Slot is available for writing
			if q.tail.CompareAndSwap(pos, pos+1) {
				break
			}
		} else if diff < 0 {
			// Queue is full
			return false
		}
		// Another producer got here first, retry
		procyield(4)
	}

	// Write the value and update sequence
	node.value = item
	node.sequence.Store(pos + 1)
	return true
}

// Dequeue removes and returns an item from the queue.
// Returns false if the queue is empty.
func (q *LockFreeQueue[T]) Dequeue() (T, bool) {
	var zero T
	var pos uint64
	var node *ringNode[T]

	for {
		pos = q.head.Load()
		node = &q.buffer[pos&q.mask]
		seq := node.sequence.Load()
		diff := int64(seq) - int64(pos+1)

		if diff == 0 {
			// Slot has data ready to read
			if q.head.CompareAndSwap(pos, pos+1) {
				break
			}
		} else if diff < 0 {
			// Queue is empty
			return zero, false
		}
		// Another consumer got here first, retry
		procyield(4)
	}

	// Read the value and update sequence
	item := node.value
	var zeroVal T
	node.value = zeroVal // Clear reference for GC
	node.sequence.Store(pos + q.capacity)
	return item, true
}

// Len returns the approximate number of items in the queue.
// This is not exact due to concurrent modifications.
func (q *LockFreeQueue[T]) Len() int {
	tail := q.tail.Load()
	head := q.head.Load()
	if tail >= head {
		return int(tail - head)
	}
	return 0
}

// Cap returns the capacity of the queue.
func (q *LockFreeQueue[T]) Cap() int {
	return int(q.capacity)
}

// IsEmpty returns true if the queue appears empty.
func (q *LockFreeQueue[T]) IsEmpty() bool {
	return q.head.Load() >= q.tail.Load()
}

// IsFull returns true if the queue appears full.
func (q *LockFreeQueue[T]) IsFull() bool {
	return q.Len() >= int(q.capacity)
}

// ============================================================================
// Priority Queue Implementation
// ============================================================================

// Priority levels
const (
	PriorityLow    = 0
	PriorityNormal = 5
	PriorityHigh   = 10
)

// PriorityTask represents a task with priority
type PriorityTask struct {
	fn        func()
	priority  int
	submitted time.Time
	index     int // Index in the heap, managed by heap.Interface
}

// priorityHeap implements heap.Interface for priority-based task scheduling
type priorityHeap []*PriorityTask

func (h priorityHeap) Len() int { return len(h) }

func (h priorityHeap) Less(i, j int) bool {
	// Higher priority first
	if h[i].priority != h[j].priority {
		return h[i].priority > h[j].priority
	}
	// Same priority: earlier submitted first (FIFO within priority)
	return h[i].submitted.Before(h[j].submitted)
}

func (h priorityHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *priorityHeap) Push(x any) {
	n := len(*h)
	task := x.(*PriorityTask)
	task.index = n
	*h = append(*h, task)
}

func (h *priorityHeap) Pop() any {
	old := *h
	n := len(old)
	task := old[n-1]
	old[n-1] = nil // Avoid memory leak
	task.index = -1
	*h = old[0 : n-1]
	return task
}

// PriorityQueue is a thread-safe priority queue for tasks
type PriorityQueue struct {
	heap priorityHeap
	lock sync.Mutex
	cond *sync.Cond
	cap  int
}

// NewPriorityQueue creates a new priority queue with optional capacity.
// If cap <= 0, the queue is unbounded.
func NewPriorityQueue(cap int) *PriorityQueue {
	pq := &PriorityQueue{
		heap: make(priorityHeap, 0),
		cap:  cap,
	}
	pq.cond = sync.NewCond(&pq.lock)
	heap.Init(&pq.heap)
	return pq
}

// Push adds a task to the queue with the given priority.
// Returns false if the queue is full (when bounded).
func (pq *PriorityQueue) Push(fn func(), priority int) bool {
	pq.lock.Lock()
	defer pq.lock.Unlock()

	if pq.cap > 0 && len(pq.heap) >= pq.cap {
		return false
	}

	task := &PriorityTask{
		fn:        fn,
		priority:  priority,
		submitted: time.Now(),
	}
	heap.Push(&pq.heap, task)
	pq.cond.Signal()
	return true
}

// Pop removes and returns the highest priority task.
// Returns nil if the queue is empty.
func (pq *PriorityQueue) Pop() func() {
	pq.lock.Lock()
	defer pq.lock.Unlock()

	if len(pq.heap) == 0 {
		return nil
	}

	task := heap.Pop(&pq.heap).(*PriorityTask)
	return task.fn
}

// PopWait removes and returns the highest priority task, waiting if empty.
// Returns nil if the done channel is closed.
// 使用定时唤醒机制来检查 done 通道，避免每次等待都创建监听 goroutine。
func (pq *PriorityQueue) PopWait(done <-chan struct{}) func() {
	// 先不持锁检查 done
	select {
	case <-done:
		return nil
	default:
	}

	pq.lock.Lock()
	defer pq.lock.Unlock()

	// 启动一个后台定时器，定期唤醒等待者检查 done 通道
	// 这比每次循环创建 goroutine 更高效
	stopTimer := make(chan struct{})
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				pq.cond.Broadcast()
				return
			case <-stopTimer:
				return
			case <-ticker.C:
				pq.cond.Broadcast()
			}
		}
	}()
	defer close(stopTimer)

	for len(pq.heap) == 0 {
		// 检查是否已关闭
		select {
		case <-done:
			return nil
		default:
		}

		// 等待条件变量（会被定时器或 Push 唤醒）
		pq.cond.Wait()

		// 再次检查 done 通道
		select {
		case <-done:
			return nil
		default:
		}
	}

	task := heap.Pop(&pq.heap).(*PriorityTask)
	return task.fn
}

// Len returns the number of tasks in the queue.
func (pq *PriorityQueue) Len() int {
	pq.lock.Lock()
	defer pq.lock.Unlock()
	return len(pq.heap)
}

// IsEmpty returns true if the queue is empty.
func (pq *PriorityQueue) IsEmpty() bool {
	return pq.Len() == 0
}

// Clear removes all tasks from the queue.
func (pq *PriorityQueue) Clear() {
	pq.lock.Lock()
	defer pq.lock.Unlock()
	pq.heap = pq.heap[:0]
}

// Signal wakes up one waiting consumer.
func (pq *PriorityQueue) Signal() {
	pq.cond.Signal()
}

// Broadcast wakes up all waiting consumers.
func (pq *PriorityQueue) Broadcast() {
	pq.cond.Broadcast()
}

// ============================================================================
// Work Stealing Deque (Double-ended queue for local worker queues)
// ============================================================================

// WorkStealingDeque is a lock-free work-stealing deque.
// The owner pushes/pops from the bottom, thieves steal from the top.
type WorkStealingDeque[T any] struct {
	_      CacheLinePad
	bottom atomic.Int64 // Owner's end
	_      CacheLinePad
	top    atomic.Int64 // Thieves' end
	_      CacheLinePad
	buffer atomic.Pointer[dequeBuffer[T]]
	_      CacheLinePad
}

type dequeBuffer[T any] struct {
	items []atomic.Pointer[T]
	mask  int64
}

func newDequeBuffer[T any](capacity int64) *dequeBuffer[T] {
	// Round up to power of 2
	cap := int64(1)
	for cap < capacity {
		cap <<= 1
	}

	buf := &dequeBuffer[T]{
		items: make([]atomic.Pointer[T], cap),
		mask:  cap - 1,
	}
	return buf
}

// NewWorkStealingDeque creates a new work-stealing deque with initial capacity.
func NewWorkStealingDeque[T any](capacity int) *WorkStealingDeque[T] {
	d := &WorkStealingDeque[T]{}
	d.buffer.Store(newDequeBuffer[T](int64(capacity)))
	return d
}

// PushBottom adds an item to the bottom (owner only).
func (d *WorkStealingDeque[T]) PushBottom(item *T) {
	bottom := d.bottom.Load()
	top := d.top.Load()
	buf := d.buffer.Load()

	size := bottom - top
	if size >= int64(len(buf.items))-1 {
		// Grow buffer
		d.grow(buf, bottom, top)
		buf = d.buffer.Load()
	}

	buf.items[bottom&buf.mask].Store(item)
	d.bottom.Store(bottom + 1)
}

// PopBottom removes and returns an item from the bottom (owner only).
func (d *WorkStealingDeque[T]) PopBottom() *T {
	bottom := d.bottom.Load() - 1
	d.bottom.Store(bottom)

	top := d.top.Load()

	if bottom < top {
		d.bottom.Store(top)
		return nil
	}

	buf := d.buffer.Load()
	item := buf.items[bottom&buf.mask].Load()

	if bottom > top {
		return item
	}

	// Last item, need to race with steal
	if !d.top.CompareAndSwap(top, top+1) {
		item = nil
	}
	d.bottom.Store(top + 1)
	return item
}

// Steal removes and returns an item from the top (thieves).
func (d *WorkStealingDeque[T]) Steal() *T {
	top := d.top.Load()
	bottom := d.bottom.Load()

	if top >= bottom {
		return nil
	}

	buf := d.buffer.Load()
	item := buf.items[top&buf.mask].Load()

	if !d.top.CompareAndSwap(top, top+1) {
		return nil // Lost race with another thief or owner
	}

	return item
}

// Len returns the approximate size of the deque.
func (d *WorkStealingDeque[T]) Len() int {
	bottom := d.bottom.Load()
	top := d.top.Load()
	size := bottom - top
	if size < 0 {
		return 0
	}
	return int(size)
}

// IsEmpty returns true if the deque appears empty.
func (d *WorkStealingDeque[T]) IsEmpty() bool {
	return d.Len() == 0
}

// grow doubles the buffer capacity
func (d *WorkStealingDeque[T]) grow(oldBuf *dequeBuffer[T], bottom, top int64) {
	newCap := int64(len(oldBuf.items)) * 2
	newBuf := newDequeBuffer[T](newCap)

	// Copy items
	for i := top; i < bottom; i++ {
		item := oldBuf.items[i&oldBuf.mask].Load()
		newBuf.items[i&newBuf.mask].Store(item)
	}

	d.buffer.Store(newBuf)
}
