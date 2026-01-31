package list

import (
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	l := New(1, 2, 3)
	if l.Len() != 3 {
		t.Errorf("expected len 3, got %d", l.Len())
	}
}

func TestNewEmpty(t *testing.T) {
	l := New[int]()
	if !l.IsEmpty() {
		t.Error("expected empty list")
	}
}

func TestList_PushFront(t *testing.T) {
	l := New[int]()
	l.PushFront(3)
	l.PushFront(2)
	l.PushFront(1)

	if l.Len() != 3 {
		t.Errorf("expected len 3, got %d", l.Len())
	}

	// Check order: 1, 2, 3
	slice := l.ToSlice()
	expected := []int{1, 2, 3}
	for i, v := range slice {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestList_PushBack(t *testing.T) {
	l := New[int]()
	l.PushBack(1)
	l.PushBack(2)
	l.PushBack(3)

	if l.Len() != 3 {
		t.Errorf("expected len 3, got %d", l.Len())
	}

	// Check order: 1, 2, 3
	slice := l.ToSlice()
	expected := []int{1, 2, 3}
	for i, v := range slice {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestList_Front(t *testing.T) {
	l := New(1, 2, 3)

	front := l.Front()
	if front == nil {
		t.Fatal("Front should not be nil")
	}
	if front.Value != 1 {
		t.Errorf("expected front value 1, got %d", front.Value)
	}
}

func TestList_FrontEmpty(t *testing.T) {
	l := New[int]()

	if l.Front() != nil {
		t.Error("Front on empty list should return nil")
	}
}

func TestList_Back(t *testing.T) {
	l := New(1, 2, 3)

	back := l.Back()
	if back == nil {
		t.Fatal("Back should not be nil")
	}
	if back.Value != 3 {
		t.Errorf("expected back value 3, got %d", back.Value)
	}
}

func TestList_BackEmpty(t *testing.T) {
	l := New[int]()

	if l.Back() != nil {
		t.Error("Back on empty list should return nil")
	}
}

func TestList_PopFront(t *testing.T) {
	l := New(1, 2, 3)

	val, ok := l.PopFront()
	if !ok {
		t.Error("PopFront should succeed")
	}
	if val != 1 {
		t.Errorf("expected 1, got %d", val)
	}
	if l.Len() != 2 {
		t.Errorf("expected len 2, got %d", l.Len())
	}
}

func TestList_PopFrontEmpty(t *testing.T) {
	l := New[int]()

	_, ok := l.PopFront()
	if ok {
		t.Error("PopFront on empty list should return false")
	}
}

func TestList_PopBack(t *testing.T) {
	l := New(1, 2, 3)

	val, ok := l.PopBack()
	if !ok {
		t.Error("PopBack should succeed")
	}
	if val != 3 {
		t.Errorf("expected 3, got %d", val)
	}
	if l.Len() != 2 {
		t.Errorf("expected len 2, got %d", l.Len())
	}
}

func TestList_PopBackEmpty(t *testing.T) {
	l := New[int]()

	_, ok := l.PopBack()
	if ok {
		t.Error("PopBack on empty list should return false")
	}
}

func TestList_Remove(t *testing.T) {
	l := New(1, 2, 3)

	// Remove middle element
	n := l.Front().Next()
	val := l.Remove(n)

	if val != 2 {
		t.Errorf("expected removed value 2, got %d", val)
	}
	if l.Len() != 2 {
		t.Errorf("expected len 2, got %d", l.Len())
	}

	// Check remaining: 1, 3
	slice := l.ToSlice()
	expected := []int{1, 3}
	for i, v := range slice {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestList_RemoveInvalidNode(t *testing.T) {
	l1 := New(1, 2, 3)
	l2 := New(4, 5, 6)

	// Try to remove node from different list
	n := l2.Front()
	l1.Remove(n)

	// l1 should be unchanged
	if l1.Len() != 3 {
		t.Errorf("l1 should be unchanged, got len %d", l1.Len())
	}
}

func TestList_InsertBefore(t *testing.T) {
	l := New(1, 3)

	mark := l.Back()
	l.InsertBefore(2, mark)

	slice := l.ToSlice()
	expected := []int{1, 2, 3}
	for i, v := range slice {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestList_InsertBeforeInvalidMark(t *testing.T) {
	l1 := New(1, 2)
	l2 := New(3, 4)

	mark := l2.Front()
	n := l1.InsertBefore(5, mark)

	if n != nil {
		t.Error("InsertBefore with invalid mark should return nil")
	}
}

func TestList_InsertAfter(t *testing.T) {
	l := New(1, 3)

	mark := l.Front()
	l.InsertAfter(2, mark)

	slice := l.ToSlice()
	expected := []int{1, 2, 3}
	for i, v := range slice {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestList_InsertAfterInvalidMark(t *testing.T) {
	l1 := New(1, 2)
	l2 := New(3, 4)

	mark := l2.Front()
	n := l1.InsertAfter(5, mark)

	if n != nil {
		t.Error("InsertAfter with invalid mark should return nil")
	}
}

func TestList_MoveToFront(t *testing.T) {
	l := New(1, 2, 3)

	// Move 3 to front
	n := l.Back()
	l.MoveToFront(n)

	slice := l.ToSlice()
	expected := []int{3, 1, 2}
	for i, v := range slice {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestList_MoveToFrontAlreadyFront(t *testing.T) {
	l := New(1, 2, 3)

	n := l.Front()
	l.MoveToFront(n)

	// Should be unchanged
	slice := l.ToSlice()
	expected := []int{1, 2, 3}
	for i, v := range slice {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestList_MoveToBack(t *testing.T) {
	l := New(1, 2, 3)

	// Move 1 to back
	n := l.Front()
	l.MoveToBack(n)

	slice := l.ToSlice()
	expected := []int{2, 3, 1}
	for i, v := range slice {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestList_MoveToBackAlreadyBack(t *testing.T) {
	l := New(1, 2, 3)

	n := l.Back()
	l.MoveToBack(n)

	// Should be unchanged
	slice := l.ToSlice()
	expected := []int{1, 2, 3}
	for i, v := range slice {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestList_MoveBefore(t *testing.T) {
	l := New(1, 2, 3)

	// Move 3 before 2
	n := l.Back()
	mark := l.Front().Next()
	l.MoveBefore(n, mark)

	slice := l.ToSlice()
	expected := []int{1, 3, 2}
	for i, v := range slice {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestList_MoveBeforeSameNode(t *testing.T) {
	l := New(1, 2, 3)

	n := l.Front().Next()
	l.MoveBefore(n, n) // Move to itself

	// Should be unchanged
	if l.Len() != 3 {
		t.Errorf("expected len 3, got %d", l.Len())
	}
}

func TestList_MoveAfter(t *testing.T) {
	l := New(1, 2, 3)

	// Move 1 after 2
	n := l.Front()
	mark := l.Front().Next()
	l.MoveAfter(n, mark)

	slice := l.ToSlice()
	expected := []int{2, 1, 3}
	for i, v := range slice {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestList_MoveAfterSameNode(t *testing.T) {
	l := New(1, 2, 3)

	n := l.Front().Next()
	l.MoveAfter(n, n) // Move to itself

	// Should be unchanged
	if l.Len() != 3 {
		t.Errorf("expected len 3, got %d", l.Len())
	}
}

func TestList_SizeAndLen(t *testing.T) {
	l := New(1, 2, 3)

	if l.Size() != 3 {
		t.Errorf("Size() expected 3, got %d", l.Size())
	}
	if l.Len() != 3 {
		t.Errorf("Len() expected 3, got %d", l.Len())
	}
}

func TestList_IsEmpty(t *testing.T) {
	l := New[int]()
	if !l.IsEmpty() {
		t.Error("should be empty")
	}

	l.PushBack(1)
	if l.IsEmpty() {
		t.Error("should not be empty")
	}
}

func TestList_Clear(t *testing.T) {
	l := New(1, 2, 3)
	l.Clear()

	if !l.IsEmpty() {
		t.Error("should be empty after clear")
	}
}

func TestList_ToSlice(t *testing.T) {
	l := New(1, 2, 3)
	slice := l.ToSlice()

	if len(slice) != 3 {
		t.Errorf("expected slice len 3, got %d", len(slice))
	}
}

func TestList_Values(t *testing.T) {
	l := New(1, 2, 3)
	values := l.Values()

	if len(values) != 3 {
		t.Errorf("expected 3 values, got %d", len(values))
	}
}

func TestList_ForEach(t *testing.T) {
	l := New(1, 2, 3)
	sum := 0
	l.ForEach(func(v int) {
		sum += v
	})

	if sum != 6 {
		t.Errorf("expected sum 6, got %d", sum)
	}
}

func TestList_ForEachNode(t *testing.T) {
	l := New(1, 2, 3)
	count := 0
	l.ForEachNode(func(n *Node[int]) {
		count++
	})

	if count != 3 {
		t.Errorf("expected 3 nodes, got %d", count)
	}
}

func TestList_ForEachReverse(t *testing.T) {
	l := New(1, 2, 3)
	result := make([]int, 0)
	l.ForEachReverse(func(v int) {
		result = append(result, v)
	})

	expected := []int{3, 2, 1}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestList_Find(t *testing.T) {
	l := New(1, 2, 3, 4, 5)

	n := l.Find(func(v int) bool { return v == 3 })
	if n == nil {
		t.Fatal("Find should return a node")
	}
	if n.Value != 3 {
		t.Errorf("expected value 3, got %d", n.Value)
	}
}

func TestList_FindNotFound(t *testing.T) {
	l := New(1, 2, 3)

	n := l.Find(func(v int) bool { return v == 10 })
	if n != nil {
		t.Error("Find should return nil for not found")
	}
}

func TestList_FindAll(t *testing.T) {
	l := New(1, 2, 3, 4, 5, 6)

	nodes := l.FindAll(func(v int) bool { return v%2 == 0 })
	if len(nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(nodes))
	}
}

func TestList_Contains(t *testing.T) {
	l := New(1, 2, 3)

	if !l.Contains(func(v int) bool { return v == 2 }) {
		t.Error("should contain 2")
	}
	if l.Contains(func(v int) bool { return v == 10 }) {
		t.Error("should not contain 10")
	}
}

func TestList_Filter(t *testing.T) {
	l := New(1, 2, 3, 4, 5, 6)

	even := l.Filter(func(v int) bool { return v%2 == 0 })
	if even.Len() != 3 {
		t.Errorf("expected 3 even numbers, got %d", even.Len())
	}
}

func TestList_Clone(t *testing.T) {
	l := New(1, 2, 3)
	cloned := l.Clone()

	if cloned.Len() != l.Len() {
		t.Error("cloned list should have same length")
	}

	// Modify original
	l.PushBack(4)
	if cloned.Len() == l.Len() {
		t.Error("cloned should not be affected by original")
	}
}

func TestList_Reverse(t *testing.T) {
	l := New(1, 2, 3, 4, 5)
	l.Reverse()

	slice := l.ToSlice()
	expected := []int{5, 4, 3, 2, 1}
	for i, v := range slice {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestList_ReverseEmpty(t *testing.T) {
	l := New[int]()
	l.Reverse() // Should not panic
}

func TestList_ReverseSingle(t *testing.T) {
	l := New(1)
	l.Reverse()

	if l.Front().Value != 1 {
		t.Error("single element reverse should keep same value")
	}
}

func TestList_PushFrontList(t *testing.T) {
	l1 := New(4, 5, 6)
	l2 := New(1, 2, 3)

	l1.PushFrontList(l2)

	slice := l1.ToSlice()
	expected := []int{1, 2, 3, 4, 5, 6}
	if len(slice) != len(expected) {
		t.Errorf("expected len %d, got %d", len(expected), len(slice))
	}
	for i, v := range slice {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestList_PushBackList(t *testing.T) {
	l1 := New(1, 2, 3)
	l2 := New(4, 5, 6)

	l1.PushBackList(l2)

	slice := l1.ToSlice()
	expected := []int{1, 2, 3, 4, 5, 6}
	if len(slice) != len(expected) {
		t.Errorf("expected len %d, got %d", len(expected), len(slice))
	}
	for i, v := range slice {
		if v != expected[i] {
			t.Errorf("expected %d at index %d, got %d", expected[i], i, v)
		}
	}
}

func TestNode_Next(t *testing.T) {
	l := New(1, 2, 3)

	n := l.Front()
	next := n.Next()
	if next == nil || next.Value != 2 {
		t.Error("Next should return second node")
	}

	// Last node's Next should be nil
	last := l.Back()
	if last.Next() != nil {
		t.Error("Last node's Next should be nil")
	}
}

func TestNode_Prev(t *testing.T) {
	l := New(1, 2, 3)

	n := l.Back()
	prev := n.Prev()
	if prev == nil || prev.Value != 2 {
		t.Error("Prev should return second node")
	}

	// First node's Prev should be nil
	first := l.Front()
	if first.Prev() != nil {
		t.Error("First node's Prev should be nil")
	}
}

func TestNode_DetachedNode(t *testing.T) {
	n := &Node[int]{Value: 1}

	if n.Next() != nil {
		t.Error("Detached node's Next should be nil")
	}
	if n.Prev() != nil {
		t.Error("Detached node's Prev should be nil")
	}
}

// --- SyncList Tests ---

func TestSyncList_Basic(t *testing.T) {
	sl := NewSyncList[int]()

	sl.PushBack(1)
	sl.PushBack(2)
	sl.PushFront(0)

	if sl.Len() != 3 {
		t.Errorf("expected len 3, got %d", sl.Len())
	}

	front, ok := sl.Front()
	if !ok || front != 0 {
		t.Error("Front should return 0")
	}

	back, ok := sl.Back()
	if !ok || back != 2 {
		t.Error("Back should return 2")
	}
}

func TestSyncList_PopFrontBack(t *testing.T) {
	sl := NewSyncList[int]()
	sl.PushBack(1)
	sl.PushBack(2)
	sl.PushBack(3)

	val, ok := sl.PopFront()
	if !ok || val != 1 {
		t.Error("PopFront should return 1")
	}

	val, ok = sl.PopBack()
	if !ok || val != 3 {
		t.Error("PopBack should return 3")
	}
}

func TestSyncList_IsEmpty(t *testing.T) {
	sl := NewSyncList[int]()

	if !sl.IsEmpty() {
		t.Error("should be empty")
	}

	sl.PushBack(1)
	if sl.IsEmpty() {
		t.Error("should not be empty")
	}
}

func TestSyncList_Clear(t *testing.T) {
	sl := NewSyncList[int]()
	sl.PushBack(1)
	sl.PushBack(2)
	sl.Clear()

	if !sl.IsEmpty() {
		t.Error("should be empty after clear")
	}
}

func TestSyncList_ToSlice(t *testing.T) {
	sl := NewSyncList[int]()
	sl.PushBack(1)
	sl.PushBack(2)
	sl.PushBack(3)

	slice := sl.ToSlice()
	if len(slice) != 3 {
		t.Errorf("expected 3 elements, got %d", len(slice))
	}
}

func TestSyncList_FrontBackEmpty(t *testing.T) {
	sl := NewSyncList[int]()

	_, ok := sl.Front()
	if ok {
		t.Error("Front on empty list should return false")
	}

	_, ok = sl.Back()
	if ok {
		t.Error("Back on empty list should return false")
	}
}

func TestSyncList_Concurrent(t *testing.T) {
	sl := NewSyncList[int]()
	var wg sync.WaitGroup

	// Concurrent push
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			sl.PushFront(n)
		}(i)
		go func(n int) {
			defer wg.Done()
			sl.PushBack(n)
		}(i)
	}

	wg.Wait()

	if sl.Len() != 200 {
		t.Errorf("expected len 200, got %d", sl.Len())
	}
}

func TestSyncList_ConcurrentPop(t *testing.T) {
	sl := NewSyncList[int]()

	// Pre-populate
	for i := 0; i < 100; i++ {
		sl.PushBack(i)
	}

	var wg sync.WaitGroup
	count := 0
	var mu sync.Mutex

	// Concurrent pop
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := sl.PopFront(); ok {
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

func BenchmarkList_PushBack(b *testing.B) {
	l := New[int]()
	for i := 0; i < b.N; i++ {
		l.PushBack(i)
	}
}

func BenchmarkList_PushFront(b *testing.B) {
	l := New[int]()
	for i := 0; i < b.N; i++ {
		l.PushFront(i)
	}
}

func BenchmarkList_PopFront(b *testing.B) {
	l := New[int]()
	for i := 0; i < b.N; i++ {
		l.PushBack(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.PopFront()
	}
}

func BenchmarkSyncList_PushBack(b *testing.B) {
	sl := NewSyncList[int]()
	for i := 0; i < b.N; i++ {
		sl.PushBack(i)
	}
}
