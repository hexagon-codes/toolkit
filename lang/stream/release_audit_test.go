package stream

import (
	"reflect"
	"testing"
)

func TestStreamZeroValueIsEmptyAndComposable(t *testing.T) {
	var zero Stream[int]
	if got := zero.Collect(); got != nil {
		t.Fatalf("zero Stream Collect = %v, want nil", got)
	}
	if zero.Count() != 0 || !zero.IsEmpty() {
		t.Fatalf("zero Stream state = (count %d, empty %v), want (0, true)", zero.Count(), zero.IsEmpty())
	}
	if got := zero.Filter(func(int) bool { return true }).Collect(); len(got) != 0 {
		t.Fatalf("filtered zero Stream = %v, want empty", got)
	}
}

func TestStreamOwnsSourceAndCollectedSlices(t *testing.T) {
	source := []int{1, 2, 3}
	values := FromSlice(source)
	source[0] = 9

	first := values.Collect()
	if want := []int{1, 2, 3}; !reflect.DeepEqual(first, want) {
		t.Fatalf("Collect after source mutation = %v, want %v", first, want)
	}
	first[1] = 8
	if got, want := values.Collect(), []int{1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("second Collect = %v, want isolated %v", got, want)
	}
}
