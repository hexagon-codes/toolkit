[中文](README.md) | English

# Slice Utility

Provides generic functions for common slice operations, simplifying slice handling.

## Features

- ✅ Generic support (Go 1.18+)
- ✅ Deduplication, search, deletion
- ✅ Reverse, shuffle, chunk
- ✅ Set operations (union, intersection, difference)
- ✅ Aggregate functions (sum, max, min)
- ✅ Higher-order functions (Any, All, GroupBy)
- ✅ Zero external dependencies

## Quick Start

### Basic Operations

```go
import "github.com/everyday-items/toolkit/util/slice"

// Deduplication
nums := []int{1, 2, 2, 3, 3, 3}
unique := slice.Unique(nums)
// Output: [1, 2, 3]

// Find element
exists := slice.Contains([]string{"a", "b", "c"}, "b")  // true

// Find index
index := slice.IndexOf([]int{1, 2, 3}, 2)  // 1
index := slice.LastIndexOf([]int{1, 2, 2, 3}, 2)  // 2

// Delete elements
nums := []int{1, 2, 3, 4, 5}
result := slice.Remove(nums, 3)     // [1, 2, 4, 5]
result := slice.RemoveAt(nums, 2)   // [1, 2, 4, 5]
result := slice.RemoveAll([]int{1, 2, 2, 3}, 2)  // [1, 3]
```

### Array Transformation

```go
// Reverse
nums := []int{1, 2, 3, 4, 5}
reversed := slice.Reverse(nums)  // [5, 4, 3, 2, 1]

// Shuffle (simplified randomization)
shuffled := slice.Shuffle(nums)  // [3, 1, 5, 2, 4]

// Chunk
nums := []int{1, 2, 3, 4, 5, 6, 7}
chunks := slice.Chunk(nums, 3)
// Output: [[1, 2, 3], [4, 5, 6], [7]]

// Flatten
nested := [][]int{{1, 2}, {3, 4}, {5}}
flat := slice.Flatten(nested)  // [1, 2, 3, 4, 5]
```

### Set Operations

```go
a := []int{1, 2, 3, 4}
b := []int{3, 4, 5, 6}

// Union
union := slice.Union(a, b)  // [1, 2, 3, 4, 5, 6]

// Intersection
intersect := slice.Intersect(a, b)  // [3, 4]

// Difference (in a but not in b)
diff := slice.Difference(a, b)  // [1, 2]

// Check equality
equal := slice.Equal([]int{1, 2, 3}, []int{1, 2, 3})  // true
```

### Aggregate Functions

```go
nums := []int{1, 2, 3, 4, 5}

// Sum
sum := slice.Sum(nums)  // 15

// Max
max := slice.Max(nums)  // 5

// Min
min := slice.Min(nums)  // 1

// Float sum
floats := []float64{1.5, 2.5, 3.0}
sum := slice.SumFloat(floats)  // 7.0
```

### Higher-Order Functions

```go
nums := []int{1, 2, 3, 4, 5}

// Check if any element satisfies condition
hasEven := slice.Any(nums, func(n int) bool {
    return n%2 == 0
})  // true

// Check if all elements satisfy condition
allPositive := slice.All(nums, func(n int) bool {
    return n > 0
})  // true

// Group by
type User struct {
    Name string
    Age  int
}

users := []User{
    {"Alice", 30},
    {"Bob", 25},
    {"Charlie", 30},
}

grouped := slice.GroupBy(users, func(u User) int {
    return u.Age
})
// Output: map[25:[{Bob 25}] 30:[{Alice 30} {Charlie 30}]]

// Count by
counts := slice.CountBy(users, func(u User) int {
    return u.Age
})
// Output: map[25:1 30:2]
```

### First and Last Elements

```go
nums := []int{1, 2, 3, 4, 5}

// Get first element
first, ok := slice.First(nums)  // 1, true

// Get last element
last, ok := slice.Last(nums)  // 5, true

// Empty slice
empty := []int{}
first, ok := slice.First(empty)  // 0, false
```

## API Reference

### Basic Operations

```go
// Unique deduplicates (preserving order)
Unique[T comparable](slice []T) []T

// Contains checks if slice contains an element
Contains[T comparable](slice []T, item T) bool

// IndexOf finds element index, returns -1 if not found
IndexOf[T comparable](slice []T, item T) int

// LastIndexOf finds last occurrence index of element
LastIndexOf[T comparable](slice []T, item T) int

// Remove removes the first matching element
Remove[T comparable](slice []T, item T) []T

// RemoveAll removes all matching elements
RemoveAll[T comparable](slice []T, item T) []T

// RemoveAt removes element at specified index
RemoveAt[T any](slice []T, index int) []T
```

### Array Transformation

```go
// Reverse reverses the slice
Reverse[T any](slice []T) []T

// Shuffle randomizes the order of elements
Shuffle[T any](slice []T) []T

// Chunk splits slice into sub-slices
Chunk[T any](slice []T, size int) [][]T

// Flatten flattens a 2D slice
Flatten[T any](slices [][]T) []T
```

### Set Operations

```go
// Union returns the union
Union[T comparable](slice1, slice2 []T) []T

// Intersect returns the intersection
Intersect[T comparable](slice1, slice2 []T) []T

// Difference returns elements in slice1 but not in slice2
Difference[T comparable](slice1, slice2 []T) []T

// Equal checks if two slices are equal
Equal[T comparable](slice1, slice2 []T) bool
```

### Aggregate Functions

```go
// Sum calculates the sum (integers)
Sum[T int | int64 | int32](slice []T) T

// SumFloat calculates the sum (floats)
SumFloat[T float32 | float64](slice []T) T

// Max gets the maximum value
Max[T int | int64 | int32 | float32 | float64](slice []T) T

// Min gets the minimum value
Min[T int | int64 | int32 | float32 | float64](slice []T) T
```

### Higher-Order Functions

```go
// GroupBy groups elements by key
GroupBy[T any, K comparable](slice []T, keyFunc func(T) K) map[K][]T

// CountBy counts elements by key
CountBy[T any, K comparable](slice []T, keyFunc func(T) K) map[K]int

// Any checks if any element satisfies the condition
Any[T any](slice []T, predicate func(T) bool) bool

// All checks if all elements satisfy the condition
All[T any](slice []T, predicate func(T) bool) bool
```

### First and Last

```go
// First gets the first element
First[T any](slice []T) (T, bool)

// Last gets the last element
Last[T any](slice []T) (T, bool)
```

## Use Cases

### 1. Data Deduplication

```go
// User ID deduplication
userIDs := []int64{1, 2, 2, 3, 3, 3, 4}
uniqueIDs := slice.Unique(userIDs)
// Output: [1, 2, 3, 4]

// String deduplication
tags := []string{"go", "python", "go", "java"}
uniqueTags := slice.Unique(tags)
// Output: ["go", "python", "java"]
```

### 2. Permission Checking

```go
func HasPermission(userPerms []string, required string) bool {
    return slice.Contains(userPerms, required)
}

// Usage
userPerms := []string{"read", "write", "delete"}
if HasPermission(userPerms, "write") {
    // Allow write
}
```

### 3. Data Pagination

```go
func PaginateData[T any](data []T, page, pageSize int) []T {
    chunks := slice.Chunk(data, pageSize)

    if page < 1 || page > len(chunks) {
        return []T{}
    }

    return chunks[page-1]
}

// Usage
users := []User{...}  // 100 users
page2Users := PaginateData(users, 2, 10)  // page 2, 10 per page
```

### 4. Data Statistics

```go
type Order struct {
    ID     int
    Amount float64
    Status string
}

orders := []Order{...}

// Group by status
grouped := slice.GroupBy(orders, func(o Order) string {
    return o.Status
})
// grouped["completed"] = [...completed orders...]
// grouped["pending"] = [...pending orders...]

// Count orders per status
counts := slice.CountBy(orders, func(o Order) string {
    return o.Status
})
// counts["completed"] = 15
// counts["pending"] = 8
```

### 5. Data Filtering and Validation

```go
// Check if all prices are positive
prices := []float64{10.5, 20.0, 30.5}
allPositive := slice.All(prices, func(p float64) bool {
    return p > 0
})

// Check if any price exceeds 100
hasExpensive := slice.Any(prices, func(p float64) bool {
    return p > 100
})
```

### 6. Data Merging and Comparison

```go
// Merge two tag lists
userTags := []string{"go", "python"}
systemTags := []string{"admin", "go"}

allTags := slice.Union(userTags, systemTags)
// Output: ["go", "python", "admin"]

// Find common tags
commonTags := slice.Intersect(userTags, systemTags)
// Output: ["go"]

// Find user-only tags
userOnlyTags := slice.Difference(userTags, systemTags)
// Output: ["python"]
```

### 7. Batch Processing

```go
// Batch send emails
emails := []string{...}  // 1000 emails
batches := slice.Chunk(emails, 100)  // 100 per batch

for _, batch := range batches {
    sendEmails(batch)
    time.Sleep(time.Second)  // Rate limiting
}
```

### 8. Data Transformation

```go
// Extract all user IDs
type User struct {
    ID   int
    Name string
}

users := []User{{1, "Alice"}, {2, "Bob"}, {3, "Charlie"}}

// Use GroupBy trick to extract IDs
idMap := slice.GroupBy(users, func(u User) int {
    return u.ID
})
ids := make([]int, 0, len(idMap))
for id := range idMap {
    ids = append(ids, id)
}
// ids = [1, 2, 3]
```

### 9. Leaderboard

```go
scores := []int{95, 87, 92, 88, 100, 76}

// Get highest score
highest := slice.Max(scores)  // 100

// Get lowest score
lowest := slice.Min(scores)  // 76

// Calculate average score
average := float64(slice.Sum(scores)) / float64(len(scores))  // 89.67
```

### 10. Data Cleaning

```go
// Remove all empty strings
data := []string{"a", "", "b", "", "c"}
cleaned := slice.RemoveAll(data, "")
// Output: ["a", "b", "c"]

// Remove specific value
numbers := []int{1, 2, 3, 0, 4, 0, 5}
nonZero := slice.RemoveAll(numbers, 0)
// Output: [1, 2, 3, 4, 5]
```

## Generic Constraints

### comparable Constraint

Functions requiring equality comparison use the `comparable` constraint:

```go
// ✅ Supported
slice.Contains([]int{1, 2, 3}, 2)
slice.Contains([]string{"a", "b"}, "a")
slice.Unique([]float64{1.1, 2.2, 1.1})

// ❌ Not supported
type User struct { Name string }
slice.Contains([]User{{Name: "Alice"}}, User{Name: "Alice"})
// Error: User type does not satisfy comparable constraint
```

### Numeric Constraint

Aggregate functions use numeric type constraints:

```go
// ✅ Supported
slice.Sum([]int{1, 2, 3})
slice.Max([]float64{1.1, 2.2, 3.3})
slice.Min([]int32{10, 20, 30})

// ❌ Not supported
slice.Sum([]string{"a", "b"})  // Compile error
```

## Performance

```
Unique():       O(n)
Contains():     O(n)
Remove():       O(n)
Reverse():      O(n)
Chunk():        O(n)
Union():        O(n+m)
Intersect():    O(n*m)
Sum():          O(n)
GroupBy():      O(n)
```

## Notes

1. **Does Not Modify Original Slice**:
   - All functions return new slices
   - Original slice remains unchanged

2. **Shuffle Implementation**:
   - Current implementation is a simplified version
   - Does not use true random numbers
   - Not suitable for security-sensitive scenarios

3. **Empty Slice Handling**:
   - `Max/Min` returns zero value
   - `First/Last` returns `false`

4. **Memory Usage**:
   - Returning new slices allocates memory
   - Be mindful of performance with large slices

5. **Concurrency Safety**:
   - Functions themselves are stateless
   - But slices themselves are not concurrency-safe

## Dependencies

```bash
# Zero external dependencies, pure Go standard library
# Requires Go 1.18+ (generics support)
```

## Extension Suggestions

For more powerful slice operations, consider:
- `github.com/samber/lo` - Lodash-style Go utility library
- `golang.org/x/exp/slices` - Go official experimental slices package (included in standard library since Go 1.21)
- `github.com/thoas/go-funk` - Functional programming utilities

## Upgrade Suggestions

Go 1.21+ can use the standard library `slices` package:

```go
import "slices"

// Standard library provides more functionality
slices.Sort(nums)
slices.Reverse(nums)
slices.Contains(nums, 3)
slices.Index(nums, 3)
```
