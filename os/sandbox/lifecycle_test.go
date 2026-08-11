package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSandboxInterfaceRequiresDeterministicClose(t *testing.T) {
	sandboxType := reflect.TypeOf((*Sandbox)(nil)).Elem()
	method, ok := sandboxType.MethodByName("Close")
	if !ok {
		t.Fatal("Sandbox.Close is missing")
	}
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	if method.Type.NumIn() != 0 || method.Type.NumOut() != 1 || method.Type.Out(0) != errorType {
		t.Fatalf("Sandbox.Close type = %s, want func() error", method.Type)
	}
}

type lifecycleTestBackend struct {
	blockedOperation string
	entered          chan struct{}
	release          chan struct{}
	closeEntered     chan struct{}
	blockOnce        sync.Once
	releaseOnce      sync.Once
	closeOnce        sync.Once
	execCalls        atomic.Int32
	capabilityCalls  atomic.Int32
	closeCalls       atomic.Int32
	closeErr         error
	closeShouldPanic bool
	closePanic       any
	closeRelease     <-chan struct{}
}

func (backend *lifecycleTestBackend) releaseBlockedOperation() {
	backend.releaseOnce.Do(func() { close(backend.release) })
}

func newLifecycleTestBackend(blockedOperation string) *lifecycleTestBackend {
	return &lifecycleTestBackend{
		blockedOperation: blockedOperation,
		entered:          make(chan struct{}),
		release:          make(chan struct{}),
		closeEntered:     make(chan struct{}),
	}
}

func (backend *lifecycleTestBackend) block(operation string) {
	if operation != backend.blockedOperation {
		return
	}
	backend.blockOnce.Do(func() {
		close(backend.entered)
		<-backend.release
	})
}

func (backend *lifecycleTestBackend) Exec(context.Context, Command) (*ExecResult, error) {
	backend.execCalls.Add(1)
	backend.block("exec")
	return &ExecResult{}, nil
}

func (backend *lifecycleTestBackend) sandboxCapabilities(context.Context) (CapabilitySet, error) {
	backend.capabilityCalls.Add(1)
	backend.block("capabilities")
	return CapabilityFilesystem | CapabilityProcessContainment | CapabilityOutput, nil
}

func (backend *lifecycleTestBackend) Close() error {
	backend.closeCalls.Add(1)
	backend.closeOnce.Do(func() { close(backend.closeEntered) })
	if backend.closeRelease != nil {
		<-backend.closeRelease
	}
	if backend.closeShouldPanic {
		panic(backend.closePanic)
	}
	return backend.closeErr
}

func newLifecycleTestSandbox(t *testing.T, backend *lifecycleTestBackend) *capabilitySandbox {
	t.Helper()
	workspace, identity, err := snapshotSandboxWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &capabilitySandbox{
		backend: backend,
		cfg: Config{
			Workspace:            workspace,
			RequiredCapabilities: CapabilityFilesystem | CapabilityProcessContainment | CapabilityOutput,
			workspaceIdentity:    identity,
		},
	}
}

func TestSandboxCloseWaitsForActiveOperationsAndRejectsNewOnes(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		invoke    func(*capabilitySandbox) error
	}{
		{
			name:      "exec",
			operation: "exec",
			invoke: func(sandboxInstance *capabilitySandbox) error {
				executable, err := os.Executable()
				if err != nil {
					return err
				}
				_, err = sandboxInstance.Exec(context.Background(), Command{Path: executable})
				return err
			},
		},
		{
			name:      "capabilities",
			operation: "capabilities",
			invoke: func(sandboxInstance *capabilitySandbox) error {
				_, err := sandboxInstance.Capabilities(context.Background())
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newLifecycleTestBackend(test.operation)
			defer backend.releaseBlockedOperation()
			sandboxInstance := newLifecycleTestSandbox(t, backend)

			operationDone := make(chan error, 1)
			go func() { operationDone <- test.invoke(sandboxInstance) }()
			awaitLifecycleSignal(t, backend.entered, "active operation did not start")

			closeDone := make(chan error, 1)
			go func() { closeDone <- sandboxInstance.Close() }()
			awaitSandboxClosing(t, sandboxInstance)

			select {
			case <-backend.closeEntered:
				t.Fatal("backend resources were closed while an operation was active")
			default:
			}

			beforeExec := backend.execCalls.Load()
			newExecDone := make(chan error, 1)
			go func() {
				_, execErr := sandboxInstance.Exec(context.Background(), Command{})
				newExecDone <- execErr
			}()
			execErr := awaitLifecycleResult(t, newExecDone, "new Exec() waited for the active operation during Close")
			if !errors.Is(execErr, ErrSandboxClosed) {
				t.Fatalf("new Exec() during Close error = %v, want ErrSandboxClosed", execErr)
			}
			if got := backend.execCalls.Load(); got != beforeExec {
				t.Fatalf("backend Exec() calls during Close = %d, want %d", got, beforeExec)
			}

			newCapabilitiesDone := make(chan error, 1)
			go func() {
				_, capabilityErr := sandboxInstance.Capabilities(context.Background())
				newCapabilitiesDone <- capabilityErr
			}()
			capabilityErr := awaitLifecycleResult(t, newCapabilitiesDone, "new Capabilities() waited for the active operation during Close")
			if !errors.Is(capabilityErr, ErrSandboxClosed) {
				t.Fatalf("new Capabilities() during Close error = %v, want ErrSandboxClosed", capabilityErr)
			}

			backend.releaseBlockedOperation()
			if err := awaitLifecycleResult(t, operationDone, "active operation did not finish"); err != nil {
				t.Fatalf("active operation error = %v", err)
			}
			if err := awaitLifecycleResult(t, closeDone, "Close() did not finish"); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if got := backend.closeCalls.Load(); got != 1 {
				t.Fatalf("backend Close() calls = %d, want 1", got)
			}
		})
	}
}

func TestSandboxCloseIsConcurrentIdempotentAndPreservesErrorChain(t *testing.T) {
	errRoot := errors.New("close root")
	errGuard := errors.New("close root guard")
	backend := newLifecycleTestBackend("")
	backend.closeErr = errors.Join(errRoot, errGuard)
	sandboxInstance := newLifecycleTestSandbox(t, backend)

	const closers = 64
	results := make(chan error, closers)
	var ready sync.WaitGroup
	ready.Add(closers)
	start := make(chan struct{})
	for range closers {
		go func() {
			ready.Done()
			<-start
			results <- sandboxInstance.Close()
		}()
	}
	ready.Wait()
	close(start)

	for range closers {
		err := <-results
		if !errors.Is(err, errRoot) || !errors.Is(err, errGuard) {
			t.Fatalf("Close() error = %v, want complete backend error chain", err)
		}
	}
	if got := backend.closeCalls.Load(); got != 1 {
		t.Fatalf("backend Close() calls = %d, want 1", got)
	}
	if err := sandboxInstance.Close(); !errors.Is(err, errRoot) || !errors.Is(err, errGuard) {
		t.Fatalf("repeated Close() error = %v, want cached complete error chain", err)
	}
}

func TestSandboxCloseConvertsAndCachesBackendPanic(t *testing.T) {
	panicErr := errors.New("close panic")
	backend := newLifecycleTestBackend("")
	backend.closeShouldPanic = true
	backend.closePanic = panicErr
	sandboxInstance := newLifecycleTestSandbox(t, backend)

	type closeResult struct {
		err        error
		panicValue any
	}
	const closers = 64
	results := make(chan closeResult, closers)
	var ready sync.WaitGroup
	ready.Add(closers)
	start := make(chan struct{})
	for range closers {
		go func() {
			ready.Done()
			<-start
			result := closeResult{}
			defer func() {
				result.panicValue = recover()
				results <- result
			}()
			result.err = sandboxInstance.Close()
		}()
	}
	ready.Wait()
	close(start)

	const expected = "sandbox: backend close panicked"
	var cached error
	for range closers {
		result := <-results
		if result.panicValue != nil {
			t.Fatalf("Close() panicked: %v", result.panicValue)
		}
		err := result.err
		if err == nil || err.Error() != expected || !errors.Is(err, panicErr) {
			t.Fatalf("Close() error = %v, want cached panic error %q", err, expected)
		}
		if cached == nil {
			cached = err
		} else if err != cached {
			t.Fatal("concurrent Close() calls did not receive the cached panic result")
		}
	}
	if got := backend.closeCalls.Load(); got != 1 {
		t.Fatalf("backend Close() calls = %d, want 1", got)
	}
	if err := sandboxInstance.Close(); err != cached {
		t.Fatal("repeated Close() did not return the cached panic result")
	}
}

type lifecycleClosePanicValue interface {
	PanicValue() any
}

type lifecycleExplodingError struct {
	calls *atomic.Int32
}

func (panicErr *lifecycleExplodingError) Error() string {
	panicErr.calls.Add(1)
	panic("Error must not be called")
}

func (panicErr *lifecycleExplodingError) Format(fmt.State, rune) {
	panicErr.calls.Add(1)
	panic("Format must not be called")
}

type lifecycleExplodingStringer struct {
	calls *atomic.Int32
}

func (value lifecycleExplodingStringer) String() string {
	value.calls.Add(1)
	panic("String must not be called")
}

type lifecycleExplodingFormatter struct {
	calls *atomic.Int32
}

func (value lifecycleExplodingFormatter) Format(fmt.State, rune) {
	value.calls.Add(1)
	panic("Format must not be called")
}

type lifecycleTypedNilError struct{}

func (*lifecycleTypedNilError) Error() string {
	panic("typed nil Error must not be called")
}

func TestSandboxClosePanicErrorNeverFormatsRecoveredValue(t *testing.T) {
	tests := []struct {
		name  string
		value func(*atomic.Int32) any
	}{
		{
			name: "error and formatter",
			value: func(calls *atomic.Int32) any {
				return &lifecycleExplodingError{calls: calls}
			},
		},
		{
			name: "stringer",
			value: func(calls *atomic.Int32) any {
				return lifecycleExplodingStringer{calls: calls}
			},
		},
		{
			name: "formatter",
			value: func(calls *atomic.Int32) any {
				return lifecycleExplodingFormatter{calls: calls}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			panicValue := test.value(&calls)
			backend := newLifecycleTestBackend("")
			backend.closeShouldPanic = true
			backend.closePanic = panicValue
			sandboxInstance := newLifecycleTestSandbox(t, backend)

			escaped, err := callSandboxClose(sandboxInstance)
			if escaped != nil {
				t.Fatalf("Close() leaked backend panic of type %T", escaped)
			}
			if err == nil {
				t.Fatal("Close() returned nil after backend panic")
			}
			if got, want := err.Error(), "sandbox: backend close panicked"; got != want {
				t.Fatalf("Close() error = %q, want %q", got, want)
			}
			if got := calls.Load(); got != 0 {
				t.Fatalf("panic value formatting callbacks = %d, want 0", got)
			}
			repeatedPanic, repeatedErr := callSandboxClose(sandboxInstance)
			if repeatedPanic != nil {
				t.Fatalf("repeated Close() leaked cached panic of type %T", repeatedPanic)
			}
			if repeatedErr != err {
				t.Fatal("repeated Close() did not return the same cached panic error")
			}
			var carrier lifecycleClosePanicValue
			if !errors.As(err, &carrier) {
				t.Fatalf("Close() error type %T does not expose PanicValue", err)
			}
			if got := carrier.PanicValue(); !reflect.DeepEqual(got, panicValue) {
				t.Fatalf("PanicValue type = %T, want %T", got, panicValue)
			}
		})
	}
}

func TestSandboxClosePanicErrorPreservesErrorChainsAndTypedNil(t *testing.T) {
	t.Run("joined error", func(t *testing.T) {
		first := errors.New("first close failure")
		second := errors.New("second close failure")
		panicValue := errors.Join(first, second)
		backend := newLifecycleTestBackend("")
		backend.closeShouldPanic = true
		backend.closePanic = panicValue
		sandboxInstance := newLifecycleTestSandbox(t, backend)

		escaped, err := callSandboxClose(sandboxInstance)
		if escaped != nil {
			t.Fatalf("Close() leaked backend panic of type %T", escaped)
		}
		if err == nil || !errors.Is(err, first) || !errors.Is(err, second) {
			t.Fatal("Close() did not preserve the complete joined error chain")
		}
	})

	t.Run("typed nil error", func(t *testing.T) {
		var panicValue *lifecycleTypedNilError
		backend := newLifecycleTestBackend("")
		backend.closeShouldPanic = true
		backend.closePanic = panicValue
		sandboxInstance := newLifecycleTestSandbox(t, backend)

		escaped, err := callSandboxClose(sandboxInstance)
		if escaped != nil {
			t.Fatalf("Close() leaked backend panic of type %T", escaped)
		}
		if err == nil || err.Error() != "sandbox: backend close panicked" {
			t.Fatal("Close() did not return the stable panic error")
		}
		if !errors.Is(err, panicValue) {
			t.Fatal("Close() did not preserve the typed nil error chain")
		}
		var typedNil *lifecycleTypedNilError
		if !errors.As(err, &typedNil) {
			t.Fatal("Close() error does not support errors.As for the typed nil panic")
		}
	})
}

func TestSandboxCloseConvertsPanicNilWithLegacyRuntimeBehavior(t *testing.T) {
	const helperEnvironment = "TOOLKIT_SANDBOX_CLOSE_PANIC_NIL_HELPER"
	if os.Getenv(helperEnvironment) == "1" {
		backend := newLifecycleTestBackend("")
		backend.closeShouldPanic = true
		backend.closePanic = nil
		sandboxInstance := newLifecycleTestSandbox(t, backend)

		err := sandboxInstance.Close()
		if err == nil || err.Error() != "sandbox: backend close panicked" {
			t.Fatal("Close() did not convert panic(nil) into the stable panic error")
		}
		if repeatedErr := sandboxInstance.Close(); repeatedErr != err {
			t.Fatal("repeated Close() did not return the cached panic(nil) error")
		}
		if got := backend.closeCalls.Load(); got != 1 {
			t.Fatalf("backend Close() calls = %d, want 1", got)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSandboxCloseConvertsPanicNilWithLegacyRuntimeBehavior$")
	command.Env = append(os.Environ(), helperEnvironment+"=1", "GODEBUG=panicnil=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("panic(nil) helper failed: %v\n%s", err, output)
	}
}

func TestSandboxClosePanicResultIsStableForLeaderAndFollowers(t *testing.T) {
	panicErr := errors.New("close panic")
	releaseBackend := make(chan struct{})
	backend := newLifecycleTestBackend("")
	backend.closeShouldPanic = true
	backend.closePanic = panicErr
	backend.closeRelease = releaseBackend
	sandboxInstance := newLifecycleTestSandbox(t, backend)

	const followers = 63
	closeEntries := make(chan struct{}, followers+1)
	sandboxInstance.lifecycle.beforeCloseOnce = func() {
		closeEntries <- struct{}{}
	}
	results := make(chan error, followers+1)
	go func() { results <- sandboxInstance.Close() }()
	awaitLifecycleSignal(t, closeEntries, "leader did not enter lifecycle Close()")
	awaitLifecycleSignal(t, backend.closeEntered, "leader did not enter backend Close()")

	ready := make(chan struct{}, followers)
	start := make(chan struct{})
	for range followers {
		go func() {
			ready <- struct{}{}
			<-start
			results <- sandboxInstance.Close()
		}()
	}
	for range followers {
		<-ready
	}
	close(start)
	for range followers {
		awaitLifecycleSignal(t, closeEntries, "follower did not enter lifecycle Close()")
	}
	select {
	case <-results:
		t.Fatal("Close() returned before the blocked backend was released")
	default:
	}

	close(releaseBackend)
	var cached error
	for range followers + 1 {
		err := awaitLifecycleResult(t, results, "Close() did not return after the backend was released")
		if err == nil || !errors.Is(err, panicErr) {
			t.Fatal("Close() did not preserve the backend panic error chain")
		}
		if cached == nil {
			cached = err
		} else if err != cached {
			t.Fatal("Close() callers did not receive the same cached error")
		}
	}
	if got := backend.closeCalls.Load(); got != 1 {
		t.Fatalf("backend Close() calls = %d, want 1", got)
	}
}

func callSandboxClose(sandboxInstance *capabilitySandbox) (panicValue any, err error) {
	defer func() {
		panicValue = recover()
	}()
	err = sandboxInstance.Close()
	return nil, err
}

func TestSandboxMethodsRejectAfterCloseWithoutCallingBackend(t *testing.T) {
	backend := newLifecycleTestBackend("")
	sandboxInstance := newLifecycleTestSandbox(t, backend)
	if err := sandboxInstance.Close(); err != nil {
		t.Fatal(err)
	}

	beforeExec := backend.execCalls.Load()
	beforeCapabilities := backend.capabilityCalls.Load()

	if _, err := sandboxInstance.Exec(context.Background(), Command{}); !errors.Is(err, ErrSandboxClosed) {
		t.Fatalf("Exec() after Close error = %v, want ErrSandboxClosed", err)
	}
	if _, err := sandboxInstance.Capabilities(context.Background()); !errors.Is(err, ErrSandboxClosed) {
		t.Fatalf("Capabilities() after Close error = %v, want ErrSandboxClosed", err)
	}
	if _, err := AvailableCapabilities(context.Background(), sandboxInstance); !errors.Is(err, ErrSandboxClosed) {
		t.Fatalf("AvailableCapabilities() after Close error = %v, want ErrSandboxClosed", err)
	}

	if got := backend.execCalls.Load(); got != beforeExec {
		t.Fatalf("backend Exec() calls after Close = %d, want %d", got, beforeExec)
	}
	if got := backend.capabilityCalls.Load(); got != beforeCapabilities {
		t.Fatalf("backend capability calls after Close = %d, want %d", got, beforeCapabilities)
	}
}

func awaitSandboxClosing(t *testing.T, sandboxInstance *capabilitySandbox) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !sandboxInstance.lifecycle.isClosing() {
		if time.Now().After(deadline) {
			t.Fatal("sandbox did not enter closing state")
		}
		runtime.Gosched()
	}
}

func awaitLifecycleSignal(t *testing.T, signal <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal(failure)
	}
}

func awaitLifecycleResult(t *testing.T, result <-chan error, failure string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal(failure)
		return nil
	}
}
