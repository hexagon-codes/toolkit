package sandbox

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOwnedWindowsResourcesRetainFailedOwnershipAndRetryWithoutDoubleClose(t *testing.T) {
	first := newOwnedWindowsResource(11)
	second := newOwnedWindowsResource(22)
	closeFailure := errors.New("close handle failed")
	closeCalls := map[int]int{}
	failSecond := true
	closeResource := func(value int) error {
		closeCalls[value]++
		if value == 22 && failSecond {
			return closeFailure
		}
		return nil
	}

	err := releaseOwnedWindowsResources(closeResource, first, second)
	if !errors.Is(err, closeFailure) {
		t.Fatalf("first release error = %v, want close failure", err)
	}
	if got := first.value(); got != 0 {
		t.Fatalf("successfully closed resource remains owned: %d", got)
	}
	if got := second.value(); got != 22 {
		t.Fatalf("failed resource ownership = %d, want 22", got)
	}

	failSecond = false
	if err := releaseOwnedWindowsResources(closeResource, first, second); err != nil {
		t.Fatalf("retry release: %v", err)
	}
	if got := second.value(); got != 0 {
		t.Fatalf("retried resource remains owned: %d", got)
	}
	if err := releaseOwnedWindowsResources(closeResource, first, second); err != nil {
		t.Fatalf("idempotent release: %v", err)
	}
	if want := map[int]int{11: 1, 22: 2}; !reflect.DeepEqual(closeCalls, want) {
		t.Fatalf("close calls = %#v, want %#v", closeCalls, want)
	}
}

func TestOwnedWindowsResourceReleaseAfterPreservesErrorsAndRetry(t *testing.T) {
	resource := newOwnedWindowsResource(33)
	beforeFailure := errors.New("cancel failed")
	closeFailure := errors.New("close failed")
	beforeCalls := 0
	closeCalls := 0
	failClose := true

	err := resource.releaseAfter(func(value int) error {
		beforeCalls++
		if value != 33 {
			t.Fatalf("before value = %d, want 33", value)
		}
		return beforeFailure
	}, func(value int) error {
		closeCalls++
		if failClose {
			return closeFailure
		}
		return nil
	})
	for _, want := range []error{beforeFailure, closeFailure} {
		if !errors.Is(err, want) {
			t.Fatalf("release error %v does not preserve %v", err, want)
		}
	}
	if got := resource.value(); got != 33 {
		t.Fatalf("failed close ownership = %d, want 33", got)
	}

	failClose = false
	if err := resource.releaseAfter(func(int) error {
		beforeCalls++
		return nil
	}, func(int) error {
		closeCalls++
		return nil
	}); err != nil {
		t.Fatalf("retry release: %v", err)
	}
	if got := resource.value(); got != 0 {
		t.Fatalf("retried resource remains owned: %d", got)
	}
	if err := resource.releaseAfter(func(int) error {
		beforeCalls++
		return nil
	}, func(int) error {
		closeCalls++
		return nil
	}); err != nil {
		t.Fatalf("idempotent release: %v", err)
	}
	if beforeCalls != 2 || closeCalls != 2 {
		t.Fatalf("before/close calls = %d/%d, want 2/2", beforeCalls, closeCalls)
	}
}

func TestOwnedWindowsResourceReleaseAfterSerializesConcurrentClose(t *testing.T) {
	resource := newOwnedWindowsResource(44)
	beforeEntered := make(chan struct{})
	continueBefore := make(chan struct{})
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	closeCalls := 0
	closeResource := func(int) error {
		closeCalls++
		return nil
	}

	go func() {
		firstDone <- resource.releaseAfter(func(int) error {
			close(beforeEntered)
			<-continueBefore
			return nil
		}, closeResource)
	}()
	<-beforeEntered
	go func() {
		secondDone <- resource.release(closeResource)
	}()

	select {
	case err := <-secondDone:
		t.Fatalf("concurrent close escaped ownership lock: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(continueBefore)
	if err := <-firstDone; err != nil {
		t.Fatalf("release after: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("concurrent idempotent release: %v", err)
	}
	if closeCalls != 1 {
		t.Fatalf("close calls = %d, want 1", closeCalls)
	}
}

func TestSettleWindowsJobDoesNotTerminateAnEmptyJob(t *testing.T) {
	var calls []string
	err := settleWindowsJob(windowsJobLifecycle{
		wait: func(time.Duration) (bool, error) {
			calls = append(calls, "wait")
			return true, nil
		},
		terminate: func() error {
			calls = append(calls, "terminate")
			return nil
		},
		close: func() error {
			calls = append(calls, "close")
			return nil
		},
	}, time.Second, time.Second)
	if err != nil {
		t.Fatalf("settle empty job: %v", err)
	}
	if want := []string{"wait", "close"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestRunWindowsWaitLifecycleUsesDeterministicOrderAndPreservesErrors(t *testing.T) {
	processFailure := errors.New("process wait failed")
	jobFailure := errors.New("job settle failed")
	outputFailure := errors.New("output drain failed")
	cleanupFailure := errors.New("cleanup failed")
	var calls []string

	state, err := runWindowsWaitLifecycle(windowsWaitLifecycle[string]{
		waitProcess: func() (string, error) {
			calls = append(calls, "process")
			return "state", processFailure
		},
		settleJob: func() error {
			calls = append(calls, "job")
			return jobFailure
		},
		waitOutput: func() error {
			calls = append(calls, "output")
			return outputFailure
		},
		cleanup: func() error {
			calls = append(calls, "cleanup")
			return cleanupFailure
		},
	})
	if state != "state" {
		t.Fatalf("state = %q, want state", state)
	}
	if want := []string{"process", "job", "output", "cleanup"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for _, want := range []error{processFailure, jobFailure, outputFailure, cleanupFailure} {
		if !errors.Is(err, want) {
			t.Fatalf("wait error %v does not preserve %v", err, want)
		}
	}
}

func TestTerminateWindowsProcessWaitsBoundedlyAndPreservesErrors(t *testing.T) {
	terminateFailure := errors.New("terminate failed")
	waitFailure := errors.New("wait failed")
	var calls []string
	err := terminateWindowsProcess(windowsProcessTerminationLifecycle{
		terminate: func() error {
			calls = append(calls, "terminate")
			return terminateFailure
		},
		wait: func(timeout time.Duration) (bool, error) {
			calls = append(calls, "wait:"+timeout.String())
			return false, waitFailure
		},
	}, 25*time.Millisecond)
	if want := []string{"terminate", "wait:25ms"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for _, want := range []error{terminateFailure, waitFailure} {
		if !errors.Is(err, want) {
			t.Fatalf("termination error %v does not preserve %v", err, want)
		}
	}
}

func TestTerminateWindowsProcessReportsBoundedTimeout(t *testing.T) {
	err := terminateWindowsProcess(windowsProcessTerminationLifecycle{
		terminate: func() error { return nil },
		wait:      func(time.Duration) (bool, error) { return false, nil },
	}, 30*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "did not exit within 30ms") {
		t.Fatalf("termination error = %v, want bounded timeout", err)
	}
}

func TestSettleWindowsJobTerminatesOnlyAfterBoundedGrace(t *testing.T) {
	var calls []string
	waits := 0
	err := settleWindowsJob(windowsJobLifecycle{
		wait: func(timeout time.Duration) (bool, error) {
			calls = append(calls, "wait:"+timeout.String())
			waits++
			return waits == 2, nil
		},
		terminate: func() error {
			calls = append(calls, "terminate")
			return nil
		},
		close: func() error {
			calls = append(calls, "close")
			return nil
		},
	}, 10*time.Millisecond, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("settle active job: %v", err)
	}
	want := []string{"wait:10ms", "terminate", "wait:20ms", "close"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestSettleWindowsJobPreservesWaitTerminateAndCloseErrors(t *testing.T) {
	waitFailure := errors.New("wait failed")
	terminateFailure := errors.New("terminate failed")
	closeFailure := errors.New("close failed")
	err := settleWindowsJob(windowsJobLifecycle{
		wait:      func(time.Duration) (bool, error) { return false, waitFailure },
		terminate: func() error { return terminateFailure },
		close:     func() error { return closeFailure },
	}, time.Millisecond, time.Millisecond)
	for _, want := range []error{waitFailure, terminateFailure, closeFailure} {
		if !errors.Is(err, want) {
			t.Fatalf("settle error %v does not preserve %v", err, want)
		}
	}
}

func TestWaitForWindowsOutputIsBoundedAndDrainsAfterAbort(t *testing.T) {
	stdoutDone := make(chan error, 1)
	stderrDone := make(chan error, 1)
	stdoutDone <- nil
	close(stdoutDone)
	abortFailure := errors.New("abort readers failed")
	readerFailure := errors.New("stderr reader failed")
	abortCalls := 0
	started := time.Now()

	err := waitForWindowsOutput(stdoutDone, stderrDone, 20*time.Millisecond, func() error {
		abortCalls++
		stderrDone <- readerFailure
		close(stderrDone)
		return abortFailure
	})
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("output wait was not bounded: %s", elapsed)
	}
	if abortCalls != 1 {
		t.Fatalf("abort calls = %d, want 1", abortCalls)
	}
	for _, want := range []error{abortFailure, readerFailure} {
		if !errors.Is(err, want) {
			t.Fatalf("output error %v does not preserve %v", err, want)
		}
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("output error = %v, want timeout context", err)
	}
}

func TestWindowsLauncherQueriesActiveProcessesForJobDrain(t *testing.T) {
	launcher := mustReadSandboxSource(t, "win_launcher.go")
	for _, required := range []string{
		"QueryInformationJobObject",
		"JobObjectBasicAccountingInformation",
		"ActiveProcesses",
	} {
		if !strings.Contains(launcher, required) {
			t.Errorf("win_launcher.go is missing %q", required)
		}
	}
	if strings.Contains(launcher, "WaitForSingleObject(windows.Handle(job)") {
		t.Error("win_launcher.go must not infer an empty job from the job object's signaled state")
	}
}

func TestWindowsReaderCancellationKeepsHandleOwnershipAtomic(t *testing.T) {
	launcher := mustReadSandboxSource(t, "win_launcher.go")
	if !strings.Contains(launcher, "reader.releaseAfter(") {
		t.Error("reader cancellation must run under the resource ownership lock")
	}
	if strings.Contains(launcher, "handle := reader.value()") {
		t.Error("reader cancellation must not retain an unlocked raw handle")
	}
}

func TestWindowsLauncherAssignsJobBeforeFalliblePostCreationCleanup(t *testing.T) {
	launcher := mustReadSandboxSource(t, "win_launcher.go")
	assign := strings.Index(launcher, "assignProcessToJob(")
	if assign < 0 {
		t.Fatal("win_launcher.go is missing job assignment")
	}
	for _, marker := range []string{
		"releaseOwnedWindowsResources(closeWindowsToken",
		"releaseOwnedWindowsResources(closeWindowsHandle, stdinR",
		"os.FindProcess(",
		"resumeWindowsThread(",
	} {
		index := strings.Index(launcher, marker)
		if index < 0 {
			t.Fatalf("win_launcher.go is missing %q", marker)
		}
		if assign > index {
			t.Errorf("job assignment must precede %q", marker)
		}
	}
	if !strings.Contains(launcher, "waitForWindowsProcess(") {
		t.Error("launch failure termination must wait for the process handle")
	}
}

func TestWindowsKillDoesNotFallBackToReleasedProcessAfterJobCleanup(t *testing.T) {
	body := mustFunctionBody(t, "win_launcher.go", "func (p *windowsSandboxedProcess) Kill")
	for _, required := range []string{
		"if p.job != nil",
		"if job == 0",
		"return nil",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("Kill is missing %q", required)
		}
	}
}

func TestWaitForWindowsJobProcessesPollsUntilEmpty(t *testing.T) {
	active := []uint32{2, 1, 0}
	calls := 0
	exited, err := waitForWindowsJobProcesses(func() (uint32, error) {
		value := active[calls]
		calls++
		return value, nil
	}, 50*time.Millisecond, time.Millisecond)
	if err != nil {
		t.Fatalf("wait for job processes: %v", err)
	}
	if !exited {
		t.Fatal("job processes did not converge to empty")
	}
	if calls != len(active) {
		t.Fatalf("active process queries = %d, want %d", calls, len(active))
	}
}

func TestWaitForWindowsJobProcessesPreservesQueryError(t *testing.T) {
	queryFailure := errors.New("query job failed")
	exited, err := waitForWindowsJobProcesses(func() (uint32, error) {
		return 0, queryFailure
	}, time.Second, time.Millisecond)
	if exited {
		t.Fatal("job query failure reported an empty job")
	}
	if !errors.Is(err, queryFailure) {
		t.Fatalf("wait error = %v, want query failure", err)
	}
}

func TestWaitForWindowsJobProcessesTimesOutBoundedly(t *testing.T) {
	started := time.Now()
	exited, err := waitForWindowsJobProcesses(func() (uint32, error) {
		return 1, nil
	}, 20*time.Millisecond, time.Millisecond)
	if err != nil {
		t.Fatalf("wait for active job: %v", err)
	}
	if exited {
		t.Fatal("active job was reported as empty")
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("job wait was not bounded: %s", elapsed)
	}
}
