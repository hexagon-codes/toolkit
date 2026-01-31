package set

import (
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
		t.Error("expected empty set")
	}
}

func TestNewWithSize(t *testing.T) {
	s := NewWithSize[int](100)
	if !s.IsEmpty() {
		t.Error("expected empty set")
	}
}

func TestFromSlice(t *testing.T) {
	slice := []string{"a", "b", "c"}
	s := FromSlice(slice)
	if s.Size() != 3 {
		t.Errorf("expected size 3, got %d", s.Size())
	}
}

func TestAdd(t *testing.T) {
	s := New[int]()
	s.Add(1, 2, 3)
	if s.Size() != 3 {
		t.Errorf("expected size 3, got %d", s.Size())
	}

	// Add duplicate
	s.Add(1)
	if s.Size() != 3 {
		t.Errorf("expected size 3 after adding duplicate, got %d", s.Size())
	}
}

func TestAddChaining(t *testing.T) {
	s := New[int]().Add(1).Add(2).Add(3)
	if s.Size() != 3 {
		t.Errorf("expected size 3, got %d", s.Size())
	}
}

func TestRemove(t *testing.T) {
	s := New(1, 2, 3)
	s.Remove(2)
	if s.Size() != 2 {
		t.Errorf("expected size 2, got %d", s.Size())
	}
	if s.Contains(2) {
		t.Error("should not contain 2")
	}

	// Remove non-existent
	s.Remove(99)
	if s.Size() != 2 {
		t.Errorf("expected size 2, got %d", s.Size())
	}
}

func TestContains(t *testing.T) {
	s := New(1, 2, 3)

	if !s.Contains(1) {
		t.Error("should contain 1")
	}
	if s.Contains(4) {
		t.Error("should not contain 4")
	}
}

func TestContainsAll(t *testing.T) {
	s := New(1, 2, 3, 4, 5)

	if !s.ContainsAll(1, 2, 3) {
		t.Error("should contain all")
	}
	if s.ContainsAll(1, 2, 6) {
		t.Error("should not contain all")
	}
}

func TestContainsAny(t *testing.T) {
	s := New(1, 2, 3)

	if !s.ContainsAny(1, 9, 8) {
		t.Error("should contain any")
	}
	if s.ContainsAny(7, 8, 9) {
		t.Error("should not contain any")
	}
}

func TestSizeAndLen(t *testing.T) {
	s := New(1, 2, 3)

	if s.Size() != 3 {
		t.Errorf("Size() expected 3, got %d", s.Size())
	}
	if s.Len() != 3 {
		t.Errorf("Len() expected 3, got %d", s.Len())
	}
}

func TestIsEmpty(t *testing.T) {
	s := New[int]()
	if !s.IsEmpty() {
		t.Error("should be empty")
	}

	s.Add(1)
	if s.IsEmpty() {
		t.Error("should not be empty")
	}
}

func TestClear(t *testing.T) {
	s := New(1, 2, 3)
	s.Clear()

	if !s.IsEmpty() {
		t.Error("should be empty after clear")
	}
}

func TestToSlice(t *testing.T) {
	s := New(1, 2, 3)
	slice := s.ToSlice()

	if len(slice) != 3 {
		t.Errorf("expected slice length 3, got %d", len(slice))
	}
}

func TestValues(t *testing.T) {
	s := New(1, 2, 3)
	values := s.Values()

	if len(values) != 3 {
		t.Errorf("expected 3 values, got %d", len(values))
	}
}

func TestClone(t *testing.T) {
	s := New(1, 2, 3)
	cloned := s.Clone()

	if !s.Equal(cloned) {
		t.Error("cloned set should equal original")
	}

	// Modify original
	s.Add(4)
	if cloned.Contains(4) {
		t.Error("cloned should not be affected by original")
	}
}

func TestUnion(t *testing.T) {
	s1 := New(1, 2, 3)
	s2 := New(3, 4, 5)

	union := s1.Union(s2)

	expected := New(1, 2, 3, 4, 5)
	if !union.Equal(expected) {
		t.Errorf("union incorrect: %v", union.ToSlice())
	}
}

func TestIntersection(t *testing.T) {
	s1 := New(1, 2, 3, 4)
	s2 := New(3, 4, 5, 6)

	intersection := s1.Intersection(s2)

	if intersection.Size() != 2 {
		t.Errorf("expected size 2, got %d", intersection.Size())
	}
	if !intersection.ContainsAll(3, 4) {
		t.Error("should contain 3 and 4")
	}
}

func TestIntersectionEmpty(t *testing.T) {
	s1 := New(1, 2, 3)
	s2 := New(4, 5, 6)

	intersection := s1.Intersection(s2)

	if !intersection.IsEmpty() {
		t.Error("intersection should be empty")
	}
}

func TestDifference(t *testing.T) {
	s1 := New(1, 2, 3, 4)
	s2 := New(3, 4, 5, 6)

	diff := s1.Difference(s2)

	if diff.Size() != 2 {
		t.Errorf("expected size 2, got %d", diff.Size())
	}
	if !diff.ContainsAll(1, 2) {
		t.Error("should contain 1 and 2")
	}
}

func TestSymmetricDifference(t *testing.T) {
	s1 := New(1, 2, 3)
	s2 := New(2, 3, 4)

	symDiff := s1.SymmetricDifference(s2)

	expected := New(1, 4)
	if !symDiff.Equal(expected) {
		t.Errorf("symmetric difference incorrect: %v", symDiff.ToSlice())
	}
}

func TestIsSubset(t *testing.T) {
	s1 := New(1, 2)
	s2 := New(1, 2, 3, 4)

	if !s1.IsSubset(s2) {
		t.Error("s1 should be subset of s2")
	}
	if s2.IsSubset(s1) {
		t.Error("s2 should not be subset of s1")
	}

	// Equal sets
	s3 := New(1, 2)
	if !s1.IsSubset(s3) {
		t.Error("equal sets should be subsets of each other")
	}
}

func TestIsSuperset(t *testing.T) {
	s1 := New(1, 2, 3, 4)
	s2 := New(1, 2)

	if !s1.IsSuperset(s2) {
		t.Error("s1 should be superset of s2")
	}
	if s2.IsSuperset(s1) {
		t.Error("s2 should not be superset of s1")
	}
}

func TestIsDisjoint(t *testing.T) {
	s1 := New(1, 2, 3)
	s2 := New(4, 5, 6)
	s3 := New(3, 4, 5)

	if !s1.IsDisjoint(s2) {
		t.Error("s1 and s2 should be disjoint")
	}
	if s1.IsDisjoint(s3) {
		t.Error("s1 and s3 should not be disjoint")
	}
}

func TestEqual(t *testing.T) {
	s1 := New(1, 2, 3)
	s2 := New(1, 2, 3)
	s3 := New(1, 2, 4)
	s4 := New(1, 2)

	if !s1.Equal(s2) {
		t.Error("s1 and s2 should be equal")
	}
	if s1.Equal(s3) {
		t.Error("s1 and s3 should not be equal")
	}
	if s1.Equal(s4) {
		t.Error("s1 and s4 should not be equal")
	}
}

func TestForEach(t *testing.T) {
	s := New(1, 2, 3)
	sum := 0
	s.ForEach(func(item int) {
		sum += item
	})

	if sum != 6 {
		t.Errorf("expected sum 6, got %d", sum)
	}
}

func TestFilter(t *testing.T) {
	s := New(1, 2, 3, 4, 5, 6)
	even := s.Filter(func(n int) bool {
		return n%2 == 0
	})

	if even.Size() != 3 {
		t.Errorf("expected 3 even numbers, got %d", even.Size())
	}
	if !even.ContainsAll(2, 4, 6) {
		t.Error("should contain 2, 4, 6")
	}
}

func TestAny(t *testing.T) {
	s := New(1, 2, 3, 4, 5)

	if !s.Any(func(n int) bool { return n > 3 }) {
		t.Error("should have elements > 3")
	}
	if s.Any(func(n int) bool { return n > 10 }) {
		t.Error("should not have elements > 10")
	}
}

func TestAll(t *testing.T) {
	s := New(2, 4, 6, 8)

	if !s.All(func(n int) bool { return n%2 == 0 }) {
		t.Error("all elements should be even")
	}
	if s.All(func(n int) bool { return n > 5 }) {
		t.Error("not all elements should be > 5")
	}
}

func TestNone(t *testing.T) {
	s := New(1, 2, 3, 4, 5)

	if !s.None(func(n int) bool { return n > 10 }) {
		t.Error("no elements should be > 10")
	}
	if s.None(func(n int) bool { return n > 3 }) {
		t.Error("some elements should be > 3")
	}
}

func TestCount(t *testing.T) {
	s := New(1, 2, 3, 4, 5, 6)

	count := s.Count(func(n int) bool { return n%2 == 0 })
	if count != 3 {
		t.Errorf("expected 3 even numbers, got %d", count)
	}
}

func TestPop(t *testing.T) {
	s := New(1, 2, 3)

	item, ok := s.Pop()
	if !ok {
		t.Error("Pop should succeed")
	}
	if s.Contains(item) {
		t.Error("popped item should be removed")
	}
	if s.Size() != 2 {
		t.Errorf("expected size 2, got %d", s.Size())
	}
}

func TestPopEmpty(t *testing.T) {
	s := New[int]()

	_, ok := s.Pop()
	if ok {
		t.Error("Pop on empty set should return false")
	}
}

func TestString(t *testing.T) {
	s := New(1)
	str := s.String()

	if str != "Set{1}" {
		t.Errorf("expected 'Set{1}', got '%s'", str)
	}
}

func TestUnionPackageLevel(t *testing.T) {
	s1 := New(1, 2)
	s2 := New(3, 4)
	s3 := New(5, 6)

	union := Union(s1, s2, s3)

	if union.Size() != 6 {
		t.Errorf("expected size 6, got %d", union.Size())
	}
}

func TestUnionPackageLevelEmpty(t *testing.T) {
	union := Union[int]()

	if !union.IsEmpty() {
		t.Error("union of no sets should be empty")
	}
}

func TestIntersectionPackageLevel(t *testing.T) {
	s1 := New(1, 2, 3, 4, 5)
	s2 := New(3, 4, 5, 6, 7)
	s3 := New(4, 5, 6, 7, 8)

	intersection := Intersection(s1, s2, s3)

	expected := New(4, 5)
	if !intersection.Equal(expected) {
		t.Errorf("intersection incorrect: %v", intersection.ToSlice())
	}
}

func TestIntersectionPackageLevelSingle(t *testing.T) {
	s := New(1, 2, 3)
	intersection := Intersection(s)

	if !intersection.Equal(s) {
		t.Error("intersection of single set should equal itself")
	}
}

func TestDifferencePackageLevel(t *testing.T) {
	s1 := New(1, 2, 3, 4, 5)
	s2 := New(2, 3)
	s3 := New(4, 5)

	diff := Difference(s1, s2, s3)

	expected := New(1)
	if !diff.Equal(expected) {
		t.Errorf("difference incorrect: %v", diff.ToSlice())
	}
}

func TestDifferencePackageLevelEmpty(t *testing.T) {
	diff := Difference[int]()

	if !diff.IsEmpty() {
		t.Error("difference of no sets should be empty")
	}
}

func TestWithStrings(t *testing.T) {
	s := New("apple", "banana", "cherry")

	if !s.Contains("apple") {
		t.Error("should contain apple")
	}
	if s.Contains("grape") {
		t.Error("should not contain grape")
	}
}

func TestWithStructs(t *testing.T) {
	type Point struct {
		X, Y int
	}

	s := New(Point{1, 2}, Point{3, 4})

	if !s.Contains(Point{1, 2}) {
		t.Error("should contain Point{1, 2}")
	}
	if s.Contains(Point{5, 6}) {
		t.Error("should not contain Point{5, 6}")
	}
}

func BenchmarkAdd(b *testing.B) {
	s := New[int]()
	for i := 0; i < b.N; i++ {
		s.Add(i)
	}
}

func BenchmarkContains(b *testing.B) {
	s := New[int]()
	for i := 0; i < 1000; i++ {
		s.Add(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Contains(i % 1000)
	}
}

func BenchmarkUnion(b *testing.B) {
	s1 := New[int]()
	s2 := New[int]()
	for i := 0; i < 1000; i++ {
		s1.Add(i)
		s2.Add(i + 500)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s1.Union(s2)
	}
}

func BenchmarkIntersection(b *testing.B) {
	s1 := New[int]()
	s2 := New[int]()
	for i := 0; i < 1000; i++ {
		s1.Add(i)
		s2.Add(i + 500)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s1.Intersection(s2)
	}
}
