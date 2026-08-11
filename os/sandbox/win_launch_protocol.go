package sandbox

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// 本文件刻意不绑定 Win32 类型，使 Windows 启动协议可在非 Windows 主机执行普通与 race 测试。

type windowsLaunchStage uint8

const (
	windowsLaunchStageAssignJob windowsLaunchStage = iota
	windowsLaunchStageCloseCreationTokens
	windowsLaunchStageCloseInheritedPipes
	windowsLaunchStageFindProcess
	windowsLaunchStageResumeThread
	windowsLaunchStageCloseLaunchHandles
)

func (stage windowsLaunchStage) String() string {
	switch stage {
	case windowsLaunchStageAssignJob:
		return "assign-job"
	case windowsLaunchStageCloseCreationTokens:
		return "close-creation-tokens"
	case windowsLaunchStageCloseInheritedPipes:
		return "close-inherited-pipes"
	case windowsLaunchStageFindProcess:
		return "find-process"
	case windowsLaunchStageResumeThread:
		return "resume-thread"
	case windowsLaunchStageCloseLaunchHandles:
		return "close-launch-handles"
	default:
		return fmt.Sprintf("windows-launch-stage-%d", uint8(stage))
	}
}

type windowsLaunchFailure[Process any] struct {
	stage         windowsLaunchStage
	assignedToJob bool
	process       Process
	err           error
}

// windowsLaunchProtocolOps 固定 CreateProcess 成功后的六阶段单向协议。
// 每次启动持有自己的函数集合，测试和生产均不依赖可变全局 hook。
type windowsLaunchProtocolOps[Process any] struct {
	assignJob           func() error
	closeCreationTokens func() error
	closeInheritedPipes func() error
	findProcess         func() (Process, error)
	resumeThread        func() error
	closeLaunchHandles  func() error
	onFailure           func(windowsLaunchFailure[Process]) error
}

func (ops windowsLaunchProtocolOps[Process]) validate() error {
	missing := ""
	switch {
	case ops.assignJob == nil:
		missing = "assign job"
	case ops.closeCreationTokens == nil:
		missing = "close creation tokens"
	case ops.closeInheritedPipes == nil:
		missing = "close inherited pipes"
	case ops.findProcess == nil:
		missing = "find process"
	case ops.resumeThread == nil:
		missing = "resume thread"
	case ops.closeLaunchHandles == nil:
		missing = "close launch handles"
	case ops.onFailure == nil:
		missing = "failure handler"
	}
	if missing != "" {
		return fmt.Errorf("windows launch protocol %s operation is required", missing)
	}
	return nil
}

func runWindowsLaunchProtocol[Process any](ops windowsLaunchProtocolOps[Process]) (Process, error) {
	var zero Process
	if err := ops.validate(); err != nil {
		return zero, err
	}

	assignedToJob := false
	process := zero
	fail := func(stage windowsLaunchStage, err error) (Process, error) {
		failure := windowsLaunchFailure[Process]{
			stage:         stage,
			assignedToJob: assignedToJob,
			process:       process,
			err:           err,
		}
		return zero, errors.Join(err, ops.onFailure(failure))
	}

	if err := ops.assignJob(); err != nil {
		return fail(windowsLaunchStageAssignJob, err)
	}
	assignedToJob = true
	if err := ops.closeCreationTokens(); err != nil {
		return fail(windowsLaunchStageCloseCreationTokens, err)
	}
	if err := ops.closeInheritedPipes(); err != nil {
		return fail(windowsLaunchStageCloseInheritedPipes, err)
	}
	openedProcess, err := ops.findProcess()
	if err != nil {
		return fail(windowsLaunchStageFindProcess, err)
	}
	process = openedProcess
	if err := ops.resumeThread(); err != nil {
		return fail(windowsLaunchStageResumeThread, err)
	}
	if err := ops.closeLaunchHandles(); err != nil {
		return fail(windowsLaunchStageCloseLaunchHandles, err)
	}
	return process, nil
}

type windowsLaunchOwnedResources[Token, Handle comparable] struct {
	token        *ownedWindowsResource[Token]
	lowBoxToken  *ownedWindowsResource[Token]
	thread       *ownedWindowsResource[Handle]
	process      *ownedWindowsResource[Handle]
	stdinReader  *ownedWindowsResource[Handle]
	stdoutReader *ownedWindowsResource[Handle]
	stdoutWriter *ownedWindowsResource[Handle]
	stderrReader *ownedWindowsResource[Handle]
	stderrWriter *ownedWindowsResource[Handle]
	job          *ownedWindowsResource[Handle]
}

// windowsLaunchOwnership 统一持有失败启动的进程对象、令牌和句柄。
// 只有进程生命周期确认且所有资源成功释放后，reclaim 才报告成功。
type windowsLaunchOwnership[Token, Handle, Process comparable] struct {
	mu               sync.Mutex
	assignedToJob    bool
	processConfirmed bool
	process          *ownedWindowsResource[Process]
	resources        windowsLaunchOwnedResources[Token, Handle]
	settle           func(bool, *ownedWindowsResource[Handle], *ownedWindowsResource[Handle], time.Duration) (bool, error)
	releaseProcess   func(Process) error
	closeToken       func(Token) error
	closeHandle      func(Handle) error
}

func (ownership *windowsLaunchOwnership[Token, Handle, Process]) reclaim(
	timeout time.Duration,
) (bool, error) {
	if ownership == nil {
		return false, fmt.Errorf("windows launch ownership is required")
	}
	ownership.mu.Lock()
	defer ownership.mu.Unlock()
	if ownership.settle == nil || ownership.releaseProcess == nil ||
		ownership.closeToken == nil || ownership.closeHandle == nil {
		return false, fmt.Errorf("windows launch ownership operations are incomplete")
	}

	var result error
	if !ownership.processConfirmed {
		confirmed, err := ownership.settle(
			ownership.assignedToJob,
			ownership.resources.process,
			ownership.resources.job,
			timeout,
		)
		result = errors.Join(result, err)
		if !confirmed {
			return false, result
		}
		ownership.processConfirmed = true
	}

	result = errors.Join(
		result,
		ownership.process.release(ownership.releaseProcess),
		releaseOwnedWindowsResources(
			ownership.closeToken,
			ownership.resources.lowBoxToken,
			ownership.resources.token,
		),
		releaseOwnedWindowsResources(
			ownership.closeHandle,
			ownership.resources.thread,
			ownership.resources.process,
			ownership.resources.stdinReader,
			ownership.resources.stdoutReader,
			ownership.resources.stdoutWriter,
			ownership.resources.stderrReader,
			ownership.resources.stderrWriter,
			ownership.resources.job,
		),
	)
	return ownership.released(), result
}

func (ownership *windowsLaunchOwnership[Token, Handle, Process]) released() bool {
	var zeroProcess Process
	var zeroToken Token
	return ownership.process.value() == zeroProcess &&
		ownership.resources.token.value() == zeroToken &&
		ownership.resources.lowBoxToken.value() == zeroToken &&
		windowsLaunchResourcesReleased(
			ownership.resources.thread,
			ownership.resources.process,
			ownership.resources.stdinReader,
			ownership.resources.stdoutReader,
			ownership.resources.stdoutWriter,
			ownership.resources.stderrReader,
			ownership.resources.stderrWriter,
			ownership.resources.job,
		)
}

func windowsLaunchResourcesReleased[Resource comparable](
	resources ...*ownedWindowsResource[Resource],
) bool {
	var zero Resource
	for _, resource := range resources {
		if resource.value() != zero {
			return false
		}
	}
	return true
}

func (ownership *windowsLaunchOwnership[Token, Handle, Process]) processContainmentConfirmed() bool {
	if ownership == nil {
		return false
	}
	ownership.mu.Lock()
	defer ownership.mu.Unlock()
	return ownership.processConfirmed
}

func settleOrRetainWindowsLaunchFailure(
	quarantine *windowsProcessQuarantine,
	lifecycle *windowsRetainedLifecycle,
	processContainmentConfirmed func() bool,
	launchErr error,
	timeout time.Duration,
) error {
	if lifecycle == nil {
		return errors.Join(launchErr, fmt.Errorf("windows launch lifecycle is required"))
	}
	reclaimed, reclaimErr := lifecycle.reclaim(timeout)
	if reclaimed {
		return errors.Join(launchErr, reclaimErr)
	}
	result := errors.Join(
		launchErr,
		reclaimErr,
		lifecycle.retain(quarantine),
		errWindowsProcessLifecycleUnconfirmed,
	)
	if processContainmentConfirmed == nil || !processContainmentConfirmed() {
		result = errors.Join(result, errWindowsProcessContainmentUnconfirmed)
	}
	return result
}

func retainWindowsExecutionLifecycle(
	quarantine *windowsProcessQuarantine,
	retain func(*windowsProcessQuarantine) error,
	processContainmentConfirmed bool,
	executionErr error,
) error {
	if retain == nil {
		return errors.Join(executionErr, fmt.Errorf("windows execution retention operation is required"))
	}
	result := errors.Join(
		executionErr,
		retain(quarantine),
		errWindowsProcessLifecycleUnconfirmed,
	)
	if !processContainmentConfirmed {
		result = errors.Join(result, errWindowsProcessContainmentUnconfirmed)
	}
	return result
}
