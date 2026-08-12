package sandbox

import (
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type retryableWindowsCloseTestBackend struct {
	*lifecycleTestBackend
	closeAttempts atomic.Int32
	firstErr      error
}

func (backend *retryableWindowsCloseTestBackend) Close() error {
	if backend.closeAttempts.Add(1) == 1 {
		return backend.firstErr
	}
	return nil
}

func (*retryableWindowsCloseTestBackend) sandboxCloseRetryable() {}

func TestWindowsSandboxPublicCloseRetriesAnUnconfirmedBackend(t *testing.T) {
	unconfirmed := errors.Join(
		errors.New("first reclaim failed"),
		errWindowsProcessContainmentUnconfirmed,
	)
	backend := &retryableWindowsCloseTestBackend{
		lifecycleTestBackend: newLifecycleTestBackend(""),
		firstErr:             unconfirmed,
	}
	sandboxInstance := &capabilitySandbox{backend: backend}

	if err := sandboxInstance.Close(); !errors.Is(err, unconfirmed) {
		t.Fatalf("first Close() error = %v, want unconfirmed lifecycle", err)
	}
	if release, err := sandboxInstance.lifecycle.begin(); !errors.Is(err, ErrSandboxClosed) {
		if release != nil {
			release()
		}
		t.Fatalf("operation after failed Close() error = %v, want ErrSandboxClosed", err)
	}
	if err := sandboxInstance.Close(); err != nil {
		t.Fatalf("retried Close() error = %v", err)
	}
	if got := backend.closeAttempts.Load(); got != 2 {
		t.Fatalf("backend Close() attempts = %d, want 2", got)
	}
}

func TestWindowsRetainedLifecycleTransfersOwnershipExactlyOnce(t *testing.T) {
	reclaimCalls := atomic.Int32{}
	lifecycle := newWindowsRetainedLifecycle(func(time.Duration) (bool, error) {
		return reclaimCalls.Add(1) >= 2, nil
	})

	confirmed, err := lifecycle.reclaim(time.Millisecond)
	if err != nil {
		t.Fatalf("initial reclaim error = %v", err)
	}
	if confirmed {
		t.Fatal("initial reclaim unexpectedly confirmed the lifecycle")
	}

	quarantine := &windowsProcessQuarantine{}
	if retainErr := lifecycle.retain(quarantine); retainErr != nil {
		t.Fatalf("retain lifecycle: %v", retainErr)
	}
	if retainErr := lifecycle.retain(quarantine); retainErr == nil || !strings.Contains(retainErr.Error(), "already retained") {
		t.Fatalf("duplicate ownership transfer error = %v, want already retained", retainErr)
	}
	if got := quarantine.count(); got != 1 {
		t.Fatalf("retained lifecycle count = %d, want 1", got)
	}

	confirmed, err = quarantine.reclaim(1, time.Millisecond)
	if err != nil {
		t.Fatalf("reclaim retained lifecycle: %v", err)
	}
	if !confirmed || quarantine.count() != 0 {
		t.Fatalf("reclaimed lifecycle = confirmed:%t count:%d", confirmed, quarantine.count())
	}
	if err := lifecycle.retain(quarantine); err == nil || !strings.Contains(err.Error(), "already reclaimed") {
		t.Fatalf("post-reclaim ownership transfer error = %v, want already reclaimed", err)
	}
	if got := reclaimCalls.Load(); got != 2 {
		t.Fatalf("lifecycle reclaim calls = %d, want 2", got)
	}
}

func TestWindowsRetainedLifecycleDoesNotTransferDuringCallerReclaim(t *testing.T) {
	reclaimEntered := make(chan struct{})
	reclaimRelease := make(chan struct{})
	lifecycle := newWindowsRetainedLifecycle(func(time.Duration) (bool, error) {
		close(reclaimEntered)
		<-reclaimRelease
		return true, nil
	})
	quarantine := &windowsProcessQuarantine{}
	reclaimDone := make(chan error, 1)
	go func() {
		confirmed, err := lifecycle.reclaim(time.Second)
		if !confirmed && err == nil {
			err = errors.New("caller reclaim was not confirmed")
		}
		reclaimDone <- err
	}()
	<-reclaimEntered

	retainDone := make(chan error, 1)
	go func() { retainDone <- lifecycle.retain(quarantine) }()
	select {
	case err := <-retainDone:
		t.Fatalf("ownership transfer raced with caller reclaim: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(reclaimRelease)
	if err := <-reclaimDone; err != nil {
		t.Fatalf("caller reclaim error = %v", err)
	}
	if err := <-retainDone; err == nil || !strings.Contains(err.Error(), "already reclaimed") {
		t.Fatalf("post-reclaim transfer error = %v, want already reclaimed", err)
	}
	if got := quarantine.count(); got != 0 {
		t.Fatalf("post-reclaim quarantine count = %d, want 0", got)
	}
}

func TestWindowsExecutionCompletionHasOneOwnedWaiterAndSharedResult(t *testing.T) {
	completion := newWindowsExecutionCompletion()
	waitStarted := make(chan struct{})
	waitRelease := make(chan struct{})
	waitCalls := atomic.Int32{}
	completion.start(func() windowsExecutionWaitResult {
		waitCalls.Add(1)
		close(waitStarted)
		<-waitRelease
		return windowsExecutionWaitResult{err: errors.New("wait result")}
	})
	completion.start(func() windowsExecutionWaitResult {
		waitCalls.Add(1000)
		return windowsExecutionWaitResult{}
	})

	select {
	case <-waitStarted:
	case <-time.After(time.Second):
		t.Fatal("owned Windows wait goroutine did not start")
	}
	if _, finished := completion.wait(0); finished {
		t.Fatal("blocked Windows wait was reported as finished")
	}
	close(waitRelease)
	first, finished := completion.wait(time.Second)
	if !finished || first.err == nil || first.err.Error() != "wait result" {
		t.Fatalf("first completion result = %+v, finished:%t", first, finished)
	}
	second, finished := completion.wait(0)
	if !finished || second.err != first.err {
		t.Fatalf("shared completion result = %+v, finished:%t", second, finished)
	}
	if got := waitCalls.Load(); got != 1 {
		t.Fatalf("owned Windows wait goroutines = %d, want 1", got)
	}
}

func TestWindowsWaitReleasedProcessIsTreatedAsClosed(t *testing.T) {
	body := mustFunctionBody(t, "win_launcher.go", "func releaseWindowsProcess")
	for _, required := range []string{"os.ErrProcessDone", "syscall.EINVAL"} {
		if !strings.Contains(body, required) {
			t.Errorf("releaseWindowsProcess is missing completed state %q", required)
		}
	}
}

func TestWindowsRequiredSandboxTestsRegisterDeterministicCleanup(t *testing.T) {
	helper := mustReadSandboxSource(t, "windows_test_helpers_test.go")
	for _, required := range []string{
		"t.Cleanup(func()",
		"sandboxValue.Close()",
		"t.Errorf(\"close Windows sandbox: %v\"",
	} {
		if !strings.Contains(helper, required) {
			t.Errorf("Windows test cleanup helper is missing %q", required)
		}
	}

	tests := []struct {
		file     string
		function string
		cleanup  string
	}{
		{"win_integration_test.go", "func TestWindows_SandboxCreation", "newWindowsTestSandbox("},
		{"win_integration_test.go", "func TestWindows_ExecSimpleCommand", "newWindowsTestSandbox("},
		{"win_integration_test.go", "func TestWindows_WorkspaceIsolation", "newWindowsTestSandbox("},
		{"win_integration_test.go", "func TestWindows_HostPathEscapeDenied", "newWindowsTestSandbox("},
		{"win_acl_policy_test.go", "func TestWindowsWorkspaceRejectsExternalHardLink", "newWindowsTestSandbox("},
		{"win_acl_policy_test.go", "func TestWindowsWorkspaceRejectsReparsePoint", "newWindowsTestSandbox("},
		{"win_acl_policy_test.go", "func TestWindowsWorkspaceRejectsInternalReparsePoint", "newWindowsTestSandbox("},
		{"windows_root_contract_regression_test.go", "func TestWindowsNetworkPolicyMatrix", "registerWindowsTestSandboxCleanup("},
		{"windows_security_boundary_test.go", "func TestWindows_NetworkDisabledSocketMatrix", "newWindowsTestSandbox("},
		{"win_integration_test.go", "func TestWindows_Timeout", "newWindowsTestSandbox("},
		{"win_integration_test.go", "func TestWindows_TimeoutTerminatesProcessContainment", "newWindowsTestSandbox("},
		{"windows_security_boundary_test.go", "func TestWindows_ContextCanceledTerminatesProcessContainment", "newWindowsTestSandbox("},
		{"win_integration_test.go", "func runWindowsJobMemoryScenario", "newWindowsTestSandbox("},
		{"win_integration_test.go", "func runWindowsJobProcessScenario", "newWindowsTestSandbox("},
		{"win_integration_test.go", "func TestWindows_ExecPreservesNonZeroExitCode", "newWindowsTestSandbox("},
	}
	for _, test := range tests {
		body := mustFunctionBody(t, test.file, test.function)
		if !strings.Contains(body, test.cleanup) {
			t.Errorf("%s in %s does not register Windows sandbox cleanup", test.function, test.file)
		}
	}
}

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

func TestWindowsActiveResourceClosesOnlyAfterPendingReadStops(t *testing.T) {
	resource := newWindowsActiveResource(55)
	value, ok := resource.beginRead()
	if !ok || value != 55 {
		t.Fatalf("begin read = %d, %t, want 55, true", value, ok)
	}

	cancelEntered := make(chan struct{})
	releaseCalled := make(chan struct{}, 1)
	closeDone := make(chan error, 1)
	go func() {
		closeDone <- resource.closeAfterReads(
			func(got int) error {
				if got != 55 {
					t.Errorf("cancel resource = %d, want 55", got)
				}
				close(cancelEntered)
				return nil
			},
			func(got int) error {
				if got != 55 {
					t.Errorf("release resource = %d, want 55", got)
				}
				releaseCalled <- struct{}{}
				return nil
			},
			time.Second,
		)
	}()

	<-cancelEntered
	if _, ok := resource.beginRead(); ok {
		t.Fatal("closing resource accepted a new read")
	}
	select {
	case <-releaseCalled:
		t.Fatal("resource was released before the active read stopped")
	default:
	}
	resource.endRead()
	if err := <-closeDone; err != nil {
		t.Fatalf("close active resource: %v", err)
	}
	select {
	case <-releaseCalled:
	default:
		t.Fatal("resource was not released after the active read stopped")
	}
	if got := resource.value(); got != 0 {
		t.Fatalf("released resource remains owned: %d", got)
	}
}

func TestWindowsActiveResourceTimeoutRetainsOwnershipForRetry(t *testing.T) {
	resource := newWindowsActiveResource(66)
	_, ok := resource.beginRead()
	if !ok {
		t.Fatal("active read was not acquired")
	}
	cancelFailure := errors.New("cancel failed")
	releaseCalls := 0

	err := resource.closeAfterReads(
		func(int) error { return cancelFailure },
		func(int) error {
			releaseCalls++
			return nil
		},
		10*time.Millisecond,
	)
	if !errors.Is(err, cancelFailure) {
		t.Fatalf("close error %v does not preserve cancel failure", err)
	}
	if !errors.Is(err, errWindowsActiveReadDrainTimeout) {
		t.Fatalf("close error %v does not preserve drain timeout", err)
	}
	if releaseCalls != 0 {
		t.Fatalf("release calls before read drain = %d, want 0", releaseCalls)
	}
	if got := resource.value(); got != 66 {
		t.Fatalf("timed-out resource ownership = %d, want 66", got)
	}

	resource.endRead()
	if err := resource.closeAfterReads(nil, func(int) error {
		releaseCalls++
		return nil
	}, time.Second); err != nil {
		t.Fatalf("retry close active resource: %v", err)
	}
	if releaseCalls != 1 {
		t.Fatalf("release calls after retry = %d, want 1", releaseCalls)
	}
}

func TestWindowsActiveResourceCancellationPreventsHandleReuseWindow(t *testing.T) {
	const iterations = 500
	for iteration := 1; iteration <= iterations; iteration++ {
		resource := newWindowsActiveResource(iteration)
		value, ok := resource.beginRead()
		if !ok || value != iteration {
			t.Fatalf("iteration %d begin read = %d, %t", iteration, value, ok)
		}
		var readFinished atomic.Bool
		closeDone := make(chan error, 1)
		cancelEntered := make(chan struct{})
		go func(expected int) {
			closeDone <- resource.closeAfterReads(
				func(got int) error {
					if got != expected {
						t.Errorf("iteration %d cancel resource = %d", expected, got)
					}
					close(cancelEntered)
					return nil
				},
				func(got int) error {
					if !readFinished.Load() {
						t.Errorf("iteration %d released before read finished", expected)
					}
					if got != expected {
						t.Errorf("iteration %d released reused resource %d", expected, got)
					}
					return nil
				},
				time.Second,
			)
		}(iteration)
		<-cancelEntered
		readFinished.Store(true)
		resource.endRead()
		if err := <-closeDone; err != nil {
			t.Fatalf("iteration %d close active resource: %v", iteration, err)
		}
	}
}

func TestWindowsProcessQuarantineRejectsInvalidConstruction(t *testing.T) {
	lifecycle := newWindowsRetainedLifecycle(nil)
	if err := lifecycle.retain(&windowsProcessQuarantine{}); err == nil {
		t.Fatal("nil retained lifecycle reclaimer was accepted")
	}
}

func TestWindowsLaunchFailureTransfersOwnershipToQuarantine(t *testing.T) {
	reclaimed := 0
	lifecycle := newWindowsRetainedLifecycle(func(time.Duration) (bool, error) {
		reclaimed++
		return true, nil
	})
	quarantine := &windowsProcessQuarantine{}

	if err := lifecycle.retain(quarantine); err != nil {
		t.Fatalf("retain failed launch lifecycle: %v", err)
	}
	if got := quarantine.count(); got != 1 {
		t.Fatalf("quarantined resources = %d, want 1", got)
	}
	if reclaimed != 0 {
		t.Fatalf("resources reclaimed before Close = %d, want 0", reclaimed)
	}

	confirmed, closeErr := quarantine.reclaim(3, time.Second)
	if closeErr != nil {
		t.Fatalf("reclaim quarantine: %v", closeErr)
	}
	if !confirmed || quarantine.count() != 0 || reclaimed != 1 {
		t.Fatalf("reclaim state = confirmed:%t count:%d calls:%d", confirmed, quarantine.count(), reclaimed)
	}
}

func TestWindowsProcessQuarantineRetriesBoundedlyAndPreservesErrors(t *testing.T) {
	firstFailure := errors.New("first reclaim failed")
	secondFailure := errors.New("second reclaim failed")
	attempts := 0
	lifecycle := newWindowsRetainedLifecycle(func(timeout time.Duration) (bool, error) {
		if timeout != 25*time.Millisecond {
			t.Errorf("reclaim timeout = %s, want 25ms", timeout)
		}
		attempts++
		switch attempts {
		case 1:
			return false, firstFailure
		case 2:
			return false, secondFailure
		default:
			return true, nil
		}
	})
	quarantine := &windowsProcessQuarantine{}
	if retainErr := lifecycle.retain(quarantine); retainErr != nil {
		t.Fatal(retainErr)
	}

	confirmed, err := quarantine.reclaim(3, 25*time.Millisecond)
	if !confirmed {
		t.Fatal("quarantine was not confirmed after the bounded retry")
	}
	for _, want := range []error{firstFailure, secondFailure} {
		if !errors.Is(err, want) {
			t.Fatalf("reclaim error %v does not preserve %v", err, want)
		}
	}
	if attempts != 3 || quarantine.count() != 0 {
		t.Fatalf("reclaim attempts/count = %d/%d, want 3/0", attempts, quarantine.count())
	}
	confirmed, err = quarantine.reclaim(3, 25*time.Millisecond)
	if !confirmed || err != nil || attempts != 3 {
		t.Fatalf("idempotent reclaim = confirmed:%t err:%v attempts:%d", confirmed, err, attempts)
	}
}

func TestWindowsProcessQuarantineRetainsUnconfirmedResources(t *testing.T) {
	attempts := 0
	lifecycle := newWindowsRetainedLifecycle(func(time.Duration) (bool, error) {
		attempts++
		return false, nil
	})
	quarantine := &windowsProcessQuarantine{}
	if retainErr := lifecycle.retain(quarantine); retainErr != nil {
		t.Fatal(retainErr)
	}

	confirmed, err := quarantine.reclaim(2, time.Millisecond)
	if confirmed {
		t.Fatal("unconfirmed quarantine was reported as reclaimed")
	}
	if !errors.Is(err, errWindowsProcessLifecycleUnconfirmed) {
		t.Fatalf("reclaim error = %v, want lifecycle error", err)
	}
	if attempts != 2 || quarantine.count() != 1 {
		t.Fatalf("reclaim attempts/count = %d/%d, want 2/1", attempts, quarantine.count())
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

func TestWaitForWindowsExecutionResultIsBounded(t *testing.T) {
	completion := newWindowsExecutionCompletion()
	release := make(chan struct{})
	completion.start(func() windowsExecutionWaitResult {
		<-release
		return windowsExecutionWaitResult{}
	})
	started := time.Now()
	_, ok := completion.wait(20 * time.Millisecond)
	if ok {
		t.Fatal("unfinished process was reported as completed")
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("execution wait was not bounded: %s", elapsed)
	}
	close(release)
	if _, finished := completion.wait(time.Second); !finished {
		t.Fatal("owned Windows wait goroutine did not stop after release")
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

func TestSettleWindowsJobDoesNotCloseAnUnconfirmedProcessContainment(t *testing.T) {
	waitFailure := errors.New("wait failed")
	terminateFailure := errors.New("terminate failed")
	closeCalls := 0
	confirmed, err := settleWindowsJobConfirmed(windowsJobLifecycle{
		wait:      func(time.Duration) (bool, error) { return false, waitFailure },
		terminate: func() error { return terminateFailure },
		close: func() error {
			closeCalls++
			return nil
		},
	}, time.Millisecond, time.Millisecond)
	if confirmed {
		t.Fatal("unconfirmed process tree was reported as empty")
	}
	if closeCalls != 0 {
		t.Fatalf("unconfirmed Job was closed %d times", closeCalls)
	}
	for _, want := range []error{waitFailure, terminateFailure} {
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
	if !strings.Contains(launcher, "reader.closeAfterReads(") {
		t.Error("reader cancellation must wait for active reads before releasing the handle")
	}
	if strings.Contains(launcher, "handle := reader.value()") {
		t.Error("reader cancellation must not retain an unlocked raw handle")
	}
	readerBody := mustFunctionBody(t, "win_launcher.go", "func readHandle")
	for _, required := range []string{"handle.beginRead()", "handle.endRead()", "handle.isClosing()"} {
		if !strings.Contains(readerBody, required) {
			t.Errorf("readHandle is missing %q", required)
		}
	}
	if strings.Contains(readerBody, "handle.value()") {
		t.Error("readHandle must not use a raw handle outside the active-read lifecycle")
	}
}

func TestWindowsSandboxCloseReclaimsQuarantineBeforeWorkspace(t *testing.T) {
	closeSource := mustReadSandboxSource(t, "close_windows.go")
	body := mustFunctionBody(t, "close_windows.go", "func (s *windowsSandbox) Close")
	reclaim := strings.Index(body, "s.quarantine.reclaim(")
	workspaceClose := strings.Index(body, "s.workspace.close()")
	if reclaim < 0 || workspaceClose < 0 {
		t.Fatalf("Windows Close is missing quarantine/workspace cleanup:\n%s", body)
	}
	if reclaim > workspaceClose {
		t.Error("Windows Close must reclaim quarantined processes before closing the workspace")
	}
	if !strings.Contains(closeSource, "windowsQuarantineReclaimAttempts = 3") {
		t.Error("Windows Close must use a fixed bounded quarantine retry count")
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
