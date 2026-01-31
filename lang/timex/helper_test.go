package timex

import (
	"testing"
	"time"
)

func TestIsToday(t *testing.T) {
	now := time.Now()

	if !IsToday(now) {
		t.Error("IsToday should return true for now")
	}

	yesterday := now.AddDate(0, 0, -1)
	if IsToday(yesterday) {
		t.Error("IsToday should return false for yesterday")
	}
}

func TestIsYesterday(t *testing.T) {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)

	if !IsYesterday(yesterday) {
		t.Error("IsYesterday should return true")
	}

	if IsYesterday(now) {
		t.Error("IsYesterday should return false for today")
	}
}

func TestIsTomorrow(t *testing.T) {
	now := time.Now()
	tomorrow := now.AddDate(0, 0, 1)

	if !IsTomorrow(tomorrow) {
		t.Error("IsTomorrow should return true")
	}

	if IsTomorrow(now) {
		t.Error("IsTomorrow should return false for today")
	}
}

func TestIsThisWeek(t *testing.T) {
	now := time.Now()

	if !IsThisWeek(now) {
		t.Error("IsThisWeek should return true for now")
	}

	// 14 days ago is definitely not this week
	twoWeeksAgo := now.AddDate(0, 0, -14)
	if IsThisWeek(twoWeeksAgo) {
		t.Error("IsThisWeek should return false for 2 weeks ago")
	}
}

func TestIsThisMonth(t *testing.T) {
	now := time.Now()

	if !IsThisMonth(now) {
		t.Error("IsThisMonth should return true for now")
	}

	lastYear := now.AddDate(-1, 0, 0)
	if IsThisMonth(lastYear) {
		t.Error("IsThisMonth should return false for last year")
	}
}

func TestIsThisYear(t *testing.T) {
	now := time.Now()

	if !IsThisYear(now) {
		t.Error("IsThisYear should return true for now")
	}

	lastYear := now.AddDate(-1, 0, 0)
	if IsThisYear(lastYear) {
		t.Error("IsThisYear should return false for last year")
	}
}

func TestIsWeekend(t *testing.T) {
	// Find a Saturday
	now := time.Now()
	for now.Weekday() != time.Saturday {
		now = now.AddDate(0, 0, 1)
	}

	if !IsWeekend(now) {
		t.Error("Saturday should be weekend")
	}

	// Sunday
	sunday := now.AddDate(0, 0, 1)
	if !IsWeekend(sunday) {
		t.Error("Sunday should be weekend")
	}

	// Monday
	monday := now.AddDate(0, 0, 2)
	if IsWeekend(monday) {
		t.Error("Monday should not be weekend")
	}
}

func TestIsLeapYear(t *testing.T) {
	tests := []struct {
		year   int
		isLeap bool
	}{
		{2000, true},  // divisible by 400
		{2020, true},  // divisible by 4, not by 100
		{1900, false}, // divisible by 100, not by 400
		{2019, false}, // not divisible by 4
	}

	for _, tt := range tests {
		if IsLeapYear(tt.year) != tt.isLeap {
			t.Errorf("IsLeapYear(%d) = %v, want %v", tt.year, IsLeapYear(tt.year), tt.isLeap)
		}
	}
}

func TestStartOfDay(t *testing.T) {
	now := time.Now()
	start := StartOfDay(now)

	if start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 {
		t.Error("StartOfDay should be 00:00:00")
	}

	if start.Year() != now.Year() || start.Month() != now.Month() || start.Day() != now.Day() {
		t.Error("StartOfDay should preserve date")
	}
}

func TestEndOfDay(t *testing.T) {
	now := time.Now()
	end := EndOfDay(now)

	if end.Hour() != 23 || end.Minute() != 59 || end.Second() != 59 {
		t.Error("EndOfDay should be 23:59:59")
	}
}

func TestStartOfWeek(t *testing.T) {
	// 2024-01-15 is a Monday
	monday := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	start := StartOfWeek(monday)

	if start.Weekday() != time.Monday {
		t.Errorf("StartOfWeek should be Monday, got %v", start.Weekday())
	}

	// Test from Wednesday
	wednesday := time.Date(2024, 1, 17, 12, 0, 0, 0, time.UTC)
	start = StartOfWeek(wednesday)

	if start.Weekday() != time.Monday || start.Day() != 15 {
		t.Error("StartOfWeek from Wednesday should be previous Monday")
	}

	// Test from Sunday
	sunday := time.Date(2024, 1, 21, 12, 0, 0, 0, time.UTC)
	start = StartOfWeek(sunday)

	if start.Weekday() != time.Monday || start.Day() != 15 {
		t.Error("StartOfWeek from Sunday should be previous Monday")
	}
}

func TestEndOfWeek(t *testing.T) {
	monday := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	end := EndOfWeek(monday)

	if end.Weekday() != time.Sunday {
		t.Errorf("EndOfWeek should be Sunday, got %v", end.Weekday())
	}

	if end.Hour() != 23 || end.Minute() != 59 {
		t.Error("EndOfWeek should be end of day")
	}
}

func TestStartOfMonth(t *testing.T) {
	date := time.Date(2024, 3, 15, 12, 30, 45, 0, time.UTC)
	start := StartOfMonth(date)

	if start.Day() != 1 {
		t.Error("StartOfMonth should be first day")
	}

	if start.Hour() != 0 || start.Minute() != 0 {
		t.Error("StartOfMonth should be 00:00:00")
	}
}

func TestEndOfMonth(t *testing.T) {
	// February 2024 (leap year)
	date := time.Date(2024, 2, 15, 12, 0, 0, 0, time.UTC)
	end := EndOfMonth(date)

	if end.Day() != 29 {
		t.Errorf("EndOfMonth for Feb 2024 should be 29, got %d", end.Day())
	}

	// February 2023 (non-leap year)
	date = time.Date(2023, 2, 15, 12, 0, 0, 0, time.UTC)
	end = EndOfMonth(date)

	if end.Day() != 28 {
		t.Errorf("EndOfMonth for Feb 2023 should be 28, got %d", end.Day())
	}
}

func TestStartOfYear(t *testing.T) {
	date := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	start := StartOfYear(date)

	if start.Month() != 1 || start.Day() != 1 {
		t.Error("StartOfYear should be Jan 1")
	}
}

func TestEndOfYear(t *testing.T) {
	date := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	end := EndOfYear(date)

	if end.Month() != 12 || end.Day() != 31 {
		t.Error("EndOfYear should be Dec 31")
	}
}

func TestBetween(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)

	mid := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	if !Between(mid, start, end) {
		t.Error("Mid should be between start and end")
	}

	// Test boundaries
	if !Between(start, start, end) {
		t.Error("Start should be included")
	}

	if !Between(end, start, end) {
		t.Error("End should be included")
	}

	before := start.AddDate(0, 0, -1)
	if Between(before, start, end) {
		t.Error("Before should not be between")
	}

	after := end.AddDate(0, 0, 1)
	if Between(after, start, end) {
		t.Error("After should not be between")
	}
}

func TestDaysBetween(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 10, 23, 59, 59, 0, time.UTC)

	days := DaysBetween(t1, t2)
	if days != 9 {
		t.Errorf("expected 9 days, got %d", days)
	}

	// Order shouldn't matter
	days = DaysBetween(t2, t1)
	if days != 9 {
		t.Errorf("expected 9 days (reversed), got %d", days)
	}
}

func TestHoursBetween(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	hours := HoursBetween(t1, t2)
	if hours != 12 {
		t.Errorf("expected 12 hours, got %d", hours)
	}
}

func TestMinutesBetween(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 0, 30, 0, 0, time.UTC)

	minutes := MinutesBetween(t1, t2)
	if minutes != 30 {
		t.Errorf("expected 30 minutes, got %d", minutes)
	}
}

func TestAddDays(t *testing.T) {
	date := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	result := AddDays(date, 10)

	if result.Day() != 11 {
		t.Errorf("expected day 11, got %d", result.Day())
	}

	// Negative
	result = AddDays(date, -1)
	if result.Day() != 31 || result.Month() != 12 {
		t.Error("AddDays negative should work")
	}
}

func TestAddWeeks(t *testing.T) {
	date := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	result := AddWeeks(date, 2)

	if result.Day() != 15 {
		t.Errorf("expected day 15, got %d", result.Day())
	}
}

func TestAddMonths(t *testing.T) {
	date := time.Date(2024, 1, 31, 12, 0, 0, 0, time.UTC)
	result := AddMonths(date, 1)

	// January 31 + 1 month = March 2 (Feb only has 29 days in 2024)
	if result.Month() != 3 || result.Day() != 2 {
		t.Errorf("expected March 2, got %v", result)
	}
}

func TestAddYears(t *testing.T) {
	date := time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC) // Leap day
	result := AddYears(date, 1)

	// 2025 is not a leap year
	if result.Month() != 3 || result.Day() != 1 {
		t.Errorf("expected March 1 2025, got %v", result)
	}
}

func TestDaysInMonth(t *testing.T) {
	tests := []struct {
		year  int
		month time.Month
		days  int
	}{
		{2024, time.January, 31},
		{2024, time.February, 29}, // leap year
		{2023, time.February, 28}, // non-leap year
		{2024, time.April, 30},
	}

	for _, tt := range tests {
		days := DaysInMonth(tt.year, tt.month)
		if days != tt.days {
			t.Errorf("DaysInMonth(%d, %v) = %d, want %d", tt.year, tt.month, days, tt.days)
		}
	}
}

func TestDaysInYear(t *testing.T) {
	if DaysInYear(2024) != 366 {
		t.Error("2024 should have 366 days")
	}

	if DaysInYear(2023) != 365 {
		t.Error("2023 should have 365 days")
	}
}

func TestAge(t *testing.T) {
	// Mock current time
	originalNow := Now
	Now = func() time.Time {
		return time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	}
	defer func() { Now = originalNow }()

	// Birthday already passed this year
	birthday := time.Date(2000, 3, 15, 0, 0, 0, 0, time.UTC)
	age := Age(birthday)
	if age != 24 {
		t.Errorf("expected age 24, got %d", age)
	}

	// Birthday not yet this year
	birthday = time.Date(2000, 12, 15, 0, 0, 0, 0, time.UTC)
	age = Age(birthday)
	if age != 23 {
		t.Errorf("expected age 23, got %d", age)
	}

	// Future birthday
	birthday = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	age = Age(birthday)
	if age != 0 {
		t.Errorf("expected age 0 for future, got %d", age)
	}
}

func TestUnix(t *testing.T) {
	ts := int64(1704067200) // 2024-01-01 00:00:00 UTC
	tm := Unix(ts)

	if tm.Year() != 2024 || tm.Month() != 1 || tm.Day() != 1 {
		t.Error("Unix conversion failed")
	}
}

func TestUnixMilli(t *testing.T) {
	ts := int64(1704067200000) // 2024-01-01 00:00:00 UTC
	tm := UnixMilli(ts)

	if tm.Year() != 2024 {
		t.Error("UnixMilli conversion failed")
	}
}

func TestToUnix(t *testing.T) {
	tm := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ts := ToUnix(tm)

	if ts != 1704067200 {
		t.Errorf("expected 1704067200, got %d", ts)
	}
}

func TestToUnixMilli(t *testing.T) {
	tm := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ts := ToUnixMilli(tm)

	if ts != 1704067200000 {
		t.Errorf("expected 1704067200000, got %d", ts)
	}
}
