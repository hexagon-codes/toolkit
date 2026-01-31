package mapx

import (
	"testing"
)

func TestKeys(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	keys := Keys(m)

	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}

	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}

	for k := range m {
		if !keySet[k] {
			t.Errorf("key %s not found", k)
		}
	}

	// Test nil map
	if Keys[string, int](nil) != nil {
		t.Error("expected nil for nil map")
	}
}

func TestValues(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	values := Values(m)

	if len(values) != 3 {
		t.Errorf("expected 3 values, got %d", len(values))
	}

	sum := 0
	for _, v := range values {
		sum += v
	}
	if sum != 6 {
		t.Errorf("expected sum 6, got %d", sum)
	}

	// Test nil map
	if Values[string, int](nil) != nil {
		t.Error("expected nil for nil map")
	}
}

func TestEntries(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	entries := Entries(m)

	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Test nil map
	if Entries[string, int](nil) != nil {
		t.Error("expected nil for nil map")
	}
}

func TestFromEntries(t *testing.T) {
	entries := []Entry[string, int]{
		{Key: "a", Value: 1},
		{Key: "b", Value: 2},
	}
	m := FromEntries(entries)

	if m["a"] != 1 || m["b"] != 2 {
		t.Error("FromEntries failed")
	}

	// Test nil entries
	if FromEntries[string, int](nil) != nil {
		t.Error("expected nil for nil entries")
	}
}

func TestFilter(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	filtered := Filter(m, func(k string, v int) bool {
		return v > 1
	})

	if len(filtered) != 2 {
		t.Errorf("expected 2 items, got %d", len(filtered))
	}

	if _, ok := filtered["a"]; ok {
		t.Error("'a' should not be in filtered map")
	}

	// Test nil map
	if Filter[string, int](nil, func(k string, v int) bool { return true }) != nil {
		t.Error("expected nil for nil map")
	}
}

func TestFilterKeys(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	filtered := FilterKeys(m, func(k string) bool {
		return k != "a"
	})

	if len(filtered) != 2 {
		t.Errorf("expected 2 items, got %d", len(filtered))
	}
}

func TestFilterValues(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	filtered := FilterValues(m, func(v int) bool {
		return v >= 2
	})

	if len(filtered) != 2 {
		t.Errorf("expected 2 items, got %d", len(filtered))
	}
}

func TestMapValues(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	result := MapValues(m, func(v int) int {
		return v * 2
	})

	if result["a"] != 2 || result["b"] != 4 {
		t.Error("MapValues failed")
	}

	// Test nil map
	if MapValues[string, int, int](nil, func(v int) int { return v }) != nil {
		t.Error("expected nil for nil map")
	}
}

func TestMapKeys(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	result := MapKeys(m, func(k string) string {
		return k + k
	})

	if result["aa"] != 1 || result["bb"] != 2 {
		t.Error("MapKeys failed")
	}
}

func TestMerge(t *testing.T) {
	m1 := map[string]int{"a": 1, "b": 2}
	m2 := map[string]int{"b": 3, "c": 4}

	result := Merge(m1, m2)

	if result["a"] != 1 || result["b"] != 3 || result["c"] != 4 {
		t.Error("Merge failed")
	}
}

func TestMergeWith(t *testing.T) {
	m1 := map[string]int{"a": 1, "b": 2}
	m2 := map[string]int{"b": 3, "c": 4}

	result := MergeWith(func(v1, v2 int) int {
		return v1 + v2
	}, m1, m2)

	if result["a"] != 1 || result["b"] != 5 || result["c"] != 4 {
		t.Error("MergeWith failed")
	}
}

func TestInvert(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	result := Invert(m)

	if result[1] != "a" || result[2] != "b" {
		t.Error("Invert failed")
	}

	// Test nil map
	if Invert[string, int](nil) != nil {
		t.Error("expected nil for nil map")
	}
}

func TestPick(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	result := Pick(m, "a", "c")

	if len(result) != 2 || result["a"] != 1 || result["c"] != 3 {
		t.Error("Pick failed")
	}

	// Test picking non-existent key
	result = Pick(m, "a", "x")
	if len(result) != 1 {
		t.Error("Pick should ignore non-existent keys")
	}
}

func TestOmit(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	result := Omit(m, "a", "c")

	if len(result) != 1 || result["b"] != 2 {
		t.Error("Omit failed")
	}
}

func TestContains(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}

	if !Contains(m, "a") {
		t.Error("Contains should return true for existing key")
	}

	if Contains(m, "x") {
		t.Error("Contains should return false for non-existent key")
	}
}

func TestContainsAll(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}

	if !ContainsAll(m, "a", "b") {
		t.Error("ContainsAll should return true")
	}

	if ContainsAll(m, "a", "x") {
		t.Error("ContainsAll should return false")
	}
}

func TestContainsAny(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}

	if !ContainsAny(m, "a", "x") {
		t.Error("ContainsAny should return true")
	}

	if ContainsAny(m, "x", "y") {
		t.Error("ContainsAny should return false")
	}
}

func TestGetOrDefault(t *testing.T) {
	m := map[string]int{"a": 1}

	if GetOrDefault(m, "a", 0) != 1 {
		t.Error("GetOrDefault should return existing value")
	}

	if GetOrDefault(m, "x", 99) != 99 {
		t.Error("GetOrDefault should return default value")
	}
}

func TestGetOrCompute(t *testing.T) {
	m := map[string]int{"a": 1}

	// Existing key
	v := GetOrCompute(m, "a", func() int { return 99 })
	if v != 1 {
		t.Error("GetOrCompute should return existing value")
	}

	// New key
	v = GetOrCompute(m, "b", func() int { return 2 })
	if v != 2 || m["b"] != 2 {
		t.Error("GetOrCompute should compute and store new value")
	}
}

func TestClone(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	cloned := Clone(m)

	if cloned["a"] != 1 || cloned["b"] != 2 {
		t.Error("Clone failed")
	}

	// Modify original
	m["a"] = 99
	if cloned["a"] != 1 {
		t.Error("Clone should create independent copy")
	}

	// Test nil map
	if Clone[string, int](nil) != nil {
		t.Error("expected nil for nil map")
	}
}

func TestEqual(t *testing.T) {
	m1 := map[string]int{"a": 1, "b": 2}
	m2 := map[string]int{"a": 1, "b": 2}
	m3 := map[string]int{"a": 1, "b": 3}
	m4 := map[string]int{"a": 1}

	if !Equal(m1, m2) {
		t.Error("Equal should return true for equal maps")
	}

	if Equal(m1, m3) {
		t.Error("Equal should return false for different values")
	}

	if Equal(m1, m4) {
		t.Error("Equal should return false for different lengths")
	}
}

func TestIsEmpty(t *testing.T) {
	if !IsEmpty(map[string]int{}) {
		t.Error("IsEmpty should return true for empty map")
	}

	if IsEmpty(map[string]int{"a": 1}) {
		t.Error("IsEmpty should return false for non-empty map")
	}
}

func TestForEach(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	sum := 0
	ForEach(m, func(k string, v int) {
		sum += v
	})

	if sum != 3 {
		t.Error("ForEach failed")
	}
}

func TestAny(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}

	if !Any(m, func(k string, v int) bool { return v > 2 }) {
		t.Error("Any should return true")
	}

	if Any(m, func(k string, v int) bool { return v > 10 }) {
		t.Error("Any should return false")
	}
}

func TestAll(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}

	if !All(m, func(k string, v int) bool { return v > 0 }) {
		t.Error("All should return true")
	}

	if All(m, func(k string, v int) bool { return v > 1 }) {
		t.Error("All should return false")
	}
}

func TestNone(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}

	if !None(m, func(k string, v int) bool { return v > 10 }) {
		t.Error("None should return true")
	}

	if None(m, func(k string, v int) bool { return v > 2 }) {
		t.Error("None should return false")
	}
}

func TestCount(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2, "c": 3}

	count := Count(m, func(k string, v int) bool { return v > 1 })
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}
}
