package mathx

import (
	"math"
	"testing"
)

func TestRoundToDoesNotOverflowFiniteValue(t *testing.T) {
	if got := RoundTo(math.MaxFloat64, 15); got != math.MaxFloat64 {
		t.Fatalf("RoundTo(MaxFloat64, 15) = %v, want finite original value", got)
	}
}
