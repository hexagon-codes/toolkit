package circuit

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const circuitAuditWaitTimeout = 5 * time.Second

func TestFailureClockCallbackRunsOutsideBreakerLock(t *testing.T) {
	clockEntered := make(chan struct{})
	releaseClock := make(chan struct{})
	var enteredOnce sync.Once
	breaker := newTestBreaker(t,
		WithThreshold(1),
		WithNow(func() time.Time {
			enteredOnce.Do(func() { close(clockEntered) })
			<-releaseClock
			return time.Unix(100, 0)
		}),
	)
	permit, err := breaker.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	completeResult := make(chan error, 1)
	go func() { completeResult <- permit.Complete(errors.New("operation failed")) }()

	select {
	case <-clockEntered:
	case <-time.After(circuitAuditWaitTimeout):
		t.Fatal("clock callback did not run")
	}
	stateResult := make(chan State, 1)
	go func() { stateResult <- breaker.State() }()

	select {
	case state := <-stateResult:
		if state != StateClosed {
			t.Fatalf("State() = %s, want closed while completion is pending", state)
		}
	case <-time.After(circuitAuditWaitTimeout):
		close(releaseClock)
		<-completeResult
		t.Fatal("State() blocked behind clock callback")
	}
	close(releaseClock)
	if err := <-completeResult; err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
}

func TestRecoveryClockCallbackRunsOutsideBreakerLock(t *testing.T) {
	var currentUnixNano atomic.Int64
	currentUnixNano.Store(time.Unix(100, 0).UnixNano())
	var blockClock atomic.Bool
	clockEntered := make(chan struct{})
	releaseClock := make(chan struct{})
	var enteredOnce sync.Once
	breaker := newTestBreaker(t,
		WithThreshold(1),
		WithTimeout(time.Second),
		WithNow(func() time.Time {
			if blockClock.Load() {
				enteredOnce.Do(func() { close(clockEntered) })
				<-releaseClock
			}
			return time.Unix(0, currentUnixNano.Load())
		}),
	)
	permit, err := breaker.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if err := permit.Complete(errors.New("operation failed")); err != nil {
		t.Fatal(err)
	}
	currentUnixNano.Add(int64(2 * time.Second))
	blockClock.Store(true)

	type acquireResult struct {
		permit *Permit
		err    error
	}
	acquired := make(chan acquireResult, 1)
	go func() {
		permit, err := breaker.Acquire()
		acquired <- acquireResult{permit: permit, err: err}
	}()
	select {
	case <-clockEntered:
	case <-time.After(circuitAuditWaitTimeout):
		t.Fatal("recovery clock callback did not run")
	}
	stateResult := make(chan State, 1)
	go func() { stateResult <- breaker.State() }()

	select {
	case state := <-stateResult:
		if state != StateOpen {
			t.Fatalf("State() = %s, want open while recovery check is pending", state)
		}
	case <-time.After(circuitAuditWaitTimeout):
		close(releaseClock)
		<-acquired
		t.Fatal("State() blocked behind recovery clock callback")
	}
	close(releaseClock)
	result := <-acquired
	if result.err != nil {
		t.Fatalf("Acquire() error = %v", result.err)
	}
	if result.permit == nil {
		t.Fatal("Acquire() permit = nil")
	}
	if err := result.permit.Complete(nil); err != nil {
		t.Fatal(err)
	}
}

func TestClockPanicDoesNotPoisonBreakerLock(t *testing.T) {
	var panicClock atomic.Bool
	panicClock.Store(true)
	breaker := newTestBreaker(t,
		WithThreshold(1),
		WithNow(func() time.Time {
			if panicClock.CompareAndSwap(true, false) {
				panic("clock panic")
			}
			return time.Unix(100, 0)
		}),
	)
	permit, err := breaker.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	panicked := func() (panicked bool) {
		defer func() { panicked = recover() != nil }()
		_ = permit.Complete(errors.New("operation failed"))
		return false
	}()
	if !panicked {
		t.Fatal("Complete() did not propagate clock panic")
	}

	stateResult := make(chan State, 1)
	go func() { stateResult <- breaker.State() }()
	select {
	case state := <-stateResult:
		if state != StateClosed {
			t.Fatalf("State() = %s, want closed after clock panic", state)
		}
	case <-time.After(circuitAuditWaitTimeout):
		t.Fatal("breaker lock remained held after clock panic")
	}
}

func TestOpeningFailureSamplesClockOnce(t *testing.T) {
	base := time.Unix(100, 0)
	var calls atomic.Int32
	breaker := newTestBreaker(t,
		WithThreshold(1),
		WithNow(func() time.Time {
			return base.Add(time.Duration(calls.Add(1)) * time.Second)
		}),
	)
	permit, err := breaker.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if err := permit.Complete(errors.New("operation failed")); err != nil {
		t.Fatal(err)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("clock calls = %d, want 1 for one failure", got)
	}
	stats := breaker.Stats()
	if !stats.LastFailureAt.Equal(stats.OpenedAt) {
		t.Fatalf("LastFailureAt = %v, OpenedAt = %v, want identical timestamps", stats.LastFailureAt, stats.OpenedAt)
	}
}

func TestConcurrentFailureClockSamplesDoNotMoveTimestampsBackward(t *testing.T) {
	earlier := time.Unix(100, 0)
	later := earlier.Add(time.Second)
	firstClockEntered := make(chan struct{})
	releaseFirstClock := make(chan struct{})
	var clockCalls atomic.Int32
	breaker := newTestBreaker(t,
		WithThreshold(2),
		WithNow(func() time.Time {
			if clockCalls.Add(1) == 1 {
				close(firstClockEntered)
				<-releaseFirstClock
				return earlier
			}
			return later
		}),
	)
	first, err := breaker.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	second, err := breaker.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	firstResult := make(chan error, 1)
	go func() { firstResult <- first.Complete(errors.New("first failure")) }()
	select {
	case <-firstClockEntered:
	case <-time.After(circuitAuditWaitTimeout):
		t.Fatal("first clock sample did not start")
	}
	if err := second.Complete(errors.New("second failure")); err != nil {
		t.Fatal(err)
	}
	close(releaseFirstClock)
	if err := <-firstResult; err != nil {
		t.Fatal(err)
	}

	stats := breaker.Stats()
	if !stats.LastFailureAt.Equal(later) || !stats.OpenedAt.Equal(later) {
		t.Fatalf("Stats() timestamps = (%v, %v), want %v", stats.LastFailureAt, stats.OpenedAt, later)
	}
}

func TestRecoveryClockCallbackCanResetBreaker(t *testing.T) {
	var currentUnixNano atomic.Int64
	currentUnixNano.Store(time.Unix(100, 0).UnixNano())
	var resetOnClock atomic.Bool
	var breaker *Breaker
	breaker = newTestBreaker(t,
		WithThreshold(1),
		WithTimeout(time.Second),
		WithNow(func() time.Time {
			if resetOnClock.CompareAndSwap(true, false) {
				if err := breaker.Reset(); err != nil {
					panic(err)
				}
			}
			return time.Unix(0, currentUnixNano.Load())
		}),
	)
	permit, err := breaker.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if err := permit.Complete(errors.New("operation failed")); err != nil {
		t.Fatal(err)
	}
	currentUnixNano.Add(int64(2 * time.Second))
	resetOnClock.Store(true)

	result := make(chan struct {
		permit *Permit
		err    error
	}, 1)
	go func() {
		permit, err := breaker.Acquire()
		result <- struct {
			permit *Permit
			err    error
		}{permit: permit, err: err}
	}()
	select {
	case acquired := <-result:
		if acquired.err != nil {
			t.Fatalf("Acquire() error = %v", acquired.err)
		}
		if acquired.permit == nil || acquired.permit.state != StateClosed {
			t.Fatalf("Acquire() permit = %#v, want closed-state permit", acquired.permit)
		}
		if err := acquired.permit.Complete(nil); err != nil {
			t.Fatal(err)
		}
	case <-time.After(circuitAuditWaitTimeout):
		t.Fatal("reentrant Reset() deadlocked recovery clock callback")
	}
}

func TestResetClearsFailureTimestamps(t *testing.T) {
	now := time.Unix(100, 0)
	breaker := newTestBreaker(t,
		WithThreshold(1),
		WithNow(func() time.Time { return now }),
	)
	permit, err := breaker.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if err := permit.Complete(errors.New("operation failed")); err != nil {
		t.Fatal(err)
	}
	if err := breaker.Reset(); err != nil {
		t.Fatal(err)
	}

	stats := breaker.Stats()
	if stats.State != StateClosed || stats.Failures != 0 || stats.Successes != 0 ||
		stats.HalfOpenInFlight != 0 || !stats.LastFailureAt.IsZero() || !stats.OpenedAt.IsZero() {
		t.Fatalf("Stats() after Reset = %+v, want a zeroed closed state", stats)
	}
}

func TestExecuteContextRejectsAlreadyCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	breaker := newTestBreaker(t, WithThreshold(1))
	var called atomic.Bool

	result, err := breaker.ExecuteContext(ctx, func(context.Context) (any, error) {
		called.Store(true)
		return "unexpected", nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ExecuteContext() error = %v, want context.Canceled", err)
	}
	if result != nil {
		t.Fatalf("ExecuteContext() result = %v, want nil", result)
	}
	if called.Load() {
		t.Fatal("ExecuteContext() invoked run with an already canceled context")
	}
	if stats := breaker.Stats(); stats.Failures != 0 || stats.HalfOpenInFlight != 0 {
		t.Fatalf("Stats() = %+v, want no admitted work", stats)
	}
}

func TestExecuteContextCancellationWhileAcquiringCompletesPermit(t *testing.T) {
	var currentUnixNano atomic.Int64
	currentUnixNano.Store(time.Unix(100, 0).UnixNano())
	transitionEntered := make(chan struct{})
	releaseTransition := make(chan struct{})
	breaker := newTestBreaker(t,
		WithThreshold(1),
		WithTimeout(time.Second),
		WithNow(func() time.Time { return time.Unix(0, currentUnixNano.Load()) }),
		WithOnStateChange(func(from, to State) {
			if from == StateOpen && to == StateHalfOpen {
				close(transitionEntered)
				<-releaseTransition
			}
		}),
	)
	if _, err := breaker.Execute(func() (any, error) {
		return nil, errors.New("operation failed")
	}); err == nil {
		t.Fatal("Execute() error = nil, want operation failure")
	}
	currentUnixNano.Add(int64(2 * time.Second))

	ctx, cancel := context.WithCancel(context.Background())
	var called atomic.Bool
	result := make(chan error, 1)
	go func() {
		_, err := breaker.ExecuteContext(ctx, func(context.Context) (any, error) {
			called.Store(true)
			return nil, nil
		})
		result <- err
	}()
	select {
	case <-transitionEntered:
	case <-time.After(circuitAuditWaitTimeout):
		t.Fatal("half-open transition callback did not run")
	}
	cancel()
	close(releaseTransition)

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ExecuteContext() error = %v, want context.Canceled", err)
		}
	case <-time.After(circuitAuditWaitTimeout):
		t.Fatal("ExecuteContext() did not return after cancellation")
	}
	if called.Load() {
		t.Fatal("ExecuteContext() invoked run after context cancellation")
	}
	stats := breaker.Stats()
	if stats.State != StateOpen || stats.HalfOpenInFlight != 0 {
		t.Fatalf("Stats() = %+v, want open state without an in-flight probe", stats)
	}
}

func TestPermitConcurrentCompletionAppliesExactlyOneResult(t *testing.T) {
	breaker := newTestBreaker(t, WithThreshold(1))
	permit, err := breaker.Acquire()
	if err != nil {
		t.Fatal(err)
	}

	const workers = 64
	start := make(chan struct{})
	var wait sync.WaitGroup
	var completed atomic.Int32
	var duplicates atomic.Int32
	var unexpected atomic.Int32
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			err := permit.Complete(errors.New("operation failed"))
			switch {
			case err == nil:
				completed.Add(1)
			case errors.Is(err, ErrPermitCompleted):
				duplicates.Add(1)
			default:
				unexpected.Add(1)
			}
		}()
	}
	close(start)
	wait.Wait()

	if got := completed.Load(); got != 1 {
		t.Fatalf("successful completions = %d, want 1", got)
	}
	if got := duplicates.Load(); got != workers-1 {
		t.Fatalf("duplicate completions = %d, want %d", got, workers-1)
	}
	if got := unexpected.Load(); got != 0 {
		t.Fatalf("unexpected completion errors = %d, want 0", got)
	}
	stats := breaker.Stats()
	if stats.State != StateOpen || stats.Failures != 1 {
		t.Fatalf("Stats() = %+v, want one applied failure and open state", stats)
	}
}

func TestCopiedPermitSharesSingleCompletion(t *testing.T) {
	breaker := newTestBreaker(t, WithThreshold(2))
	permit, err := breaker.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	copied := *permit
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		results <- permit.Complete(errors.New("original failure"))
	}()
	go func() {
		<-start
		results <- copied.Complete(errors.New("copied failure"))
	}()
	close(start)

	var completed, duplicate int
	for range 2 {
		switch err := <-results; {
		case err == nil:
			completed++
		case errors.Is(err, ErrPermitCompleted):
			duplicate++
		default:
			t.Fatalf("Complete() error = %v", err)
		}
	}
	if completed != 1 || duplicate != 1 {
		t.Fatalf("completion results = (%d completed, %d duplicate), want (1, 1)", completed, duplicate)
	}
	stats := breaker.Stats()
	if stats.State != StateClosed || stats.Failures != 1 {
		t.Fatalf("Stats() = %+v, want one applied failure", stats)
	}
}

func TestStalePermitDoesNotInvokeFailureClassifier(t *testing.T) {
	var classifierCalls atomic.Int32
	breaker := newTestBreaker(t, WithIsFailure(func(error) bool {
		classifierCalls.Add(1)
		return true
	}))
	permit, err := breaker.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if err := breaker.Reset(); err != nil {
		t.Fatal(err)
	}
	if err := permit.Complete(errors.New("stale failure")); err != nil {
		t.Fatalf("stale Complete() error = %v", err)
	}
	if got := classifierCalls.Load(); got != 0 {
		t.Fatalf("failure classifier calls = %d, want 0 for a stale permit", got)
	}
}

func TestHalfOpenConcurrentFailureInvalidatesRemainingPermits(t *testing.T) {
	var currentUnixNano atomic.Int64
	currentUnixNano.Store(time.Unix(100, 0).UnixNano())
	const probes = 32
	breaker := newTestBreaker(t,
		WithThreshold(1),
		WithTimeout(time.Second),
		WithHalfOpenMaxRequests(probes),
		WithSuccessThreshold(probes),
		WithNow(func() time.Time { return time.Unix(0, currentUnixNano.Load()) }),
	)
	closedPermit, err := breaker.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if err := closedPermit.Complete(errors.New("operation failed")); err != nil {
		t.Fatal(err)
	}
	currentUnixNano.Add(int64(2 * time.Second))

	permits := make([]*Permit, 0, probes)
	for range probes {
		permit, err := breaker.Acquire()
		if err != nil {
			t.Fatal(err)
		}
		permits = append(permits, permit)
	}
	start := make(chan struct{})
	var wait sync.WaitGroup
	var completionErrors atomic.Int32
	for index, permit := range permits {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			var resultErr error
			if index == 0 {
				resultErr = errors.New("probe failed")
			}
			if err := permit.Complete(resultErr); err != nil {
				completionErrors.Add(1)
			}
		}()
	}
	close(start)
	wait.Wait()

	if got := completionErrors.Load(); got != 0 {
		t.Fatalf("completion errors = %d, want 0", got)
	}
	stats := breaker.Stats()
	if stats.State != StateOpen || stats.HalfOpenInFlight != 0 {
		t.Fatalf("Stats() = %+v, want open state without stale probes", stats)
	}
}

func TestClassifierAndStateListenerRunOutsideBreakerLock(t *testing.T) {
	t.Run("classifier", func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		breaker := newTestBreaker(t, WithIsFailure(func(error) bool {
			close(entered)
			<-release
			return true
		}))
		permit, err := breaker.Acquire()
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- permit.Complete(errors.New("operation failed")) }()
		<-entered
		if state := breaker.State(); state != StateClosed {
			t.Fatalf("State() = %s, want closed", state)
		}
		close(release)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})

	t.Run("listener", func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		breaker := newTestBreaker(t,
			WithThreshold(1),
			WithOnStateChange(func(State, State) {
				close(entered)
				<-release
			}),
		)
		permit, err := breaker.Acquire()
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- permit.Complete(errors.New("operation failed")) }()
		<-entered
		if state := breaker.State(); state != StateOpen {
			t.Fatalf("State() = %s, want open", state)
		}
		close(release)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}

func TestCircuitCallbacksHandlePanicAndReentry(t *testing.T) {
	t.Run("classifier panic counts as failure", func(t *testing.T) {
		breaker := newTestBreaker(t,
			WithThreshold(1),
			WithIsFailure(func(error) bool { panic("classifier panic") }),
		)
		if _, err := breaker.Execute(func() (any, error) { return nil, nil }); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if state := breaker.State(); state != StateOpen {
			t.Fatalf("State() = %s, want open", state)
		}
	})

	t.Run("listener panic is isolated", func(t *testing.T) {
		breaker := newTestBreaker(t,
			WithThreshold(1),
			WithOnStateChange(func(State, State) { panic("listener panic") }),
		)
		if _, err := breaker.Execute(func() (any, error) {
			return nil, errors.New("operation failed")
		}); err == nil {
			t.Fatal("Execute() error = nil, want operation failure")
		}
		if state := breaker.State(); state != StateOpen {
			t.Fatalf("State() = %s, want open", state)
		}
	})

	t.Run("listener can reset", func(t *testing.T) {
		var breaker *Breaker
		var resetErr error
		breaker = newTestBreaker(t,
			WithThreshold(1),
			WithOnStateChange(func(from, to State) {
				_ = breaker.State()
				if from == StateClosed && to == StateOpen {
					resetErr = breaker.Reset()
				}
			}),
		)
		if _, err := breaker.Execute(func() (any, error) {
			return nil, errors.New("operation failed")
		}); err == nil {
			t.Fatal("Execute() error = nil, want operation failure")
		}
		if resetErr != nil {
			t.Fatalf("reentrant Reset() error = %v", resetErr)
		}
		if state := breaker.State(); state != StateClosed {
			t.Fatalf("State() = %s, want closed after reentrant reset", state)
		}
	})
}

func TestExecuteErrorChainIncludesRunAndCompletionFailures(t *testing.T) {
	breaker := newTestBreaker(t)
	runEntered := make(chan struct{})
	releaseRun := make(chan struct{})
	operationErr := errors.New("operation failed")
	result := make(chan error, 1)
	go func() {
		_, err := breaker.Execute(func() (any, error) {
			close(runEntered)
			<-releaseRun
			return nil, operationErr
		})
		result <- err
	}()
	<-runEntered
	breaker.Close()
	close(releaseRun)
	err := <-result
	if !errors.Is(err, operationErr) || !errors.Is(err, ErrBreakerClosed) {
		t.Fatalf("Execute() error = %v, want operation and close failures", err)
	}
}

func TestHTTPClassifiersRejectOutOfRangeStatusCodes(t *testing.T) {
	err := &testHTTPError{code: 600}
	if IsRateLimitOrServerError(err) {
		t.Fatal("IsRateLimitOrServerError(600) = true, want false")
	}
	if IsServerError(err) {
		t.Fatal("IsServerError(600) = true, want false")
	}
}

func FuzzBreakerSequentialStateMachine(f *testing.F) {
	f.Add([]byte{0, 1, 0, 2, 3, 0, 1})
	f.Add([]byte{0, 2, 4, 0, 1, 0, 1})
	f.Add([]byte{3, 3, 0, 2, 4, 0})

	f.Fuzz(func(t *testing.T, operations []byte) {
		now := time.Unix(100, 0)
		breaker, err := New(
			WithThreshold(2),
			WithTimeout(time.Second),
			WithHalfOpenMaxRequests(2),
			WithSuccessThreshold(2),
			WithNow(func() time.Time { return now }),
		)
		if err != nil {
			t.Fatal(err)
		}
		permits := make([]*Permit, 0, 16)
		for index, operation := range operations {
			if index >= 256 {
				break
			}
			switch operation % 5 {
			case 0:
				permit, acquireErr := breaker.Acquire()
				if acquireErr == nil {
					permits = append(permits, permit)
				}
			case 1, 2:
				if len(permits) == 0 {
					continue
				}
				permit := permits[0]
				permits = permits[1:]
				var resultErr error
				if operation%5 == 2 {
					resultErr = errors.New("operation failed")
				}
				_ = permit.Complete(resultErr)
			case 3:
				_ = breaker.Reset()
			case 4:
				now = now.Add(2 * time.Second)
			}

			stats := breaker.Stats()
			if stats.State < StateClosed || stats.State > StateHalfOpen {
				t.Fatalf("State = %d, want a valid state", stats.State)
			}
			if stats.Failures < 0 || stats.Successes < 0 || stats.HalfOpenInFlight < 0 {
				t.Fatalf("Stats() contains a negative counter: %+v", stats)
			}
			if stats.State != StateHalfOpen && stats.HalfOpenInFlight != 0 {
				t.Fatalf("Stats() = %+v, want no probes outside half-open state", stats)
			}
			if stats.HalfOpenInFlight > 2 {
				t.Fatalf("half-open probes = %d, want at most 2", stats.HalfOpenInFlight)
			}
		}
	})
}
