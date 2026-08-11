package pagination

import (
	"math"
	"testing"
)

func TestPaginationUsesInt64ForCountDerivedFields(t *testing.T) {
	pagination := New(1, 1, math.MaxInt64)
	var _ int64 = pagination.Page
	var _ int64 = pagination.TotalPages
	var _ int64 = pagination.Offset
	if pagination.TotalPages != math.MaxInt64 {
		t.Fatalf("TotalPages = %d, want %d", pagination.TotalPages, int64(math.MaxInt64))
	}
}

func TestNewNormalizesNegativeAndEmptyTotals(t *testing.T) {
	for _, total := range []int64{-1, 0} {
		pagination := New(math.MaxInt64, 1000, total)
		if pagination.Page != 1 || pagination.Total != 0 || pagination.TotalPages != 0 || pagination.Offset != 0 {
			t.Fatalf("New(max page, total=%d) = %+v, want empty first page", total, pagination)
		}
	}
}

func TestNewWithOffsetPreservesExactOffset(t *testing.T) {
	pagination := NewWithOffset(15, 10, 100)
	if pagination.Page != 2 || pagination.Offset != 15 {
		t.Fatalf("NewWithOffset(15, 10, 100) = page %d offset %d, want page 2 offset 15", pagination.Page, pagination.Offset)
	}
}

func TestNewWithOffsetNormalizesNegativeOffset(t *testing.T) {
	pagination := NewWithOffset(-1, 10, 100)
	if pagination.Page != 1 || pagination.Offset != 0 {
		t.Fatalf("NewWithOffset(-1, 10, 100) = page %d offset %d, want page 1 offset 0", pagination.Page, pagination.Offset)
	}
}

func TestGetPageNumbersHonorsExactDisplayLimit(t *testing.T) {
	pagination := New(50, 10, 1000)
	pages := pagination.GetPageNumbers(10)
	if len(pages) != 10 {
		t.Fatalf("GetPageNumbers(10) length = %d, want 10", len(pages))
	}
	if pages[0] != 46 || pages[len(pages)-1] != 55 {
		t.Fatalf("GetPageNumbers(10) = %v, want [46..55]", pages)
	}
}

func TestGetPageNumbersBoundsAllocation(t *testing.T) {
	pagination := New(1000, 1, 2000)
	pages := pagination.GetPageNumbers(math.MaxInt)
	if len(pages) != MaxPageNumbers {
		t.Fatalf("GetPageNumbers(MaxInt) length = %d, want %d", len(pages), MaxPageNumbers)
	}
}

func TestPaginationMethodsDefendAgainstInvalidStructState(t *testing.T) {
	pagination := &Pagination{Page: -1, PageSize: -1, Total: -1, TotalPages: -1, Offset: -1}
	if start, end := pagination.GetRange(); start != 0 || end != 0 {
		t.Fatalf("GetRange() = (%d, %d), want (0, 0)", start, end)
	}
	if pages := pagination.GetPageNumbers(10); len(pages) != 0 {
		t.Fatalf("GetPageNumbers() = %v, want empty", pages)
	}
}

func TestPaginationNavigationDoesNotOverflowInvalidState(t *testing.T) {
	if got := (&Pagination{Page: math.MinInt64, HasPrev: true}).PrevPage(); got != 1 {
		t.Fatalf("PrevPage() = %d, want normalized page 1", got)
	}
	if got := (&Pagination{Page: math.MaxInt64, HasNext: true}).NextPage(); got != math.MaxInt64 {
		t.Fatalf("NextPage() = %d, want saturated MaxInt64", got)
	}
	var pagination *Pagination
	if got := pagination.PrevPage(); got != 1 {
		t.Fatalf("nil PrevPage() = %d, want 1", got)
	}
	if got := pagination.NextPage(); got != 1 {
		t.Fatalf("nil NextPage() = %d, want 1", got)
	}
}
