package slicex

import (
	"reflect"
	"testing"
)

// TestContains 测试 Contains 函数
func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []int
		item     int
		expected bool
	}{
		{"找到元素", []int{1, 2, 3}, 2, true},
		{"未找到元素", []int{1, 2, 3}, 4, false},
		{"空切片", []int{}, 1, false},
		{"单元素-找到", []int{1}, 1, true},
		{"单元素-未找到", []int{1}, 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Contains(tt.slice, tt.item)
			if result != tt.expected {
				t.Errorf("Contains(%v, %d) = %v, want %v",
					tt.slice, tt.item, result, tt.expected)
			}
		})
	}
}

// TestContainsFunc 测试 ContainsFunc 函数
func TestContainsFunc(t *testing.T) {
	isEven := func(n int) bool { return n%2 == 0 }

	tests := []struct {
		name     string
		slice    []int
		expected bool
	}{
		{"有满足条件的", []int{1, 2, 3}, true},
		{"无满足条件的", []int{1, 3, 5}, false},
		{"空切片", []int{}, false},
		{"全满足", []int{2, 4, 6}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContainsFunc(tt.slice, isEven)
			if result != tt.expected {
				t.Errorf("ContainsFunc(%v, isEven) = %v, want %v",
					tt.slice, result, tt.expected)
			}
		})
	}
}

// TestUnique 测试 Unique 函数
func TestUnique(t *testing.T) {
	tests := []struct {
		name     string
		slice    []int
		expected []int
	}{
		{"有重复", []int{1, 2, 2, 3, 3, 4}, []int{1, 2, 3, 4}},
		{"无重复", []int{1, 2, 3}, []int{1, 2, 3}},
		{"空切片", []int{}, []int{}},
		{"全部重复", []int{1, 1, 1}, []int{1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Unique(tt.slice)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Unique(%v) = %v, want %v",
					tt.slice, result, tt.expected)
			}
		})
	}
}

// TestUniqueFunc 测试 UniqueFunc 函数
func TestUniqueFunc(t *testing.T) {
	type User struct {
		ID   int
		Name string
	}

	t.Run("正常去重", func(t *testing.T) {
		users := []User{
			{1, "Alice"},
			{2, "Bob"},
			{1, "Alice2"},
			{3, "Charlie"},
		}

		result := UniqueFunc(users, func(u User) int {
			return u.ID
		})

		if len(result) != 3 {
			t.Errorf("UniqueFunc() returned %d users, want 3", len(result))
		}

		if result[0].Name != "Alice" || result[1].Name != "Bob" || result[2].Name != "Charlie" {
			t.Errorf("UniqueFunc() returned unexpected users")
		}
	})

	t.Run("空切片", func(t *testing.T) {
		var users []User
		result := UniqueFunc(users, func(u User) int {
			return u.ID
		})

		if len(result) != 0 {
			t.Errorf("UniqueFunc([]) returned %d users, want 0", len(result))
		}
	})
}

// TestFilter 测试 Filter 函数
func TestFilter(t *testing.T) {
	isEven := func(n int) bool { return n%2 == 0 }

	tests := []struct {
		name     string
		slice    []int
		expected []int
	}{
		{"过滤偶数", []int{1, 2, 3, 4}, []int{2, 4}},
		{"全部过滤", []int{1, 3, 5}, []int{}},
		{"全部保留", []int{2, 4, 6}, []int{2, 4, 6}},
		{"空切片", []int{}, []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Filter(tt.slice, isEven)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Filter(%v, isEven) = %v, want %v",
					tt.slice, result, tt.expected)
			}
		})
	}
}

// TestReject 测试 Reject 函数
func TestReject(t *testing.T) {
	isEven := func(n int) bool { return n%2 == 0 }

	tests := []struct {
		name     string
		slice    []int
		expected []int
	}{
		{"排除偶数", []int{1, 2, 3, 4}, []int{1, 3}},
		{"全部排除", []int{2, 4, 6}, []int{}},
		{"全部保留", []int{1, 3, 5}, []int{1, 3, 5}},
		{"空切片", []int{}, []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Reject(tt.slice, isEven)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Reject(%v, isEven) = %v, want %v",
					tt.slice, result, tt.expected)
			}
		})
	}
}

// TestMap 测试 Map 函数
func TestMap(t *testing.T) {
	double := func(n int) int { return n * 2 }

	tests := []struct {
		name     string
		slice    []int
		expected []int
	}{
		{"正常映射", []int{1, 2, 3}, []int{2, 4, 6}},
		{"空切片", []int{}, []int{}},
		{"单元素", []int{5}, []int{10}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Map(tt.slice, double)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Map(%v, double) = %v, want %v",
					tt.slice, result, tt.expected)
			}
		})
	}
}

// TestMapWithIndex 测试 MapWithIndex 函数
func TestMapWithIndex(t *testing.T) {
	addIndex := func(i int, n int) int { return n + i }

	tests := []struct {
		name     string
		slice    []int
		expected []int
	}{
		{"正常映射", []int{10, 20, 30}, []int{10, 21, 32}},
		{"空切片", []int{}, []int{}},
		{"单元素", []int{5}, []int{5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapWithIndex(tt.slice, addIndex)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("MapWithIndex(%v, addIndex) = %v, want %v",
					tt.slice, result, tt.expected)
			}
		})
	}
}

// TestFlatMap 测试 FlatMap 函数
func TestFlatMap(t *testing.T) {
	duplicate := func(n int) []int { return []int{n, n} }

	tests := []struct {
		name     string
		slice    []int
		expected []int
	}{
		{"正常展开", []int{1, 2, 3}, []int{1, 1, 2, 2, 3, 3}},
		{"空切片", []int{}, []int{}},
		{"单元素", []int{5}, []int{5, 5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FlatMap(tt.slice, duplicate)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("FlatMap(%v, duplicate) = %v, want %v",
					tt.slice, result, tt.expected)
			}
		})
	}
}

// TestFind 测试 Find 函数
func TestFind(t *testing.T) {
	greaterThan2 := func(n int) bool { return n > 2 }

	tests := []struct {
		name          string
		slice         []int
		expectedValue int
		expectedFound bool
	}{
		{"找到元素", []int{1, 2, 3, 4}, 3, true},
		{"未找到", []int{1, 2}, 0, false},
		{"空切片", []int{}, 0, false},
		{"找到第一个", []int{1, 3, 4}, 3, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, found := Find(tt.slice, greaterThan2)
			if value != tt.expectedValue || found != tt.expectedFound {
				t.Errorf("Find(%v, greaterThan2) = (%d, %v), want (%d, %v)",
					tt.slice, value, found, tt.expectedValue, tt.expectedFound)
			}
		})
	}
}

// TestFindIndex 测试 FindIndex 函数
func TestFindIndex(t *testing.T) {
	greaterThan2 := func(n int) bool { return n > 2 }

	tests := []struct {
		name     string
		slice    []int
		expected int
	}{
		{"找到元素", []int{1, 2, 3, 4}, 2},
		{"未找到", []int{1, 2}, -1},
		{"空切片", []int{}, -1},
		{"找到第一个", []int{1, 3, 4}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FindIndex(tt.slice, greaterThan2)
			if result != tt.expected {
				t.Errorf("FindIndex(%v, greaterThan2) = %d, want %d",
					tt.slice, result, tt.expected)
			}
		})
	}
}

// TestFindLast 测试 FindLast 函数
func TestFindLast(t *testing.T) {
	isEven := func(n int) bool { return n%2 == 0 }

	tests := []struct {
		name          string
		slice         []int
		expectedValue int
		expectedFound bool
	}{
		{"找到最后一个", []int{1, 2, 3, 4}, 4, true},
		{"未找到", []int{1, 3, 5}, 0, false},
		{"空切片", []int{}, 0, false},
		{"单个元素", []int{2}, 2, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, found := FindLast(tt.slice, isEven)
			if value != tt.expectedValue || found != tt.expectedFound {
				t.Errorf("FindLast(%v, isEven) = (%d, %v), want (%d, %v)",
					tt.slice, value, found, tt.expectedValue, tt.expectedFound)
			}
		})
	}
}

// TestIndexOf 测试 IndexOf 函数
func TestIndexOf(t *testing.T) {
	tests := []struct {
		name     string
		slice    []int
		item     int
		expected int
	}{
		{"找到元素", []int{1, 2, 3}, 2, 1},
		{"未找到", []int{1, 2, 3}, 4, -1},
		{"空切片", []int{}, 1, -1},
		{"第一个元素", []int{5, 2, 3}, 5, 0},
		{"最后一个元素", []int{1, 2, 5}, 5, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IndexOf(tt.slice, tt.item)
			if result != tt.expected {
				t.Errorf("IndexOf(%v, %d) = %d, want %d",
					tt.slice, tt.item, result, tt.expected)
			}
		})
	}
}

// TestReverse 测试 Reverse 函数
func TestReverse(t *testing.T) {
	tests := []struct {
		name     string
		slice    []int
		expected []int
	}{
		{"正常反转", []int{1, 2, 3}, []int{3, 2, 1}},
		{"空切片", []int{}, []int{}},
		{"单元素", []int{1}, []int{1}},
		{"两元素", []int{1, 2}, []int{2, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Reverse(tt.slice)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Reverse(%v) = %v, want %v",
					tt.slice, result, tt.expected)
			}
		})
	}
}

// TestReverseInPlace 测试 ReverseInPlace 函数
func TestReverseInPlace(t *testing.T) {
	tests := []struct {
		name     string
		slice    []int
		expected []int
	}{
		{"正常反转", []int{1, 2, 3}, []int{3, 2, 1}},
		{"空切片", []int{}, []int{}},
		{"单元素", []int{1}, []int{1}},
		{"两元素", []int{1, 2}, []int{2, 1}},
		{"四元素", []int{1, 2, 3, 4}, []int{4, 3, 2, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slice := make([]int, len(tt.slice))
			copy(slice, tt.slice)
			ReverseInPlace(slice)
			if !reflect.DeepEqual(slice, tt.expected) {
				t.Errorf("ReverseInPlace(%v) = %v, want %v",
					tt.slice, slice, tt.expected)
			}
		})
	}
}

// TestTake 测试 Take 函数
func TestTake(t *testing.T) {
	tests := []struct {
		name     string
		slice    []int
		n        int
		expected []int
	}{
		{"取前3个", []int{1, 2, 3, 4, 5}, 3, []int{1, 2, 3}},
		{"n为0", []int{1, 2, 3}, 0, []int{}},
		{"n小于0", []int{1, 2, 3}, -1, []int{}},
		{"n大于长度", []int{1, 2}, 5, []int{1, 2}},
		{"空切片", []int{}, 3, []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Take(tt.slice, tt.n)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Take(%v, %d) = %v, want %v",
					tt.slice, tt.n, result, tt.expected)
			}
		})
	}
}

// TestDrop 测试 Drop 函数
func TestDrop(t *testing.T) {
	tests := []struct {
		name     string
		slice    []int
		n        int
		expected []int
	}{
		{"跳过前2个", []int{1, 2, 3, 4, 5}, 2, []int{3, 4, 5}},
		{"n为0", []int{1, 2, 3}, 0, []int{1, 2, 3}},
		{"n小于0", []int{1, 2, 3}, -1, []int{1, 2, 3}},
		{"n大于长度", []int{1, 2}, 5, []int{}},
		{"空切片", []int{}, 3, []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Drop(tt.slice, tt.n)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Drop(%v, %d) = %v, want %v",
					tt.slice, tt.n, result, tt.expected)
			}
		})
	}
}

// TestChunk 测试 Chunk 函数
func TestChunk(t *testing.T) {
	tests := []struct {
		name     string
		slice    []int
		size     int
		expected [][]int
	}{
		{"正常分块", []int{1, 2, 3, 4, 5}, 2, [][]int{{1, 2}, {3, 4}, {5}}},
		{"整除", []int{1, 2, 3, 4}, 2, [][]int{{1, 2}, {3, 4}}},
		{"大于长度", []int{1, 2}, 5, [][]int{{1, 2}}},
		{"空切片", []int{}, 2, [][]int{}},
		{"size为0", []int{1, 2}, 0, [][]int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Chunk(tt.slice, tt.size)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Chunk(%v, %d) = %v, want %v",
					tt.slice, tt.size, result, tt.expected)
			}
		})
	}
}

// TestReduce 测试 Reduce 函数
func TestReduce(t *testing.T) {
	sum := func(acc, n int) int { return acc + n }

	tests := []struct {
		name     string
		slice    []int
		initial  int
		expected int
	}{
		{"求和", []int{1, 2, 3, 4}, 0, 10},
		{"空切片", []int{}, 0, 0},
		{"带初始值", []int{1, 2, 3}, 10, 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Reduce(tt.slice, tt.initial, sum)
			if result != tt.expected {
				t.Errorf("Reduce(%v, %d, sum) = %d, want %d",
					tt.slice, tt.initial, result, tt.expected)
			}
		})
	}
}

// TestSome 测试 Some 函数
func TestSome(t *testing.T) {
	isEven := func(n int) bool { return n%2 == 0 }

	tests := []struct {
		name     string
		slice    []int
		expected bool
	}{
		{"有满足的", []int{1, 2, 3}, true},
		{"无满足的", []int{1, 3, 5}, false},
		{"空切片", []int{}, false},
		{"全满足", []int{2, 4, 6}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Some(tt.slice, isEven)
			if result != tt.expected {
				t.Errorf("Some(%v, isEven) = %v, want %v",
					tt.slice, result, tt.expected)
			}
		})
	}
}

// TestEvery 测试 Every 函数
func TestEvery(t *testing.T) {
	isPositive := func(n int) bool { return n > 0 }

	tests := []struct {
		name     string
		slice    []int
		expected bool
	}{
		{"全满足", []int{1, 2, 3}, true},
		{"部分满足", []int{1, -1, 3}, false},
		{"全不满足", []int{-1, -2, -3}, false},
		{"空切片", []int{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Every(tt.slice, isPositive)
			if result != tt.expected {
				t.Errorf("Every(%v, isPositive) = %v, want %v",
					tt.slice, result, tt.expected)
			}
		})
	}
}

// TestCount 测试 Count 函数
func TestCount(t *testing.T) {
	isEven := func(n int) bool { return n%2 == 0 }

	tests := []struct {
		name     string
		slice    []int
		expected int
	}{
		{"有偶数", []int{1, 2, 3, 4}, 2},
		{"全是偶数", []int{2, 4, 6}, 3},
		{"没有偶数", []int{1, 3, 5}, 0},
		{"空切片", []int{}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Count(tt.slice, isEven)
			if result != tt.expected {
				t.Errorf("Count(%v, isEven) = %d, want %d",
					tt.slice, result, tt.expected)
			}
		})
	}
}

// TestGroupBy 测试 GroupBy 函数
func TestGroupBy(t *testing.T) {
	type User struct {
		City string
		Name string
	}

	users := []User{
		{"Beijing", "Alice"},
		{"Shanghai", "Bob"},
		{"Beijing", "Charlie"},
	}

	result := GroupBy(users, func(u User) string {
		return u.City
	})

	if len(result) != 2 {
		t.Errorf("GroupBy() returned %d groups, want 2", len(result))
	}

	if len(result["Beijing"]) != 2 {
		t.Errorf("Beijing group has %d users, want 2", len(result["Beijing"]))
	}

	if len(result["Shanghai"]) != 1 {
		t.Errorf("Shanghai group has %d users, want 1", len(result["Shanghai"]))
	}
}

// Benchmark 测试
func BenchmarkContains(b *testing.B) {
	slice := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for i := 0; i < b.N; i++ {
		Contains(slice, 5)
	}
}

func BenchmarkFilter(b *testing.B) {
	slice := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	isEven := func(n int) bool { return n%2 == 0 }
	for i := 0; i < b.N; i++ {
		Filter(slice, isEven)
	}
}

func BenchmarkMap(b *testing.B) {
	slice := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	double := func(n int) int { return n * 2 }
	for i := 0; i < b.N; i++ {
		Map(slice, double)
	}
}
