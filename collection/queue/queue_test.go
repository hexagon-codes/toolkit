package queue

import (
	"sync"
	"testing"
)

// --- Queue Tests ---

func TestNew(t *testing.T) {
	q := New(1, 2, 3)
	if q.Size() != 3 {
		t.Errorf("expected size 3, got %d", q.Size())
	}
}

func TestNewEmpty(t *testing.T) {
	q := New[int]()
	if !q.IsEmpty() {
		t.Error("expected empty queue")
	}
}

func TestNewWithCapacity(t *testing.T) {
	q := NewWithCapacity[int](100)
	if !q.IsEmpty() {
		t.Error("expected empty queue")
	}
}

func TestQueue_Enqueue(t *testing.T) {
	q := New[int]()
	q.Enqueue(1, 2, 3)
	if q.Size() != 3 {
		t.Errorf("expected size 3, got %d", q.Size())
	}
}

func TestQueue_EnqueueChaining(t *testing.T) {
	q := New[int]().Enqueue(1).Enqueue(2).Enqueue(3)
	if q.Size() != 3 {
		t.Errorf("expected size 3, got %d", q.Size())
	}
}

func TestQueue_Push(t *testing.T) {
	q := New[int]()
	q.Push(1, 2, 3)
	if q.Size() != 3 {
		t.Errorf("expected size 3, got %d", q.Size())
	}
}

func TestQueue_Dequeue(t *testing.T) {
	q := New(1, 2, 3)

	item, ok := q.Dequeue()
	if !ok {
		t.Error("Dequeue should succeed")
	}
	if item != 1 {
		t.Errorf("expected 1, got %d", item)
	}

	if q.Size() != 2 {
		t.Errorf("expected size 2, got %d", q.Size())
	}
}

func TestQueue_DequeueEmpty(t *testing.T) {
	q := New[int]()

	_, ok := q.Dequeue()
	if ok {
		t.Error("Dequeue on empty queue should return false")
	}
}

func TestQueue_Pop(t *testing.T) {
	q := New(1, 2, 3)

	item, ok := q.Pop()
	if !ok {
		t.Error("Pop should succeed")
	}
	if item != 1 {
		t.Errorf("expected 1, got %d", item)
	}
}

func TestQueue_Peek(t *testing.T) {
	q := New(1, 2, 3)

	item, ok := q.Peek()
	if !ok {
		t.Error("Peek should succeed")
	}
	if item != 1 {
		t.Errorf("expected 1, got %d", item)
	}

	// Size should not change
	if q.Size() != 3 {
		t.Errorf("expected size 3, got %d", q.Size())
	}
}

func TestQueue_PeekEmpty(t *testing.T) {
	q := New[int]()

	_, ok := q.Peek()
	if ok {
		t.Error("Peek on empty queue should return false")
	}
}

func TestQueue_Front(t *testing.T) {
	q := New(1, 2, 3)

	item, ok := q.Front()
	if !ok || item != 1 {
		t.Error("Front should return first element")
	}
}

func TestQueue_Back(t *testing.T) {
	q := New(1, 2, 3)

	item, ok := q.Back()
	if !ok {
		t.Error("Back should succeed")
	}
	if item != 3 {
		t.Errorf("expected 3, got %d", item)
	}
}

func TestQueue_BackEmpty(t *testing.T) {
	q := New[int]()

	_, ok := q.Back()
	if ok {
		t.Error("Back on empty queue should return false")
	}
}

func TestQueue_SizeAndLen(t *testing.T) {
	q := New(1, 2, 3)

	if q.Size() != 3 {
		t.Errorf("Size() expected 3, got %d", q.Size())
	}
	if q.Len() != 3 {
		t.Errorf("Len() expected 3, got %d", q.Len())
	}
}

func TestQueue_IsEmpty(t *testing.T) {
	q := New[int]()
	if !q.IsEmpty() {
		t.Error("should be empty")
	}

	q.Enqueue(1)
	if q.IsEmpty() {
		t.Error("should not be empty")
	}
}

func TestQueue_Clear(t *testing.T) {
	q := New(1, 2, 3)
	q.Clear()

	if !q.IsEmpty() {
		t.Error("should be empty after clear")
	}
}

func TestQueue_ToSlice(t *testing.T) {
	q := New(1, 2, 3)
	slice := q.ToSlice()

	if len(slice) != 3 {
		t.Errorf("expected slice length 3, got %d", len(slice))
	}

	// Verify order
	expected := []int{1, 2, 3}
	for i, v := range slice {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestQueue_Values(t *testing.T) {
	q := New(1, 2, 3)
	values := q.Values()

	if len(values) != 3 {
		t.Errorf("expected 3 values, got %d", len(values))
	}
}

func TestQueue_ForEach(t *testing.T) {
	q := New(1, 2, 3)
	sum := 0
	q.ForEach(func(item int) {
		sum += item
	})

	if sum != 6 {
		t.Errorf("expected sum 6, got %d", sum)
	}
}

func TestQueue_Filter(t *testing.T) {
	q := New(1, 2, 3, 4, 5, 6)
	even := q.Filter(func(n int) bool {
		return n%2 == 0
	})

	if even.Size() != 3 {
		t.Errorf("expected 3 even numbers, got %d", even.Size())
	}
}

func TestQueue_Clone(t *testing.T) {
	q := New(1, 2, 3)
	cloned := q.Clone()

	if cloned.Size() != q.Size() {
		t.Error("cloned queue should have same size")
	}

	// Modify original
	q.Enqueue(4)
	if cloned.Size() == q.Size() {
		t.Error("cloned should not be affected by original")
	}
}

func TestQueue_FIFO(t *testing.T) {
	q := New[int]()

	// Enqueue in order
	for i := 1; i <= 5; i++ {
		q.Enqueue(i)
	}

	// Dequeue should return in same order
	for i := 1; i <= 5; i++ {
		item, ok := q.Dequeue()
		if !ok {
			t.Fatalf("Dequeue failed at %d", i)
		}
		if item != i {
			t.Errorf("expected %d, got %d", i, item)
		}
	}
}

// --- Deque Tests ---

func TestNewDeque(t *testing.T) {
	d := NewDeque(1, 2, 3)
	if d.Size() != 3 {
		t.Errorf("expected size 3, got %d", d.Size())
	}
}

func TestNewDequeEmpty(t *testing.T) {
	d := NewDeque[int]()
	if !d.IsEmpty() {
		t.Error("expected empty deque")
	}
}

func TestNewDequeWithCapacity(t *testing.T) {
	d := NewDequeWithCapacity[int](100)
	if !d.IsEmpty() {
		t.Error("expected empty deque")
	}
}

func TestDeque_PushBack(t *testing.T) {
	d := NewDeque[int]()
	d.PushBack(1, 2, 3)

	back, _ := d.Back()
	if back != 3 {
		t.Errorf("expected back 3, got %d", back)
	}
}

func TestDeque_PushFront(t *testing.T) {
	d := NewDeque[int]()
	d.PushFront(1, 2, 3)

	front, _ := d.Front()
	if front != 1 {
		t.Errorf("expected front 1, got %d", front)
	}
}

func TestDeque_PopBack(t *testing.T) {
	d := NewDeque(1, 2, 3)

	item, ok := d.PopBack()
	if !ok {
		t.Error("PopBack should succeed")
	}
	if item != 3 {
		t.Errorf("expected 3, got %d", item)
	}
	if d.Size() != 2 {
		t.Errorf("expected size 2, got %d", d.Size())
	}
}

func TestDeque_PopBackEmpty(t *testing.T) {
	d := NewDeque[int]()

	_, ok := d.PopBack()
	if ok {
		t.Error("PopBack on empty deque should return false")
	}
}

func TestDeque_PopFront(t *testing.T) {
	d := NewDeque(1, 2, 3)

	item, ok := d.PopFront()
	if !ok {
		t.Error("PopFront should succeed")
	}
	if item != 1 {
		t.Errorf("expected 1, got %d", item)
	}
	if d.Size() != 2 {
		t.Errorf("expected size 2, got %d", d.Size())
	}
}

func TestDeque_PopFrontEmpty(t *testing.T) {
	d := NewDeque[int]()

	_, ok := d.PopFront()
	if ok {
		t.Error("PopFront on empty deque should return false")
	}
}

func TestDeque_Front(t *testing.T) {
	d := NewDeque(1, 2, 3)

	item, ok := d.Front()
	if !ok || item != 1 {
		t.Error("Front should return first element")
	}
}

func TestDeque_FrontEmpty(t *testing.T) {
	d := NewDeque[int]()

	_, ok := d.Front()
	if ok {
		t.Error("Front on empty deque should return false")
	}
}

func TestDeque_Back(t *testing.T) {
	d := NewDeque(1, 2, 3)

	item, ok := d.Back()
	if !ok || item != 3 {
		t.Error("Back should return last element")
	}
}

func TestDeque_BackEmpty(t *testing.T) {
	d := NewDeque[int]()

	_, ok := d.Back()
	if ok {
		t.Error("Back on empty deque should return false")
	}
}

func TestDeque_SizeAndLen(t *testing.T) {
	d := NewDeque(1, 2, 3)

	if d.Size() != 3 {
		t.Errorf("Size() expected 3, got %d", d.Size())
	}
	if d.Len() != 3 {
		t.Errorf("Len() expected 3, got %d", d.Len())
	}
}

func TestDeque_IsEmpty(t *testing.T) {
	d := NewDeque[int]()
	if !d.IsEmpty() {
		t.Error("should be empty")
	}

	d.PushBack(1)
	if d.IsEmpty() {
		t.Error("should not be empty")
	}
}

func TestDeque_Clear(t *testing.T) {
	d := NewDeque(1, 2, 3)
	d.Clear()

	if !d.IsEmpty() {
		t.Error("should be empty after clear")
	}
}

func TestDeque_ToSlice(t *testing.T) {
	d := NewDeque(1, 2, 3)
	slice := d.ToSlice()

	if len(slice) != 3 {
		t.Errorf("expected slice length 3, got %d", len(slice))
	}
}

func TestDeque_Clone(t *testing.T) {
	d := NewDeque(1, 2, 3)
	cloned := d.Clone()

	if cloned.Size() != d.Size() {
		t.Error("cloned deque should have same size")
	}

	// Modify original
	d.PushBack(4)
	if cloned.Size() == d.Size() {
		t.Error("cloned should not be affected by original")
	}
}

func TestDeque_Chaining(t *testing.T) {
	d := NewDeque[int]().PushBack(1, 2).PushFront(0)
	if d.Size() != 3 {
		t.Errorf("expected size 3, got %d", d.Size())
	}

	front, _ := d.Front()
	if front != 0 {
		t.Errorf("expected front 0, got %d", front)
	}
}

// --- PriorityQueue Tests ---

func TestNewPriorityQueue(t *testing.T) {
	pq := NewPriorityQueue[int](func(a, b int) bool {
		return a < b
	})

	if pq == nil {
		t.Fatal("PriorityQueue is nil")
	}
	if !pq.IsEmpty() {
		t.Error("expected empty priority queue")
	}
}

func TestNewMinHeap(t *testing.T) {
	pq := NewMinHeap[int]()
	pq.Push(3, 1, 4, 1, 5, 9, 2, 6)

	// Should pop in ascending order
	prev := -1
	for !pq.IsEmpty() {
		item, ok := pq.Pop()
		if !ok {
			t.Fatal("Pop should succeed")
		}
		if item < prev {
			t.Errorf("min heap order violated: %d after %d", item, prev)
		}
		prev = item
	}
}

func TestNewMaxHeap(t *testing.T) {
	pq := NewMaxHeap[int]()
	pq.Push(3, 1, 4, 1, 5, 9, 2, 6)

	// Should pop in descending order
	prev := 100
	for !pq.IsEmpty() {
		item, ok := pq.Pop()
		if !ok {
			t.Fatal("Pop should succeed")
		}
		if item > prev {
			t.Errorf("max heap order violated: %d after %d", item, prev)
		}
		prev = item
	}
}

func TestPriorityQueue_Push(t *testing.T) {
	pq := NewMinHeap[int]()
	pq.Push(5, 3, 7, 1)

	if pq.Size() != 4 {
		t.Errorf("expected size 4, got %d", pq.Size())
	}

	// Min should be 1
	item, _ := pq.Peek()
	if item != 1 {
		t.Errorf("expected peek 1, got %d", item)
	}
}

func TestPriorityQueue_PushChaining(t *testing.T) {
	pq := NewMinHeap[int]().Push(5).Push(3).Push(1)
	if pq.Size() != 3 {
		t.Errorf("expected size 3, got %d", pq.Size())
	}
}

func TestPriorityQueue_Pop(t *testing.T) {
	pq := NewMinHeap[int]()
	pq.Push(5, 3, 1)

	item, ok := pq.Pop()
	if !ok {
		t.Error("Pop should succeed")
	}
	if item != 1 {
		t.Errorf("expected 1, got %d", item)
	}
	if pq.Size() != 2 {
		t.Errorf("expected size 2, got %d", pq.Size())
	}
}

func TestPriorityQueue_PopEmpty(t *testing.T) {
	pq := NewMinHeap[int]()

	_, ok := pq.Pop()
	if ok {
		t.Error("Pop on empty queue should return false")
	}
}

func TestPriorityQueue_Peek(t *testing.T) {
	pq := NewMinHeap[int]()
	pq.Push(5, 3, 1)

	item, ok := pq.Peek()
	if !ok {
		t.Error("Peek should succeed")
	}
	if item != 1 {
		t.Errorf("expected 1, got %d", item)
	}

	// Size should not change
	if pq.Size() != 3 {
		t.Errorf("expected size 3, got %d", pq.Size())
	}
}

func TestPriorityQueue_PeekEmpty(t *testing.T) {
	pq := NewMinHeap[int]()

	_, ok := pq.Peek()
	if ok {
		t.Error("Peek on empty queue should return false")
	}
}

func TestPriorityQueue_SizeAndLen(t *testing.T) {
	pq := NewMinHeap[int]()
	pq.Push(1, 2, 3)

	if pq.Size() != 3 {
		t.Errorf("Size() expected 3, got %d", pq.Size())
	}
	if pq.Len() != 3 {
		t.Errorf("Len() expected 3, got %d", pq.Len())
	}
}

func TestPriorityQueue_IsEmpty(t *testing.T) {
	pq := NewMinHeap[int]()
	if !pq.IsEmpty() {
		t.Error("should be empty")
	}

	pq.Push(1)
	if pq.IsEmpty() {
		t.Error("should not be empty")
	}
}

func TestPriorityQueue_Clear(t *testing.T) {
	pq := NewMinHeap[int]()
	pq.Push(1, 2, 3)
	pq.Clear()

	if !pq.IsEmpty() {
		t.Error("should be empty after clear")
	}
}

func TestPriorityQueue_ToSlice(t *testing.T) {
	pq := NewMinHeap[int]()
	pq.Push(5, 3, 1, 4, 2)

	slice := pq.ToSlice()

	// Should be in priority order
	expected := []int{1, 2, 3, 4, 5}
	for i, v := range slice {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}

	// Original should not be modified
	if pq.Size() != 5 {
		t.Errorf("original size should be 5, got %d", pq.Size())
	}
}

func TestPriorityQueue_CustomPriority(t *testing.T) {
	type Task struct {
		Name     string
		Priority int
	}

	// Higher priority value = higher priority
	pq := NewPriorityQueue[Task](func(a, b Task) bool {
		return a.Priority > b.Priority
	})

	pq.Push(
		Task{"low", 1},
		Task{"high", 10},
		Task{"medium", 5},
	)

	item, _ := pq.Pop()
	if item.Name != "high" {
		t.Errorf("expected 'high', got '%s'", item.Name)
	}
}

func TestPriorityQueue_StringHeap(t *testing.T) {
	pq := NewMinHeap[string]()
	pq.Push("banana", "apple", "cherry")

	item, _ := pq.Pop()
	if item != "apple" {
		t.Errorf("expected 'apple', got '%s'", item)
	}
}

// --- SyncQueue Tests ---

func TestSyncQueue_Basic(t *testing.T) {
	sq := NewSyncQueue[int]()

	sq.Enqueue(1, 2, 3)
	if sq.Size() != 3 {
		t.Errorf("expected size 3, got %d", sq.Size())
	}

	item, ok := sq.Dequeue()
	if !ok || item != 1 {
		t.Error("Dequeue should return first element")
	}

	item, ok = sq.Peek()
	if !ok || item != 2 {
		t.Error("Peek should return current first element")
	}
}

func TestSyncQueue_Concurrent(t *testing.T) {
	sq := NewSyncQueue[int]()
	var wg sync.WaitGroup

	// Concurrent enqueue
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			sq.Enqueue(n)
		}(i)
	}

	wg.Wait()

	if sq.Size() != 100 {
		t.Errorf("expected size 100, got %d", sq.Size())
	}
}

func TestSyncQueue_ConcurrentDequeue(t *testing.T) {
	sq := NewSyncQueue[int]()

	// Pre-populate
	for i := 0; i < 100; i++ {
		sq.Enqueue(i)
	}

	var wg sync.WaitGroup
	count := 0
	var mu sync.Mutex

	// Concurrent dequeue
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := sq.Dequeue(); ok {
				mu.Lock()
				count++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if count != 100 {
		t.Errorf("expected 100 successful dequeues, got %d", count)
	}
}

func TestSyncQueue_IsEmpty(t *testing.T) {
	sq := NewSyncQueue[int]()

	if !sq.IsEmpty() {
		t.Error("should be empty")
	}

	sq.Enqueue(1)
	if sq.IsEmpty() {
		t.Error("should not be empty")
	}
}

func TestSyncQueue_Clear(t *testing.T) {
	sq := NewSyncQueue[int]()
	sq.Enqueue(1, 2, 3)
	sq.Clear()

	if !sq.IsEmpty() {
		t.Error("should be empty after clear")
	}
}

// --- SyncDeque Tests ---

func TestSyncDeque_Basic(t *testing.T) {
	sd := NewSyncDeque[int]()

	sd.PushBack(1, 2, 3)
	sd.PushFront(0)

	if sd.Size() != 4 {
		t.Errorf("expected size 4, got %d", sd.Size())
	}

	front, ok := sd.Front()
	if !ok || front != 0 {
		t.Error("Front should return 0")
	}

	back, ok := sd.Back()
	if !ok || back != 3 {
		t.Error("Back should return 3")
	}
}

func TestSyncDeque_Concurrent(t *testing.T) {
	sd := NewSyncDeque[int]()
	var wg sync.WaitGroup

	// Concurrent push from both ends
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			sd.PushBack(n)
		}(i)
		go func(n int) {
			defer wg.Done()
			sd.PushFront(n)
		}(i)
	}

	wg.Wait()

	if sd.Size() != 100 {
		t.Errorf("expected size 100, got %d", sd.Size())
	}
}

func TestSyncDeque_PopFrontBack(t *testing.T) {
	sd := NewSyncDeque[int]()
	sd.PushBack(1, 2, 3)

	item, ok := sd.PopFront()
	if !ok || item != 1 {
		t.Error("PopFront should return 1")
	}

	item, ok = sd.PopBack()
	if !ok || item != 3 {
		t.Error("PopBack should return 3")
	}
}

func TestSyncDeque_IsEmpty(t *testing.T) {
	sd := NewSyncDeque[int]()

	if !sd.IsEmpty() {
		t.Error("should be empty")
	}

	sd.PushBack(1)
	if sd.IsEmpty() {
		t.Error("should not be empty")
	}
}

func TestSyncDeque_Clear(t *testing.T) {
	sd := NewSyncDeque[int]()
	sd.PushBack(1, 2, 3)
	sd.Clear()

	if !sd.IsEmpty() {
		t.Error("should be empty after clear")
	}
}

// --- Benchmarks ---

func BenchmarkQueue_Enqueue(b *testing.B) {
	q := New[int]()
	for i := 0; i < b.N; i++ {
		q.Enqueue(i)
	}
}

func BenchmarkQueue_Dequeue(b *testing.B) {
	q := New[int]()
	for i := 0; i < b.N; i++ {
		q.Enqueue(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Dequeue()
	}
}

func BenchmarkPriorityQueue_Push(b *testing.B) {
	pq := NewMinHeap[int]()
	for i := 0; i < b.N; i++ {
		pq.Push(i)
	}
}

func BenchmarkPriorityQueue_Pop(b *testing.B) {
	pq := NewMinHeap[int]()
	for i := 0; i < b.N; i++ {
		pq.Push(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pq.Pop()
	}
}

func BenchmarkSyncQueue_Enqueue(b *testing.B) {
	sq := NewSyncQueue[int]()
	for i := 0; i < b.N; i++ {
		sq.Enqueue(i)
	}
}
