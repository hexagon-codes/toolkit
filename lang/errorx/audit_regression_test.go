package errorx

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExecutionHelpersRejectNilOperation(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "Try", run: func() error { return Try(nil) }},
		{name: "TryWithValue", run: func() error {
			_, err := TryWithValue[int](nil)
			return err
		}},
		{name: "TryWithError", run: func() error {
			_, err := TryWithError[int](nil)
			return err
		}},
		{name: "Safe", run: func() error { return Safe(nil) }},
		{name: "CollectErrors", run: func() error { return CollectErrors(nil) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil {
				t.Fatal("nil operation returned no error")
			}
			if !strings.Contains(err.Error(), "operation must not be nil") {
				t.Fatalf("nil operation error = %q, want an explicit validation error", err)
			}
		})
	}
}

func TestGoReturnsNilWhenAllOperationsSucceed(t *testing.T) {
	err := Go(func() error { return nil })
	if err != nil {
		t.Fatalf("Go success error = %v, want nil", err)
	}
}

func TestGoUsesBoundedDefaultConcurrency(t *testing.T) {
	const operationCount = DefaultGoLimit + 64

	release := make(chan struct{})
	started := make(chan struct{}, operationCount)
	operations := make([]func() error, operationCount)
	for index := range operations {
		operations[index] = func() error {
			started <- struct{}{}
			<-release
			return nil
		}
	}

	done := make(chan error, 1)
	go func() {
		done <- Go(operations...)
	}()
	cleaned := false
	defer func() {
		if !cleaned {
			close(release)
			<-done
		}
	}()

	for range DefaultGoLimit {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("default workers did not start")
		}
	}
	for range 100 {
		runtime.Gosched()
	}
	select {
	case <-started:
		t.Fatalf("Go exceeded DefaultGoLimit=%d", DefaultGoLimit)
	default:
	}

	close(release)
	cleaned = true
	if err := <-done; err != nil {
		t.Fatalf("Go() error = %v", err)
	}
}

func TestGoWithLimitReturnsNilWhenAllOperationsSucceed(t *testing.T) {
	err := GoWithLimit(1, func() error { return nil })
	if err != nil {
		t.Fatalf("GoWithLimit success error = %v, want nil", err)
	}
}

func TestGoWithLimitRejectsInvalidLimitBeforeRunningOperations(t *testing.T) {
	for _, limit := range []int{0, -1} {
		t.Run(fmt.Sprintf("limit_%d", limit), func(t *testing.T) {
			ran := false
			err := GoWithLimit(limit, func() error {
				ran = true
				return nil
			})

			if err == nil {
				t.Fatal("invalid limit returned no error")
			}
			if !strings.Contains(err.Error(), "limit must be greater than zero") {
				t.Fatalf("invalid limit error = %q, want an explicit validation error", err)
			}
			if ran {
				t.Fatal("operation ran despite an invalid limit")
			}
		})
	}
}

func TestGoWithLimitBoundsWorkerGoroutines(t *testing.T) {
	const (
		operationCount = 128
		limit          = 3
	)

	release := make(chan struct{})
	started := make(chan struct{}, operationCount)
	operations := make([]func() error, operationCount)
	for i := range operations {
		operations[i] = func() error {
			started <- struct{}{}
			<-release
			return nil
		}
	}

	baseline := runtime.NumGoroutine()
	done := make(chan error, 1)
	go func() {
		done <- GoWithLimit(limit, operations...)
	}()
	cleaned := false
	defer func() {
		if !cleaned {
			close(release)
			<-done
		}
	}()

	for i := 0; i < limit; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for workers to start")
		}
	}

	// 让调度器有机会运行所有已创建但尚未执行的 goroutine。
	for i := 0; i < 100; i++ {
		runtime.Gosched()
	}
	created := runtime.NumGoroutine() - baseline
	if created > limit+2 {
		t.Fatalf("GoWithLimit created %d goroutines for limit %d; want at most %d including the caller", created, limit, limit+2)
	}

	close(release)
	cleaned = true
	if err := <-done; err != nil {
		t.Fatalf("GoWithLimit returned error after successful operations: %v", err)
	}
}

func TestParallelOperationsContainPanicsAndRejectNil(t *testing.T) {
	for _, mode := range []string{"go-panic", "go-with-limit-panic", "go-nil", "go-with-limit-nil"} {
		t.Run(mode, func(t *testing.T) {
			runAuditProcessHelper(t, mode)
		})
	}
}

func TestMultiErrorRejectsCyclesWithoutHanging(t *testing.T) {
	for _, mode := range []string{"multi-self", "multi-cycle"} {
		t.Run(mode, func(t *testing.T) {
			runAuditProcessHelper(t, mode)
		})
	}
}

func TestMultiErrorBoundsRetainedErrorsAndReportsOmissions(t *testing.T) {
	const total = 10_000
	me := NewMultiError()
	for i := 0; i < total; i++ {
		me.Append(fmt.Errorf("error %d", i))
	}

	retained := me.Errors()
	if len(retained) >= total {
		t.Fatalf("retained %d of %d errors; aggregation is unbounded", len(retained), total)
	}
	if me.Len() != total {
		t.Fatalf("Len = %d, want total observed count %d", me.Len(), total)
	}
	if got := me.First().Error(); got != "error 0" {
		t.Fatalf("First = %q, want first observed error", got)
	}
	if got := me.Last().Error(); got != "error 9999" {
		t.Fatalf("Last = %q, want latest observed error", got)
	}
	if text := me.Error(); !strings.Contains(text, "errors omitted") {
		t.Fatalf("bounded aggregate text = %q, want omission diagnostics", text)
	}
}

func runAuditProcessHelper(t *testing.T, mode string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAuditProcessHelper$")
	cmd.Env = append(os.Environ(), "ERRORX_AUDIT_HELPER="+mode)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("helper %s hung: %v\n%s", mode, ctx.Err(), output)
	}
	if err != nil {
		t.Fatalf("helper %s crashed: %v\n%s", mode, err, output)
	}
}

func TestAuditProcessHelper(t *testing.T) {
	mode := os.Getenv("ERRORX_AUDIT_HELPER")
	if mode == "" {
		t.Skip("subprocess helper")
	}

	switch mode {
	case "go-panic":
		assertOperationFailure(t, Go(func() error { panic("boom") }), "panic: boom")
	case "go-with-limit-panic":
		assertOperationFailure(t, GoWithLimit(1, func() error { panic("boom") }), "panic: boom")
	case "go-nil":
		assertOperationFailure(t, Go(nil), "operation must not be nil")
	case "go-with-limit-nil":
		assertOperationFailure(t, GoWithLimit(1, nil), "operation must not be nil")
	case "multi-self":
		me := NewMultiError()
		me.Append(me)
		assertCyclicAggregate(t, me)
	case "multi-cycle":
		first := NewMultiError()
		second := NewMultiError()
		first.Append(second)
		second.Append(first)
		assertCyclicAggregate(t, second)
	default:
		t.Fatalf("unknown helper mode %q", mode)
	}
}

func assertOperationFailure(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatal("operation returned no error")
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("operation error = %q, want substring %q", err, want)
	}
}

func assertCyclicAggregate(t *testing.T, err error) {
	t.Helper()
	if !strings.Contains(err.Error(), "cyclic error reference") {
		t.Fatalf("cyclic aggregate error = %q, want cycle diagnostics", err)
	}
}
