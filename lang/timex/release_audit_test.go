package timex

import (
	"math"
	"sync"
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
	first := func() time.Time { return time.Unix(1, 0) }
	second := func() time.Time { return time.Unix(2, 0) }
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 10_000 {
			_ = IsToday(time.Unix(1, 0))
		}
	}()
	restores := make([]func(), 0, 10_000)
	for index := range 10_000 {
		if index%2 == 0 {
			restores = append(restores, SetNowProvider(first))
		} else {
			restores = append(restores, SetNowProvider(second))
		}
	}
	<-done
	for index := len(restores) - 1; index >= 0; index-- {
		restores[index]()
	}
}

func TestNowProviderOutOfOrderRestoreDoesNotOverwriteNewerProvider(t *testing.T) {
	rootTime := time.Unix(10, 0)
	firstTime := time.Unix(20, 0)
	secondTime := time.Unix(30, 0)
	restoreRoot := SetNowProvider(func() time.Time { return rootTime })
	defer restoreRoot()
	restoreFirst := SetNowProvider(func() time.Time { return firstTime })
	restoreSecond := SetNowProvider(func() time.Time { return secondTime })

	restoreFirst()
	if got := Now(); !got.Equal(secondTime) {
		t.Fatalf("Now() after stale restore = %v, want newest provider %v", got, secondTime)
	}
	restoreSecond()
	if got := Now(); !got.Equal(rootTime) {
		t.Fatalf("Now() after all nested restores = %v, want root provider %v", got, rootTime)
	}
}

func TestNowProviderConcurrentRestoreIsIdempotent(t *testing.T) {
	rootTime := time.Unix(100, 0)
	restoreRoot := SetNowProvider(func() time.Time { return rootTime })
	defer restoreRoot()

	const providerCount = 128
	restores := make([]func(), 0, providerCount)
	for index := range providerCount {
		fixed := time.Unix(int64(index+101), 0)
		restores = append(restores, SetNowProvider(func() time.Time { return fixed }))
	}

	var wait sync.WaitGroup
	for _, restore := range restores {
		wait.Add(2)
		go func() {
			defer wait.Done()
			restore()
		}()
		go func() {
			defer wait.Done()
			restore()
		}()
	}
	wait.Wait()
	if got := Now(); !got.Equal(rootTime) {
		t.Fatalf("Now() after concurrent restores = %v, want root provider %v", got, rootTime)
	}
}
