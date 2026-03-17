[中文](README.md) | English

# Slicex - Generic Slice Utilities

Provides functional slice operation utilities supporting Map, Filter, Reduce, and other common operations.

## Features

- ✅ Generics support - Type-safe with compile-time checks
- ✅ Functional operations - Map/Filter/Reduce/FlatMap
- ✅ Search and check - Contains/Find/IndexOf
- ✅ Aggregation - GroupBy/Count/Some/Every
- ✅ Utility functions - Reverse/Chunk/Take/Drop/Unique
- ✅ Zero external dependencies - Uses only the Go standard library
- ✅ Does not modify original slice - All functions return new slices (except *InPlace suffix)

## Quick Start

### Basic Operations

```go
import "github.com/everyday-items/toolkit/lang/slicex"

// Contains check
found := slicex.Contains([]int{1, 2, 3}, 2)  // true

// Find element
user, found := slicex.Find(users, func(u User) bool {
    return u.Role == "admin"
})

// Find index
index := slicex.IndexOf([]string{"a", "b", "c"}, "b")  // 1
```

### Functional Operations

```go
// Map - transform elements
doubled := slicex.Map([]int{1, 2, 3}, func(n int) int {
    return n * 2  // [2, 4, 6]
})

names := slicex.Map(users, func(u User) string {
    return u.Name  // extract all usernames
})

// Filter - filter elements
even := slicex.Filter([]int{1, 2, 3, 4}, func(n int) bool {
    return n%2 == 0  // [2, 4]
})

activeUsers := slicex.Filter(users, func(u User) bool {
    return u.Status == "active"
})

// Reduce - aggregate
sum := slicex.Reduce([]int{1, 2, 3, 4}, 0, func(acc, n int) int {
    return acc + n  // 10
})

concat := slicex.Reduce([]string{"a", "b", "c"}, "", func(acc, s string) string {
    return acc + s  // "abc"
})
```

### Advanced Operations

```go
// FlatMap - map then flatten
result := slicex.FlatMap([]int{1, 2, 3}, func(n int) []int {
    return []int{n, n * 10}  // [1, 10, 2, 20, 3, 30]
})

// GroupBy - group by key
type User struct { City string; Name string }
users := []User{
    {"Beijing", "Alice"},
    {"Shanghai", "Bob"},
    {"Beijing", "Charlie"},
}
groups := slicex.GroupBy(users, func(u User) string {
    return u.City
})
// map[string][]User{
//     "Beijing": [{"Beijing", "Alice"}, {"Beijing", "Charlie"}],
//     "Shanghai": [{"Shanghai", "Bob"}],
// }

// Unique - deduplicate
unique := slicex.Unique([]int{1, 2, 2, 3, 3, 4})  // [1, 2, 3, 4]

// UniqueFunc - custom deduplication
uniqueUsers := slicex.UniqueFunc(users, func(u User) int {
    return u.ID  // deduplicate by ID
})
```

## API Reference

### Search and Check

```go
// Contains - check if element is present
Contains[T comparable](slice []T, item T) bool

// ContainsFunc - check using a custom function
ContainsFunc[T any](slice []T, fn func(T) bool) bool

// Find - find first element matching condition
Find[T any](slice []T, fn func(T) bool) (T, bool)

// FindIndex - find index of first element matching condition
FindIndex[T any](slice []T, fn func(T) bool) int

// FindLast - find last element matching condition
FindLast[T any](slice []T, fn func(T) bool) (T, bool)

// IndexOf - find element index (uses == comparison)
IndexOf[T comparable](slice []T, item T) int
```

### Transform and Map

```go
// Map - transform elements
Map[T any, R any](slice []T, fn func(T) R) []R

// MapWithIndex - transform with index access
MapWithIndex[T any, R any](slice []T, fn func(int, T) R) []R

// FlatMap - map then flatten
FlatMap[T any, R any](slice []T, fn func(T) []R) []R

// Filter - filter elements
Filter[T any](slice []T, fn func(T) bool) []T

// Reject - filter (inverse operation)
Reject[T any](slice []T, fn func(T) bool) []T

// Unique - deduplicate
Unique[T comparable](slice []T) []T

// UniqueFunc - custom deduplication
UniqueFunc[T any, K comparable](slice []T, keyFn func(T) K) []T
```

### Aggregation

```go
// Reduce - aggregate to a single value
Reduce[T any, R any](slice []T, initial R, fn func(R, T) R) R

// Some - check if at least one element matches condition
Some[T any](slice []T, fn func(T) bool) bool

// Every - check if all elements match condition
Every[T any](slice []T, fn func(T) bool) bool

// Count - count elements matching condition
Count[T any](slice []T, fn func(T) bool) int

// GroupBy - group by key
GroupBy[T any, K comparable](slice []T, keyFn func(T) K) map[K][]T
```

### Utility Functions

```go
// Reverse - reverse slice (creates new slice)
Reverse[T any](slice []T) []T

// ReverseInPlace - reverse in-place (modifies original slice)
ReverseInPlace[T any](slice []T)

// Chunk - split slice into chunks
Chunk[T any](slice []T, size int) [][]T

// Take - take first n elements
Take[T any](slice []T, n int) []T

// Drop - skip first n elements
Drop[T any](slice []T, n int) []T
```

## Use Cases

### 1. Data Transformation

```go
// Extract user IDs
userIDs := slicex.Map(users, func(u User) int64 {
    return u.ID
})

// Format as strings
labels := slicex.Map(items, func(item Item) string {
    return fmt.Sprintf("%s (%d)", item.Name, item.Count)
})

// Transform with index
indexed := slicex.MapWithIndex([]string{"a", "b", "c"}, func(i int, s string) string {
    return fmt.Sprintf("%d:%s", i, s)  // ["0:a", "1:b", "2:c"]
})
```

### 2. Data Filtering

```go
// Filter active users
activeUsers := slicex.Filter(users, func(u User) bool {
    return u.Status == "active" && u.LastLogin.After(time.Now().AddDate(0, -1, 0))
})

// Exclude deleted records
validRecords := slicex.Reject(records, func(r Record) bool {
    return r.DeletedAt != nil
})

// Count matching elements
adminCount := slicex.Count(users, func(u User) bool {
    return u.Role == "admin"
})
```

### 3. Data Search

```go
// Find first admin
admin, found := slicex.Find(users, func(u User) bool {
    return u.Role == "admin"
})
if found {
    fmt.Println("Found admin:", admin.Name)
}

// Check if exists
hasAdmin := slicex.Some(users, func(u User) bool {
    return u.Role == "admin"
})

// Check if all match
allActive := slicex.Every(users, func(u User) bool {
    return u.Status == "active"
})
```

### 4. Data Aggregation

```go
// Calculate total price
total := slicex.Reduce(items, 0.0, func(sum float64, item Item) float64 {
    return sum + item.Price
})

// Concatenate strings
names := slicex.Reduce(users, []string{}, func(acc []string, u User) []string {
    return append(acc, u.Name)
})

// Group by city
usersByCity := slicex.GroupBy(users, func(u User) string {
    return u.City
})
// Count users per city
for city, cityUsers := range usersByCity {
    fmt.Printf("%s: %d users\n", city, len(cityUsers))
}
```

### 5. Slicing and Reorganizing

```go
// Pagination
pageSize := 20
pages := slicex.Chunk(items, pageSize)
for i, page := range pages {
    fmt.Printf("Page %d: %d items\n", i+1, len(page))
}

// Take top 10
top10 := slicex.Take(sortedItems, 10)

// Skip first 5
remaining := slicex.Drop(items, 5)

// Reverse order
reversed := slicex.Reverse(items)
```

### 6. Deduplication

```go
// Simple deduplication
unique := slicex.Unique([]int{1, 2, 2, 3, 3, 4})  // [1, 2, 3, 4]

// Deduplicate by field
type User struct { ID int; Name string }
users := []User{{1, "Alice"}, {2, "Bob"}, {1, "Alice2"}}
uniqueUsers := slicex.UniqueFunc(users, func(u User) int {
    return u.ID  // deduplicate by ID, keep first
})  // [{1, "Alice"}, {2, "Bob"}]
```

### 7. Flattening Nested Structures

```go
// Flatten tags
type Article struct {
    Title string
    Tags  []string
}
articles := []Article{
    {"Article 1", []string{"go", "tech"}},
    {"Article 2", []string{"go", "web"}},
}
allTags := slicex.FlatMap(articles, func(a Article) []string {
    return a.Tags
})  // ["go", "tech", "go", "web"]

// After deduplication
uniqueTags := slicex.Unique(allTags)  // ["go", "tech", "web"]
```

## Performance Notes

### Memory Allocation

- **Pre-allocated capacity**: Most functions pre-allocate slice capacity to reduce allocation count
- **Does not modify original slice**: All functions (except *InPlace suffix) return new slices, thread-safe
- **Zero-copy optimization**: `Take`/`Drop` etc. use `copy` to avoid element-by-element copying

### Performance Benchmarks

```
Map:             100 ns/op (n=100)
Filter:          150 ns/op (n=100)
Reduce:          80 ns/op (n=100)
Contains:        50 ns/op (n=100, average case)
Unique:          500 ns/op (n=100, uses map)
GroupBy:         800 ns/op (n=100)
```

### Performance Recommendations

1. **Large datasets**: For millions of records, consider batch processing
2. **Chained calls**: Each call creates a new slice, consider single-pass processing
3. **Concurrent usage**: slicex is stateless and safe for concurrent use

```go
// Not recommended: multiple allocations
result := slicex.Map(items, transformFunc)
result = slicex.Filter(result, filterFunc)
result = slicex.Unique(result)

// Recommended: single-pass processing
result := slicex.Unique(
    slicex.Filter(
        slicex.Map(items, transformFunc),
        filterFunc,
    ),
)

// Better: manual implementation to avoid multiple traversals
result := make([]T, 0, len(items))
seen := make(map[K]struct{})
for _, item := range items {
    transformed := transformFunc(item)
    if filterFunc(transformed) {
        key := keyFunc(transformed)
        if _, ok := seen[key]; !ok {
            seen[key] = struct{}{}
            result = append(result, transformed)
        }
    }
}
```

## Design Principles

1. **Type safety**: Uses generics with compile-time type checks
2. **Does not modify original slice**: Except for functions with the `*InPlace` suffix
3. **Empty slice friendly**: Returns empty slice instead of nil when input is empty
4. **Performance-oriented**: Pre-allocated capacity to reduce memory allocation

## Notes

1. **Concurrency safety**:
   - All functions are pure functions with no state
   - Can be safely used in multiple goroutines
   - Note: Does not modify original slice (except *InPlace functions)

2. **InPlace functions**:
   - `ReverseInPlace` modifies the original slice
   - Not suitable for concurrent scenarios
   - Ensure no other goroutines are reading before use

3. **Nil value handling**:
   - Empty slice returns empty slice (not nil)
   - `Find` etc. return zero value and false when not found
   - `IndexOf` returns -1 when not found

4. **Generic constraints**:
   - `Contains`/`IndexOf`/`Unique` require `comparable` types
   - `Map`/`Filter` etc. support any type `any`
   - `GroupBy` keys must be `comparable`

## Dependencies

Zero external dependencies, uses only the Go standard library.

## Extension Suggestions

For more functionality, consider:
- `github.com/samber/lo` - Richer functional utilities
- `golang.org/x/exp/slices` - Go official experimental slice utilities
- `github.com/thoas/go-funk` - Another popular functional library
