package idgen

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

type failingEntropyReader struct {
	err error
}

func (r failingEntropyReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestTryNanoIDCustomRejectsInvalidAlphabet(t *testing.T) {
	tooLarge := make([]rune, maxNanoIDAlphabetSize+1)
	for i := range tooLarge {
		tooLarge[i] = rune(0x1000 + i)
	}

	tests := []struct {
		name     string
		alphabet string
	}{
		{name: "single character", alphabet: "x"},
		{name: "duplicate character", alphabet: "aabc"},
		{name: "too many characters", alphabet: string(tooLarge)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := TryNanoIDCustom(test.alphabet, 16); !errors.Is(err, ErrInvalidAlphabet) {
				t.Fatalf("TryNanoIDCustom() error = %v, want ErrInvalidAlphabet", err)
			}
		})
	}
}

func TestTryNanoIDCustomRejectsInvalidSize(t *testing.T) {
	for _, size := range []int{-1, 0, maxNanoIDSize + 1} {
		if _, err := TryNanoIDCustom(DefaultAlphabet, size); !errors.Is(err, ErrInvalidSize) {
			t.Fatalf("TryNanoIDCustom(size=%d) error = %v, want ErrInvalidSize", size, err)
		}
	}
}

func TestGenerateNanoIDPropagatesEntropyFailure(t *testing.T) {
	sourceErr := errors.New("entropy unavailable")
	_, err := generateNanoID(failingEntropyReader{err: sourceErr}, DefaultAlphabet, DefaultSize)
	if !errors.Is(err, ErrInsufficientEntropy) || !errors.Is(err, sourceErr) {
		t.Fatalf("generateNanoID() error = %v, want both entropy errors", err)
	}
}

func TestGenerateUUIDPropagatesEntropyFailure(t *testing.T) {
	sourceErr := errors.New("entropy unavailable")
	_, err := generateUUID(failingEntropyReader{err: sourceErr}, false)
	if !errors.Is(err, ErrInsufficientEntropy) || !errors.Is(err, sourceErr) {
		t.Fatalf("generateUUID() error = %v, want both entropy errors", err)
	}
}

func TestTryUUIDVariants(t *testing.T) {
	standard, err := TryUUID()
	if err != nil {
		t.Fatalf("TryUUID() error = %v", err)
	}
	if len(standard) != 36 || strings.Count(standard, "-") != 4 {
		t.Fatalf("TryUUID() = %q, want canonical UUID", standard)
	}

	compact, err := TryUUIDWithoutHyphen()
	if err != nil {
		t.Fatalf("TryUUIDWithoutHyphen() error = %v", err)
	}
	if len(compact) != 32 || strings.Contains(compact, "-") {
		t.Fatalf("TryUUIDWithoutHyphen() = %q, want compact UUID", compact)
	}
}

func TestTryNanoIDConcurrentUniqueness(t *testing.T) {
	const (
		workers   = 32
		perWorker = 128
	)

	ids := make(chan string, workers*perWorker)
	errs := make(chan error, workers)
	var waitGroup sync.WaitGroup
	waitGroup.Add(workers)
	for range workers {
		go func() {
			defer waitGroup.Done()
			for range perWorker {
				id, err := TryNanoID()
				if err != nil {
					errs <- err
					return
				}
				ids <- id
			}
		}()
	}
	waitGroup.Wait()
	close(ids)
	close(errs)

	for err := range errs {
		t.Fatalf("TryNanoID() error = %v", err)
	}

	seen := make(map[string]struct{}, workers*perWorker)
	for id := range ids {
		if len([]rune(id)) != DefaultSize {
			t.Fatalf("TryNanoID() length = %d, want %d", len([]rune(id)), DefaultSize)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate NanoID: %s", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != workers*perWorker {
		t.Fatalf("unique ID count = %d, want %d", len(seen), workers*perWorker)
	}
}

func TestTryNanoIDCustomSupportsWholeUnicodeCharacters(t *testing.T) {
	id, err := TryNanoIDCustom("甲乙丙丁", 64)
	if err != nil {
		t.Fatalf("TryNanoIDCustom() error = %v", err)
	}
	if len([]rune(id)) != 64 {
		t.Fatalf("TryNanoIDCustom() rune length = %d, want 64", len([]rune(id)))
	}
	for _, character := range id {
		if !strings.ContainsRune("甲乙丙丁", character) {
			t.Fatalf("TryNanoIDCustom() contains unexpected character %s", fmt.Sprintf("%q", character))
		}
	}
}

func TestSnowflakeRejectsInvalidClockSkewBudget(t *testing.T) {
	if generator, err := NewSnowflakeWithOptions(1, -time.Nanosecond); generator != nil || !errors.Is(err, ErrInvalidClockSkewWait) {
		t.Fatalf("NewSnowflakeWithOptions() = (%v, %v), want nil ErrInvalidClockSkewWait", generator, err)
	}
}

func TestSnowflakeWaitsThroughBoundedClockSkew(t *testing.T) {
	current := time.UnixMilli(Epoch + 100)
	generator, err := NewSnowflakeWithOptions(1, 5*time.Millisecond)
	if err != nil {
		t.Fatalf("NewSnowflakeWithOptions() error = %v", err)
	}
	generator.clock = func() time.Time { return current }
	generator.sleep = func(duration time.Duration) { current = current.Add(duration) }

	first, err := generator.GenerateSafe()
	if err != nil {
		t.Fatalf("first GenerateSafe() error = %v", err)
	}
	current = current.Add(-time.Millisecond)
	second, err := generator.GenerateSafe()
	if err != nil {
		t.Fatalf("GenerateSafe() after bounded skew error = %v", err)
	}
	if second <= first {
		t.Fatalf("IDs are not monotonic after bounded skew: first=%d second=%d", first, second)
	}
}

func TestSnowflakeRejectsExcessiveClockSkew(t *testing.T) {
	current := time.UnixMilli(Epoch + 100)
	generator, err := NewSnowflakeWithOptions(1, 5*time.Millisecond)
	if err != nil {
		t.Fatalf("NewSnowflakeWithOptions() error = %v", err)
	}
	generator.clock = func() time.Time { return current }
	generator.sleep = func(time.Duration) {}
	if _, err := generator.GenerateSafe(); err != nil {
		t.Fatalf("first GenerateSafe() error = %v", err)
	}
	current = current.Add(-10 * time.Millisecond)
	if _, err := generator.GenerateSafe(); !errors.Is(err, ErrClockSkew) {
		t.Fatalf("GenerateSafe() error = %v, want ErrClockSkew", err)
	}
}

func TestSnowflakeSequenceExhaustionIsBounded(t *testing.T) {
	current := time.UnixMilli(Epoch + 100)
	generator, err := NewSnowflakeWithOptions(1, 3*time.Millisecond)
	if err != nil {
		t.Fatalf("NewSnowflakeWithOptions() error = %v", err)
	}
	generator.clock = func() time.Time { return current }
	var slept time.Duration
	generator.sleep = func(duration time.Duration) { slept += duration }
	generator.lastTimestamp = current.UnixMilli()
	generator.sequence = maxSequence

	if _, err := generator.GenerateSafe(); !errors.Is(err, ErrSequenceExhausted) {
		t.Fatalf("GenerateSafe() error = %v, want ErrSequenceExhausted", err)
	}
	if slept > generator.maxClockSkewWait {
		t.Fatalf("GenerateSafe() slept %s, budget %s", slept, generator.maxClockSkewWait)
	}
}

func TestSnowflakeRejectsTimestampOutsideBitRange(t *testing.T) {
	tests := []time.Time{
		time.UnixMilli(Epoch - 1),
		time.UnixMilli(Epoch + maxTimestampDelta + 1),
	}
	for _, current := range tests {
		generator, err := NewSnowflake(1)
		if err != nil {
			t.Fatalf("NewSnowflake() error = %v", err)
		}
		generator.clock = func() time.Time { return current }
		if _, err := generator.GenerateSafe(); !errors.Is(err, ErrTimestampOutOfRange) {
			t.Fatalf("GenerateSafe(%s) error = %v, want ErrTimestampOutOfRange", current, err)
		}
	}
}

func TestSnowflakeGeneratePanicsInsteadOfReturningDuplicateSentinel(t *testing.T) {
	generator, err := NewSnowflake(1)
	if err != nil {
		t.Fatalf("NewSnowflake() error = %v", err)
	}
	generator.clock = func() time.Time { return time.UnixMilli(Epoch - 1) }

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("Generate() did not panic for an invalid timestamp")
		}
	}()
	_ = generator.Generate()
}

func TestSnowflakeFailedGenerationDoesNotMutateState(t *testing.T) {
	current := time.UnixMilli(Epoch + 100)
	generator, err := NewSnowflakeWithOptions(1, time.Millisecond)
	if err != nil {
		t.Fatalf("NewSnowflakeWithOptions() error = %v", err)
	}
	generator.clock = func() time.Time { return current }
	generator.sleep = func(time.Duration) {
		current = time.UnixMilli(Epoch + maxTimestampDelta + 1)
	}
	generator.lastTimestamp = current.UnixMilli()
	generator.sequence = maxSequence

	if _, err := generator.GenerateSafe(); !errors.Is(err, ErrTimestampOutOfRange) {
		t.Fatalf("GenerateSafe() error = %v, want ErrTimestampOutOfRange", err)
	}
	if generator.lastTimestamp != Epoch+100 || generator.sequence != maxSequence {
		t.Fatalf("failed generation mutated state: last=%d sequence=%d", generator.lastTimestamp, generator.sequence)
	}
}

func TestSnowflakeZeroValueReturnsInitializationError(t *testing.T) {
	var generator Snowflake
	if _, err := generator.GenerateSafe(); !errors.Is(err, ErrUninitializedGenerator) {
		t.Fatalf("zero-value GenerateSafe() error = %v, want ErrUninitializedGenerator", err)
	}

	var nilGenerator *Snowflake
	if _, err := nilGenerator.GenerateSafe(); !errors.Is(err, ErrUninitializedGenerator) {
		t.Fatalf("nil GenerateSafe() error = %v, want ErrUninitializedGenerator", err)
	}
}
