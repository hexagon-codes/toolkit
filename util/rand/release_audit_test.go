package rand

import (
	"errors"
	"sync"
	"testing"
)

type failingReader struct {
	err error
}

func (r failingReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestTryStringFromRejectsBiasedCharset(t *testing.T) {
	for _, charset := range []string{"x", "aabc"} {
		if value, err := TryStringFrom(charset, 16); value != "" || !errors.Is(err, ErrInvalidCharset) {
			t.Fatalf("TryStringFrom(%q) = (%q, %v), want empty ErrInvalidCharset", charset, value, err)
		}
	}
}

func TestTryGeneratorsRejectInvalidLength(t *testing.T) {
	stringValue, stringErr := TryString(-1)
	if stringValue != "" || !errors.Is(stringErr, ErrInvalidLength) {
		t.Fatalf("TryString(-1) = (%q, %v), want empty ErrInvalidLength", stringValue, stringErr)
	}
	bytesValue, bytesErr := TryBytes(-1)
	if bytesValue != nil || !errors.Is(bytesErr, ErrInvalidLength) {
		t.Fatalf("TryBytes(-1) = (%v, %v), want nil ErrInvalidLength", bytesValue, bytesErr)
	}
	if _, err := TryString(MaxGeneratedLength + 1); !errors.Is(err, ErrInvalidLength) {
		t.Fatalf("TryString(oversized) error = %v, want ErrInvalidLength", err)
	}
}

func TestTryIntegersRejectInvalidRange(t *testing.T) {
	if value, err := TryInt(7, 7); value != 7 || !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("TryInt(7, 7) = (%d, %v), want 7 ErrInvalidRange", value, err)
	}
	if value, err := TryInt64(8, 7); value != 8 || !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("TryInt64(8, 7) = (%d, %v), want 8 ErrInvalidRange", value, err)
	}
}

func TestRandomHelpersPreserveEntropyErrorChain(t *testing.T) {
	sourceErr := errors.New("entropy unavailable")
	reader := failingReader{err: sourceErr}

	if _, err := stringFrom(reader, AlphaNumeric, 8); !errors.Is(err, ErrInsufficientEntropy) || !errors.Is(err, sourceErr) {
		t.Fatalf("stringFrom() error = %v, want both entropy errors", err)
	}
	if _, err := randomInt64(reader, 0, 10); !errors.Is(err, ErrInsufficientEntropy) || !errors.Is(err, sourceErr) {
		t.Fatalf("randomInt64() error = %v, want both entropy errors", err)
	}
	if _, err := randomBytes(reader, 8); !errors.Is(err, ErrInsufficientEntropy) || !errors.Is(err, sourceErr) {
		t.Fatalf("randomBytes() error = %v, want both entropy errors", err)
	}
}

func TestTryTokenConcurrentUniqueness(t *testing.T) {
	const (
		workers   = 32
		perWorker = 128
	)

	values := make(chan string, workers*perWorker)
	errorsFound := make(chan error, workers)
	var waitGroup sync.WaitGroup
	waitGroup.Add(workers)
	for range workers {
		go func() {
			defer waitGroup.Done()
			for range perWorker {
				value, err := TryToken(32)
				if err != nil {
					errorsFound <- err
					return
				}
				values <- value
			}
		}()
	}
	waitGroup.Wait()
	close(values)
	close(errorsFound)

	for err := range errorsFound {
		t.Fatalf("TryToken() error = %v", err)
	}
	seen := make(map[string]struct{}, workers*perWorker)
	for value := range values {
		if _, exists := seen[value]; exists {
			t.Fatalf("duplicate token: %s", value)
		}
		seen[value] = struct{}{}
	}
	if len(seen) != workers*perWorker {
		t.Fatalf("unique token count = %d, want %d", len(seen), workers*perWorker)
	}
}
