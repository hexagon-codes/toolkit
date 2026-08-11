package slicex

import (
	"math"
	"reflect"
	"testing"
	"time"
)

func TestRangeHandlesNarrowIntegerBoundaries(t *testing.T) {
	ascending := Range[int8](math.MinInt8, math.MaxInt8, 1)
	if len(ascending) != 255 || ascending[0] != math.MinInt8 || ascending[len(ascending)-1] != math.MaxInt8-1 {
		t.Fatalf("ascending int8 range = %v, want all values in [MinInt8, MaxInt8)", ascending)
	}

	unsigned := Range[uint8](0, math.MaxUint8, 2)
	if len(unsigned) != 128 || unsigned[len(unsigned)-1] != 254 {
		t.Fatalf("uint8 range length/last = (%d, %d), want (128, 254)", len(unsigned), unsigned[len(unsigned)-1])
	}

	descending := Range[int8](math.MaxInt8, math.MinInt8, -1)
	if len(descending) != 255 || descending[len(descending)-1] != math.MinInt8+1 {
		t.Fatalf("descending int8 range = %v, want all values in (MinInt8, MaxInt8]", descending)
	}
}

func TestAverageAvoidsAccumulatorOverflow(t *testing.T) {
	values := []int64{math.MaxInt64, math.MaxInt64}
	if got, want := Average(values), float64(math.MaxInt64); got != want {
		t.Fatalf("Average(MaxInt64, MaxInt64) = %v, want %v", got, want)
	}
	if got, want := AverageBy(values, func(value int64) int64 { return value }), float64(math.MaxInt64); got != want {
		t.Fatalf("AverageBy(MaxInt64, MaxInt64) = %v, want %v", got, want)
	}
}

func TestToChannelBufferedCannotRetainBlockedProducer(t *testing.T) {
	ch := ToChannelBuffered([]int{1, 2, 3}, 1)
	if got, want := cap(ch), 3; got != want {
		t.Fatalf("channel capacity = %d, want at least source length %d", got, want)
	}
	if got, want := FromChannel(ch), []int{1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("channel values = %v, want %v", got, want)
	}
}

func TestFromNilChannelReturnsImmediately(t *testing.T) {
	done := make(chan []int, 1)
	go func() {
		done <- FromChannel[int](nil)
	}()

	select {
	case got := <-done:
		if got != nil {
			t.Fatalf("FromChannel(nil) = %v, want nil", got)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("FromChannel(nil) blocked")
	}
}
