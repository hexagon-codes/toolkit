package sandbox

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestWindowsLaunchProtocolRoutesEveryPostCreationFailureExactlyOnce(t *testing.T) {
	t.Parallel()

	stages := []windowsLaunchStage{
		windowsLaunchStageAssignJob,
		windowsLaunchStageCloseCreationTokens,
		windowsLaunchStageCloseInheritedPipes,
		windowsLaunchStageFindProcess,
		windowsLaunchStageResumeThread,
		windowsLaunchStageCloseLaunchHandles,
	}
	for _, failedStage := range stages {
		failedStage := failedStage
		t.Run(failedStage.String(), func(t *testing.T) {
			t.Parallel()
			injected := errors.New("injected Windows launch failure")
			var calls []windowsLaunchStage
			failureCalls := 0
			process, err := runWindowsLaunchProtocol(windowsLaunchProtocolOps[int]{
				assignJob: func() error {
					calls = append(calls, windowsLaunchStageAssignJob)
					return failWindowsLaunchStage(failedStage, windowsLaunchStageAssignJob, injected)
				},
				closeCreationTokens: func() error {
					calls = append(calls, windowsLaunchStageCloseCreationTokens)
					return failWindowsLaunchStage(failedStage, windowsLaunchStageCloseCreationTokens, injected)
				},
				closeInheritedPipes: func() error {
					calls = append(calls, windowsLaunchStageCloseInheritedPipes)
					return failWindowsLaunchStage(failedStage, windowsLaunchStageCloseInheritedPipes, injected)
				},
				findProcess: func() (int, error) {
					calls = append(calls, windowsLaunchStageFindProcess)
					return 73, failWindowsLaunchStage(failedStage, windowsLaunchStageFindProcess, injected)
				},
				resumeThread: func() error {
					calls = append(calls, windowsLaunchStageResumeThread)
					return failWindowsLaunchStage(failedStage, windowsLaunchStageResumeThread, injected)
				},
				closeLaunchHandles: func() error {
					calls = append(calls, windowsLaunchStageCloseLaunchHandles)
					return failWindowsLaunchStage(failedStage, windowsLaunchStageCloseLaunchHandles, injected)
				},
				onFailure: func(failure windowsLaunchFailure[int]) error {
					failureCalls++
					if failure.stage != failedStage {
						t.Errorf("failure stage = %s, want %s", failure.stage, failedStage)
					}
					if failure.assignedToJob != (failedStage != windowsLaunchStageAssignJob) {
						t.Errorf("assignedToJob = %t at %s", failure.assignedToJob, failedStage)
					}
					wantProcess := 0
					if failedStage >= windowsLaunchStageResumeThread {
						wantProcess = 73
					}
					if failure.process != wantProcess {
						t.Errorf("process = %d, want %d", failure.process, wantProcess)
					}
					if !errors.Is(failure.err, injected) {
						t.Errorf("failure error = %v, want injected error", failure.err)
					}
					return nil
				},
			})
			if process != 0 {
				t.Fatalf("failed launch process = %d, want zero", process)
			}
			if !errors.Is(err, injected) {
				t.Fatalf("launch error = %v, want injected error", err)
			}
			if failureCalls != 1 {
				t.Fatalf("failure routes = %d, want 1", failureCalls)
			}
			wantCalls := append([]windowsLaunchStage(nil), stages[:int(failedStage)+1]...)
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("stage calls = %v, want %v", calls, wantCalls)
			}
		})
	}
}

func TestWindowsLaunchProtocolCompletesInStrictOrder(t *testing.T) {
	t.Parallel()

	var calls []windowsLaunchStage
	process, err := runWindowsLaunchProtocol(windowsLaunchProtocolOps[int]{
		assignJob: func() error {
			calls = append(calls, windowsLaunchStageAssignJob)
			return nil
		},
		closeCreationTokens: func() error {
			calls = append(calls, windowsLaunchStageCloseCreationTokens)
			return nil
		},
		closeInheritedPipes: func() error {
			calls = append(calls, windowsLaunchStageCloseInheritedPipes)
			return nil
		},
		findProcess: func() (int, error) {
			calls = append(calls, windowsLaunchStageFindProcess)
			return 73, nil
		},
		resumeThread: func() error {
			calls = append(calls, windowsLaunchStageResumeThread)
			return nil
		},
		closeLaunchHandles: func() error {
			calls = append(calls, windowsLaunchStageCloseLaunchHandles)
			return nil
		},
		onFailure: func(windowsLaunchFailure[int]) error {
			t.Fatal("successful launch called the failure handler")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("complete launch protocol: %v", err)
	}
	if process != 73 {
		t.Fatalf("process = %d, want 73", process)
	}
	if want := allWindowsLaunchStages(); !reflect.DeepEqual(calls, want) {
		t.Fatalf("stage calls = %v, want %v", calls, want)
	}
}

func TestWindowsLaunchProtocolRejectsIncompleteInstanceOpsBeforeSideEffects(t *testing.T) {
	t.Parallel()

	baseCalls := 0
	base := windowsLaunchProtocolOps[int]{
		assignJob:           func() error { baseCalls++; return nil },
		closeCreationTokens: func() error { baseCalls++; return nil },
		closeInheritedPipes: func() error { baseCalls++; return nil },
		findProcess:         func() (int, error) { baseCalls++; return 73, nil },
		resumeThread:        func() error { baseCalls++; return nil },
		closeLaunchHandles:  func() error { baseCalls++; return nil },
		onFailure:           func(windowsLaunchFailure[int]) error { baseCalls++; return nil },
	}
	tests := []struct {
		name   string
		remove func(*windowsLaunchProtocolOps[int])
	}{
		{name: "assign job", remove: func(ops *windowsLaunchProtocolOps[int]) { ops.assignJob = nil }},
		{name: "close creation tokens", remove: func(ops *windowsLaunchProtocolOps[int]) { ops.closeCreationTokens = nil }},
		{name: "close inherited pipes", remove: func(ops *windowsLaunchProtocolOps[int]) { ops.closeInheritedPipes = nil }},
		{name: "find process", remove: func(ops *windowsLaunchProtocolOps[int]) { ops.findProcess = nil }},
		{name: "resume thread", remove: func(ops *windowsLaunchProtocolOps[int]) { ops.resumeThread = nil }},
		{name: "close launch handles", remove: func(ops *windowsLaunchProtocolOps[int]) { ops.closeLaunchHandles = nil }},
		{name: "failure handler", remove: func(ops *windowsLaunchProtocolOps[int]) { ops.onFailure = nil }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			ops := base
			test.remove(&ops)
			if _, err := runWindowsLaunchProtocol(ops); err == nil {
				t.Fatal("incomplete Windows launch protocol was accepted")
			}
		})
	}
	if baseCalls != 0 {
		t.Fatalf("incomplete protocol side-effect calls = %d, want 0", baseCalls)
	}
}

func TestWindowsLaunchFailureOwnershipClosesEveryResourceExactlyOnce(t *testing.T) {
	t.Parallel()

	for _, failedStage := range allWindowsLaunchStages() {
		failedStage := failedStage
		t.Run(failedStage.String(), func(t *testing.T) {
			t.Parallel()
			harness := newWindowsLaunchOwnershipHarness(t)
			injected := errors.New("injected Windows launch failure")
			quarantine := &windowsProcessQuarantine{}

			_, err := runWindowsLaunchProtocol(harness.protocolOps(failedStage, injected, quarantine, true))
			if !errors.Is(err, injected) {
				t.Fatalf("launch error = %v, want injected error", err)
			}
			if got := quarantine.count(); got != 0 {
				t.Fatalf("quarantine count = %d, want synchronous reclaim", got)
			}
			harness.assertReleasedExactlyOnce(t, failedStage)
		})
	}
}

func TestWindowsLaunchFailureOwnershipTransfersOnceAndCloseCanRetry(t *testing.T) {
	t.Parallel()

	for _, failedStage := range allWindowsLaunchStages() {
		failedStage := failedStage
		t.Run(failedStage.String(), func(t *testing.T) {
			t.Parallel()
			harness := newWindowsLaunchOwnershipHarness(t)
			injected := errors.New("injected Windows launch failure")
			quarantine := &windowsProcessQuarantine{}

			_, err := runWindowsLaunchProtocol(harness.protocolOps(failedStage, injected, quarantine, false))
			if !errors.Is(err, injected) || !errors.Is(err, errWindowsProcessLifecycleUnconfirmed) {
				t.Fatalf("retained launch error = %v, want injected and lifecycle errors", err)
			}
			if got := quarantine.count(); got != 1 {
				t.Fatalf("quarantine count after transfer = %d, want 1", got)
			}

			confirmed, firstCloseErr := quarantine.reclaim(1, time.Millisecond)
			if confirmed || !errors.Is(firstCloseErr, errWindowsProcessLifecycleUnconfirmed) {
				t.Fatalf("first Close model = confirmed:%t err:%v", confirmed, firstCloseErr)
			}
			if got := quarantine.count(); got != 1 {
				t.Fatalf("quarantine count after failed Close model = %d, want 1", got)
			}

			confirmed, secondCloseErr := quarantine.reclaim(1, time.Millisecond)
			if !confirmed || secondCloseErr != nil {
				t.Fatalf("retried Close model = confirmed:%t err:%v", confirmed, secondCloseErr)
			}
			if got := quarantine.count(); got != 0 {
				t.Fatalf("quarantine count after retry = %d, want 0", got)
			}
			harness.assertReleasedExactlyOnce(t, failedStage)
		})
	}
}

func TestWindowsUnconfirmedExecutionPathsRetainTheCompleteLifecycle(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"normal-wait", "timeout-cancel"} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			pathErr := errors.New(path + " lifecycle is unconfirmed")
			quarantine := &windowsProcessQuarantine{}
			reclaimCalls := 0
			lifecycle := newWindowsRetainedLifecycle(func(time.Duration) (bool, error) {
				reclaimCalls++
				return true, nil
			})

			err := retainWindowsExecutionLifecycle(
				quarantine,
				lifecycle.retain,
				false,
				pathErr,
			)
			for _, want := range []error{
				pathErr,
				errWindowsProcessLifecycleUnconfirmed,
				errWindowsProcessContainmentUnconfirmed,
			} {
				if !errors.Is(err, want) {
					t.Fatalf("retention error = %v, want %v", err, want)
				}
			}
			if got := quarantine.count(); got != 1 {
				t.Fatalf("quarantine count = %d, want 1", got)
			}
			if reclaimCalls != 0 {
				t.Fatalf("reclaim calls before Close model = %d, want 0", reclaimCalls)
			}

			confirmed, closeErr := quarantine.reclaim(1, time.Millisecond)
			if !confirmed || closeErr != nil {
				t.Fatalf("Close model = confirmed:%t err:%v", confirmed, closeErr)
			}
			if reclaimCalls != 1 || quarantine.count() != 0 {
				t.Fatalf("reclaim calls/count = %d/%d, want 1/0", reclaimCalls, quarantine.count())
			}
		})
	}
}

func failWindowsLaunchStage(failed, current windowsLaunchStage, err error) error {
	if failed == current {
		return err
	}
	return nil
}

func allWindowsLaunchStages() []windowsLaunchStage {
	return []windowsLaunchStage{
		windowsLaunchStageAssignJob,
		windowsLaunchStageCloseCreationTokens,
		windowsLaunchStageCloseInheritedPipes,
		windowsLaunchStageFindProcess,
		windowsLaunchStageResumeThread,
		windowsLaunchStageCloseLaunchHandles,
	}
}

type windowsLaunchOwnershipHarness struct {
	resources      windowsLaunchOwnedResources[int, int]
	releaseCalls   map[int]int
	settleCalls    int
	processRelease int
}

func newWindowsLaunchOwnershipHarness(t *testing.T) *windowsLaunchOwnershipHarness {
	t.Helper()
	return &windowsLaunchOwnershipHarness{
		resources: windowsLaunchOwnedResources[int, int]{
			token:        newOwnedWindowsResource(1),
			lowBoxToken:  newOwnedWindowsResource(2),
			thread:       newOwnedWindowsResource(3),
			process:      newOwnedWindowsResource(4),
			stdinReader:  newOwnedWindowsResource(5),
			stdoutReader: newOwnedWindowsResource(6),
			stdoutWriter: newOwnedWindowsResource(7),
			stderrReader: newOwnedWindowsResource(8),
			stderrWriter: newOwnedWindowsResource(9),
			job:          newOwnedWindowsResource(10),
		},
		releaseCalls: make(map[int]int),
	}
}

func (h *windowsLaunchOwnershipHarness) protocolOps(
	failedStage windowsLaunchStage,
	injected error,
	quarantine *windowsProcessQuarantine,
	synchronous bool,
) windowsLaunchProtocolOps[int] {
	release := func(resource int) error {
		h.releaseCalls[resource]++
		return nil
	}
	closeTokens := func() error {
		if err := failWindowsLaunchStage(failedStage, windowsLaunchStageCloseCreationTokens, injected); err != nil {
			return err
		}
		return releaseOwnedWindowsResources(release, h.resources.lowBoxToken, h.resources.token)
	}
	closePipes := func() error {
		if err := failWindowsLaunchStage(failedStage, windowsLaunchStageCloseInheritedPipes, injected); err != nil {
			return err
		}
		return releaseOwnedWindowsResources(
			release,
			h.resources.stdinReader,
			h.resources.stdoutWriter,
			h.resources.stderrWriter,
		)
	}
	return windowsLaunchProtocolOps[int]{
		assignJob: func() error {
			return failWindowsLaunchStage(failedStage, windowsLaunchStageAssignJob, injected)
		},
		closeCreationTokens: closeTokens,
		closeInheritedPipes: closePipes,
		findProcess: func() (int, error) {
			if err := failWindowsLaunchStage(failedStage, windowsLaunchStageFindProcess, injected); err != nil {
				return 0, err
			}
			return 11, nil
		},
		resumeThread: func() error {
			return failWindowsLaunchStage(failedStage, windowsLaunchStageResumeThread, injected)
		},
		closeLaunchHandles: func() error {
			if err := failWindowsLaunchStage(failedStage, windowsLaunchStageCloseLaunchHandles, injected); err != nil {
				return err
			}
			return releaseOwnedWindowsResources(release, h.resources.thread, h.resources.process)
		},
		onFailure: func(failure windowsLaunchFailure[int]) error {
			settleSuccessAt := 1
			if !synchronous {
				settleSuccessAt = 3
			}
			ownership := &windowsLaunchOwnership[int, int, int]{
				assignedToJob: failure.assignedToJob,
				process:       newOwnedWindowsResource(failure.process),
				resources:     h.resources,
				settle: func(bool, *ownedWindowsResource[int], *ownedWindowsResource[int], time.Duration) (bool, error) {
					h.settleCalls++
					return h.settleCalls >= settleSuccessAt, nil
				},
				releaseProcess: func(process int) error {
					if process != 0 {
						h.processRelease++
					}
					return nil
				},
				closeToken:  release,
				closeHandle: release,
			}
			lifecycle := newWindowsRetainedLifecycle(ownership.reclaim)
			return settleOrRetainWindowsLaunchFailure(
				quarantine,
				lifecycle,
				ownership.processContainmentConfirmed,
				nil,
				time.Millisecond,
			)
		},
	}
}

func (h *windowsLaunchOwnershipHarness) assertReleasedExactlyOnce(
	t *testing.T,
	failedStage windowsLaunchStage,
) {
	t.Helper()
	for resource := 1; resource <= 10; resource++ {
		if got := h.releaseCalls[resource]; got != 1 {
			t.Errorf("resource %d release calls = %d, want 1", resource, got)
		}
	}
	wantProcessRelease := 0
	if failedStage >= windowsLaunchStageResumeThread {
		wantProcessRelease = 1
	}
	if h.processRelease != wantProcessRelease {
		t.Errorf("process object release calls = %d, want %d", h.processRelease, wantProcessRelease)
	}
}
