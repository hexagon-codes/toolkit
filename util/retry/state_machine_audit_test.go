package retry

import (
	"context"
	"errors"
	"math"
	"net"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

type contextOperationError struct {
	cause error
}

const retryAuditWaitTimeout = 5 * time.Second

func (e *contextOperationError) Error() string { return "operation canceled" }
func (e *contextOperationError) Unwrap() error { return e.cause }

func TestDoWithContextCancellationAfterFailurePreservesCauseAndStopsCallbacks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	operationErr := errors.New("operation failed")
	var predicateCalls atomic.Int32
	var retryCalls atomic.Int32
	var delayCalls atomic.Int32

	err := DoWithContext(ctx, func() error {
		cancel()
		return operationErr
	},
		Attempts(3),
		If(func(error) bool {
			predicateCalls.Add(1)
			return true
		}),
		OnRetry(func(int, error) {
			retryCalls.Add(1)
		}),
		DelayType(func(int, *Config) time.Duration {
			delayCalls.Add(1)
			return time.Hour
		}),
	)

	if !errors.Is(err, context.Canceled) || !errors.Is(err, operationErr) {
		t.Fatalf("DoWithContext() error = %v, want context cancellation and operation failure", err)
	}
	if got := predicateCalls.Load(); got != 0 {
		t.Fatalf("retry predicate calls = %d, want 0 after cancellation", got)
	}
	if got := retryCalls.Load(); got != 0 {
		t.Fatalf("OnRetry calls = %d, want 0 after cancellation", got)
	}
	if got := delayCalls.Load(); got != 0 {
		t.Fatalf("delay callback calls = %d, want 0 after cancellation", got)
	}
}

func TestDoWithContextCancellationFromPredicateStopsLaterCallbacks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	operationErr := errors.New("operation failed")
	var retryCalls atomic.Int32
	var delayCalls atomic.Int32

	err := DoWithContext(ctx, func() error { return operationErr },
		Attempts(3),
		If(func(error) bool {
			cancel()
			return true
		}),
		OnRetry(func(int, error) {
			retryCalls.Add(1)
		}),
		DelayType(func(int, *Config) time.Duration {
			delayCalls.Add(1)
			return time.Hour
		}),
	)

	if !errors.Is(err, context.Canceled) || !errors.Is(err, operationErr) {
		t.Fatalf("DoWithContext() error = %v, want context cancellation and operation failure", err)
	}
	if got := retryCalls.Load(); got != 0 {
		t.Fatalf("OnRetry calls = %d, want 0 after predicate cancellation", got)
	}
	if got := delayCalls.Load(); got != 0 {
		t.Fatalf("delay callback calls = %d, want 0 after predicate cancellation", got)
	}
}

func TestDoWithContextCancellationFromOnRetryStopsDelayCalculation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	operationErr := errors.New("operation failed")
	var delayCalls atomic.Int32

	err := DoWithContext(ctx, func() error { return operationErr },
		Attempts(3),
		If(func(error) bool { return true }),
		OnRetry(func(int, error) { cancel() }),
		DelayType(func(int, *Config) time.Duration {
			delayCalls.Add(1)
			return time.Hour
		}),
	)

	if !errors.Is(err, context.Canceled) || !errors.Is(err, operationErr) {
		t.Fatalf("DoWithContext() error = %v, want context cancellation and operation failure", err)
	}
	if got := delayCalls.Load(); got != 0 {
		t.Fatalf("delay callback calls = %d, want 0 after OnRetry cancellation", got)
	}
}

func TestDoWithContextDeadlineAfterFailurePreservesCause(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	operationErr := errors.New("operation failed")
	var predicateCalls atomic.Int32

	err := DoWithContext(ctx, func() error {
		<-ctx.Done()
		return operationErr
	},
		Attempts(3),
		If(func(error) bool {
			predicateCalls.Add(1)
			return true
		}),
	)

	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, operationErr) {
		t.Fatalf("DoWithContext() error = %v, want deadline and operation failure", err)
	}
	if got := predicateCalls.Load(); got != 0 {
		t.Fatalf("retry predicate calls = %d, want 0 after deadline", got)
	}
}

func TestDoWithContextCancellationDuringDelayPreservesCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	operationErr := errors.New("operation failed")
	delayCalculated := make(chan struct{})
	result := make(chan error, 1)

	go func() {
		result <- DoWithContext(ctx, func() error { return operationErr },
			Attempts(3),
			DelayType(func(int, *Config) time.Duration {
				close(delayCalculated)
				return time.Hour
			}),
		)
	}()

	select {
	case <-delayCalculated:
	case <-time.After(retryAuditWaitTimeout):
		t.Fatal("delay callback did not run")
	}
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) || !errors.Is(err, operationErr) {
			t.Fatalf("DoWithContext() error = %v, want context cancellation and operation failure", err)
		}
	case <-time.After(retryAuditWaitTimeout):
		t.Fatal("DoWithContext() did not stop after cancellation")
	}
}

func TestDoWithContextPreservesOperationWrapperContainingCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	operationErr := &contextOperationError{cause: context.Canceled}
	err := DoWithContext(ctx, func() error {
		cancel()
		return operationErr
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("DoWithContext() error = %v, want context.Canceled", err)
	}
	var target *contextOperationError
	if !errors.As(err, &target) {
		t.Fatalf("DoWithContext() error = %v, want operation wrapper", err)
	}
}

func TestMultiplierDoesNotSelectDelayStrategy(t *testing.T) {
	config, err := newConfig(Delay(time.Second), Multiplier(3))
	if err != nil {
		t.Fatal(err)
	}
	if got := calculateDelay(2, config); got != time.Second {
		t.Fatalf("second delay with Multiplier only = %v, want fixed %v", got, time.Second)
	}
}

func TestDefaultConfigReturnsIndependentBackoffSnapshots(t *testing.T) {
	first := DefaultConfig()
	second := DefaultConfig()
	if first == second {
		t.Fatal("DefaultConfig() returned the same config pointer")
	}

	first.Delay = 7 * time.Second
	first.Multiplier = 9
	If(func(error) bool { return false })(first)

	if second.Delay != time.Second || second.Multiplier != 2 {
		t.Fatalf("second config was mutated through first: %+v", second)
	}
	if !second.predicate(errors.New("operation failed")) {
		t.Fatal("second config predicate was mutated through first")
	}
}

func TestExplicitDelayStrategyIsIndependentOfOptionOrder(t *testing.T) {
	tests := []struct {
		name    string
		options []Option
		want    time.Duration
	}{
		{
			name: "exponential before multiplier",
			options: []Option{
				Delay(time.Second),
				DelayType(ExponentialBackoff),
				Multiplier(3),
			},
			want: 3 * time.Second,
		},
		{
			name: "exponential after multiplier",
			options: []Option{
				Delay(time.Second),
				Multiplier(3),
				DelayType(ExponentialBackoff),
			},
			want: 3 * time.Second,
		},
		{
			name: "linear before multiplier",
			options: []Option{
				Delay(time.Second),
				DelayType(LinearBackoff),
				Multiplier(3),
			},
			want: 2 * time.Second,
		},
		{
			name: "linear after multiplier",
			options: []Option{
				Delay(time.Second),
				Multiplier(3),
				DelayType(LinearBackoff),
			},
			want: 2 * time.Second,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := newConfig(test.options...)
			if err != nil {
				t.Fatal(err)
			}
			if got := calculateDelay(2, config); got != test.want {
				t.Fatalf("second delay = %v, want %v", got, test.want)
			}
		})
	}
}

func TestIfIsTheOnlyPredicateConfigurationAPI(t *testing.T) {
	configType := reflect.TypeOf(Config{})
	if field, ok := configType.FieldByName("RetryIf"); ok && field.IsExported() {
		t.Fatalf("Config exposes legacy predicate field %q", field.Name)
	}
}

func TestDecorrelatedJitterSaturatesInsteadOfWrappingToZero(t *testing.T) {
	const maximumDuration = time.Duration(1<<63 - 1)
	config := &Config{
		Delay:      maximumDuration,
		MaxDelay:   maximumDuration,
		DelayFunc:  FixedDelay,
		JitterType: DecorrelatedJitter,
	}

	if got := calculateDelay(1, config); got != maximumDuration {
		t.Fatalf("calculateDelay() = %v, want saturated %v", got, maximumDuration)
	}
}

func TestExponentialBackoffUnderflowReturnsZeroInsteadOfMaximumDelay(t *testing.T) {
	config := &Config{
		Delay:      time.Nanosecond,
		MaxDelay:   time.Hour,
		Multiplier: 0.5,
	}
	if got := ExponentialBackoff(2, config); got != 0 {
		t.Fatalf("ExponentialBackoff() = %v, want 0 after sub-nanosecond underflow", got)
	}
}

func TestExponentialBackoffRejectsNonPositiveAttempt(t *testing.T) {
	config := &Config{
		Delay:      time.Second,
		MaxDelay:   time.Hour,
		Multiplier: 2,
	}
	if got := ExponentialBackoff(0, config); got != 0 {
		t.Fatalf("ExponentialBackoff(0) = %v, want 0", got)
	}
}

func TestNetworkErrorsRetryNonTimeoutDNSAndOperationFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "dns",
			err:  &net.DNSError{Err: "host not found", Name: "example.invalid"},
		},
		{
			name: "operation",
			err: &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: errors.New("connection refused"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !IsRetryableHTTPError(test.err) {
				t.Fatalf("IsRetryableHTTPError(%T) = false, want true", test.err)
			}
		})
	}
}

func TestRetryCallbacksPropagatePanicWithoutPoisoningLaterCalls(t *testing.T) {
	tests := []struct {
		name    string
		options []Option
	}{
		{
			name: "predicate",
			options: []Option{If(func(error) bool {
				panic("predicate panic")
			})},
		},
		{
			name: "on retry",
			options: []Option{OnRetry(func(int, error) {
				panic("retry panic")
			})},
		},
		{
			name: "delay",
			options: []Option{DelayType(func(int, *Config) time.Duration {
				panic("delay panic")
			})},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			panicked := func() (panicked bool) {
				defer func() { panicked = recover() != nil }()
				_ = Do(func() error { return errors.New("operation failed") },
					append([]Option{Attempts(2), Delay(0)}, test.options...)...,
				)
				return false
			}()
			if !panicked {
				t.Fatal("Do() did not propagate callback panic")
			}
			if err := Do(func() error { return nil }, Delay(0)); err != nil {
				t.Fatalf("later Do() error = %v", err)
			}
		})
	}
}

func TestOnRetryCanReenterRetry(t *testing.T) {
	var attempts atomic.Int32
	var innerCalls atomic.Int32
	err := Do(func() error {
		if attempts.Add(1) == 1 {
			return errors.New("retry outer operation")
		}
		return nil
	},
		Attempts(2),
		Delay(0),
		OnRetry(func(int, error) {
			if err := Do(func() error {
				innerCalls.Add(1)
				return nil
			}, Delay(0)); err != nil {
				panic(err)
			}
		}),
	)
	if err != nil {
		t.Fatalf("outer Do() error = %v", err)
	}
	if got := innerCalls.Load(); got != 1 {
		t.Fatalf("inner retry calls = %d, want 1", got)
	}
}

func FuzzDelayStrategiesStayBoundedAndMonotonic(f *testing.F) {
	f.Add(int64(1), int64(time.Second), int64(time.Minute), math.Float64bits(2))
	f.Add(int64(2), int64(time.Nanosecond), int64(time.Hour), math.Float64bits(0.5))
	f.Add(int64(10), int64(time.Hour), int64(time.Second), math.Float64bits(1.1))

	f.Fuzz(func(t *testing.T, rawAttempt, rawDelay, rawMaximum int64, multiplierBits uint64) {
		if rawAttempt <= 0 || rawAttempt >= 1_000_000 || rawDelay <= 0 || rawMaximum <= 0 {
			return
		}
		multiplier := math.Float64frombits(multiplierBits)
		if multiplier <= 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
			return
		}
		attempt := int(rawAttempt)
		config := &Config{
			Delay:      time.Duration(rawDelay),
			MaxDelay:   time.Duration(rawMaximum),
			Multiplier: multiplier,
		}

		exponential := ExponentialBackoff(attempt, config)
		nextExponential := ExponentialBackoff(attempt+1, config)
		linear := LinearBackoff(attempt, config)
		nextLinear := LinearBackoff(attempt+1, config)
		for name, delay := range map[string]time.Duration{
			"exponential":      exponential,
			"next exponential": nextExponential,
			"linear":           linear,
			"next linear":      nextLinear,
		} {
			if delay < 0 || delay > config.MaxDelay {
				t.Fatalf("%s delay = %v, want [0, %v]", name, delay, config.MaxDelay)
			}
		}
		if multiplier >= 1 && nextExponential < exponential {
			t.Fatalf("exponential delays moved backward: %v then %v", exponential, nextExponential)
		}
		if multiplier < 1 && nextExponential > exponential {
			t.Fatalf("decreasing exponential delays moved forward: %v then %v", exponential, nextExponential)
		}
		if nextLinear < linear {
			t.Fatalf("linear delays moved backward: %v then %v", linear, nextLinear)
		}
	})
}
