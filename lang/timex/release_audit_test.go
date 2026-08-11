package timex

import (
	"math"
	"testing"
	"time"
)

func TestFormatDurationShortHandlesMinimumDuration(t *testing.T) {
	if got, want := FormatDurationShort(time.Duration(math.MinInt64)), "-106751d23h"; got != want {
		t.Fatalf("FormatDurationShort(MinInt64) = %q, want %q", got, want)
	}
}

func TestParseDurationRejectsOverflow(t *testing.T) {
	for _, input := range []string{"106752d", "-106752d", "999999999999999999999d"} {
		if got, err := ParseDuration(input); err == nil {
			t.Fatalf("ParseDuration(%q) = %v, want out-of-range error", input, got)
		}
	}
}

func TestDaysBetweenUsesCivilDatesAcrossDST(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("time zone unavailable: %v", err)
	}
	before := time.Date(2026, time.March, 8, 0, 0, 0, 0, location)
	after := time.Date(2026, time.March, 9, 0, 0, 0, 0, location)
	if got := DaysBetween(before, after); got != 1 {
		t.Fatalf("DaysBetween across spring DST = %d, want 1 civil day", got)
	}
}

func TestNowProviderConcurrentAccess(t *testing.T) {
	original := Now
	defer func() { Now = original }()
	first := func() time.Time { return time.Unix(1, 0) }
	second := func() time.Time { return time.Unix(2, 0) }
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 10_000 {
			_ = IsToday(time.Unix(1, 0))
		}
	}()
	for index := range 10_000 {
		if index%2 == 0 {
			Now = first
		} else {
			Now = second
		}
	}
	<-done
}
