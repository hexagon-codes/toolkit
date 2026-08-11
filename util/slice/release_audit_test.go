package slice

import "testing"

type auditSigned int16
type auditUnsigned uint32
type auditFloat float32

func TestRemoveFallbackResultsDoNotAliasInput(t *testing.T) {
	tests := []struct {
		name string
		call func([]int) []int
	}{
		{name: "missing item", call: func(input []int) []int { return Remove(input, 99) }},
		{name: "negative index", call: func(input []int) []int { return RemoveAt(input, -1) }},
		{name: "large index", call: func(input []int) []int { return RemoveAt(input, 99) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := []int{1, 2, 3}
			result := test.call(input)
			result[0] = 99
			if input[0] == 99 {
				t.Fatal("result aliases the input slice")
			}
		})
	}
}

func TestChunkReturnsIndependentExactSizedStorage(t *testing.T) {
	input := []int{1, 2, 3, 4}
	chunks := Chunk(input, 2)
	if len(chunks) != 2 {
		t.Fatalf("Chunk() returned %d chunks, want 2", len(chunks))
	}
	for index, chunk := range chunks {
		if cap(chunk) != len(chunk) {
			t.Errorf("chunk %d capacity = %d, want %d", index, cap(chunk), len(chunk))
		}
	}
	chunks[0][0] = 99
	if input[0] == 99 {
		t.Fatal("chunk aliases the input slice")
	}
	if chunks[1][0] == 99 {
		t.Fatal("chunks alias each other")
	}

	whole := Chunk(input, 0)
	whole[0][0] = 77
	if input[0] == 77 {
		t.Fatal("non-positive-size chunk aliases the input slice")
	}
}

func TestFilteringResultsDoNotRetainUnusedCapacity(t *testing.T) {
	input := make([]int, 1024)
	for index := range input {
		input[index] = 1
	}

	unique := Unique(input)
	if cap(unique) != len(unique) {
		t.Errorf("Unique() capacity = %d, want %d", cap(unique), len(unique))
	}
	removed := RemoveAll(input, 1)
	if cap(removed) != len(removed) {
		t.Errorf("RemoveAll() capacity = %d, want %d", cap(removed), len(removed))
	}
	union := Union(input, input)
	if cap(union) != len(union) {
		t.Errorf("Union() capacity = %d, want %d", cap(union), len(union))
	}
}

func TestNumericHelpersAcceptNamedTypes(t *testing.T) {
	if got := Sum([]auditSigned{1, 2, 3}); got != 6 {
		t.Errorf("Sum(named signed) = %d, want 6", got)
	}
	if got := Sum([]auditUnsigned{1, 2, 3}); got != 6 {
		t.Errorf("Sum(named unsigned) = %d, want 6", got)
	}
	if got := SumFloat([]auditFloat{1.5, 2.5}); got != 4 {
		t.Errorf("SumFloat(named float) = %v, want 4", got)
	}
	if got := Max([]auditUnsigned{1, 9, 3}); got != 9 {
		t.Errorf("Max(named unsigned) = %d, want 9", got)
	}
	if got := Min([]auditFloat{3, 1, 2}); got != 1 {
		t.Errorf("Min(named float) = %v, want 1", got)
	}
}
