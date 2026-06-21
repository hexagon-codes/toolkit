package conv

import "testing"

// Bug5: the overflow guard `value > math.MaxInt64` is ineffective for float64
// because float64(math.MaxInt64) rounds up to 2^63. So 9223372036854775808.0
// (== 2^63) passes the check, then int64(value) overflows to a negative number.
// Int64/TryInt64 must reject any float >= 2^63 (and the symmetric Uint64 bound).
func TestBug5_Int64_FloatOverflowBoundary(t *testing.T) {
	const twoPow63 = 9223372036854775808.0 // 2^63, one past math.MaxInt64

	if got := Int64(float64(twoPow63)); got != 0 {
		t.Errorf("Int64(2^63) = %d, want 0 (overflow must be rejected)", got)
	}
	if v, ok := TryInt64(float64(twoPow63)); ok || v != 0 {
		t.Errorf("TryInt64(2^63) = (%d,%v), want (0,false)", v, ok)
	}

	// float32 path: 2^63 in float32 also overflows int64.
	if got := Int64(float32(twoPow63)); got != 0 {
		t.Errorf("Int64(float32 2^63) = %d, want 0", got)
	}

	// A representable in-range large value must still convert.
	const safe = 9007199254740992.0 // 2^53, exactly representable, well in range
	if got := Int64(float64(safe)); got != 9007199254740992 {
		t.Errorf("Int64(2^53) = %d, want 9007199254740992", got)
	}
}

// Uint64 must reject floats >= 2^64 (float64(math.MaxUint64) rounds up to 2^64).
func TestBug5_Uint64_FloatOverflowBoundary(t *testing.T) {
	const twoPow64 = 18446744073709551616.0 // 2^64, one past math.MaxUint64
	if got := Uint64(float64(twoPow64)); got != 0 {
		t.Errorf("Uint64(2^64) = %d, want 0 (overflow must be rejected)", got)
	}
}
