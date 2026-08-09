package stream

import (
	"reflect"
	"strconv"
	"testing"
)

func TestOf(t *testing.T) {
	result := Of(1, 2, 3, 4, 5).Collect()
	if len(result) != 5 {
		t.Errorf("expected 5 elements, got %d", len(result))
	}
}

func TestFromSlice(t *testing.T) {
	slice := []string{"a", "b", "c"}
	result := FromSlice(slice).Collect()
	if len(result) != 3 {
		t.Errorf("expected 3 elements, got %d", len(result))
	}
}

func TestGenerate(t *testing.T) {
	result := Generate(5, func(i int) int { return i * 2 }).Collect()
	expected := []int{0, 2, 4, 6, 8}
	if len(result) != len(expected) {
		t.Errorf("expected %d elements, got %d", len(expected), len(result))
	}
	for i := range result {
		if result[i] != expected[i] {
			t.Errorf("index %d: expected %d, got %d", i, expected[i], result[i])
		}
	}
}

func TestRange(t *testing.T) {
	result := Range(0, 5).Collect()
	expected := []int{0, 1, 2, 3, 4}
	if len(result) != len(expected) {
		t.Errorf("expected %d elements, got %d", len(expected), len(result))
	}
	for i := range result {
		if result[i] != expected[i] {
			t.Errorf("index %d: expected %d, got %d", i, expected[i], result[i])
		}
	}
}

func TestRange_Empty(t *testing.T) {
	result := Range(5, 0).Collect()
	if len(result) != 0 {
		t.Errorf("expected empty, got %d elements", len(result))
	}
}

func TestRepeat(t *testing.T) {
	result := Repeat("hello", 3).Collect()
	if len(result) != 3 {
		t.Errorf("expected 3 elements, got %d", len(result))
	}
	for _, v := range result {
		if v != "hello" {
			t.Errorf("expected 'hello', got %v", v)
		}
	}
}

func TestFilter(t *testing.T) {
	result := Of(1, 2, 3, 4, 5).
		Filter(func(n int) bool { return n%2 == 0 }).
		Collect()
	expected := []int{2, 4}
	if len(result) != len(expected) {
		t.Errorf("expected %d elements, got %d", len(expected), len(result))
	}
}

func TestMap(t *testing.T) {
	result := Of(1, 2, 3).
		Map(func(n int) int { return n * 2 }).
		Collect()
	expected := []int{2, 4, 6}
	if len(result) != len(expected) {
		t.Errorf("expected %d elements, got %d", len(expected), len(result))
	}
}

func TestDistinct(t *testing.T) {
	result := Of(1, 2, 2, 3, 3, 3).Distinct().Collect()
	expected := []int{1, 2, 3}
	if len(result) != len(expected) {
		t.Errorf("expected %d elements, got %d", len(expected), len(result))
	}
}

func TestDistinctHandlesMixedComparableValues(t *testing.T) {
	firstSlice := []int{2}
	secondSlice := []int{2}
	input := []any{1, 1, firstSlice, secondSlice, "x", "x", map[string]int{"a": 1}, nil, nil}
	want := []any{1, firstSlice, secondSlice, "x", map[string]int{"a": 1}, nil}

	if got := FromSlice(input).Distinct().Collect(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Distinct() = %#v, want %#v", got, want)
	}
}

func TestSorted(t *testing.T) {
	result := Of(3, 1, 4, 1, 5).
		Sorted(func(a, b int) bool { return a < b }).
		Collect()
	expected := []int{1, 1, 3, 4, 5}
	if len(result) != len(expected) {
		t.Errorf("expected %d elements, got %d", len(expected), len(result))
	}
	for i := range result {
		if result[i] != expected[i] {
			t.Errorf("index %d: expected %d, got %d", i, expected[i], result[i])
		}
	}
}

func TestLimit(t *testing.T) {
	result := Of(1, 2, 3, 4, 5).Limit(3).Collect()
	if len(result) != 3 {
		t.Errorf("expected 3 elements, got %d", len(result))
	}
}

func TestLimit_Zero(t *testing.T) {
	result := Of(1, 2, 3).Limit(0).Collect()
	if len(result) != 0 {
		t.Errorf("expected 0 elements, got %d", len(result))
	}
}

func TestSkip(t *testing.T) {
	result := Of(1, 2, 3, 4, 5).Skip(2).Collect()
	expected := []int{3, 4, 5}
	if len(result) != len(expected) {
		t.Errorf("expected %d elements, got %d", len(expected), len(result))
	}
}

func TestSkip_All(t *testing.T) {
	result := Of(1, 2, 3).Skip(10).Collect()
	if len(result) != 0 {
		t.Errorf("expected 0 elements, got %d", len(result))
	}
}

func TestPeek(t *testing.T) {
	var peeked []int
	result := Of(1, 2, 3).
		Peek(func(n int) { peeked = append(peeked, n) }).
		Collect()

	if len(peeked) != 3 {
		t.Errorf("expected 3 peeked elements, got %d", len(peeked))
	}
	if len(result) != 3 {
		t.Errorf("expected 3 result elements, got %d", len(result))
	}
}

func TestReverse(t *testing.T) {
	result := Of(1, 2, 3).Reverse().Collect()
	expected := []int{3, 2, 1}
	if len(result) != len(expected) {
		t.Errorf("expected %d elements, got %d", len(expected), len(result))
	}
	for i := range result {
		if result[i] != expected[i] {
			t.Errorf("index %d: expected %d, got %d", i, expected[i], result[i])
		}
	}
}

func TestTakeWhile(t *testing.T) {
	result := Of(1, 2, 3, 4, 5).
		TakeWhile(func(n int) bool { return n < 4 }).
		Collect()
	expected := []int{1, 2, 3}
	if len(result) != len(expected) {
		t.Errorf("expected %d elements, got %d", len(expected), len(result))
	}
}

func TestDropWhile(t *testing.T) {
	result := Of(1, 2, 3, 4, 5).
		DropWhile(func(n int) bool { return n < 3 }).
		Collect()
	expected := []int{3, 4, 5}
	if len(result) != len(expected) {
		t.Errorf("expected %d elements, got %d", len(expected), len(result))
	}
}

func TestReduce(t *testing.T) {
	sum := Of(1, 2, 3, 4, 5).Reduce(0, func(acc, n int) int { return acc + n })
	if sum != 15 {
		t.Errorf("expected 15, got %d", sum)
	}
}

func TestCount(t *testing.T) {
	count := Of(1, 2, 3, 4, 5).Count()
	if count != 5 {
		t.Errorf("expected 5, got %d", count)
	}
}

func TestFirst(t *testing.T) {
	first, ok := Of(1, 2, 3).First()
	if !ok || first != 1 {
		t.Errorf("expected 1, true; got %d, %v", first, ok)
	}

	_, ok = Of[int]().First()
	if ok {
		t.Error("expected false for empty stream")
	}
}

func TestLast(t *testing.T) {
	last, ok := Of(1, 2, 3).Last()
	if !ok || last != 3 {
		t.Errorf("expected 3, true; got %d, %v", last, ok)
	}

	_, ok = Of[int]().Last()
	if ok {
		t.Error("expected false for empty stream")
	}
}

func TestAny(t *testing.T) {
	hasEven := Of(1, 2, 3).Any(func(n int) bool { return n%2 == 0 })
	if !hasEven {
		t.Error("expected true")
	}

	hasNegative := Of(1, 2, 3).Any(func(n int) bool { return n < 0 })
	if hasNegative {
		t.Error("expected false")
	}
}

func TestAll(t *testing.T) {
	allPositive := Of(1, 2, 3).All(func(n int) bool { return n > 0 })
	if !allPositive {
		t.Error("expected true")
	}

	allEven := Of(1, 2, 3).All(func(n int) bool { return n%2 == 0 })
	if allEven {
		t.Error("expected false")
	}
}

func TestNone(t *testing.T) {
	noNegative := Of(1, 2, 3).None(func(n int) bool { return n < 0 })
	if !noNegative {
		t.Error("expected true")
	}

	noEven := Of(1, 2, 3).None(func(n int) bool { return n%2 == 0 })
	if noEven {
		t.Error("expected false")
	}
}

func TestFindFirst(t *testing.T) {
	even, ok := Of(1, 2, 3, 4).FindFirst(func(n int) bool { return n%2 == 0 })
	if !ok || even != 2 {
		t.Errorf("expected 2, true; got %d, %v", even, ok)
	}

	_, ok = Of(1, 3, 5).FindFirst(func(n int) bool { return n%2 == 0 })
	if ok {
		t.Error("expected false")
	}
}

func TestIsEmpty(t *testing.T) {
	if !Of[int]().IsEmpty() {
		t.Error("expected empty")
	}

	if Of(1, 2, 3).IsEmpty() {
		t.Error("expected not empty")
	}
}

func TestChainOperations(t *testing.T) {
	// 复杂链式操作测试
	result := Of(1, 2, 3, 4, 5, 6, 7, 8, 9, 10).
		Filter(func(n int) bool { return n%2 == 0 }).
		Map(func(n int) int { return n * 2 }).
		Limit(3).
		Collect()

	expected := []int{4, 8, 12}
	if len(result) != len(expected) {
		t.Errorf("expected %d elements, got %d", len(expected), len(result))
	}
	for i := range result {
		if result[i] != expected[i] {
			t.Errorf("index %d: expected %d, got %d", i, expected[i], result[i])
		}
	}
}

func TestToMap(t *testing.T) {
	type User struct {
		ID   int
		Name string
	}
	users := FromSlice([]User{{1, "Alice"}, {2, "Bob"}})
	m := ToMap(users, func(u User) int { return u.ID })

	if len(m) != 2 {
		t.Errorf("expected 2 elements, got %d", len(m))
	}
	if m[1].Name != "Alice" {
		t.Errorf("expected Alice, got %s", m[1].Name)
	}
}

func TestGroupBy(t *testing.T) {
	groups := GroupBy(Of(1, 2, 3, 4, 5), func(n int) string {
		if n%2 == 0 {
			return "even"
		}
		return "odd"
	})

	if len(groups["even"]) != 2 {
		t.Errorf("expected 2 even, got %d", len(groups["even"]))
	}
	if len(groups["odd"]) != 3 {
		t.Errorf("expected 3 odd, got %d", len(groups["odd"]))
	}
}

func TestMapTo(t *testing.T) {
	result := MapTo(Of(1, 2, 3), func(n int) string {
		return strconv.Itoa(n)
	}).Collect()

	if len(result) != 3 {
		t.Errorf("expected 3 elements, got %d", len(result))
	}
	if result[0] != "1" {
		t.Errorf("expected '1', got %s", result[0])
	}
}

func TestFlatMapTo(t *testing.T) {
	result := FlatMapTo(Of([]int{1, 2}, []int{3, 4}), func(arr []int) []int {
		return arr
	}).Collect()

	expected := []int{1, 2, 3, 4}
	if len(result) != len(expected) {
		t.Errorf("expected %d elements, got %d", len(expected), len(result))
	}
}

func TestReduceTo(t *testing.T) {
	sum := ReduceTo(Of("a", "bb", "ccc"), 0, func(acc int, s string) int {
		return acc + len(s)
	})
	if sum != 6 {
		t.Errorf("expected 6, got %d", sum)
	}
}

func TestConcat(t *testing.T) {
	result := Concat(Of(1, 2), Of(3, 4), Of(5)).Collect()
	expected := []int{1, 2, 3, 4, 5}
	if len(result) != len(expected) {
		t.Errorf("expected %d elements, got %d", len(expected), len(result))
	}
}

func TestForEach(t *testing.T) {
	var sum int
	Of(1, 2, 3).ForEach(func(n int) {
		sum += n
	})
	if sum != 6 {
		t.Errorf("expected 6, got %d", sum)
	}
}
