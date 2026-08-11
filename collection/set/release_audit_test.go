package set

import (
	"testing"
	"time"
)

func TestSetZeroValueIsUsable(t *testing.T) {
	var values Set[int]
	values.Add(1, 2)
	if !values.ContainsAll(1, 2) {
		t.Fatalf("zero-value Set does not contain inserted values: %v", values.ToSlice())
	}
}

func TestNewWithSizeAcceptsNegativeHint(t *testing.T) {
	values := NewWithSize[int](-1)
	values.Add(1)
	if !values.Contains(1) {
		t.Fatal("Set created from a negative capacity hint is unusable")
	}
}

func TestSyncSetZeroValueAndPairOperationsAreUsable(t *testing.T) {
	var left, right SyncSet[int]
	left.Add(1, 2)
	right.Add(2, 3)

	done := make(chan struct{})
	go func() {
		_ = left.Union(&right)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("pair operation on zero-value SyncSet instances did not terminate")
	}
}

func TestSyncSetPredicateCanReenterSet(t *testing.T) {
	values := NewSyncSet(1)
	done := make(chan struct{})
	go func() {
		_ = values.Any(func(value int) bool {
			values.Add(value + 1)
			return true
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Any deadlocked when its predicate reentered the set")
	}
}

func TestSetIterationUsesSourceSnapshot(t *testing.T) {
	values := New(1, 2, 3)
	visited := 0
	values.ForEach(func(int) {
		visited++
		if visited == 1 {
			values.Remove(1, 2, 3)
		}
	})

	if visited != 3 {
		t.Fatalf("visited %d values, want all 3 values from source snapshot", visited)
	}
}

func TestSyncSetOppositePairOperationsDoNotDeadlockForZeroValues(t *testing.T) {
	var left, right SyncSet[int]
	left.Add(1)
	right.Add(2)
	start := make(chan struct{})
	done := make(chan struct{}, 2)

	go func() {
		<-start
		_ = left.Union(&right)
		done <- struct{}{}
	}()
	go func() {
		<-start
		_ = right.Union(&left)
		done <- struct{}{}
	}()
	close(start)

	for range 2 {
		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("opposite pair operations deadlocked")
		}
	}
}
