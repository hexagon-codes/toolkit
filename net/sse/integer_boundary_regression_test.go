package sse

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestReaderTotalByteCounterCannotOverflow(t *testing.T) {
	t.Parallel()

	reader := MustNewReader(strings.NewReader("data: value\n\n"))
	reader.totalBytes = math.MaxInt64

	if _, err := reader.Read(); !errors.Is(err, ErrMaxBytesExceeded) {
		t.Fatalf("Read() error = %v, want ErrMaxBytesExceeded", err)
	}
	if reader.totalBytes != math.MaxInt64 {
		t.Fatalf("totalBytes = %d, want %d", reader.totalBytes, int64(math.MaxInt64))
	}
}
