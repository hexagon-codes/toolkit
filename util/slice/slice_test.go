package slice

import (
	"testing"
)

func TestUnique(t *testing.T) {
	tests := []struct {
		name  string
		input []int
		want  []int
	}{
		{"with duplicates", []int{1, 2, 2, 3, 3, 3}, []int{1, 2, 3}},
		{"no duplicates", []int{1, 2, 3}, []int{1, 2, 3}},
		{"empty", []int{}, []int{}},
		{"single", []int{1}, []int{1}},
		{"all same", []int{5, 5, 5}, []int{5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Unique(tt.input)
			if !Equal(got, tt.want) {
				t.Errorf("Unique(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestContains(t *testing.T) {
	slice := []int{1, 2, 3, 4, 5}

	if !Contains(slice, 3) {
		t.Error("Contains should return true for existing element")
	}

	if Contains(slice, 10) {
		t.Error("Contains should return false for non-existing element")
	}

	if Contains([]int{}, 1) {
		t.Error("Contains should return false for empty slice")
	}
}

func TestIndexOf(t *testing.T) {
	slice := []int{1, 2, 3, 2, 4}

	if idx := IndexOf(slice, 2); idx != 1 {
		t.Errorf("IndexOf = %d, want 1", idx)
	}

	if idx := IndexOf(slice, 10); idx != -1 {
		t.Errorf("IndexOf for non-existing = %d, want -1", idx)
	}
}

func TestLastIndexOf(t *testing.T) {
	slice := []int{1, 2, 3, 2, 4}

	if idx := LastIndexOf(slice, 2); idx != 3 {
		t.Errorf("LastIndexOf = %d, want 3", idx)
	}

	if idx := LastIndexOf(slice, 10); idx != -1 {
		t.Errorf("LastIndexOf for non-existing = %d, want -1", idx)
	}
}

func TestRemove(t *testing.T) {
	slice := []int{1, 2, 3, 2, 4}
	result := Remove(slice, 2)

	expected := []int{1, 3, 2, 4}
	if !Equal(result, expected) {
		t.Errorf("Remove = %v, want %v", result, expected)
	}

	// Remove non-existing
	result2 := Remove(slice, 10)
	if !Equal(result2, slice) {
		t.Error("Remove non-existing should return original slice")
	}
}

func TestRemoveAll(t *testing.T) {
	slice := []int{1, 2, 3, 2, 4, 2}
	result := RemoveAll(slice, 2)

	expected := []int{1, 3, 4}
	if !Equal(result, expected) {
		t.Errorf("RemoveAll = %v, want %v", result, expected)
	}
}

func TestRemoveAt(t *testing.T) {
	slice := []int{1, 2, 3, 4, 5}
	result := RemoveAt(slice, 2)

	expected := []int{1, 2, 4, 5}
	if !Equal(result, expected) {
		t.Errorf("RemoveAt = %v, want %v", result, expected)
	}

	// Invalid index
	result2 := RemoveAt(slice, 10)
	if !Equal(result2, slice) {
		t.Error("RemoveAt with invalid index should return original slice")
	}

	result3 := RemoveAt(slice, -1)
	if !Equal(result3, slice) {
		t.Error("RemoveAt with negative index should return original slice")
	}
}

func TestReverse(t *testing.T) {
	tests := []struct {
		input []int
		want  []int
	}{
		{[]int{1, 2, 3, 4, 5}, []int{5, 4, 3, 2, 1}},
		{[]int{1, 2}, []int{2, 1}},
		{[]int{1}, []int{1}},
		{[]int{}, []int{}},
	}

	for _, tt := range tests {
		got := Reverse(tt.input)
		if !Equal(got, tt.want) {
			t.Errorf("Reverse(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestChunk(t *testing.T) {
	tests := []struct {
		name  string
		input []int
		size  int
		want  [][]int
	}{
		{"normal", []int{1, 2, 3, 4, 5}, 2, [][]int{{1, 2}, {3, 4}, {5}}},
		{"exact", []int{1, 2, 3, 4}, 2, [][]int{{1, 2}, {3, 4}}},
		{"size 1", []int{1, 2, 3}, 1, [][]int{{1}, {2}, {3}}},
		{"size 0", []int{1, 2, 3}, 0, [][]int{{1, 2, 3}}},
		{"empty", []int{}, 2, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Chunk(tt.input, tt.size)
			if len(got) != len(tt.want) {
				t.Errorf("Chunk len = %d, want %d", len(got), len(tt.want))
			}
		})
	}
}

func TestFlatten(t *testing.T) {
	input := [][]int{{1, 2}, {3, 4}, {5}}
	result := Flatten(input)

	expected := []int{1, 2, 3, 4, 5}
	if !Equal(result, expected) {
		t.Errorf("Flatten = %v, want %v", result, expected)
	}
}

func TestUnion(t *testing.T) {
	slice1 := []int{1, 2, 3}
	slice2 := []int{3, 4, 5}
	result := Union(slice1, slice2)

	expected := []int{1, 2, 3, 4, 5}
	if !Equal(result, expected) {
		t.Errorf("Union = %v, want %v", result, expected)
	}
}

func TestIntersect(t *testing.T) {
	slice1 := []int{1, 2, 3, 4}
	slice2 := []int{3, 4, 5, 6}
	result := Intersect(slice1, slice2)

	expected := []int{3, 4}
	if !Equal(result, expected) {
		t.Errorf("Intersect = %v, want %v", result, expected)
	}
}

func TestDifference(t *testing.T) {
	slice1 := []int{1, 2, 3, 4}
	slice2 := []int{3, 4, 5, 6}
	result := Difference(slice1, slice2)

	expected := []int{1, 2}
	if !Equal(result, expected) {
		t.Errorf("Difference = %v, want %v", result, expected)
	}
}

func TestEqual(t *testing.T) {
	if !Equal([]int{1, 2, 3}, []int{1, 2, 3}) {
		t.Error("Equal should return true for same slices")
	}

	if Equal([]int{1, 2, 3}, []int{1, 2, 4}) {
		t.Error("Equal should return false for different slices")
	}

	if Equal([]int{1, 2, 3}, []int{1, 2}) {
		t.Error("Equal should return false for different lengths")
	}
}

func TestSum(t *testing.T) {
	slice := []int{1, 2, 3, 4, 5}
	if sum := Sum(slice); sum != 15 {
		t.Errorf("Sum = %d, want 15", sum)
	}

	if sum := Sum([]int{}); sum != 0 {
		t.Errorf("Sum of empty = %d, want 0", sum)
	}
}

func TestSumFloat(t *testing.T) {
	slice := []float64{1.5, 2.5, 3.0}
	if sum := SumFloat(slice); sum != 7.0 {
		t.Errorf("SumFloat = %f, want 7.0", sum)
	}
}

func TestMax(t *testing.T) {
	slice := []int{1, 5, 3, 9, 2}
	if max := Max(slice); max != 9 {
		t.Errorf("Max = %d, want 9", max)
	}

	if max := Max([]int{}); max != 0 {
		t.Errorf("Max of empty = %d, want 0", max)
	}
}

func TestMin(t *testing.T) {
	slice := []int{5, 1, 3, 9, 2}
	if min := Min(slice); min != 1 {
		t.Errorf("Min = %d, want 1", min)
	}

	if min := Min([]int{}); min != 0 {
		t.Errorf("Min of empty = %d, want 0", min)
	}
}

func TestGroupBy(t *testing.T) {
	type Item struct {
		Category string
		Name     string
	}

	items := []Item{
		{"A", "Item1"},
		{"B", "Item2"},
		{"A", "Item3"},
	}

	groups := GroupBy(items, func(item Item) string {
		return item.Category
	})

	if len(groups["A"]) != 2 {
		t.Errorf("Group A has %d items, want 2", len(groups["A"]))
	}
	if len(groups["B"]) != 1 {
		t.Errorf("Group B has %d items, want 1", len(groups["B"]))
	}
}

func TestCountBy(t *testing.T) {
	slice := []string{"a", "b", "a", "c", "a", "b"}
	counts := CountBy(slice, func(s string) string { return s })

	if counts["a"] != 3 {
		t.Errorf("Count of 'a' = %d, want 3", counts["a"])
	}
	if counts["b"] != 2 {
		t.Errorf("Count of 'b' = %d, want 2", counts["b"])
	}
}

func TestAny(t *testing.T) {
	slice := []int{1, 2, 3, 4, 5}

	if !Any(slice, func(n int) bool { return n > 3 }) {
		t.Error("Any should return true when condition is met")
	}

	if Any(slice, func(n int) bool { return n > 10 }) {
		t.Error("Any should return false when condition is not met")
	}
}

func TestAll(t *testing.T) {
	slice := []int{2, 4, 6, 8}

	if !All(slice, func(n int) bool { return n%2 == 0 }) {
		t.Error("All should return true when all meet condition")
	}

	if All(slice, func(n int) bool { return n > 5 }) {
		t.Error("All should return false when not all meet condition")
	}
}

func TestFirst(t *testing.T) {
	slice := []int{1, 2, 3}

	first, ok := First(slice)
	if !ok || first != 1 {
		t.Errorf("First = %d, %v, want 1, true", first, ok)
	}

	_, ok = First([]int{})
	if ok {
		t.Error("First of empty slice should return false")
	}
}

func TestLast(t *testing.T) {
	slice := []int{1, 2, 3}

	last, ok := Last(slice)
	if !ok || last != 3 {
		t.Errorf("Last = %d, %v, want 3, true", last, ok)
	}

	_, ok = Last([]int{})
	if ok {
		t.Error("Last of empty slice should return false")
	}
}

func TestShuffle(t *testing.T) {
	slice := []int{1, 2, 3, 4, 5}
	result := Shuffle(slice)

	// Should have same length
	if len(result) != len(slice) {
		t.Errorf("Shuffle len = %d, want %d", len(result), len(slice))
	}

	// Original should not be modified
	if !Equal(slice, []int{1, 2, 3, 4, 5}) {
		t.Error("Shuffle should not modify original slice")
	}
}
