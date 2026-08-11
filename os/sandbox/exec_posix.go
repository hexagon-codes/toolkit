//go:build !windows

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"
)

const posixTerminationWaitLimit = 2 * time.Second

const posixRlimitShellScript = `set -eu
memory_kib=$1
environment_launcher=$2
shift 2
if [ "$memory_kib" -gt 0 ]; then
  ulimit -v "$memory_kib"
fi
exec "$environment_launcher" -i -- "$@"
`

// posixExecutionCapabilities 由具体平台后端声明本次执行能够证明的隔离能力。
// 使用具名字段避免把文件系统隔离与进程树回收两个同类型状态传反。
type posixExecutionCapabilities struct {
	Filesystem          LimitStatus
	Network             LimitStatus
	Memory              LimitStatus
	Processes           LimitStatus
	ProcessContainment  LimitStatus
	processGroupInspect posixProcessGroupInspector
}

// runBoundedCommand 执行受限命令。capabilities 由平台后端基于实际隔离机制判定，
// 原样写入 ExecResult.Limits，不能从通用 POSIX 进程组行为推断全树终止能力。
func runBoundedCommand(ctx context.Context, command Command, cfg Config, capabilities posixExecutionCapabilities) (*ExecResult, error) {
	var executions posixExecutionRegistry
	return executions.runBoundedCommand(ctx, command, cfg, capabilities)
}

// runBoundedPreparedCommandWithPreflight 在启动隔离载荷前执行最后一次对象身份复核。
// 复核失败时不得调用 Start，确保载荷没有机会产生副作用。
func runBoundedPreparedCommandWithPreflight(
	ctx context.Context,
	command Command,
	cfg Config,
	capabilities posixExecutionCapabilities,
	preflight func(context.Context) error,
) (*ExecResult, error) {
	var executions posixExecutionRegistry
	return executions.runBoundedPreparedCommandWithPreflight(ctx, command, cfg, capabilities, preflight)
}

func runBoundedCommandWithSysProcAttr(
	ctx context.Context,
	command Command,
	cfg Config,
	capabilities posixExecutionCapabilities,
	sysProcAttr *syscall.SysProcAttr,
) (*ExecResult, error) {
	var executions posixExecutionRegistry
	return executions.runBoundedCommandWithOptions(ctx, command, cfg, capabilities, posixExecutionOptions{
		sysProcAttr:        sysProcAttr,
		applyResourceLimit: true,
	})
}

func (executions *posixExecutionRegistry) runBoundedCommand(
	ctx context.Context,
	command Command,
	cfg Config,
	capabilities posixExecutionCapabilities,
) (*ExecResult, error) {
	return executions.runBoundedCommandWithOptions(ctx, command, cfg, capabilities, posixExecutionOptions{
		sysProcAttr:        &syscall.SysProcAttr{Setpgid: true},
		applyResourceLimit: true,
	})

}

func (executions *posixExecutionRegistry) runBoundedPreparedCommandWithPreflight(
	ctx context.Context,
	command Command,
	cfg Config,
	capabilities posixExecutionCapabilities,
	preflight func(context.Context) error,
) (*ExecResult, error) {
	return executions.runBoundedCommandWithOptions(ctx, command, cfg, capabilities, posixExecutionOptions{
		sysProcAttr: &syscall.SysProcAttr{Setpgid: true},
		preflight:   preflight,
	})
}

// posixTerminationLimitReport 根据本次真实终止证据生成报告和完整诊断链。
// kill 或真正的 Wait 收敛失败意味着后代不存在性无法证明，必须动态降级。
func posixTerminationLimitReport(
	cfg Config,
	capabilities posixExecutionCapabilities,
	waitErr error,
	terminationErr error,
) (LimitReport, error) {
	report := posixLimitReport(cfg, capabilities)
	var convergenceErr error
	if terminationErr != nil {
		convergenceErr = errors.Join(convergenceErr, terminationErr)
	}
	if waitErr != nil {
		convergenceErr = errors.Join(convergenceErr, fmt.Errorf("sandbox exec wait failed: %w", waitErr))
	}
	if terminationErr != nil || posixWaitConvergenceFailed(waitErr) {
		report.ProcessContainment = LimitStatusUnsupported
	}
	return report, convergenceErr
}

func posixWaitConvergenceFailed(waitErr error) bool {
	if waitErr == nil {
		return false
	}
	// 非零退出和信号终止仍由 Wait 返回 ExitError，但已经确认根进程被回收。
	_, confirmedReap := waitErr.(*exec.ExitError)
	return !confirmedReap
}

func terminatePosixCommand(
	cmd *exec.Cmd,
	done <-chan error,
	killProcessGroup func(pid int) error,
	waitLimit time.Duration,
) (waitErr, terminationErr error) {
	terminationErr = signalPOSIXCommand(cmd, killProcessGroup)
	if waitLimit <= 0 {
		waitLimit = posixTerminationWaitLimit
	}
	timer := time.NewTimer(waitLimit)
	defer timer.Stop()
	select {
	case waitErr = <-done:
		return waitErr, terminationErr
	case <-timer.C:
		return fmt.Errorf("%w after %s", ErrProcessReapTimeout, waitLimit), terminationErr
	}
}

func posixResourceLimitedCommand(command Command, cfg Config) (Command, error) {
	return posixResourceLimitedCommandContext(context.Background(), command, cfg)
}

func posixResourceLimitedCommandContext(ctx context.Context, command Command, cfg Config) (Command, error) {
	if cfg.MaxMemoryBytes <= 0 {
		return command, nil
	}
	if runtime.GOOS == "linux" {
		return linuxPrlimitCommand(command, cfg)
	}
	return trustedPOSIXRlimitCommand(ctx, command, cfg)
}

func trustedPOSIXRlimitCommand(ctx context.Context, command Command, cfg Config) (Command, error) {
	if err := checkPOSIXPreparationContext(ctx, "prepare POSIX resource limits"); err != nil {
		return Command{}, err
	}
	shell, err := resolveTrustedPOSIXLauncher("POSIX resource limit shell", []string{"/bin/sh"})
	if err != nil {
		return Command{}, err
	}
	environmentLauncher, err := resolveTrustedPOSIXLauncher("POSIX environment launcher", []string{"/usr/bin/env", "/bin/env"})
	if err != nil {
		return Command{}, err
	}

	memoryKiB := int64(0)
	memoryCapability, _, capabilityErr := posixResourceLimitCapabilitiesContext(ctx, posixExecutionCapabilities{})
	if capabilityErr != nil {
		return Command{}, capabilityErr
	}
	if cfg.MaxMemoryBytes > 0 && memoryCapability == LimitStatusEnforced {
		memoryKiB = (cfg.MaxMemoryBytes + 1023) / 1024
	}
	launcherArgs := make([]string, 0, len(command.Env)+len(command.Args)+8)
	launcherArgs = append(launcherArgs,
		"-c",
		posixRlimitShellScript,
		"sandbox-rlimit-launcher",
		strconv.FormatInt(memoryKiB, 10),
		environmentLauncher,
	)
	launcherArgs = append(launcherArgs, command.Env...)
	launcherArgs = append(launcherArgs, command.Path)
	launcherArgs = append(launcherArgs, command.Args...)

	return Command{
		Path: shell,
		Args: launcherArgs,
		Dir:  command.Dir,
		Env: []string{
			"HOME=/var/empty",
			"LANG=C",
			"LC_ALL=C",
			"PATH=/usr/bin:/bin",
		},
	}, nil
}

func linuxPrlimitCommand(command Command, cfg Config) (Command, error) {
	prlimit, err := linuxPrlimitPath()
	if err != nil {
		return Command{}, fmt.Errorf("resolve POSIX rlimit helper prlimit: %w", err)
	}

	prlimitArgs := make([]string, 0, len(command.Args)+3)
	if cfg.MaxMemoryBytes > 0 {
		prlimitArgs = append(prlimitArgs, fmt.Sprintf("--as=%d", cfg.MaxMemoryBytes))
	}
	prlimitArgs = append(prlimitArgs, "--", command.Path)
	prlimitArgs = append(prlimitArgs, command.Args...)

	prlimitEnv := stripPosixRlimitEnv(command.Env)

	return Command{Path: prlimit, Args: prlimitArgs, Dir: command.Dir, Env: prlimitEnv}, nil
}

// posixMemoryProbeBytes 是内存限额能力探测所用的现实代表值。darwin 内核仅对现实量级
// (≲百 GiB)的下调返回 EINVAL, 对天文数字级
// 的值反而「成功」但无实际约束意义, 故探测值必须落在真实配置会用到的量级。
const posixMemoryProbeBytes = 256 * 1024 * 1024

var (
	posixLimitCapabilityCache = struct {
		sync.Mutex
		ready   bool
		probing chan struct{}
		status  LimitStatus
	}{}
)

// posixLimitReport 报告当前平台各资源限制项的实际执行状态。
//
// Storage 仅有执行前后 walk 检查，不是实时配额，必须报告 unsupported；
// Output 由有界缓冲实时约束，报告 enforced；
// Memory 优先使用后端对本次执行路径的预检结果，未提供时才使用一次性平台探测；
// Filesystem/Network/ProcessContainment 由调用方按实际隔离机制判定后传入；
// Processes 必须由后端显式提供树级总预算证据，禁止用 RLIMIT_NPROC 近似推断。
func posixResourceLimitCapabilities(capabilities posixExecutionCapabilities) (memory, processes LimitStatus) {
	memory, processes, err := posixResourceLimitCapabilitiesContext(context.Background(), capabilities)
	if err != nil && memory == "" {
		memory = LimitStatusUnsupported
	}
	return memory, processes
}

func posixResourceLimitCapabilitiesContext(
	ctx context.Context,
	capabilities posixExecutionCapabilities,
) (memory, processes LimitStatus, err error) {
	memory = capabilities.Memory
	if memory == "" {
		memory, err = cachedPOSIXMemoryLimitCapability(ctx)
		if err != nil {
			return "", "", err
		}
	}
	processes = capabilities.Processes
	if processes == "" {
		processes = LimitStatusUnsupported
	}
	return memory, processes, nil
}

func posixLimitReport(cfg Config, capabilities posixExecutionCapabilities) LimitReport {
	memory, processes := posixResourceLimitCapabilities(capabilities)
	return LimitReport{
		Network:            capabilities.Network,
		Memory:             requestedLimitStatus(cfg.MaxMemoryBytes > 0, memory),
		Processes:          requestedLimitStatus(cfg.MaxProcesses > 0, processes),
		ProcessContainment: capabilities.ProcessContainment,
		Storage: requestedLimitStatus(
			cfg.MaxWorkspaceBytes > 0 || cfg.MaxArtifactBytes > 0,
			LimitStatusUnsupported,
		),
		Output:     LimitStatusEnforced,
		Filesystem: capabilities.Filesystem,
	}
}

// probePosixMemoryLimitCapability 探测当前平台能否在载荷子进程中真实下调内存 rlimit。
//
// linux 内核允许无特权进程下调 AS/DATA/RSS 软限, 编译期即可判定为 enforced,
// 且避免在父进程上做「真下调」探测(哪怕瞬时压低自身内存上限也有风险)。
// 其余 POSIX 平台只在一次性辅助子进程内执行与真实启动器相同的 ulimit，
// 无论成功、失败或恢复异常都不会改变宿主进程限制。
func probePosixMemoryLimitCapability() LimitStatus {
	status, err := probePosixMemoryLimitCapabilityContext(context.Background())
	if err != nil {
		return LimitStatusUnsupported
	}
	return status
}

func probePosixMemoryLimitCapabilityContext(ctx context.Context) (LimitStatus, error) {
	if runtime.GOOS == "linux" {
		return LimitStatusEnforced, nil
	}
	if err := checkPOSIXPreparationContext(ctx, "probe POSIX memory limit"); err != nil {
		return "", err
	}
	shell, err := resolveTrustedPOSIXLauncher("POSIX memory limit probe shell", []string{"/bin/sh"})
	if err != nil {
		return LimitStatusUnsupported, nil
	}
	probeContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	probeKiB := strconv.FormatInt(posixMemoryProbeBytes/1024, 10)
	cmd := exec.CommandContext( // #nosec G204 -- 路径来自受信任固定候选，脚本和参数由本包固定。
		probeContext,
		shell,
		"-c",
		`ulimit -v "$1" >/dev/null 2>&1`,
		"sandbox-memory-limit-probe",
		probeKiB,
	)
	cmd.Env = posixTrustedLauncherEnvironment()
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("probe POSIX memory limit: %w", ctxErr)
		}
		return LimitStatusUnsupported, nil
	}
	return LimitStatusEnforced, nil
}

func cachedPOSIXMemoryLimitCapability(ctx context.Context) (LimitStatus, error) {
	for {
		posixLimitCapabilityCache.Lock()
		if posixLimitCapabilityCache.ready {
			status := posixLimitCapabilityCache.status
			posixLimitCapabilityCache.Unlock()
			return status, nil
		}
		if probing := posixLimitCapabilityCache.probing; probing != nil {
			posixLimitCapabilityCache.Unlock()
			select {
			case <-probing:
				continue
			case <-ctx.Done():
				return "", fmt.Errorf("probe POSIX memory limit: %w", ctx.Err())
			}
		}
		probing := make(chan struct{})
		posixLimitCapabilityCache.probing = probing
		posixLimitCapabilityCache.Unlock()

		status, err := probePosixMemoryLimitCapabilityContext(ctx)
		posixLimitCapabilityCache.Lock()
		if err == nil {
			posixLimitCapabilityCache.status = status
			posixLimitCapabilityCache.ready = true
		}
		posixLimitCapabilityCache.probing = nil
		close(probing)
		posixLimitCapabilityCache.Unlock()
		return status, err
	}
}

func stripPosixRlimitEnv(env []string) []string {
	return append([]string(nil), env...)
}

func posixTrustedLauncherEnvironment() []string {
	return []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		"LANG=C",
		"LC_ALL=C",
	}
}

func linuxPrlimitPath() (string, error) {
	return resolveTrustedPOSIXLauncher("prlimit", []string{"/usr/bin/prlimit", "/bin/prlimit"})
}

func resolveTrustedPOSIXLauncher(name string, candidates []string) (string, error) {
	var candidateErrors []error
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" || !filepath.IsAbs(candidate) {
			candidateErrors = append(candidateErrors, fmt.Errorf("launcher candidate %q must be absolute", candidate))
			continue
		}
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			candidateErrors = append(candidateErrors, fmt.Errorf("inspect launcher candidate %q: %w", candidate, err))
			continue
		}
		resolved = filepath.Clean(resolved)
		if _, exists := seen[resolved]; exists {
			continue
		}
		seen[resolved] = struct{}{}
		if err := validateTrustedPOSIXLauncher(resolved); err != nil {
			candidateErrors = append(candidateErrors, fmt.Errorf("inspect launcher candidate %q: %w", candidate, err))
			continue
		}
		return resolved, nil
	}
	if len(candidateErrors) == 0 {
		candidateErrors = append(candidateErrors, fmt.Errorf("no launcher candidates configured"))
	}
	return "", fmt.Errorf("resolve trusted %s launcher: %w", name, errors.Join(candidateErrors...))
}

func validateTrustedPOSIXLauncher(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("launcher must be a regular executable file")
	}
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Stat(current)
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("launcher ownership is unavailable")
		}
		if stat.Uid != 0 {
			return fmt.Errorf("launcher path %q must be owned by root", current)
		}
		if info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("launcher path %q must not be group- or world-writable", current)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return nil
}
