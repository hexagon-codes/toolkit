//go:build !windows

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const (
	posixOutputDrainLimit        = time.Second
	posixProcessGroupWaitLimit   = 500 * time.Millisecond
	maxPOSIXConcurrentExecutions = 32
)

func checkPOSIXPreparationContext(ctx context.Context, action string) error {
	if err := validateExecContext(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("sandbox: %s canceled: %w", action, err)
	}
	return nil
}

// posixExecutionOptions 固定单次执行在启动边界前使用的全部策略。
type posixExecutionOptions struct {
	sysProcAttr        *syscall.SysProcAttr
	applyResourceLimit bool
	preflight          func(context.Context) error
	beforeStart        func()
	waitLimit          time.Duration
	outputDrainLimit   time.Duration
}

type posixProcessIdentity struct {
	pid       int
	startSec  int64
	startUsec int64
}

type posixProcessGroupInspector func(int) ([]posixProcessIdentity, error)

// posixExecutionRegistry 对单个沙箱仍在内核中等待回收的根进程保留所有权。
// 并发入口有固定上界，因此同时失败时保留表也不会无界增长。
type posixExecutionRegistry struct {
	mu            sync.Mutex
	closing       bool
	running       int
	retained      []*posixRetainedExecution
	settlementErr error
}

type posixExecutionTicket struct {
	registry *posixExecutionRegistry
	done     bool
}

type posixRetainedExecution struct {
	cmd              *exec.Cmd
	waitDone         <-chan error
	killProcessGroup func(int) error
}

func (registry *posixExecutionRegistry) begin() (*posixExecutionTicket, error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.collectCompletedLocked()
	if registry.closing {
		return nil, ErrSandboxClosed
	}
	if len(registry.retained) > 0 {
		return nil, ErrPOSIXExecutionUnsettled
	}
	if registry.running >= maxPOSIXConcurrentExecutions {
		return nil, ErrPOSIXExecutionCapacity
	}
	registry.running++
	return &posixExecutionTicket{registry: registry}, nil
}

func (registry *posixExecutionRegistry) ensureReady() error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.collectCompletedLocked()
	if registry.closing {
		return ErrSandboxClosed
	}
	if len(registry.retained) > 0 {
		return ErrPOSIXExecutionUnsettled
	}
	return nil
}

func (ticket *posixExecutionTicket) release() {
	if ticket == nil || ticket.done {
		return
	}
	ticket.registry.mu.Lock()
	ticket.registry.running--
	ticket.done = true
	ticket.registry.mu.Unlock()
}

func (ticket *posixExecutionTicket) retain(execution *posixRetainedExecution) {
	if ticket == nil || ticket.done || execution == nil {
		return
	}
	ticket.registry.mu.Lock()
	ticket.registry.running--
	ticket.registry.retained = append(ticket.registry.retained, execution)
	ticket.done = true
	ticket.registry.mu.Unlock()
}

func (registry *posixExecutionRegistry) collectCompletedLocked() {
	remaining := registry.retained[:0]
	for _, execution := range registry.retained {
		select {
		case waitErr := <-execution.waitDone:
			if posixWaitConvergenceFailed(waitErr) {
				registry.settlementErr = errors.Join(
					registry.settlementErr,
					fmt.Errorf("sandbox retained process wait failed: %w", waitErr),
				)
			}
		default:
			remaining = append(remaining, execution)
		}
	}
	registry.retained = remaining
}

// Close 在固定边界内重试终止并回收全部保留执行；失败后仍保留所有权供再次调用。
func (registry *posixExecutionRegistry) Close(waitLimit time.Duration) error {
	if waitLimit <= 0 {
		waitLimit = posixTerminationWaitLimit
	}
	registry.mu.Lock()
	registry.closing = true
	registry.collectCompletedLocked()
	if registry.running != 0 {
		err := fmt.Errorf("%w: %d POSIX executions are still active", ErrPOSIXExecutionUnsettled, registry.running)
		registry.mu.Unlock()
		return err
	}
	retained := append([]*posixRetainedExecution(nil), registry.retained...)
	registry.mu.Unlock()

	deadline := time.NewTimer(waitLimit)
	defer deadline.Stop()
	settled := make(map[*posixRetainedExecution]error, len(retained))
	var closeErr error
	for _, execution := range retained {
		closeErr = errors.Join(closeErr, signalPOSIXCommand(execution.cmd, execution.killProcessGroup))
		select {
		case waitErr := <-execution.waitDone:
			settled[execution] = waitErr
		case <-deadline.C:
			closeErr = errors.Join(closeErr, fmt.Errorf("%w after %s", ErrProcessReapTimeout, waitLimit))
			goto update
		}
	}

update:
	registry.mu.Lock()
	remaining := registry.retained[:0]
	for _, execution := range registry.retained {
		waitErr, ok := settled[execution]
		if !ok {
			remaining = append(remaining, execution)
			continue
		}
		if posixWaitConvergenceFailed(waitErr) {
			closeErr = errors.Join(closeErr, fmt.Errorf("sandbox retained process wait failed: %w", waitErr))
		}
	}
	registry.retained = remaining
	closeErr = errors.Join(closeErr, registry.settlementErr)
	registry.settlementErr = nil
	if len(registry.retained) > 0 && !errors.Is(closeErr, ErrProcessReapTimeout) {
		closeErr = errors.Join(closeErr, ErrProcessReapTimeout)
	}
	registry.mu.Unlock()
	return closeErr
}

type posixCopyResult struct {
	stream string
	err    error
}

// posixCommandExecution 独占一次启动后的进程、Wait、标准流 FD 与复制协程。
type posixCommandExecution struct {
	cmd                 *exec.Cmd
	ownsProcessGroup    bool
	processGroupID      int
	processGroupInspect posixProcessGroupInspector

	stdout *boundedBuffer
	stderr *boundedBuffer

	stdoutReader *os.File
	stdoutWriter *os.File
	stderrReader *os.File
	stderrWriter *os.File

	waitDone chan error
	copyDone chan posixCopyResult
}

type posixExecutionSettlement struct {
	waitErr               error
	waitReceived          bool
	reapTimedOut          bool
	outputDrainTimed      bool
	processGroupUnsettled bool
	err                   error
}

func (registry *posixExecutionRegistry) runBoundedCommandWithOptions(
	ctx context.Context,
	command Command,
	cfg Config,
	capabilities posixExecutionCapabilities,
	options posixExecutionOptions,
) (*ExecResult, error) {
	if err := validateExecContext(ctx); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("sandbox exec canceled before preparation: %w", err)
	}
	ticket, err := registry.begin()
	if err != nil {
		return nil, err
	}
	defer ticket.release()

	if storageErr := enforceSandboxStorageLimits(cfg); storageErr != nil {
		return nil, storageErr
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, fmt.Errorf("sandbox exec canceled during preparation: %w", contextErr)
	}

	runCommand := command
	if options.applyResourceLimit {
		runCommand, err = posixResourceLimitedCommandContext(ctx, command, cfg)
		if err != nil {
			return nil, err
		}
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, fmt.Errorf("sandbox exec canceled during preparation: %w", contextErr)
	}

	execution, err := newPOSIXCommandExecution(
		runCommand,
		cfg,
		options.sysProcAttr,
		capabilities.processGroupInspect,
	)
	if err != nil {
		return nil, err
	}
	startDiagnostics, err := execution.start(ctx, options)
	if err != nil {
		return nil, err
	}

	waitLimit := options.waitLimit
	if waitLimit <= 0 {
		waitLimit = posixTerminationWaitLimit
	}
	outputDrainLimit := options.outputDrainLimit
	if outputDrainLimit <= 0 {
		outputDrainLimit = posixOutputDrainLimit
	}

	select {
	case waitErr := <-execution.waitDone:
		settlement := execution.settleAfterRootExit(waitErr, outputDrainLimit, posixProcessGroupWaitLimit)
		result := execution.result(cfg, capabilities, exitCodeFromPOSIXWait(waitErr))
		resultErr := errors.Join(startDiagnostics, settlement.err)
		if posixWaitConvergenceFailed(waitErr) {
			resultErr = errors.Join(resultErr, fmt.Errorf("sandbox exec failed: %w", waitErr))
		}
		if settlement.outputDrainTimed || settlement.processGroupUnsettled || !execution.ownsProcessGroup {
			result.Limits.ProcessContainment = LimitStatusUnsupported
		}
		if storageErr := enforceSandboxStorageLimits(cfg); storageErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("storage limit check failed: %w", storageErr))
		}
		return result, resultErr
	case <-ctx.Done():
		terminationErr := signalPOSIXCommand(execution.cmd, func(pid int) error {
			return syscall.Kill(-pid, syscall.SIGKILL)
		})
		settlement := execution.settleAfterCancellation(waitLimit, outputDrainLimit)
		if !settlement.waitReceived {
			ticket.retain(&posixRetainedExecution{
				cmd:      execution.cmd,
				waitDone: execution.waitDone,
				killProcessGroup: func(pid int) error {
					return syscall.Kill(-pid, syscall.SIGKILL)
				},
			})
		}
		result := execution.result(cfg, capabilities, -1)
		resultErr := errors.Join(
			fmt.Errorf("sandbox exec terminated by timeout/cancel: %w", ctx.Err()),
			startDiagnostics,
			terminationErr,
			settlement.err,
		)
		if settlement.waitReceived && settlement.waitErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("sandbox exec wait failed: %w", settlement.waitErr))
		}
		if terminationErr != nil || settlement.reapTimedOut || settlement.outputDrainTimed ||
			settlement.processGroupUnsettled || !execution.ownsProcessGroup {
			result.Limits.ProcessContainment = LimitStatusUnsupported
		}
		if storageErr := enforceSandboxStorageLimits(cfg); storageErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("storage limit check failed: %w", storageErr))
		}
		return result, resultErr
	}
}

func newPOSIXCommandExecution(
	command Command,
	cfg Config,
	sysProcAttr *syscall.SysProcAttr,
	processGroupInspect posixProcessGroupInspector,
) (*posixCommandExecution, error) {
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create sandbox stdout pipe: %w", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("create sandbox stderr pipe: %w", err),
			closePOSIXFile("stdout reader", stdoutReader),
			closePOSIXFile("stdout writer", stdoutWriter),
		)
	}

	// 动态程序执行是沙箱 API 的核心职责，参数直接作为 argv 交给内核。
	cmd := exec.Command(command.Path, command.Args...) //nolint:noctx // #nosec G204
	cmd.Dir = command.Dir
	cmd.Env = command.Env
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter
	cmd.SysProcAttr = sysProcAttr
	ownsProcessGroup := sysProcAttr != nil && (sysProcAttr.Setsid || sysProcAttr.Setpgid && sysProcAttr.Pgid == 0)
	return &posixCommandExecution{
		cmd:                 cmd,
		ownsProcessGroup:    ownsProcessGroup,
		processGroupInspect: processGroupInspect,
		stdout:              newBoundedBuffer(cfg.MaxOutputBytes),
		stderr:              newBoundedBuffer(cfg.MaxStderrBytes),
		stdoutReader:        stdoutReader,
		stdoutWriter:        stdoutWriter,
		stderrReader:        stderrReader,
		stderrWriter:        stderrWriter,
		waitDone:            make(chan error, 1),
		copyDone:            make(chan posixCopyResult, 2),
	}, nil
}

func (execution *posixCommandExecution) start(
	ctx context.Context,
	options posixExecutionOptions,
) (diagnostics, startErr error) {
	cleanupUnstarted := func(primary error) (error, error) {
		return nil, errors.Join(
			primary,
			closePOSIXFile("stdout reader", execution.stdoutReader),
			closePOSIXFile("stdout writer", execution.stdoutWriter),
			closePOSIXFile("stderr reader", execution.stderrReader),
			closePOSIXFile("stderr writer", execution.stderrWriter),
		)
	}
	if err := ctx.Err(); err != nil {
		return cleanupUnstarted(fmt.Errorf("sandbox exec canceled before preflight: %w", err))
	}
	if options.preflight != nil {
		if err := options.preflight(ctx); err != nil {
			return cleanupUnstarted(fmt.Errorf("sandbox exec preflight failed: %w", err))
		}
	}
	if options.beforeStart != nil {
		options.beforeStart()
	}
	// 这是启动系统调用前最后一个可观察取消点；已经取消的请求不得创建载荷。
	if err := ctx.Err(); err != nil {
		return cleanupUnstarted(fmt.Errorf("sandbox exec canceled before start: %w", err))
	}
	if err := execution.cmd.Start(); err != nil {
		return cleanupUnstarted(fmt.Errorf("sandbox exec start failed: %w", err))
	}
	if execution.ownsProcessGroup {
		execution.processGroupID = execution.cmd.Process.Pid
	}

	diagnostics = errors.Join(
		diagnostics,
		closePOSIXFile("parent stdout writer", execution.stdoutWriter),
		closePOSIXFile("parent stderr writer", execution.stderrWriter),
	)
	execution.stdoutWriter = nil
	execution.stderrWriter = nil

	go execution.copyOutput("stdout", execution.stdoutReader, execution.stdout)
	go execution.copyOutput("stderr", execution.stderrReader, execution.stderr)
	go func() {
		execution.waitDone <- execution.cmd.Wait()
	}()
	return diagnostics, nil
}

func (execution *posixCommandExecution) copyOutput(stream string, reader *os.File, buffer *boundedBuffer) {
	_, err := io.Copy(buffer, reader)
	execution.copyDone <- posixCopyResult{stream: stream, err: err}
}

func (execution *posixCommandExecution) settleAfterRootExit(
	waitErr error,
	drainLimit time.Duration,
	processGroupLimit time.Duration,
) posixExecutionSettlement {
	settlement := posixExecutionSettlement{waitErr: waitErr, waitReceived: true}
	groupUnsettled, groupErr := execution.settleProcessGroup(processGroupLimit, true)
	settlement.processGroupUnsettled = groupUnsettled
	timedOut, drainErr := execution.drainOutput(drainLimit)
	settlement.outputDrainTimed = timedOut
	settlement.err = errors.Join(groupErr, drainErr)
	return settlement
}

func (execution *posixCommandExecution) settleAfterCancellation(waitLimit, drainLimit time.Duration) posixExecutionSettlement {
	if waitLimit <= 0 {
		waitLimit = posixTerminationWaitLimit
	}
	if drainLimit <= 0 {
		drainLimit = posixOutputDrainLimit
	}
	waitTimer := time.NewTimer(waitLimit)
	drainTimer := time.NewTimer(drainLimit)
	defer waitTimer.Stop()
	defer drainTimer.Stop()

	settlement := posixExecutionSettlement{}
	copies := 0
	forcedDrain := false
	waitExpired := false
	var waitChannel <-chan error = execution.waitDone
	drainChannel := drainTimer.C
	for copies < 2 || !settlement.waitReceived && !waitExpired {
		select {
		case waitErr := <-waitChannel:
			settlement.waitErr = waitErr
			settlement.waitReceived = true
			waitChannel = nil
		case copyResult := <-execution.copyDone:
			copies++
			settlement.err = errors.Join(settlement.err, normalizePOSIXCopyError(copyResult, forcedDrain))
			if copies == 2 {
				drainChannel = nil
				if !drainTimer.Stop() {
					select {
					case <-drainTimer.C:
					default:
					}
				}
			}
		case <-drainChannel:
			forcedDrain = true
			settlement.outputDrainTimed = true
			settlement.err = errors.Join(settlement.err, fmt.Errorf("%w after %s", ErrOutputDrainTimeout, drainLimit))
			settlement.err = errors.Join(settlement.err, execution.closeOutputReaders())
			drainChannel = nil
		case <-waitTimer.C:
			waitExpired = true
			settlement.reapTimedOut = true
			settlement.err = errors.Join(settlement.err, fmt.Errorf("%w after %s", ErrProcessReapTimeout, waitLimit))
		}
	}
	// Wait 可能恰好在定时器触发后、复制协程退出前完成；消费它可避免不必要保留。
	if !settlement.waitReceived {
		select {
		case settlement.waitErr = <-execution.waitDone:
			settlement.waitReceived = true
		default:
		}
	}
	settlement.err = errors.Join(settlement.err, execution.closeOutputReaders())
	if settlement.waitReceived {
		groupUnsettled, groupErr := execution.settleProcessGroup(posixProcessGroupWaitLimit, false)
		settlement.processGroupUnsettled = groupUnsettled
		settlement.err = errors.Join(settlement.err, groupErr)
	}
	return settlement
}

func (execution *posixCommandExecution) settleProcessGroup(limit time.Duration, reportSurvivor bool) (bool, error) {
	if !execution.ownsProcessGroup || execution.processGroupID <= 0 {
		return true, nil
	}
	if limit <= 0 {
		limit = posixProcessGroupWaitLimit
	}
	var originalMembers []posixProcessIdentity
	if execution.processGroupInspect != nil {
		var inspectErr error
		originalMembers, inspectErr = execution.processGroupInspect(execution.processGroupID)
		if inspectErr != nil {
			return true, fmt.Errorf("%w: inspect failed: %w", ErrProcessGroupSettlement, inspectErr)
		}
		if len(originalMembers) == 0 {
			return false, nil
		}
	} else {
		probeErr := syscall.Kill(-execution.processGroupID, 0)
		if errors.Is(probeErr, syscall.ESRCH) {
			return false, nil
		}
		if probeErr != nil {
			return true, fmt.Errorf("%w: probe failed: %w", ErrProcessGroupSettlement, probeErr)
		}
	}

	var result error
	if reportSurvivor {
		result = ErrProcessGroupSurvivedRoot
	}
	if killErr := syscall.Kill(-execution.processGroupID, syscall.SIGKILL); killErr != nil &&
		!posixTerminationAlreadyComplete(killErr) {
		return true, errors.Join(result, fmt.Errorf("%w: kill failed: %w", ErrProcessGroupSettlement, killErr))
	}
	if execution.processGroupInspect == nil {
		// 非 macOS 后端未提供稳定进程身份枚举；SIGKILL 成功只证明信号已经投递。
		return false, result
	}

	timer := time.NewTimer(limit)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		currentMembers, inspectErr := execution.processGroupInspect(execution.processGroupID)
		if inspectErr != nil {
			return true, errors.Join(result, fmt.Errorf("%w: inspect failed: %w", ErrProcessGroupSettlement, inspectErr))
		}
		if !posixProcessIdentitiesRemain(originalMembers, currentMembers) {
			return false, result
		}
		select {
		case <-ticker.C:
		case <-timer.C:
			return true, errors.Join(result, fmt.Errorf("%w after %s", ErrProcessGroupSettlement, limit))
		}
	}
}

func posixProcessIdentitiesRemain(original, current []posixProcessIdentity) bool {
	currentSet := make(map[posixProcessIdentity]struct{}, len(current))
	for _, identity := range current {
		currentSet[identity] = struct{}{}
	}
	for _, identity := range original {
		if _, exists := currentSet[identity]; exists {
			return true
		}
	}
	return false
}

func (execution *posixCommandExecution) drainOutput(limit time.Duration) (bool, error) {
	if limit <= 0 {
		limit = posixOutputDrainLimit
	}
	timer := time.NewTimer(limit)
	defer timer.Stop()
	copies := 0
	forced := false
	var resultErr error
	for copies < 2 {
		select {
		case copyResult := <-execution.copyDone:
			copies++
			resultErr = errors.Join(resultErr, normalizePOSIXCopyError(copyResult, forced))
		case <-timer.C:
			forced = true
			resultErr = errors.Join(resultErr, fmt.Errorf("%w after %s", ErrOutputDrainTimeout, limit))
			resultErr = errors.Join(resultErr, execution.closeOutputReaders())
			for copies < 2 {
				copyResult := <-execution.copyDone
				copies++
				resultErr = errors.Join(resultErr, normalizePOSIXCopyError(copyResult, true))
			}
		}
	}
	resultErr = errors.Join(resultErr, execution.closeOutputReaders())
	return forced, resultErr
}

func normalizePOSIXCopyError(result posixCopyResult, forced bool) error {
	if result.err == nil || forced && errors.Is(result.err, os.ErrClosed) {
		return nil
	}
	return fmt.Errorf("read sandbox %s: %w", result.stream, result.err)
}

func (execution *posixCommandExecution) closeOutputReaders() error {
	return errors.Join(
		closePOSIXFile("stdout reader", execution.stdoutReader),
		closePOSIXFile("stderr reader", execution.stderrReader),
	)
}

func closePOSIXFile(label string, file *os.File) error {
	if file == nil {
		return nil
	}
	if err := file.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		return fmt.Errorf("close sandbox %s: %w", label, err)
	}
	return nil
}

func (execution *posixCommandExecution) result(cfg Config, capabilities posixExecutionCapabilities, exitCode int) *ExecResult {
	stdout := execution.stdout.Snapshot()
	stderr := execution.stderr.Snapshot()
	return &ExecResult{
		Stdout:          stdout.Text,
		Stderr:          stderr.Text,
		ExitCode:        exitCode,
		StdoutBytes:     stdout.BytesSeen,
		StderrBytes:     stderr.BytesSeen,
		StdoutTruncated: stdout.Truncated,
		StderrTruncated: stderr.Truncated,
		Limits:          posixLimitReport(cfg, capabilities),
	}
}

func exitCodeFromPOSIXWait(waitErr error) int {
	if waitErr == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// signalPOSIXCommand 总是先尝试进程组，再直接终止受跟踪根进程。
func signalPOSIXCommand(cmd *exec.Cmd, killProcessGroup func(int) error) error {
	if cmd == nil || cmd.Process == nil {
		return fmt.Errorf("sandbox process is unavailable for termination")
	}
	var result error
	if killProcessGroup != nil {
		if err := killProcessGroup(cmd.Process.Pid); err != nil && !posixTerminationAlreadyComplete(err) {
			result = errors.Join(result, fmt.Errorf("kill process group failed: %w", err))
		}
	}
	// 进程组操作成功也不能替代根进程句柄上的直接终止。
	if err := cmd.Process.Kill(); err != nil && !posixTerminationAlreadyComplete(err) {
		result = errors.Join(result, fmt.Errorf("kill sandbox root process failed: %w", err))
	}
	return result
}

func posixTerminationAlreadyComplete(err error) bool {
	return errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH)
}
