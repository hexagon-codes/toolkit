package stack

import (
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	s := New(1, 2, 3)
	if s.Size() != 3 {
		t.Errorf("expected size 3, got %d", s.Size())
	}
}

func TestNewEmpty(t *testing.T) {
	s := New[int]()
	if !s.IsEmpty() {
		t.Error("expected empty stack")
	}
}

func TestNewWithCapacity(t *testing.T) {
	s := NewWithCapacity[int](100)
	if !s.IsEmpty() {
		t.Error("expected empty stack")
	}
}

func TestStack_Push(t *testing.T) {
	s := New[int]()
	s.Push(1, 2, 3)

	if s.Size() != 3 {
		t.Errorf("expected size 3, got %d", s.Size())
	}

	// Check LIFO order
	top, _ := s.Peek()
	if top != 3 {
		t.Errorf("expected top 3, got %d", top)
	}
}

func TestStack_PushChaining(t *testing.T) {
	s := New[int]().Push(1).Push(2).Push(3)
	if s.Size() != 3 {
		t.Errorf("expected size 3, got %d", s.Size())
	}
}

func TestStack_Pop(t *testing.T) {
	s := New(1, 2, 3)

	item, ok := s.Pop()
	if !ok {
		t.Error("Pop should succeed")
	}
	if item != 3 {
		t.Errorf("expected 3, got %d", item)
	}

	if s.Size() != 2 {
		t.Errorf("expected size 2, got %d", s.Size())
	}
}

func TestStack_PopEmpty(t *testing.T) {
	s := New[int]()

	_, ok := s.Pop()
	if ok {
		t.Error("Pop on empty stack should return false")
	}
}

func TestStack_Peek(t *testing.T) {
	s := New(1, 2, 3)

	item, ok := s.Peek()
	if !ok {
		t.Error("Peek should succeed")
	}
	if item != 3 {
		t.Errorf("expected 3, got %d", item)
	}

	// Size should not change
	if s.Size() != 3 {
		t.Errorf("expected size 3, got %d", s.Size())
	}
}

func TestStack_PeekEmpty(t *testing.T) {
	s := New[int]()

	_, ok := s.Peek()
	if ok {
		t.Error("Peek on empty stack should return false")
	}
}

func TestStack_Top(t *testing.T) {
	s := New(1, 2, 3)

	item, ok := s.Top()
	if !ok || item != 3 {
		t.Error("Top should return top element")
	}
}

func TestStack_SizeAndLen(t *testing.T) {
	s := New(1, 2, 3)

	if s.Size() != 3 {
		t.Errorf("Size() expected 3, got %d", s.Size())
	}
	if s.Len() != 3 {
		t.Errorf("Len() expected 3, got %d", s.Len())
	}
}

func TestStack_IsEmpty(t *testing.T) {
	s := New[int]()
	if !s.IsEmpty() {
		t.Error("should be empty")
	}

	s.Push(1)
	if s.IsEmpty() {
		t.Error("should not be empty")
	}
}

func TestStack_Clear(t *testing.T) {
	s := New(1, 2, 3)
	s.Clear()

	if !s.IsEmpty() {
		t.Error("should be empty after clear")
	}
}

func TestStack_ToSlice(t *testing.T) {
	s := New(1, 2, 3)
	slice := s.ToSlice()

	// From bottom to top
	expected := []int{1, 2, 3}
	for i, v := range slice {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestStack_ToSliceReverse(t *testing.T) {
	s := New(1, 2, 3)
	slice := s.ToSliceReverse()

	// From top to bottom
	expected := []int{3, 2, 1}
	for i, v := range slice {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestStack_Values(t *testing.T) {
	s := New(1, 2, 3)
	values := s.Values()

	if len(values) != 3 {
		t.Errorf("expected 3 values, got %d", len(values))
	}
}

func TestStack_ForEach(t *testing.T) {
	s := New(1, 2, 3)
	result := make([]int, 0)
	s.ForEach(func(item int) {
		result = append(result, item)
	})

	// From bottom to top
	expected := []int{1, 2, 3}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestStack_ForEachReverse(t *testing.T) {
	s := New(1, 2, 3)
	result := make([]int, 0)
	s.ForEachReverse(func(item int) {
		result = append(result, item)
	})

	// From top to bottom
	expected := []int{3, 2, 1}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestStack_Clone(t *testing.T) {
	s := New(1, 2, 3)
	cloned := s.Clone()

	if cloned.Size() != s.Size() {
		t.Error("cloned stack should have same size")
	}

	// Modify original
	s.Push(4)
	if cloned.Size() == s.Size() {
		t.Error("cloned should not be affected by original")
	}
}

func TestStack_Contains(t *testing.T) {
	s := New(1, 2, 3, 4, 5)

	if !s.Contains(func(n int) bool { return n == 3 }) {
		t.Error("should contain 3")
	}
	if s.Contains(func(n int) bool { return n == 10 }) {
		t.Error("should not contain 10")
	}
}

func TestStack_Filter(t *testing.T) {
	s := New(1, 2, 3, 4, 5, 6)

	even := s.Filter(func(n int) bool { return n%2 == 0 })
	if even.Size() != 3 {
		t.Errorf("expected 3 even numbers, got %d", even.Size())
	}
}

func TestStack_PopN(t *testing.T) {
	s := New(1, 2, 3, 4, 5)

	items := s.PopN(3)
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}

	// Should be in LIFO order
	expected := []int{5, 4, 3}
	for i, v := range items {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}

	if s.Size() != 2 {
		t.Errorf("expected size 2, got %d", s.Size())
	}
}

func TestStack_PopNZero(t *testing.T) {
	s := New(1, 2, 3)

	items := s.PopN(0)
	if items != nil {
		t.Error("PopN(0) should return nil")
	}

	items = s.PopN(-1)
	if items != nil {
		t.Error("PopN(-1) should return nil")
	}
}

func TestStack_PopNMoreThanSize(t *testing.T) {
	s := New(1, 2, 3)

	items := s.PopN(10)
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}

	if !s.IsEmpty() {
		t.Error("stack should be empty")
	}
}

func TestStack_PeekN(t *testing.T) {
	s := New(1, 2, 3, 4, 5)

	items := s.PeekN(3)
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}

	// Should be in LIFO order
	expected := []int{5, 4, 3}
	for i, v := range items {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}

	// Size should not change
	if s.Size() != 5 {
		t.Errorf("expected size 5, got %d", s.Size())
	}
}

func TestStack_PeekNZero(t *testing.T) {
	s := New(1, 2, 3)

	items := s.PeekN(0)
	if items != nil {
		t.Error("PeekN(0) should return nil")
	}

	items = s.PeekN(-1)
	if items != nil {
		t.Error("PeekN(-1) should return nil")
	}
}

func TestStack_PeekNMoreThanSize(t *testing.T) {
	s := New(1, 2, 3)

	items := s.PeekN(10)
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}

	// Stack should be unchanged
	if s.Size() != 3 {
		t.Errorf("expected size 3, got %d", s.Size())
	}
}

func TestStack_Reverse(t *testing.T) {
	s := New(1, 2, 3, 4, 5)
	s.Reverse()

	// Now when we pop, we should get 1, 2, 3, 4, 5
	expected := []int{1, 2, 3, 4, 5}
	for i := 0; i < 5; i++ {
		item, _ := s.Pop()
		if item != expected[i] {
			t.Errorf("expected %d, got %d", expected[i], item)
		}
	}
}

func TestStack_ReverseEmpty(t *testing.T) {
	s := New[int]()
	s.Reverse() // Should not panic
}

func TestStack_LIFO(t *testing.T) {
	s := New[int]()

	// Push 1, 2, 3, 4, 5
	for i := 1; i <= 5; i++ {
		s.Push(i)
	}

	// Pop should return 5, 4, 3, 2, 1
	for i := 5; i >= 1; i-- {
		item, ok := s.Pop()
		if !ok {
			t.Fatalf("Pop failed at %d", i)
		}
		if item != i {
			t.Errorf("expected %d, got %d", i, item)
		}
	}
}

// --- SyncStack Tests ---

func TestSyncStack_Basic(t *testing.T) {
	ss := NewSyncStack[int]()

	ss.Push(1, 2, 3)
	if ss.Size() != 3 {
		t.Errorf("expected size 3, got %d", ss.Size())
	}

	item, ok := ss.Peek()
	if !ok || item != 3 {
		t.Error("Peek should return 3")
	}

	item, ok = ss.Pop()
	if !ok || item != 3 {
		t.Error("Pop should return 3")
	}
}

func TestSyncStack_IsEmpty(t *testing.T) {
	ss := NewSyncStack[int]()

	if !ss.IsEmpty() {
		t.Error("should be empty")
	}

	ss.Push(1)
	if ss.IsEmpty() {
		t.Error("should not be empty")
	}
}

func TestSyncStack_Clear(t *testing.T) {
	ss := NewSyncStack[int]()
	ss.Push(1, 2, 3)
	ss.Clear()

	if !ss.IsEmpty() {
		t.Error("should be empty after clear")
	}
}

func TestSyncStack_ToSlice(t *testing.T) {
	ss := NewSyncStack[int]()
	ss.Push(1, 2, 3)

	slice := ss.ToSlice()
	if len(slice) != 3 {
		t.Errorf("expected 3 elements, got %d", len(slice))
	}
}

func TestSyncStack_PopN(t *testing.T) {
	ss := NewSyncStack[int]()
	ss.Push(1, 2, 3, 4, 5)

	items := ss.PopN(3)
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}

	if ss.Size() != 2 {
		t.Errorf("expected size 2, got %d", ss.Size())
	}
}

func TestSyncStack_Concurrent(t *testing.T) {
	ss := NewSyncStack[int]()
	var wg sync.WaitGroup

	// Concurrent push
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ss.Push(n)
		}(i)
	}

	wg.Wait()

	if ss.Size() != 100 {
		t.Errorf("expected size 100, got %d", ss.Size())
	}
}

func TestSyncStack_ConcurrentPop(t *testing.T) {
	ss := NewSyncStack[int]()

	// Pre-populate
	for i := 0; i < 100; i++ {
		ss.Push(i)
	}

	var wg sync.WaitGroup
	count := 0
	var mu sync.Mutex

	// Concurrent pop
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := ss.Pop(); ok {
				mu.Lock()
				count++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if count != 100 {
		t.Errorf("expected 100 successful pops, got %d", count)
	}
}

// --- Benchmarks ---

func BenchmarkStack_Push(b *testing.B) {
	s := New[int]()
	for i := 0; i < b.N; i++ {
		s.Push(i)
	}
}

func BenchmarkStack_Pop(b *testing.B) {
	s := New[int]()
	for i := 0; i < b.N; i++ {
		s.Push(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Pop()
	}
}

func BenchmarkSyncStack_Push(b *testing.B) {
	ss := NewSyncStack[int]()
	for i := 0; i < b.N; i++ {
		ss.Push(i)
	}
}
