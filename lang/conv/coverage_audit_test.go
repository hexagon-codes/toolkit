package conv

import (
	"encoding/binary"
	"math"
	"testing"
)

// These tests raise coverage on previously-uncovered branches of the numeric
// converters (TryInt/TryInt64, Float32/Float64, Uint64/Uint32) and pin the
// documented "failure returns zero" / "ok=false" contract.

func TestTryInt64_AllTypes(t *testing.T) {
	cases := []struct {
		in     any
		want   int64
		wantOK bool
	}{
		{nil, 0, false},
		{int(7), 7, true},
		{int8(-8), -8, true},
		{int16(16), 16, true},
		{int32(-32), -32, true},
		{int64(64), 64, true},
		{uint(5), 5, true},
		{uint8(255), 255, true},
		{uint16(65535), 65535, true},
		{uint32(4294967295), 4294967295, true},
		{uint64(123), 123, true},
		{uint64(math.MaxUint64), 0, false}, // overflow guard
		{float32(3.9), 3, true},
		{float64(-2.9), -2, true},
		{float32(math.Inf(1)), 0, false},
		{math.NaN(), 0, false},
		{true, 1, true},
		{false, 0, true},
		{[]byte("42"), 42, true},
		{[]byte("nope"), 0, false},
		{"100", 100, true},
		{"abc", 0, false},
		{struct{}{}, 0, false}, // default branch -> String -> parse fails
	}
	for _, c := range cases {
		got, ok := TryInt64(c.in)
		if got != c.want || ok != c.wantOK {
			t.Errorf("TryInt64(%#v) = (%d,%v), want (%d,%v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

func TestTryInt_DelegatesToTryInt64(t *testing.T) {
	if v, ok := TryInt("123"); v != 123 || !ok {
		t.Errorf("TryInt(\"123\") = (%d,%v), want (123,true)", v, ok)
	}
	if v, ok := TryInt("x"); v != 0 || ok {
		t.Errorf("TryInt(\"x\") = (%d,%v), want (0,false)", v, ok)
	}
}

func TestUint64_NegativeAndOverflow(t *testing.T) {
	cases := []struct {
		in   any
		want uint64
	}{
		{nil, 0},
		{int(-1), 0}, // negative -> 0
		{int8(-1), 0},
		{int16(-1), 0},
		{int32(-1), 0},
		{int64(-1), 0},
		{int(9), 9},
		{uint(3), 3},
		{uint8(8), 8},
		{uint16(16), 16},
		{uint32(32), 32},
		{uint64(64), 64},
		{float64(-0.5), 0}, // negative float -> 0
		{float32(2.7), 2},
		{math.NaN(), 0},
		{math.Inf(1), 0},
		{true, 1},
		{false, 0},
		{[]byte("77"), 77},
		{"55", 55},
		{"-5", 0}, // ParseUint rejects negative -> 0
		{struct{}{}, 0},
	}
	for _, c := range cases {
		if got := Uint64(c.in); got != c.want {
			t.Errorf("Uint64(%#v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestUint32_RangeGuard(t *testing.T) {
	if got := Uint32(uint64(math.MaxUint32) + 1); got != 0 {
		t.Errorf("Uint32(MaxUint32+1) = %d, want 0 (out-of-range guard)", got)
	}
	if got := Uint32(123); got != 123 {
		t.Errorf("Uint32(123) = %d, want 123", got)
	}
	if got := Uint(5); got != 5 {
		t.Errorf("Uint(5) = %d, want 5", got)
	}
}

func TestInt32_RangeGuard(t *testing.T) {
	if got := Int32(int64(math.MaxInt32) + 1); got != 0 {
		t.Errorf("Int32(MaxInt32+1) = %d, want 0", got)
	}
	if got := Int32(int64(math.MinInt32) - 1); got != 0 {
		t.Errorf("Int32(MinInt32-1) = %d, want 0", got)
	}
	if got := Int32(-100); got != -100 {
		t.Errorf("Int32(-100) = %d, want -100", got)
	}
}

func TestFloat_AllBranches(t *testing.T) {
	// integer & uint inputs to Float64/Float32
	if Float64(int8(-3)) != -3 || Float64(uint16(7)) != 7 {
		t.Errorf("Float64 int/uint branch wrong")
	}
	if Float32(int64(9)) != 9 || Float32(uint8(4)) != 4 {
		t.Errorf("Float32 int/uint branch wrong")
	}
	// string parse via default branch
	if got := Float64("3.5"); got != 3.5 {
		t.Errorf("Float64(\"3.5\") = %v, want 3.5", got)
	}
	if got := Float32("2.5"); got != 2.5 {
		t.Errorf("Float32(\"2.5\") = %v, want 2.5", got)
	}
	// invalid string -> 0
	if got := Float64("xyz"); got != 0 {
		t.Errorf("Float64(\"xyz\") = %v, want 0", got)
	}
	// []byte little-endian decode
	buf8 := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf8, math.Float64bits(6.25))
	if got := Float64(buf8); got != 6.25 {
		t.Errorf("Float64([]byte) = %v, want 6.25", got)
	}
	buf4 := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf4, math.Float32bits(1.5))
	if got := Float32(buf4); got != 1.5 {
		t.Errorf("Float32([]byte) = %v, want 1.5", got)
	}
	// short []byte -> 0
	if Float64([]byte{1, 2}) != 0 || Float32([]byte{1}) != 0 {
		t.Errorf("short []byte should decode to 0")
	}
	// nil -> 0
	if Float64(nil) != 0 || Float32(nil) != 0 {
		t.Errorf("nil should convert to 0")
	}
}
