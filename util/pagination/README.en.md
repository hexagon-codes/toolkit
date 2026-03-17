[中文](README.md) | English

# Pagination Utility

A simple and easy-to-use pagination calculation tool that provides complete pagination information.

## Features

- ✅ Automatic offset and total page count calculation
- ✅ Previous/next page detection
- ✅ Page number list generation (for pagination navigation)
- ✅ offset/limit conversion support
- ✅ Automatic parameter validation and boundary handling
- ✅ Zero external dependencies

## Quick Start

### Basic Usage

```go
import "github.com/everyday-items/toolkit/util/pagination"

// Create pagination (page 1, 10 items per page, 95 total records)
p := pagination.New(1, 10, 95)

fmt.Println(p.Page)        // 1
fmt.Println(p.PageSize)    // 10
fmt.Println(p.Total)       // 95
fmt.Println(p.TotalPages)  // 10
fmt.Println(p.Offset)      // 0
fmt.Println(p.Limit)       // 10
fmt.Println(p.HasPrev)     // false
fmt.Println(p.HasNext)     // true
```

### Use with Database Queries

```go
// Get pagination parameters
page := 2
pageSize := 20
total := int64(156)

p := pagination.New(page, pageSize, total)

// Use in SQL query
query := "SELECT * FROM users ORDER BY id LIMIT ? OFFSET ?"
rows, err := db.Query(query, p.Limit, p.Offset)

// p.Offset = 20, p.Limit = 20
```

### Default Pagination

```go
// Create default pagination (page 1, 10 items per page)
p := pagination.NewDefault(95)

fmt.Println(p.Page)      // 1
fmt.Println(p.PageSize)  // 10
```

### Create Pagination from Offset

```go
// Create pagination object from offset/limit
p := pagination.NewWithOffset(20, 10, 95)

fmt.Println(p.Page)  // 3 (offset 20 ÷ limit 10 + 1)
```

## API Reference

### Struct

```go
type Pagination struct {
    Page       int   // Current page number (starts at 1)
    PageSize   int   // Page size
    Total      int64 // Total record count
    TotalPages int   // Total page count
    Offset     int   // Offset (for SQL OFFSET)
    Limit      int   // Limit (for SQL LIMIT)
    HasPrev    bool  // Has previous page
    HasNext    bool  // Has next page
}
```

### Constructors

```go
// New creates pagination info
New(page, pageSize int, total int64) *Pagination

// NewDefault creates default pagination (page 1, 10 items per page)
NewDefault(total int64) *Pagination

// NewWithOffset creates pagination from offset and limit
NewWithOffset(offset, limit int, total int64) *Pagination
```

### Methods

```go
// GetRange gets the data range for the current page [start, end)
GetRange() (start, end int)

// IsFirstPage checks if it's the first page
IsFirstPage() bool

// IsLastPage checks if it's the last page
IsLastPage() bool

// PrevPage gets the previous page number
PrevPage() int

// NextPage gets the next page number
NextPage() int

// GetPageNumbers gets the page number list (for pagination navigation)
GetPageNumbers(maxDisplay int) []int
```

## Use Cases

### 1. RESTful API Pagination

```go
func ListUsers(c *gin.Context) {
    // Get pagination parameters
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

    // Count total records
    var total int64
    db.Model(&User{}).Count(&total)

    // Create pagination
    p := pagination.New(page, pageSize, total)

    // Query data
    var users []User
    db.Limit(p.Limit).Offset(p.Offset).Find(&users)

    // Return response
    c.JSON(200, gin.H{
        "data":       users,
        "pagination": p,
    })
}

// Response example:
// {
//   "data": [...],
//   "pagination": {
//     "page": 2,
//     "page_size": 10,
//     "total": 95,
//     "total_pages": 10,
//     "has_prev": true,
//     "has_next": true
//   }
// }
```

### 2. Database Paginated Query

```go
func GetArticles(page, pageSize int) ([]Article, *pagination.Pagination, error) {
    // Count total records
    var total int64
    if err := db.Model(&Article{}).Count(&total).Error; err != nil {
        return nil, nil, err
    }

    // Create pagination
    p := pagination.New(page, pageSize, total)

    // Query data
    var articles []Article
    err := db.Limit(p.Limit).Offset(p.Offset).
        Order("created_at DESC").
        Find(&articles).Error

    return articles, p, err
}
```

### 3. HTML Pagination Navigation

```go
func RenderPagination(w http.ResponseWriter, p *pagination.Pagination) {
    // Get page number list (show at most 10 page numbers)
    pageNumbers := p.GetPageNumbers(10)

    html := `<div class="pagination">`

    // Previous button
    if p.HasPrev {
        html += fmt.Sprintf(`<a href="?page=%d">Previous</a>`, p.PrevPage())
    }

    // Page number list
    for _, num := range pageNumbers {
        if num == p.Page {
            html += fmt.Sprintf(`<span class="current">%d</span>`, num)
        } else {
            html += fmt.Sprintf(`<a href="?page=%d">%d</a>`, num, num)
        }
    }

    // Next button
    if p.HasNext {
        html += fmt.Sprintf(`<a href="?page=%d">Next</a>`, p.NextPage())
    }

    html += `</div>`
    w.Write([]byte(html))
}
```

### 4. Pagination Info Display

```go
func ShowPageInfo(p *pagination.Pagination) string {
    start, end := p.GetRange()
    return fmt.Sprintf("Showing %d-%d of %d", start+1, end, p.Total)
}

// Output example:
// "Showing 11-20 of 95"
```

### 5. Cursor Pagination (Infinite Scroll)

```go
func GetArticlesWithCursor(offset, limit int) ([]Article, *pagination.Pagination, error) {
    // Count total records
    var total int64
    db.Model(&Article{}).Count(&total)

    // Create pagination from offset
    p := pagination.NewWithOffset(offset, limit, total)

    // Query data
    var articles []Article
    err := db.Limit(p.Limit).Offset(p.Offset).Find(&articles).Error

    return articles, p, err
}

// Frontend calls:
// First time: offset=0, limit=20
// Second time: offset=20, limit=20
// Third time: offset=40, limit=20
```

### 6. In-Memory Data Pagination

```go
func PaginateSlice[T any](data []T, page, pageSize int) ([]T, *pagination.Pagination) {
    total := int64(len(data))
    p := pagination.New(page, pageSize, total)

    start, end := p.GetRange()

    // Boundary check
    if start >= len(data) {
        return []T{}, p
    }
    if end > len(data) {
        end = len(data)
    }

    return data[start:end], p
}

// Usage example
users := []User{...} // 100 users
pagedUsers, p := PaginateSlice(users, 2, 10)
// pagedUsers contains users 11-20
```

### 7. Multi-Condition Paginated Query

```go
func SearchProducts(keyword string, page, pageSize int) ([]Product, *pagination.Pagination, error) {
    query := db.Model(&Product{})

    // Add search conditions
    if keyword != "" {
        query = query.Where("name LIKE ?", "%"+keyword+"%")
    }

    // Count total records
    var total int64
    if err := query.Count(&total).Error; err != nil {
        return nil, nil, err
    }

    // Create pagination
    p := pagination.New(page, pageSize, total)

    // Query data
    var products []Product
    err := query.Limit(p.Limit).Offset(p.Offset).Find(&products).Error

    return products, p, err
}
```

## Automatic Parameter Validation

The pagination utility automatically validates and adjusts parameters:

```go
// page < 1 is automatically set to 1
p := pagination.New(0, 10, 100)
fmt.Println(p.Page)  // 1

// pageSize < 1 is automatically set to 10
p := pagination.New(1, 0, 100)
fmt.Println(p.PageSize)  // 10

// pageSize > 1000 is automatically capped at 1000 (prevents overly large queries)
p := pagination.New(1, 5000, 100)
fmt.Println(p.PageSize)  // 1000

// page exceeding total pages is automatically adjusted
p := pagination.New(999, 10, 100)
fmt.Println(p.Page)  // 10 (total pages)
```

## Pagination Navigation Example

```go
// Get page number list
p := pagination.New(5, 10, 200)  // page 5, 20 total pages

// Show at most 10 page numbers
pages := p.GetPageNumbers(10)
// Output: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]

// Current page in the middle
p := pagination.New(10, 10, 200)
pages := p.GetPageNumbers(10)
// Output: [6, 7, 8, 9, 10, 11, 12, 13, 14, 15]

// Near the end
p := pagination.New(18, 10, 200)
pages := p.GetPageNumbers(10)
// Output: [11, 12, 13, 14, 15, 16, 17, 18, 19, 20]
```

## Edge Case Handling

```go
// Empty data
p := pagination.New(1, 10, 0)
fmt.Println(p.TotalPages)  // 0
fmt.Println(p.HasNext)     // false
start, end := p.GetRange()
// start=0, end=0

// Data less than one page
p := pagination.New(1, 10, 5)
fmt.Println(p.TotalPages)  // 1
fmt.Println(p.HasNext)     // false
start, end := p.GetRange()
// start=0, end=5

// Last page with partial data
p := pagination.New(10, 10, 95)
fmt.Println(p.TotalPages)  // 10
start, end := p.GetRange()
// start=90, end=95
```

## Performance

```
New():           < 1μs
GetRange():      < 1μs
GetPageNumbers(): < 10μs (depends on page count)
```

Pagination calculation is very lightweight with almost no performance impact.

## Notes

1. **Page Numbers Start at 1**:
   - `Page` field starts at 1
   - `Offset` field starts at 0

2. **Maximum Page Size**:
   - `PageSize` is automatically capped at 1000
   - Prevents overly large queries from impacting performance

3. **Total Page Count Calculation**:
   - Uses ceiling division: `(total + pageSize - 1) / pageSize`
   - Total pages is 0 for empty data

4. **JSON Serialization**:
   - Struct fields have JSON tags
   - Can be returned directly to frontend

5. **Thread Safety**:
   - Pagination objects are read-only
   - Not modified after creation

## Dependencies

```bash
# Zero external dependencies, pure Go standard library
```

## Extension Suggestions

For more advanced pagination, consider:
- Cursor Pagination - suitable for real-time data
- Search Engine Pagination (Elasticsearch) - suitable for large datasets
- GraphQL Pagination (Relay Cursor) - suitable for GraphQL APIs
