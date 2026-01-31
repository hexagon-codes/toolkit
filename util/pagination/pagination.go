package pagination

// Pagination 分页信息
type Pagination struct {
	Page       int   `json:"page"`        // 当前页码（从1开始）
	PageSize   int   `json:"page_size"`   // 每页大小
	Total      int64 `json:"total"`       // 总记录数
	TotalPages int   `json:"total_pages"` // 总页数
	Offset     int   `json:"offset"`      // 偏移量（用于 SQL OFFSET）
	Limit      int   `json:"limit"`       // 限制数量（用于 SQL LIMIT）
	HasPrev    bool  `json:"has_prev"`    // 是否有上一页
	HasNext    bool  `json:"has_next"`    // 是否有下一页
}

// New 创建分页信息
func New(page, pageSize int, total int64) *Pagination {
	// 参数校验
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 1000 {
		pageSize = 1000 // 限制最大每页数量
	}

	// 计算总页数（使用 int64 避免溢出）
	totalPages64 := total / int64(pageSize)
	if total%int64(pageSize) > 0 {
		totalPages64++
	}
	// 安全转换为 int（限制最大值防止溢出）
	totalPages := int(totalPages64)
	if totalPages64 > int64(^uint(0)>>1) { // MaxInt
		totalPages = int(^uint(0) >> 1)
	}

	// 确保当前页不超过总页数
	if totalPages > 0 && page > totalPages {
		page = totalPages
	}

	// 计算偏移量
	offset := (page - 1) * pageSize

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
	return New(1, 10, total)
}

// NewWithOffset 根据 offset 和 limit 创建分页
func NewWithOffset(offset, limit int, total int64) *Pagination {
	if limit < 1 {
		limit = 10
	}
	page := offset/limit + 1
	return New(page, limit, total)
}

// GetRange 获取当前页的数据范围 [start, end)
func (p *Pagination) GetRange() (start, end int) {
	start = p.Offset
	end = start + p.PageSize
	if end > int(p.Total) {
		end = int(p.Total)
	}
	return start, end
}

// IsFirstPage 是否第一页
func (p *Pagination) IsFirstPage() bool {
	return p.Page == 1
}

// IsLastPage 是否最后一页
func (p *Pagination) IsLastPage() bool {
	return p.Page == p.TotalPages || p.TotalPages == 0
}

// PrevPage 获取上一页页码
func (p *Pagination) PrevPage() int {
	if p.HasPrev {
		return p.Page - 1
	}
	return p.Page
}

// NextPage 获取下一页页码
func (p *Pagination) NextPage() int {
	if p.HasNext {
		return p.Page + 1
	}
	return p.Page
}

// GetPageNumbers 获取页码列表（用于分页导航）
func (p *Pagination) GetPageNumbers(maxDisplay int) []int {
	if maxDisplay < 1 {
		maxDisplay = 10
	}

	if p.TotalPages <= maxDisplay {
		// 总页数不超过最大显示数，显示所有页码
		pages := make([]int, p.TotalPages)
		for i := 0; i < p.TotalPages; i++ {
			pages[i] = i + 1
		}
		return pages
	}

	// 计算显示范围
	half := maxDisplay / 2
	start := p.Page - half
	end := p.Page + half

	// 调整范围
	if start < 1 {
		start = 1
		end = maxDisplay
	}
	if end > p.TotalPages {
		end = p.TotalPages
		start = end - maxDisplay + 1
	}

	// 生成页码列表
	pages := make([]int, end-start+1)
	for i := 0; i < len(pages); i++ {
		pages[i] = start + i
	}
	return pages
}
