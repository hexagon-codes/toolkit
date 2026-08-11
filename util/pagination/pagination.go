// Package pagination 提供偏移量和页码分页工具。
package pagination

import "math"

const (
	// DefaultPageSize 是无效页面大小的默认值。
	DefaultPageSize = 10
	// MaxPageSize 限制单次查询数量。
	MaxPageSize = 1000
	// MaxPageNumbers 限制导航页码切片的单次分配。
	MaxPageNumbers = 1000
)

// Pagination 分页信息
type Pagination struct {
	Page       int64 `json:"page"`        // 当前页码（从1开始）
	PageSize   int   `json:"page_size"`   // 每页大小
	Total      int64 `json:"total"`       // 总记录数
	TotalPages int64 `json:"total_pages"` // 总页数
	Offset     int64 `json:"offset"`      // 偏移量（用于 SQL OFFSET）
	Limit      int   `json:"limit"`       // 限制数量（用于 SQL LIMIT）
	HasPrev    bool  `json:"has_prev"`    // 是否有上一页
	HasNext    bool  `json:"has_next"`    // 是否有下一页
}

// New 创建分页信息
func New(page int64, pageSize int, total int64) *Pagination {
	// 参数校验
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}
	if total < 0 {
		total = 0
	}

	// 使用商和余数计算向上取整，避免 total+pageSize-1 溢出。
	totalPages := total / int64(pageSize)
	if total%int64(pageSize) > 0 {
		totalPages++
	}

	// 确保当前页不超过总页数
	if totalPages > 0 && page > totalPages {
		page = totalPages
	} else if totalPages == 0 {
		page = 1
	}

	// 计算偏移量
	offset := (page - 1) * int64(pageSize)

	return &Pagination{
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
		Offset:     offset,
		Limit:      pageSize,
		HasPrev:    page > 1,
		HasNext:    page < totalPages,
	}
}

// NewDefault 创建默认分页（第1页，每页10条）
func NewDefault(total int64) *Pagination {
	return New(1, DefaultPageSize, total)
}

// NewWithOffset 根据 offset 和 limit 创建分页
func NewWithOffset(offset int64, limit int, total int64) *Pagination {
	if offset < 0 {
		offset = 0
	}
	if limit < 1 {
		limit = DefaultPageSize
	}
	if limit > MaxPageSize {
		limit = MaxPageSize
	}
	if total < 0 {
		total = 0
	}

	totalPages := total / int64(limit)
	if total%int64(limit) > 0 {
		totalPages++
	}
	pageIndex := offset / int64(limit)
	page := int64(math.MaxInt64)
	if pageIndex < math.MaxInt64 {
		page = pageIndex + 1
	}
	return &Pagination{
		Page:       page,
		PageSize:   limit,
		Total:      total,
		TotalPages: totalPages,
		Offset:     offset,
		Limit:      limit,
		HasPrev:    offset > 0,
		HasNext:    offset < total && int64(limit) < total-offset,
	}
}

// GetRange 获取当前页的数据范围 [start, end)
func (p *Pagination) GetRange() (start, end int64) {
	if p == nil || p.Total <= 0 || p.PageSize <= 0 || p.Offset < 0 {
		return 0, 0
	}
	if p.Offset >= p.Total {
		return p.Total, p.Total
	}
	start = p.Offset
	remaining := p.Total - start
	if int64(p.PageSize) >= remaining {
		return start, p.Total
	}
	return start, start + int64(p.PageSize)
}

// IsFirstPage 是否第一页
func (p *Pagination) IsFirstPage() bool {
	return p == nil || p.Page <= 1
}

// IsLastPage 是否最后一页
func (p *Pagination) IsLastPage() bool {
	return p == nil || !p.HasNext
}

// PrevPage 获取上一页页码
func (p *Pagination) PrevPage() int64 {
	if p == nil || p.Page < 1 {
		return 1
	}
	if p.HasPrev && p.Page > 1 {
		return p.Page - 1
	}
	return p.Page
}

// NextPage 获取下一页页码
func (p *Pagination) NextPage() int64 {
	if p == nil || p.Page < 1 {
		return 1
	}
	if p.HasNext && p.Page < math.MaxInt64 {
		return p.Page + 1
	}
	return p.Page
}

// GetPageNumbers 获取页码列表（用于分页导航）
func (p *Pagination) GetPageNumbers(maxDisplay int) []int64 {
	if p == nil || p.TotalPages <= 0 {
		return []int64{}
	}
	if maxDisplay < 1 {
		maxDisplay = DefaultPageSize
	}
	if maxDisplay > MaxPageNumbers {
		maxDisplay = MaxPageNumbers
	}

	count := int64(maxDisplay)
	if p.TotalPages < count {
		count = p.TotalPages
	}
	if p.TotalPages <= count {
		// 总页数不超过最大显示数，显示所有页码
		pages := make([]int64, int(count))
		for i := range pages {
			pages[i] = int64(i) + 1
		}
		return pages
	}

	current := p.Page
	if current < 1 {
		current = 1
	} else if current > p.TotalPages {
		current = p.TotalPages
	}
	start := current - (count-1)/2
	if start < 1 {
		start = 1
	}
	latestStart := p.TotalPages - count + 1
	if start > latestStart {
		start = latestStart
	}

	// 生成页码列表
	pages := make([]int64, int(count))
	for i := range pages {
		pages[i] = start + int64(i)
	}
	return pages
}
