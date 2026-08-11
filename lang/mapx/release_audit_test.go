package mapx

import "testing"

func TestForEachUsesSourceSnapshot(t *testing.T) {
	values := map[int]int{1: 1, 2: 2, 3: 3}
	visited := 0
	ForEach(values, func(int, int) {
		visited++
		if visited == 1 {
			delete(values, 1)
			delete(values, 2)
			delete(values, 3)
		}
	})
	if visited != 3 {
		t.Fatalf("ForEach visited %d entries, want all 3 source entries", visited)
	}
}
