package sandbox

import (
	"errors"
	"testing"
	"time"
)

func TestWaitForWindowsExecutionResultPreservesCompletedPayload(t *testing.T) {
	wantErr := errors.New("wait failed")
	completion := newWindowsExecutionCompletion()
	completion.start(func() windowsExecutionWaitResult {
		return windowsExecutionWaitResult{err: wantErr}
	})

	got, completed := completion.wait(time.Second)
	if !completed {
		t.Fatal("completed Windows wait result was not returned")
	}
	if got.state != nil || !errors.Is(got.err, wantErr) {
		t.Fatalf("Windows wait result = %#v, want preserved state and error", got)
	}
}
