package pagination

import (
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name       string
		page       int
		pageSize   int
		total      int64
		wantPage   int
		wantOffset int
		wantPages  int
	}{
		{"normal", 1, 10, 100, 1, 0, 10},
		{"page 2", 2, 10, 100, 2, 10, 10},
		{"page 0 should become 1", 0, 10, 100, 1, 0, 10},
		{"negative page should become 1", -1, 10, 100, 1, 0, 10},
		{"small page size", 1, 5, 23, 1, 0, 5},
		{"large page size capped at 1000", 1, 2000, 5000, 1, 0, 5},
		{"page beyond total should cap", 100, 10, 50, 5, 40, 5},
		{"zero total", 1, 10, 0, 1, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.page, tt.pageSize, tt.total)

			if p.Page != tt.wantPage {
				t.Errorf("Page = %d, want %d", p.Page, tt.wantPage)
			}
			if p.Offset != tt.wantOffset {
				t.Errorf("Offset = %d, want %d", p.Offset, tt.wantOffset)
			}
			if p.TotalPages != tt.wantPages {
				t.Errorf("TotalPages = %d, want %d", p.TotalPages, tt.wantPages)
			}
		})
	}
}

func TestNewDefault(t *testing.T) {
	p := NewDefault(100)

	if p.Page != 1 {
		t.Errorf("Page = %d, want 1", p.Page)
	}
	if p.PageSize != 10 {
		t.Errorf("PageSize = %d, want 10", p.PageSize)
	}
	if p.Total != 100 {
		t.Errorf("Total = %d, want 100", p.Total)
	}
}

func TestNewWithOffset(t *testing.T) {
	p := NewWithOffset(20, 10, 100)

	if p.Page != 3 {
		t.Errorf("Page = %d, want 3", p.Page)
	}
	if p.Offset != 20 {
		t.Errorf("Offset = %d, want 20", p.Offset)
	}
}

func TestPagination_HasPrevNext(t *testing.T) {
	// First page
	p1 := New(1, 10, 100)
	if p1.HasPrev {
		t.Error("First page should not have prev")
	}
	if !p1.HasNext {
		t.Error("First page should have next")
	}

	// Middle page
	p2 := New(5, 10, 100)
	if !p2.HasPrev {
		t.Error("Middle page should have prev")
	}
	if !p2.HasNext {
		t.Error("Middle page should have next")
	}

	// Last page
	p3 := New(10, 10, 100)
	if !p3.HasPrev {
		t.Error("Last page should have prev")
	}
	if p3.HasNext {
		t.Error("Last page should not have next")
	}
}

func TestPagination_GetRange(t *testing.T) {
	p := New(2, 10, 25)
	start, end := p.GetRange()

	if start != 10 {
		t.Errorf("start = %d, want 10", start)
	}
	if end != 20 {
		t.Errorf("end = %d, want 20", end)
	}

	// Last page with partial data
	p2 := New(3, 10, 25)
	start2, end2 := p2.GetRange()

	if start2 != 20 {
		t.Errorf("start = %d, want 20", start2)
	}
	if end2 != 25 {
		t.Errorf("end = %d, want 25", end2)
	}
}

func TestPagination_IsFirstLastPage(t *testing.T) {
	p1 := New(1, 10, 100)
	if !p1.IsFirstPage() {
		t.Error("Page 1 should be first page")
	}
	if p1.IsLastPage() {
		t.Error("Page 1 should not be last page")
	}

	p10 := New(10, 10, 100)
	if p10.IsFirstPage() {
		t.Error("Page 10 should not be first page")
	}
	if !p10.IsLastPage() {
		t.Error("Page 10 should be last page")
	}

	// Empty result
	pEmpty := New(1, 10, 0)
	if !pEmpty.IsFirstPage() {
		t.Error("Empty result should be first page")
	}
	if !pEmpty.IsLastPage() {
		t.Error("Empty result should be last page")
	}
}

func TestPagination_PrevNextPage(t *testing.T) {
	p := New(5, 10, 100)

	if p.PrevPage() != 4 {
		t.Errorf("PrevPage = %d, want 4", p.PrevPage())
	}
	if p.NextPage() != 6 {
		t.Errorf("NextPage = %d, want 6", p.NextPage())
	}

	// First page
	p1 := New(1, 10, 100)
	if p1.PrevPage() != 1 {
		t.Errorf("PrevPage for first page = %d, want 1", p1.PrevPage())
	}

	// Last page
	p10 := New(10, 10, 100)
	if p10.NextPage() != 10 {
		t.Errorf("NextPage for last page = %d, want 10", p10.NextPage())
	}
}

func TestPagination_GetPageNumbers(t *testing.T) {
	// Small number of pages
	p1 := New(1, 10, 30)
	pages1 := p1.GetPageNumbers(10)
	if len(pages1) != 3 {
		t.Errorf("GetPageNumbers returned %d pages, want 3", len(pages1))
	}

	// Many pages, middle position
	p2 := New(50, 10, 1000)
	pages2 := p2.GetPageNumbers(10)
	if len(pages2) < 10 || len(pages2) > 11 {
		t.Errorf("GetPageNumbers returned %d pages, want around 10", len(pages2))
	}

	// Check pages contain current page
	found := false
	for _, pg := range pages2 {
		if pg == 50 {
			found = true
			break
		}
	}
	if !found {
		t.Error("GetPageNumbers should contain current page")
	}

	// First page with many total pages
	p3 := New(1, 10, 1000)
	pages3 := p3.GetPageNumbers(10)
	if pages3[0] != 1 {
		t.Errorf("GetPageNumbers[0] = %d, want 1", pages3[0])
	}

	// Last page
	p4 := New(100, 10, 1000)
	pages4 := p4.GetPageNumbers(10)
	if pages4[len(pages4)-1] != 100 {
		t.Errorf("GetPageNumbers last = %d, want 100", pages4[len(pages4)-1])
	}
}

func TestPagination_Limit(t *testing.T) {
	p := New(1, 20, 100)
	if p.Limit != 20 {
		t.Errorf("Limit = %d, want 20", p.Limit)
	}
}
