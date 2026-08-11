package list

import (
	"reflect"
	"testing"
	"time"
)

func TestListZeroValueIsUsable(t *testing.T) {
	var values List[int]
	values.Clear()
	values.PushBack(1)
	values.PushFront(0)

	if got, want := values.ToSlice(), []int{0, 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("zero-value List contents = %v, want %v", got, want)
	}
}

func TestListPushBackListSelfUsesSourceSnapshot(t *testing.T) {
	values := New(1, 2, 3)
	done := make(chan struct{})
	go func() {
		values.PushBackList(values)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("PushBackList did not terminate when source and destination were identical")
	}

	if got, want := values.ToSlice(), []int{1, 2, 3, 1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("self append contents = %v, want %v", got, want)
	}
}

func TestSyncListZeroValueIsUsable(t *testing.T) {
	var values SyncList[int]
	values.PushBack(1)
	values.PushFront(0)

	if got, want := values.ToSlice(), []int{0, 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("zero-value SyncList contents = %v, want %v", got, want)
	}
}

func TestSyncListPredicateCanReenterList(t *testing.T) {
	values := NewSyncList[int]()
	values.PushBack(1)
	done := make(chan struct{})
	go func() {
		_, _ = values.Find(func(value int) bool {
			values.PushBack(value + 1)
			return true
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Find deadlocked when its predicate reentered the list")
	}
}

func TestListIterationUsesSourceSnapshot(t *testing.T) {
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
