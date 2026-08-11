package queue

import (
	"reflect"
	"testing"
)

func TestQueueZeroValuesAreUsable(t *testing.T) {
	var fifo Queue[int]
	fifo.Enqueue(1, 2)
	if got, want := fifo.ToSlice(), []int{1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("zero-value Queue contents = %v, want %v", got, want)
	}

	var deque Deque[int]
	deque.PushBack(2)
	deque.PushFront(1)
	if got, want := deque.ToSlice(), []int{1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("zero-value Deque contents = %v, want %v", got, want)
	}
}

func TestSyncQueueZeroValuesAreUsable(t *testing.T) {
	var fifo SyncQueue[int]
	fifo.Enqueue(1)
	if got, ok := fifo.Dequeue(); !ok || got != 1 {
		t.Fatalf("zero-value SyncQueue Dequeue = (%v, %v), want (1, true)", got, ok)
	}

	var deque SyncDeque[int]
	deque.PushFront(1)
	if got, ok := deque.PopBack(); !ok || got != 1 {
		t.Fatalf("zero-value SyncDeque PopBack = (%v, %v), want (1, true)", got, ok)
	}
}

func TestPriorityQueueClearReleasesReferences(t *testing.T) {
	pq := NewPriorityQueue[*int](func(a, b *int) bool { return *a < *b })
	first, second := 1, 2
	pq.Push(&first, &second)
	pq.Clear()

	retained := pq.heap.items[:cap(pq.heap.items)]
	for index, value := range retained {
		if value != nil {
			t.Fatalf("Clear retained reference at storage index %d", index)
		}
	}
}

func TestQueueIterationUsesSourceSnapshot(t *testing.T) {
	values := New(1, 2)
	visited := make([]int, 0, values.Len())
	values.ForEach(func(value int) {
		visited = append(visited, value)
		if value == 1 {
			values.Enqueue(3)
		}
	})

	if want := []int{1, 2}; !reflect.DeepEqual(visited, want) {
		t.Fatalf("visited values = %v, want source snapshot %v", visited, want)
	}
}

func TestPriorityQueueRejectsNilComparatorImmediately(t *testing.T) {
	defer func() {
		if got := recover(); got != "queue: priority comparator is nil" {
			t.Fatalf("panic = %v, want queue: priority comparator is nil", got)
		}
	}()
	_ = NewPriorityQueue[int](nil)
}
