# Pagination 分页工具

简单易用的分页计算工具，提供完整的分页信息。

## 特性

- ✅ 自动计算偏移量和总页数
- ✅ 上一页/下一页判断
- ✅ 页码列表生成（用于分页导航）
- ✅ 支持 offset/limit 转换
- ✅ 参数自动校验和边界处理
- ✅ 零外部依赖

## 快速开始

### 基本用法

```go
import "github.com/everyday-items/toolkit/util/pagination"

// 创建分页（第1页，每页10条，总共95条记录）
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

### 用于数据库查询

```go
// 获取分页参数
page := 2
pageSize := 20
total := int64(156)

p := pagination.New(page, pageSize, total)

// 在 SQL 查询中使用
query := "SELECT * FROM users ORDER BY id LIMIT ? OFFSET ?"
rows, err := db.Query(query, p.Limit, p.Offset)

// p.Offset = 20, p.Limit = 20
```

### 默认分页

```go
// 创建默认分页（第1页，每页10条）
p := pagination.NewDefault(95)

fmt.Println(p.Page)      // 1
fmt.Println(p.PageSize)  // 10
```

### 从 offset 创建分页

```go
// 从 offset/limit 创建分页对象
p := pagination.NewWithOffset(20, 10, 95)

fmt.Println(p.Page)  // 3 (offset 20 ÷ limit 10 + 1)
```

## API 文档

### 结构体

```go
type Pagination struct {
    Page       int   // 当前页码（从1开始）
    PageSize   int   // 每页大小
    Total      int64 // 总记录数
    TotalPages int   // 总页数
    Offset     int   // 偏移量（用于 SQL OFFSET）
    Limit      int   // 限制数量（用于 SQL LIMIT）
    HasPrev    bool  // 是否有上一页
    HasNext    bool  // 是否有下一页
}
```

### 构造函数

```go
// New 创建分页信息
New(page, pageSize int, total int64) *Pagination

// NewDefault 创建默认分页（第1页，每页10条）
NewDefault(total int64) *Pagination

// NewWithOffset 根据 offset 和 limit 创建分页
NewWithOffset(offset, limit int, total int64) *Pagination
```

### 方法

```go
// GetRange 获取当前页的数据范围 [start, end)
GetRange() (start, end int)

// IsFirstPage 是否第一页
IsFirstPage() bool

// IsLastPage 是否最后一页
IsLastPage() bool

// PrevPage 获取上一页页码
PrevPage() int

// NextPage 获取下一页页码
NextPage() int

// GetPageNumbers 获取页码列表（用于分页导航）
GetPageNumbers(maxDisplay int) []int
```

## 使用场景

### 1. RESTful API 分页

```go
func ListUsers(c *gin.Context) {
    // 获取分页参数
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

    // 查询总数
    var total int64
    db.Model(&User{}).Count(&total)

    // 创建分页
    p := pagination.New(page, pageSize, total)

    // 查询数据
    var users []User
    db.Limit(p.Limit).Offset(p.Offset).Find(&users)

    // 返回响应
    c.JSON(200, gin.H{
        "data":       users,
        "pagination": p,
    })
}

// 响应示例：
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

### 2. 数据库分页查询

```go
func GetArticles(page, pageSize int) ([]Article, *pagination.Pagination, error) {
    // 查询总数
    var total int64
    if err := db.Model(&Article{}).Count(&total).Error; err != nil {
        return nil, nil, err
    }

    // 创建分页
    p := pagination.New(page, pageSize, total)

    // 查询数据
    var articles []Article
    err := db.Limit(p.Limit).Offset(p.Offset).
        Order("created_at DESC").
        Find(&articles).Error

    return articles, p, err
}
```

### 3. HTML 分页导航

```go
func RenderPagination(w http.ResponseWriter, p *pagination.Pagination) {
    // 获取页码列表（显示最多10个页码）
    pageNumbers := p.GetPageNumbers(10)

    html := `<div class="pagination">`

    // 上一页按钮
    if p.HasPrev {
        html += fmt.Sprintf(`<a href="?page=%d">上一页</a>`, p.PrevPage())
    }

    // 页码列表
    for _, num := range pageNumbers {
        if num == p.Page {
            html += fmt.Sprintf(`<span class="current">%d</span>`, num)
        } else {
            html += fmt.Sprintf(`<a href="?page=%d">%d</a>`, num, num)
        }
    }

    // 下一页按钮
    if p.HasNext {
        html += fmt.Sprintf(`<a href="?page=%d">下一页</a>`, p.NextPage())
    }

    html += `</div>`
    w.Write([]byte(html))
}
```

### 4. 分页信息提示

```go
func ShowPageInfo(p *pagination.Pagination) string {
    start, end := p.GetRange()
    return fmt.Sprintf("显示 %d-%d 条，共 %d 条", start+1, end, p.Total)
}

// 输出示例：
// "显示 11-20 条，共 95 条"
```

### 5. 游标分页（无限滚动）

```go
func GetArticlesWithCursor(offset, limit int) ([]Article, *pagination.Pagination, error) {
    // 查询总数
    var total int64
    db.Model(&Article{}).Count(&total)

    // 从 offset 创建分页
    p := pagination.NewWithOffset(offset, limit, total)

    // 查询数据
    var articles []Article
    err := db.Limit(p.Limit).Offset(p.Offset).Find(&articles).Error

    return articles, p, err
}

// 前端调用：
// 第一次：offset=0, limit=20
// 第二次：offset=20, limit=20
// 第三次：offset=40, limit=20
```

### 6. 内存数据分页

```go
func PaginateSlice[T any](data []T, page, pageSize int) ([]T, *pagination.Pagination) {
    total := int64(len(data))
    p := pagination.New(page, pageSize, total)

    start, end := p.GetRange()

    // 边界检查
    if start >= len(data) {
        return []T{}, p
    }
    if end > len(data) {
        end = len(data)
    }

    return data[start:end], p
}

// 使用示例
users := []User{...} // 100个用户
pagedUsers, p := PaginateSlice(users, 2, 10)
// pagedUsers 包含第11-20个用户
```

### 7. 多条件分页查询

```go
func SearchProducts(keyword string, page, pageSize int) ([]Product, *pagination.Pagination, error) {
    query := db.Model(&Product{})

    // 添加搜索条件
    if keyword != "" {
        query = query.Where("name LIKE ?", "%"+keyword+"%")
    }

    // 查询总数
    var total int64
    if err := query.Count(&total).Error; err != nil {
        return nil, nil, err
    }

    // 创建分页
    p := pagination.New(page, pageSize, total)

    // 查询数据
    var products []Product
    err := query.Limit(p.Limit).Offset(p.Offset).Find(&products).Error

    return products, p, err
}
```

## 参数自动校验

分页工具会自动校验和调整参数：

```go
// page < 1 时自动设为 1
p := pagination.New(0, 10, 100)
fmt.Println(p.Page)  // 1

// pageSize < 1 时自动设为 10
p := pagination.New(1, 0, 100)
fmt.Println(p.PageSize)  // 10

// pageSize > 1000 时自动限制为 1000（防止超大查询）
p := pagination.New(1, 5000, 100)
fmt.Println(p.PageSize)  // 1000

// page 超过总页数时自动调整
p := pagination.New(999, 10, 100)
fmt.Println(p.Page)  // 10（总页数）
```

## 分页导航示例

```go
// 获取页码列表
p := pagination.New(5, 10, 200)  // 第5页，共20页

// 显示最多10个页码
pages := p.GetPageNumbers(10)
// 输出: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]

// 当前页在中间时
p := pagination.New(10, 10, 200)
pages := p.GetPageNumbers(10)
// 输出: [6, 7, 8, 9, 10, 11, 12, 13, 14, 15]

// 接近末尾时
p := pagination.New(18, 10, 200)
pages := p.GetPageNumbers(10)
// 输出: [11, 12, 13, 14, 15, 16, 17, 18, 19, 20]
```

## 边界情况处理

```go
// 空数据
p := pagination.New(1, 10, 0)
fmt.Println(p.TotalPages)  // 0
fmt.Println(p.HasNext)     // false
start, end := p.GetRange()
// start=0, end=0

// 数据量少于一页
p := pagination.New(1, 10, 5)
fmt.Println(p.TotalPages)  // 1
fmt.Println(p.HasNext)     // false
start, end := p.GetRange()
// start=0, end=5

// 最后一页数据不满
p := pagination.New(10, 10, 95)
fmt.Println(p.TotalPages)  // 10
start, end := p.GetRange()
// start=90, end=95
```

## 性能

```
New():           < 1μs
GetRange():      < 1μs
GetPageNumbers(): < 10μs (取决于页数)
```

分页计算非常轻量，对性能几乎无影响。

## 注意事项

1. **页码从1开始**：
   - `Page` 字段从 1 开始计数
   - `Offset` 字段从 0 开始

2. **最大页面大小**：
   - 自动限制 `PageSize` 最大为 1000
   - 防止超大查询影响性能

3. **总页数计算**：
   - 使用向上取整：`(total + pageSize - 1) / pageSize`
   - 空数据时总页数为 0

4. **JSON 序列化**：
   - 结构体字段有 JSON 标签
   - 可直接返回给前端

5. **线程安全**：
   - 分页对象是只读的
   - 创建后不会被修改

## 依赖

```bash
# 零外部依赖，纯 Go 标准库
```

## 扩展建议

如需更高级的分页功能，可考虑：
- 游标分页（Cursor Pagination）- 适合实时数据
- 搜索引擎分页（Elasticsearch）- 适合大数据量
- GraphQL 分页（Relay Cursor）- 适合 GraphQL API
