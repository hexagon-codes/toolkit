//go:build !windows

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	posixRlimitHelperArg          = "--hexclaw-sandbox-rlimit-helper"
	posixRlimitHelperEnv          = "HEXCLAW_SANDBOX_RLIMIT_HELPER"
	posixRlimitMemoryBytesEnv     = "HEXCLAW_SANDBOX_RLIMIT_MEMORY_BYTES"
	posixRlimitMaxProcessesEnv    = "HEXCLAW_SANDBOX_RLIMIT_MAX_PROCESSES"
	posixRlimitHelperExpectedMark = "1"
)

func init() {
	if len(os.Args) >= 3 &&
		os.Args[1] == posixRlimitHelperArg &&
		os.Getenv(posixRlimitHelperEnv) == posixRlimitHelperExpectedMark {
		os.Exit(runPosixRlimitHelper(os.Args[2:]))
	}
}

// runBoundedCommand 执行受限命令。fsContainment 为调用方(各平台后端)判定的
// 文件系统隔离强度, 原样写入 ExecResult.Limits.Filesystem 供上层消费。
func runBoundedCommand(ctx context.Context, command string, args []string, cfg Config, env []string, fsContainment LimitStatus) (*ExecResult, error) {
	if err := enforceSandboxStorageLimits(cfg); err != nil {
		return nil, err
	}

	stdout := newBoundedBuffer(cfg.MaxOutputBytes)
	stderr := newBoundedBuffer(cfg.MaxStderrBytes)

	runCommand, runArgs, runEnv, setupErr := posixResourceLimitedCommand(command, args, cfg, env)
	if setupErr != nil {
		return nil, setupErr
	}

	cmd := exec.CommandContext(ctx, runCommand, runArgs...)
	cmd.Dir = cfg.Workspace
	cmd.Env = runEnv
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("sandbox exec start failed: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		killErr := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		waitErr := <-done
		res := &ExecResult{
			Stdout:          stdout.String(),
			Stderr:          stderr.String(),
			ExitCode:        -1,
			StdoutBytes:     stdout.BytesSeen(),
			StderrBytes:     stderr.BytesSeen(),
			StdoutTruncated: stdout.Truncated(),
			StderrTruncated: stderr.Truncated(),
			Limits:          posixLimitReport(fsContainment),
		}
		if limitErr := enforceSandboxStorageLimits(cfg); limitErr != nil {
			// 存储限额错误也用 %w 包装, 保证 ErrStorageLimitExceeded 哨兵可被 errors.Is 命中
			return res, fmt.Errorf("sandbox exec terminated by timeout/cancel: %w; storage limit check failed: %w", ctx.Err(), limitErr)
		}
		if killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
			return res, fmt.Errorf("sandbox exec terminated by timeout/cancel: %w; kill process group failed: %w", ctx.Err(), killErr)
		}
		if waitErr != nil {
			return res, fmt.Errorf("sandbox exec terminated by timeout/cancel: %w; wait failed: %v", ctx.Err(), waitErr)
		}
		return res, fmt.Errorf("sandbox exec terminated by timeout/cancel: %w", ctx.Err())
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("sandbox exec failed: %w", err)
		}
	}

	res := &ExecResult{
		Stdout:          stdout.String(),
		Stderr:          stderr.String(),
		ExitCode:        exitCode,
		StdoutBytes:     stdout.BytesSeen(),
		StderrBytes:     stderr.BytesSeen(),
		StdoutTruncated: stdout.Truncated(),
		StderrTruncated: stderr.Truncated(),
		Limits:          posixLimitReport(fsContainment),
	}
	if err := enforceSandboxStorageLimits(cfg); err != nil {
		return res, err
	}
	return res, nil
}

func posixResourceLimitedCommand(command string, args []string, cfg Config, env []string) (runner string, runnerArgs, runEnv []string, err error) {
	if cfg.MaxMemoryBytes <= 0 && cfg.MaxProcesses <= 0 {
		return command, args, env, nil
	}
	if runtime.GOOS == "linux" {
		return linuxPrlimitCommand(command, args, cfg, env)
	}

	exe, err := os.Executable()
	if err != nil {
		return "", nil, nil, fmt.Errorf("resolve sandbox rlimit helper executable: %w", err)
	}

	helperArgs := make([]string, 0, len(args)+2)
	helperArgs = append(helperArgs, posixRlimitHelperArg, command)
	helperArgs = append(helperArgs, args...)

	helperEnv := stripPosixRlimitEnv(env)
	helperEnv = append(helperEnv, posixRlimitHelperEnv+"="+posixRlimitHelperExpectedMark)
	if cfg.MaxMemoryBytes > 0 {
		helperEnv = append(helperEnv, fmt.Sprintf("%s=%d", posixRlimitMemoryBytesEnv, cfg.MaxMemoryBytes))
	}
	if cfg.MaxProcesses > 0 {
		helperEnv = append(helperEnv, fmt.Sprintf("%s=%d", posixRlimitMaxProcessesEnv, cfg.MaxProcesses))
	}

	return exe, helperArgs, helperEnv, nil
}

func linuxPrlimitCommand(command string, args []string, cfg Config, env []string) (runner string, runnerArgs, runEnv []string, err error) {
	prlimit, err := exec.LookPath("prlimit")
	if err != nil {
		return "", nil, nil, fmt.Errorf("resolve POSIX rlimit helper prlimit: %w", err)
	}

	prlimitArgs := make([]string, 0, len(args)+4)
	if cfg.MaxMemoryBytes > 0 {
		prlimitArgs = append(prlimitArgs, fmt.Sprintf("--as=%d", cfg.MaxMemoryBytes))
	}
	if cfg.MaxProcesses > 0 {
		prlimitArgs = append(prlimitArgs, fmt.Sprintf("--nproc=%d", processAwareMaxProcesses(uint64(cfg.MaxProcesses))))
	}
	prlimitArgs = append(prlimitArgs, "--", command)
	prlimitArgs = append(prlimitArgs, args...)

	prlimitEnv := stripPosixRlimitEnv(env)

	return prlimit, prlimitArgs, prlimitEnv, nil
}

func runPosixRlimitHelper(argv []string) int {
	if len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "sandbox rlimit helper: missing command")
		return 127
	}
	if err := applyConfiguredPosixRlimits(); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox rlimit helper: %v\n", err)
		return 126
	}

	command := argv[0]
	path, err := exec.LookPath(command)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sandbox rlimit helper: resolve %q: %v\n", command, err)
		return 127
	}
	if err := syscall.Exec(path, argv, stripPosixRlimitEnv(os.Environ())); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox rlimit helper: exec %q: %v\n", command, err)
		return 127
	}
	return 127
}

func applyConfiguredPosixRlimits() error {
	if raw := os.Getenv(posixRlimitMemoryBytesEnv); raw != "" {
		maxMemoryBytes, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("parse MaxMemoryBytes: %w", err)
		}
		if maxMemoryBytes > 0 {
			// 平台不支持(unsupported)不阻断执行——限额缺失不该让执行失败;
			// 能力缺口经父进程侧探测写入 ExecResult.Limits 显式上报, 不在此静默假装。
			if _, err := setPosixMemoryRlimit(maxMemoryBytes); err != nil {
				return fmt.Errorf("enforce MaxMemoryBytes: %w", err)
			}
		}
	}

	if raw := os.Getenv(posixRlimitMaxProcessesEnv); raw != "" {
		maxProcesses, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("parse MaxProcesses: %w", err)
		}
		if maxProcesses > 0 {
			if err := setPosixRlimit(unix.RLIMIT_NPROC, processAwareMaxProcesses(maxProcesses)); err != nil {
				return fmt.Errorf("enforce MaxProcesses: %w", err)
			}
		}
	}

	return nil
}

func processAwareMaxProcesses(maxProcesses uint64) uint64 {
	current, ok := currentUIDProcessCount()
	if !ok {
		return maxProcesses
	}
	// uint64 加法溢出保护(此处判定的是算术溢出, 非 rlimit 无穷值)
	if current > ^uint64(0)-maxProcesses {
		return ^uint64(0)
	}
	return current + maxProcesses
}

func currentUIDProcessCount() (uint64, bool) {
	if runtime.GOOS == "linux" {
		if count, ok := currentLinuxUIDTaskCount(); ok {
			return count, true
		}
	}

	psPath := ""
	for _, candidate := range []string{"/bin/ps", "/usr/bin/ps"} {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			psPath = candidate
			break
		}
	}
	if psPath == "" {
		return 0, false
	}

	out, err := exec.CommandContext(context.Background(), psPath, "-axo", "uid=").Output()
	if err != nil {
		return 0, false
	}
	uid := strconv.Itoa(os.Getuid())
	var count uint64
	for _, field := range strings.Fields(string(out)) {
		if field == uid {
			count++
		}
	}
	return count, true
}

func currentLinuxUIDTaskCount() (uint64, bool) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, false
	}

	uid := strconv.Itoa(os.Getuid())
	var count uint64
	var matched bool
	for _, entry := range entries {
		pid := entry.Name()
		if !isDecimalString(pid) {
			continue
		}
		status, err := os.ReadFile("/proc/" + pid + "/status")
		if err != nil {
			continue
		}
		if !linuxStatusMatchesUID(string(status), uid) {
			continue
		}
		tasks, err := os.ReadDir("/proc/" + pid + "/task")
		if err != nil {
			continue
		}
		matched = true
		if len(tasks) == 0 {
			count++
			continue
		}
		count += uint64(len(tasks))
	}
	return count, matched
}

func isDecimalString(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func linuxStatusMatchesUID(status, uid string) bool {
	for _, line := range strings.Split(status, "\n") {
		if !strings.HasPrefix(line, "Uid:") {
			continue
		}
		fields := strings.Fields(line)
		return len(fields) > 1 && fields[1] == uid
	}
	return false
}

// setPosixMemoryRlimit 依次尝试用 RLIMIT_AS/DATA/RSS 下调内存软限。
//
// 返回值语义:
//   - (LimitStatusEnforced, nil): 至少一个资源下调成功, 限额真实生效;
//   - (LimitStatusUnsupported, nil): 三资源均因平台不支持(EINVAL/ENOTSUP/ENOSYS,
//     如 darwin 内核拒绝现实量级下调)而失败——能力缺口不阻断执行, 但必须
//     把 unsupported 状态传导给调用方, 不许 return nil 假装已生效;
//   - ("", err): 真实错误(权限等), 由调用方决定是否失败。
func setPosixMemoryRlimit(maxMemoryBytes uint64) (LimitStatus, error) {
	resources := []int{unix.RLIMIT_AS, unix.RLIMIT_DATA, unix.RLIMIT_RSS}
	for _, resource := range resources {
		if err := setPosixRlimit(resource, maxMemoryBytes); err != nil {
			if isPosixRlimitUnsupported(err) {
				continue
			}
			return "", err
		}
		return LimitStatusEnforced, nil
	}
	return LimitStatusUnsupported, nil
}

// isPosixRlimitUnsupported 判定 setrlimit 错误是否属于「平台不支持该限制」的能力缺口。
func isPosixRlimitUnsupported(err error) bool {
	return errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.ENOSYS)
}

func setPosixRlimit(resource int, desired uint64) error {
	if desired == 0 {
		return nil
	}

	var lim unix.Rlimit
	if err := unix.Getrlimit(resource, &lim); err != nil {
		return err
	}

	// 无上限判定必须用 unix.RLIM_INFINITY(darwin 上为 2^63-1, 与 ^uint64(0) 不同)
	next := desired
	if lim.Max != 0 && lim.Max != unix.RLIM_INFINITY && next > lim.Max {
		next = lim.Max
	}
	if lim.Cur != 0 && lim.Cur != unix.RLIM_INFINITY && lim.Cur < next {
		next = lim.Cur
	}
	lim.Cur = next
	return unix.Setrlimit(resource, &lim)
}

// posixMemoryProbeBytes 内存限额能力探测所用的代表值, 与 New() 的 MaxMemoryBytes
// 默认值同量级。darwin 内核仅对现实量级(≲百 GiB)的下调返回 EINVAL, 对天文数字级
// 的值反而「成功」但无实际约束意义, 故探测值必须落在真实配置会用到的量级。
const posixMemoryProbeBytes = 256 * 1024 * 1024

var (
	posixLimitCapabilityOnce sync.Once
	posixMemoryCapability    LimitStatus
	posixProcessCapability   LimitStatus
)

// posixLimitReport 报告当前平台各资源限制项的实际执行状态。
//
// Storage(工作区 walk 检查)与 Output(有界缓冲)由本包纯用户态实现, 恒 enforced;
// Memory/Processes 依赖内核 rlimit, 平台支持性经一次性探测后缓存;
// Filesystem 由调用方(各平台后端)按实际选用的隔离机制判定后传入。
func posixLimitReport(fsContainment LimitStatus) LimitReport {
	posixLimitCapabilityOnce.Do(func() {
		posixMemoryCapability = probePosixMemoryLimitCapability()
		posixProcessCapability = probePosixProcessLimitCapability()
	})
	return LimitReport{
		Memory:     posixMemoryCapability,
		Processes:  posixProcessCapability,
		Storage:    LimitStatusEnforced,
		Output:     LimitStatusEnforced,
		Filesystem: fsContainment,
	}
}

// probePosixMemoryLimitCapability 探测当前平台能否真实下调内存 rlimit。
//
// linux 内核允许无特权进程下调 AS/DATA/RSS 软限, 编译期即可判定为 enforced,
// 且避免在父进程上做「真下调」探测(哪怕瞬时压低自身内存上限也有风险)。
// 其余 POSIX 平台做一次性运行期探测: 逐资源尝试下调到现实量级的有限值,
// 成功则立即恢复原值(darwin 上现实量级一律 EINVAL、不产生任何改动)。
func probePosixMemoryLimitCapability() LimitStatus {
	if runtime.GOOS == "linux" {
		return LimitStatusEnforced
	}
	for _, resource := range []int{unix.RLIMIT_AS, unix.RLIMIT_DATA, unix.RLIMIT_RSS} {
		var lim unix.Rlimit
		if err := unix.Getrlimit(resource, &lim); err != nil {
			continue
		}
		orig := lim
		probe := uint64(posixMemoryProbeBytes)
		if lim.Cur != unix.RLIM_INFINITY && lim.Cur <= probe {
			if lim.Cur <= 1 {
				continue
			}
			// 保证探测是真实「下调」(等值回写在部分内核上无条件成功, 不具判别力)
			probe = lim.Cur - 1
		}
		lim.Cur = probe
		if err := unix.Setrlimit(resource, &lim); err == nil {
			if err := unix.Setrlimit(resource, &orig); err != nil {
				continue
			}
			return LimitStatusEnforced
		}
	}
	return LimitStatusUnsupported
}

// probePosixProcessLimitCapability 探测 RLIMIT_NPROC 是否可设置。
//
// 用「等值回写」探测而非「下调再恢复」: darwin 上 NPROC 软限一旦下调便无法
// 回升(哪怕仍在硬限之内也 EPERM), 下调式探测会永久压低父进程的进程数上限。
func probePosixProcessLimitCapability() LimitStatus {
	var lim unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NPROC, &lim); err != nil {
		return LimitStatusUnsupported
	}
	if err := unix.Setrlimit(unix.RLIMIT_NPROC, &lim); err != nil {
		return LimitStatusUnsupported
	}
	return LimitStatusEnforced
}

func stripPosixRlimitEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, item := range env {
		name := item
		if idx := strings.IndexByte(item, '='); idx >= 0 {
			name = item[:idx]
		}
		switch name {
		case posixRlimitHelperEnv, posixRlimitMemoryBytesEnv, posixRlimitMaxProcessesEnv:
			continue
		default:
			out = append(out, item)
		}
	}
	return out
}
