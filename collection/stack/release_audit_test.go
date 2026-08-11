package stack

import (
	"reflect"
	"testing"
)

func TestStackNegativeCapacityHintAndSyncZeroValue(t *testing.T) {
	stack := NewWithCapacity[int](-1)
	stack.Push(1)
	if got, ok := stack.Pop(); !ok || got != 1 {
		t.Fatalf("Stack created from a negative capacity hint returned (%v, %v), want (1, true)", got, ok)
	}

	var synchronized SyncStack[int]
	synchronized.Push(2)
	if got, ok := synchronized.Pop(); !ok || got != 2 {
		t.Fatalf("zero-value SyncStack returned (%v, %v), want (2, true)", got, ok)
	}
}

func TestStackIterationUsesSourceSnapshot(t *testing.T) {
	values := New(1, 2, 3)
	visited := make([]int, 0, values.Len())
	values.ForEach(func(value int) {
		visited = append(visited, value)
		if value == 1 {
			values.Clear()
		}
	})

	if want := []int{1, 2, 3}; !reflect.DeepEqual(visited, want) {
		t.Fatalf("visited values = %v, want source snapshot %v", visited, want)
	}
}
