//go:build linux

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	linuxBoundaryProbeArgument = "--toolkit-linux-boundary-probe"
	linuxBoundaryProbeEnv      = "TOOLKIT_LINUX_BOUNDARY_PROBE"
	linuxBoundaryProbeMark     = "1"
)

type linuxBwrapProbeResult struct {
	Isolation LimitStatus
}

// linuxResourceBackendProbe 冻结当前配置在实际 Linux 启动链上可证明的资源能力。
// 进程数量只有树级总预算后端才可标记为 enforced，RLIMIT_NPROC 不满足该合同。
type linuxResourceBackendProbe struct {
	Memory      LimitStatus
	Processes   LimitStatus
	prlimitPath string
	memoryErr   error
}

type linuxSandboxExecution struct {
	command      Command
	capabilities posixExecutionCapabilities
	preflight    func(context.Context) error
}

// linuxSandbox 使用 bubblewrap 提供 deny-by-default 文件系统与命名空间隔离。
type linuxSandbox struct {
	cfg             Config
	resolveBwrap    func() (string, error)
	probeBwrap      func(string, bool) linuxBwrapProbeResult
	resolvePrlimit  func() (string, error)
	resourceMu      sync.Mutex
	resourceReady   bool
	resourceProbing chan struct{}
	resourceBackend linuxResourceBackendProbe
	executions      posixExecutionRegistry
}

func init() {
	if len(os.Args) == 3 && os.Args[1] == linuxBoundaryProbeArgument &&
		os.Getenv(linuxBoundaryProbeEnv) == linuxBoundaryProbeMark {
		os.Exit(runLinuxBoundaryProbePayload(os.Args[2] == "1"))
	}
}

func newPlatformSandbox(cfg Config) (Sandbox, error) {
	return &linuxSandbox{cfg: cfg}, nil
}

// Exec 在沙箱内执行命令。
//
// Linux 强制使用 bubblewrap，因为它在普通桌面/CI Linux 上提供接近 macOS Seatbelt 的
// deny-by-default 文件系统视图：workspace 读写、系统运行时只读、ReadablePaths 只读、
// NetworkDisabled 时 unshare net。
//
// bubblewrap 不可用或能力探测失败时直接拒绝执行，不降级到暴露宿主挂载视图的后端。
func (s *linuxSandbox) Exec(ctx context.Context, requested Command) (*ExecResult, error) {
	if err := validateExecContext(ctx); err != nil {
		return nil, err
	}
	if err := s.executions.ensureReady(); err != nil {
		return nil, err
	}
	if err := checkPOSIXPreparationContext(ctx, "prepare Linux execution"); err != nil {
		return nil, err
	}
	if err := initializePOSIXRuntimeDirectories(s.cfg.Workspace); err != nil {
		return nil, err
	}
	// 应用 cfg.Timeout: 调用方 ctx 无更早 deadline 时按配置强制超时。
	ctx, cancel := withTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	command, err := prepareSandboxCommand(s.cfg, requested, func() ([]string, error) {
		return cleanLinuxEnv(s.cfg.Workspace, os.Environ())
	})
	if err != nil {
		return nil, err
	}
	if err := checkPOSIXPreparationContext(ctx, "prepare Linux execution"); err != nil {
		return nil, err
	}
	execution, err := s.planLinuxSandboxExecutionContext(ctx, command)
	if err != nil {
		return nil, err
	}
	// 资源限制已被放进 bubblewrap 内部，必须先建立用户与 PID 命名空间，再对载荷
	// 应用限制；当前入口不会在宿主侧重复应用限制，原始配置仅用于保留本次请求事实。
	res, err := s.executions.runBoundedPreparedCommandWithPreflight(
		ctx,
		execution.command,
		s.cfg,
		execution.capabilities,
		execution.preflight,
	)
	if err != nil {
		// 已产生结果说明后端成功启动，错误属于执行结果（包括取消、超时与存储
		// 限额），必须保留 Limits/stdout/stderr，不能误报为后端不可用。
		if res != nil {
			return res, err
		}
		return nil, fmt.Errorf("sandbox unavailable: linux backend failed: %w", err)
	}
	return res, nil
}

func (s *linuxSandbox) planLinuxSandboxExecution(command Command) (linuxSandboxExecution, error) {
	return s.planLinuxSandboxExecutionContext(context.Background(), command)
}

func (s *linuxSandbox) planLinuxSandboxExecutionContext(ctx context.Context, command Command) (linuxSandboxExecution, error) {
	if err := checkPOSIXPreparationContext(ctx, "plan Linux execution"); err != nil {
		return linuxSandboxExecution{}, err
	}
	plan, err := linuxCommandExecutionPlan(command, s.cfg.Workspace)
	if err != nil {
		return linuxSandboxExecution{}, err
	}
	resourceBackend, resourceErr := s.inspectLinuxResourceBackendContext(ctx)
	if resourceErr != nil {
		return linuxSandboxExecution{}, resourceErr
	}
	if s.cfg.MaxMemoryBytes > 0 && resourceBackend.Memory != LimitStatusEnforced {
		return linuxSandboxExecution{}, fmt.Errorf("%w: Linux memory limit backend is unavailable: %v", ErrRequiredCapabilitiesUnavailable, resourceBackend.memoryErr)
	}
	if s.cfg.MaxProcesses > 0 && resourceBackend.Processes != LimitStatusEnforced {
		return linuxSandboxExecution{}, fmt.Errorf("%w: Linux exact process-tree budget is unavailable", ErrRequiredCapabilitiesUnavailable)
	}
	if err := checkPOSIXPreparationContext(ctx, "plan Linux execution"); err != nil {
		return linuxSandboxExecution{}, err
	}
	guard, err := newLinuxExecutionGuardContext(ctx, s.cfg, plan.command)
	if err != nil {
		return linuxSandboxExecution{}, err
	}

	resolveBwrap := s.resolveBwrap
	if resolveBwrap == nil {
		resolveBwrap = linuxBwrapPath
	}
	bwrap, bwrapErr := resolveBwrap()
	if bwrapErr != nil {
		return linuxSandboxExecution{}, fmt.Errorf("%w: linux requires usable bubblewrap: %w", ErrFilesystemContainmentUnavailable, bwrapErr)
	}
	probeBwrap := s.probeBwrap
	if probeBwrap == nil {
		probeBwrap = linuxBwrapBackendCapabilities
	}
	probe := probeBwrap(bwrap, s.cfg.Network == NetworkHost)
	if probe.Isolation != LimitStatusEnforced {
		return linuxSandboxExecution{}, fmt.Errorf("%w: linux requires usable bubblewrap: isolation capability probe failed", ErrFilesystemContainmentUnavailable)
	}
	bwrapArgs, err := s.bwrapArgs(plan.command)
	if err != nil {
		return linuxSandboxExecution{}, err
	}
	if err := checkPOSIXPreparationContext(ctx, "plan Linux execution"); err != nil {
		return linuxSandboxExecution{}, err
	}
	preflight := func(ctx context.Context) error {
		if linuxPreLaunchAuditHook != nil {
			linuxPreLaunchAuditHook()
		}
		if err := guard.VerifyContext(ctx); err != nil {
			return err
		}
		if s.cfg.MaxMemoryBytes > 0 {
			if err := verifyLinuxPrlimitMemoryLimit(ctx, resourceBackend.prlimitPath, s.cfg.MaxMemoryBytes); err != nil {
				return fmt.Errorf("verify Linux memory limit before launch: %w", err)
			}
		}
		return nil
	}
	return linuxSandboxExecution{
		command: Command{
			Path: bwrap,
			Args: bwrapArgs,
			Dir:  s.cfg.Workspace,
			Env:  trustedPOSIXLauncherEnvironment(),
		},
		capabilities: posixExecutionCapabilities{
			Filesystem:         LimitStatusEnforced,
			Network:            LimitStatusEnforced,
			Memory:             resourceBackend.Memory,
			Processes:          resourceBackend.Processes,
			ProcessContainment: LimitStatusEnforced,
		},
		preflight: preflight,
	}, nil
}

func (s *linuxSandbox) sandboxCapabilities(ctx context.Context) (CapabilitySet, error) {
	if err := validateExecContext(ctx); err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("inspect Linux sandbox capabilities: %w", err)
	}
	resolveBwrap := s.resolveBwrap
	if resolveBwrap == nil {
		resolveBwrap = linuxBwrapPath
	}
	bwrap, err := resolveBwrap()
	if err != nil {
		return 0, nil
	}
	probeBwrap := s.probeBwrap
	if probeBwrap == nil {
		probeBwrap = linuxBwrapBackendCapabilities
	}
	probe := probeBwrap(bwrap, s.cfg.Network == NetworkHost)
	if probe.Isolation != LimitStatusEnforced {
		return 0, nil
	}
	available := CapabilityFilesystem | CapabilityNetwork | CapabilityProcessContainment | CapabilityOutput
	resourceBackend, resourceErr := s.inspectLinuxResourceBackendContext(ctx)
	if resourceErr != nil {
		return 0, resourceErr
	}
	if s.cfg.MaxMemoryBytes > 0 && resourceBackend.Memory == LimitStatusEnforced {
		available |= CapabilityMemory
	}
	if s.cfg.MaxProcesses > 0 && resourceBackend.Processes == LimitStatusEnforced {
		available |= CapabilityProcesses
	}
	if processCreationFitsBudget(s.cfg.MaxProcesses) {
		available |= CapabilityProcessCreation
	}
	return available, nil
}

func (s *linuxSandbox) inspectLinuxResourceBackend() linuxResourceBackendProbe {
	probe, err := s.inspectLinuxResourceBackendContext(context.Background())
	if err != nil {
		probe.memoryErr = err
	}
	return probe
}

func (s *linuxSandbox) inspectLinuxResourceBackendContext(ctx context.Context) (linuxResourceBackendProbe, error) {
	for {
		s.resourceMu.Lock()
		if s.resourceReady {
			probe := s.resourceBackend
			s.resourceMu.Unlock()
			return probe, nil
		}
		if probing := s.resourceProbing; probing != nil {
			s.resourceMu.Unlock()
			select {
			case <-probing:
				continue
			case <-ctx.Done():
				return linuxResourceBackendProbe{}, fmt.Errorf("inspect Linux resource backend: %w", ctx.Err())
			}
		}
		probing := make(chan struct{})
		s.resourceProbing = probing
		s.resourceMu.Unlock()

		probe, err := s.probeLinuxResourceBackend(ctx)
		s.resourceMu.Lock()
		if err == nil {
			s.resourceBackend = probe
			s.resourceReady = true
		}
		s.resourceProbing = nil
		close(probing)
		s.resourceMu.Unlock()
		return probe, err
	}
}

func (s *linuxSandbox) probeLinuxResourceBackend(ctx context.Context) (linuxResourceBackendProbe, error) {
	probe := linuxResourceBackendProbe{
		Memory:    LimitStatusNotRequested,
		Processes: LimitStatusNotRequested,
	}
	if s.cfg.MaxMemoryBytes > 0 {
		probe.Memory = LimitStatusUnsupported
		resolvePrlimit := s.resolvePrlimit
		if resolvePrlimit == nil {
			resolvePrlimit = linuxPrlimitPath
		}
		path, err := resolvePrlimit()
		if err == nil && path == "" {
			err = errors.New("trusted prlimit path is empty")
		}
		if err == nil {
			err = verifyLinuxPrlimitMemoryLimit(ctx, path, s.cfg.MaxMemoryBytes)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return linuxResourceBackendProbe{}, fmt.Errorf("inspect Linux resource backend: %w", ctxErr)
		}
		probe.memoryErr = err
		if err == nil {
			probe.Memory = LimitStatusEnforced
			probe.prlimitPath = path
		}
	}
	if s.cfg.MaxProcesses > 0 {
		// Linux RLIMIT_NPROC 按真实 UID 汇总计数，无法证明包含根载荷的沙箱树级总预算。
		probe.Processes = LimitStatusUnsupported
	}
	return probe, nil
}

func verifyLinuxPrlimitMemoryLimit(ctx context.Context, prlimitPath string, maxMemoryBytes int64) error {
	if err := checkPOSIXPreparationContext(ctx, "verify Linux memory limit"); err != nil {
		return err
	}
	if prlimitPath == "" || maxMemoryBytes <= 0 {
		return fmt.Errorf("Linux memory limit verification requires a launcher and a positive limit")
	}
	payloadPath, err := resolveTrustedPOSIXLauncher("Linux memory limit probe payload", []string{"/usr/bin/true", "/bin/true"})
	if err != nil {
		return err
	}
	// prlimit 与真实载荷使用完全相同的 --as 参数；固定 true 只验证限制链，不接触宿主 rlimit。
	cmd := exec.CommandContext( // #nosec G204 -- launcher 来自已验证的固定系统路径或同包测试注入。
		ctx,
		prlimitPath,
		fmt.Sprintf("--as=%d", maxMemoryBytes),
		"--",
		payloadPath,
	)
	cmd.Env = posixTrustedLauncherEnvironment()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Linux prlimit memory probe failed: %w", err)
	}
	return nil
}

var (
	linuxBwrapOnlineProbeOnce  sync.Once
	linuxBwrapOnlineProbe      linuxBwrapProbeResult
	linuxBwrapOfflineProbeOnce sync.Once
	linuxBwrapOfflineProbe     linuxBwrapProbeResult
	// linuxPreLaunchAuditHook 只用于同步安全回归测试中的替换窗口。
	linuxPreLaunchAuditHook func()
)

func linuxBwrapBackendUsable(bwrap string, network bool) bool {
	result := linuxBwrapBackendCapabilities(bwrap, network)
	return result.Isolation == LimitStatusEnforced
}

func linuxBwrapBackendCapabilities(bwrap string, network bool) linuxBwrapProbeResult {
	if bwrap == "" {
		return linuxBwrapProbeResult{}
	}
	if network {
		linuxBwrapOnlineProbeOnce.Do(func() {
			linuxBwrapOnlineProbe = runLinuxBwrapProbe(bwrap, true)
		})
		return linuxBwrapOnlineProbe
	}
	linuxBwrapOfflineProbeOnce.Do(func() {
		linuxBwrapOfflineProbe = runLinuxBwrapProbe(bwrap, false)
	})
	return linuxBwrapOfflineProbe
}

func runLinuxBwrapProbe(bwrap string, network bool) linuxBwrapProbeResult {
	ws, err := os.MkdirTemp("", "toolkit-bwrap-probe-*")
	if err != nil {
		return linuxBwrapProbeResult{}
	}
	defer func() {
		if removeErr := os.RemoveAll(ws); removeErr != nil {
			fmt.Fprintf(os.Stderr, "sandbox: remove bwrap probe workspace %q: %v\n", ws, removeErr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := initializePOSIXRuntimeDirectories(ws); err != nil {
		return linuxBwrapProbeResult{}
	}
	probeExecutable, err := copyLinuxBoundaryProbeExecutable(ws)
	if err != nil {
		return linuxBwrapProbeResult{}
	}
	s := &linuxSandbox{cfg: Config{Workspace: ws, Network: NetworkMode(network)}}
	env, err := cleanLinuxEnv(ws, os.Environ())
	if err != nil {
		return linuxBwrapProbeResult{}
	}
	env, err = appendLinuxParentNamespaceEnvironment(env)
	if err != nil {
		return linuxBwrapProbeResult{}
	}
	env = append(env, linuxBoundaryProbeEnv+"="+linuxBoundaryProbeMark)
	args, err := s.bwrapArgs(Command{
		Path: probeExecutable,
		Args: []string{linuxBoundaryProbeArgument, strconv.FormatBool(!network)},
		Dir:  ws,
		Env:  env,
	})
	if err != nil {
		return linuxBwrapProbeResult{}
	}
	// bwrap 已由可信系统候选冻结为绝对路径，探测参数由本包固定生成且不经过命令行解释器。
	cmd := exec.CommandContext(ctx, bwrap, args...) // #nosec G204 -- 动态程序路径来自受控的后端能力探测。
	cmd.Env = trustedPOSIXLauncherEnvironment()
	output, err := cmd.Output()
	if err != nil {
		return linuxBwrapProbeResult{}
	}
	return parseLinuxBoundaryProbeResult(string(output))
}

func (s *linuxSandbox) bwrapArgs(command Command) ([]string, error) {
	plan, err := linuxCommandExecutionPlan(command, s.cfg.Workspace)
	if err != nil {
		return nil, err
	}
	privateTemporary, err := linuxPrivateTemporaryDirectory(s.cfg)
	if err != nil {
		return nil, err
	}
	mounts := linuxSystemReadOnlyMounts()
	for _, path := range plan.runtimePaths {
		mounts = append(mounts, linuxReadOnlyMount{source: path, target: path})
	}
	mounts = linuxUniqueMounts(mounts)

	out := []string{
		"--die-with-parent",
		"--new-session",
		"--unshare-user",
		"--unshare-pid",
		"--unshare-ipc",
		"--unshare-uts",
		"--disable-userns",
		"--assert-userns-disabled",
		"--cap-drop", "ALL",
		"--uid", strconv.Itoa(linuxSandboxUID()),
		"--gid", strconv.Itoa(linuxSandboxGID()),
		"--clearenv",
		"--proc", "/proc",
		"--dev", "/dev",
		"--dir", "/tmp",
	}
	for _, item := range plan.command.Env {
		name, value, ok := strings.Cut(item, "=")
		if ok {
			out = append(out, "--setenv", name, value)
		}
	}
	if s.cfg.Network == NetworkDisabled {
		out = append(out, "--unshare-net")
	}
	for _, dir := range linuxMountParentDirectories(mounts) {
		out = append(out, "--dir", dir)
	}
	for _, mount := range mounts {
		out = append(out, "--ro-bind", mount.source, mount.target)
	}
	out = append(out, "--bind", privateTemporary, "/tmp")
	// Workspace 可能自身位于 /tmp；私有临时目录挂载后必须恢复这个精确子挂载。
	out = append(out, "--bind", s.cfg.Workspace, s.cfg.Workspace)
	for _, p := range s.cfg.ReadablePaths {
		p = cleanLinuxMountPath(p)
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			out = append(out, "--ro-bind", p, p)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect sandbox readable path %q: %w", p, err)
		}
	}
	for _, p := range s.cfg.DeniedPaths {
		p = cleanLinuxMountPath(p)
		if p == "" || p == "/" {
			return nil, fmt.Errorf("sandbox denied path is invalid")
		}
		info, err := os.Lstat(p)
		switch {
		case err == nil && info.IsDir():
			out = append(out, "--tmpfs", p, "--chmod", "000", p, "--remount-ro", p)
		case err == nil:
			out = append(out, "--ro-bind", "/dev/null", p)
		case errors.Is(err, os.ErrNotExist):
			return nil, fmt.Errorf("sandbox denied path must exist before launch: %q", p)
		default:
			return nil, fmt.Errorf("inspect sandbox denied path %q: %w", p, err)
		}
	}
	payloadPath, payloadArgs, err := s.linuxResourceLimitedPayload(plan.command)
	if err != nil {
		return nil, err
	}
	out = append(out, "--chdir", plan.command.Dir, "--", payloadPath)
	out = append(out, payloadArgs...)
	return out, nil
}

func (s *linuxSandbox) linuxResourceLimitedPayload(command Command) (string, []string, error) {
	if s.cfg.MaxMemoryBytes <= 0 {
		return command.Path, command.Args, nil
	}
	resourceBackend := s.inspectLinuxResourceBackend()
	if resourceBackend.Memory != LimitStatusEnforced || resourceBackend.prlimitPath == "" {
		return "", nil, fmt.Errorf("resolve Linux resource limit launcher: %w", resourceBackend.memoryErr)
	}
	arguments := make([]string, 0, len(command.Args)+3)
	arguments = append(arguments, fmt.Sprintf("--as=%d", s.cfg.MaxMemoryBytes))
	arguments = append(arguments, "--", command.Path)
	arguments = append(arguments, command.Args...)
	return resourceBackend.prlimitPath, arguments, nil
}

func linuxSandboxUID() int {
	if os.Geteuid() == 0 {
		return 65534
	}
	return os.Getuid()
}

func linuxSandboxGID() int {
	if os.Getegid() == 0 {
		return 65534
	}
	return os.Getgid()
}

func linuxPrivateTemporaryDirectory(cfg Config) (string, error) {
	path := filepath.Join(cfg.Workspace, posixRuntimeDirectory, "tmp")
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve sandbox private temporary directory: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect sandbox private temporary directory: %w", err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 || !linuxPathWithin(cfg.Workspace, resolved) {
		return "", fmt.Errorf("sandbox private temporary directory must be a private workspace directory")
	}
	for _, denied := range cfg.DeniedPaths {
		if linuxPathWithin(denied, resolved) || linuxPathWithin(resolved, denied) {
			return "", fmt.Errorf("sandbox private temporary directory conflicts with a denied path")
		}
	}
	return filepath.Clean(resolved), nil
}

type linuxReadOnlyMount struct {
	source string
	target string
}

type linuxCommandPlan struct {
	command      Command
	runtimePaths []string
}

func cleanLinuxEnv(workspace string, env []string) ([]string, error) {
	return buildPOSIXSandboxEnv(workspace, env, linuxSandboxPath())
}

func trustedPOSIXLauncherEnvironment() []string {
	return []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		"LANG=C",
		"LC_ALL=C",
	}
}

func linuxBwrapPath() (string, error) {
	return resolveTrustedPOSIXLauncher("bubblewrap", []string{"/usr/bin/bwrap", "/bin/bwrap"})
}

func linuxSandboxPath() string {
	var paths []string
	for _, name := range []string{
		"python", "python3", "node", "nodejs", "go", "gofmt",
		"npm", "npx", "corepack", "pnpm", "yarn", "pip", "pip3",
	} {
		for _, directory := range filepath.SplitList(os.Getenv("PATH")) {
			if !filepath.IsAbs(directory) {
				continue
			}
			resolved, err := freezePOSIXExecutable(filepath.Join(directory, name))
			if err != nil || !linuxSystemExecutable(resolved) && len(linuxRuntimePaths(name, resolved)) == 0 {
				continue
			}
			paths = append(paths, linuxRuntimePathDirectory(resolved))
			break
		}
	}
	paths = append(paths, "/usr/bin", "/bin", "/usr/sbin", "/sbin")
	return strings.Join(linuxUniquePaths(paths), string(os.PathListSeparator))
}

func linuxRuntimePathDirectory(executable string) string {
	return filepath.Dir(executable)
}

func linuxCommandExecutionPlan(command Command, workspace string) (linuxCommandPlan, error) {
	resolved, err := resolvePOSIXCommandExecutable(command.Path, command.Dir)
	if err != nil {
		return linuxCommandPlan{}, fmt.Errorf("resolve sandbox command %q", command.Path)
	}
	plan := linuxCommandPlan{command: command}
	plan.command.Path = resolved
	if !linuxPathWithin(workspace, resolved) {
		resolvedRuntimePaths := linuxRuntimePaths(command.Path, resolved)
		if !linuxSystemExecutable(resolved) && len(resolvedRuntimePaths) == 0 {
			return linuxCommandPlan{}, fmt.Errorf("sandbox command %q is outside the workspace and trusted runtime roots", command.Path)
		}
		plan.runtimePaths = append(plan.runtimePaths, resolvedRuntimePaths...)
	}
	for _, interpreter := range readPOSIXShebangCommands(resolved) {
		interpreterPath, interpreterErr := resolvePOSIXShebangExecutable(interpreter, command.Dir, command.Env)
		if interpreterErr != nil {
			return linuxCommandPlan{}, fmt.Errorf("sandbox command %q uses an unavailable interpreter %q", command.Path, interpreter)
		}
		if linuxPathWithin(workspace, interpreterPath) {
			continue
		}
		interpreterPaths := linuxRuntimePaths(interpreter, interpreterPath)
		if !linuxSystemExecutable(interpreterPath) && len(interpreterPaths) == 0 {
			return linuxCommandPlan{}, fmt.Errorf("sandbox command %q uses an untrusted interpreter %q", command.Path, interpreter)
		}
		plan.runtimePaths = append(plan.runtimePaths, interpreterPaths...)
	}
	plan.runtimePaths = linuxUniquePaths(plan.runtimePaths)
	return plan, nil
}

func linuxSystemExecutable(executable string) bool {
	for _, root := range []string{"/bin", "/sbin", "/usr/bin", "/usr/sbin", "/usr/libexec", "/nix/store"} {
		if linuxPathWithin(root, executable) {
			return true
		}
	}
	return false
}

func linuxRuntimePaths(command, executable string) []string {
	return linuxRuntimePathsForHost(command, executable, executable)
}

func linuxRuntimePathsForHost(command, executable, hostExecutable string) []string {
	name := filepath.Base(command)
	if hostExecutable != executable {
		return nil
	}
	paths, runtimeOnly := linuxInstalledRuntimePaths(executable)
	if len(paths) == 0 || runtimeOnly && !linuxSupportedRuntimeName(name) {
		return nil
	}
	return linuxUniquePaths(append(paths, executable))
}

func linuxInstalledRuntimePaths(executable string) (paths []string, runtimeOnly bool) {
	cleaned := filepath.Clean(executable)
	// Debian/Ubuntu 将 Node 内置模块与 npm 包装器安装在这个只读系统数据根；
	// 仅当已冻结命令确属受支持的 Node 工具链时授权，不扩大到整个 /usr/share。
	if cleaned == "/usr/bin/node" || cleaned == "/usr/bin/nodejs" ||
		linuxPathWithin("/usr/share/nodejs", cleaned) {
		paths = []string{cleaned}
		if dirExists("/usr/share/nodejs") {
			paths = append(paths, "/usr/share/nodejs")
		}
		return paths, true
	}
	home, homeErr := os.UserHomeDir()
	cellars := []string{"/home/linuxbrew/.linuxbrew/Cellar", "/usr/local/Cellar"}
	if homeErr == nil {
		home = filepath.Clean(home)
		cellars = append(cellars, filepath.Join(home, ".linuxbrew", "Cellar"))
	}
	for _, cellar := range cellars {
		if root := linuxRuntimeRootBelow(cleaned, cellar, 2); root != "" {
			return []string{root}, false
		}
	}
	if linuxPathWithin("/usr/local/go", cleaned) {
		return []string{"/usr/local/go"}, false
	}
	if cleaned == "/usr/local/bin/node" || cleaned == "/usr/local/bin/nodejs" {
		return []string{cleaned}, true
	}
	for _, modules := range []string{"/usr/local/lib/node_modules", "/home/linuxbrew/.linuxbrew/lib/node_modules"} {
		if root := linuxRuntimeRootBelow(cleaned, modules, 1); root != "" {
			return []string{root}, true
		}
	}
	if homeErr == nil {
		if root := linuxRuntimeRootBelow(cleaned, filepath.Join(home, ".pyenv", "versions"), 1); root != "" {
			return []string{root}, true
		}
		if root := linuxRuntimeRootBelow(cleaned, filepath.Join(home, ".nvm", "versions", "node"), 1); root != "" {
			return []string{root}, true
		}
		toolchains := filepath.Join(home, "go", "pkg", "mod", "golang.org")
		if root := linuxRuntimeRootBelow(cleaned, toolchains, 1); root != "" && strings.HasPrefix(filepath.Base(root), "toolchain@") {
			return []string{root}, true
		}
		if root := linuxRuntimeRootBelow(cleaned, filepath.Join(home, "hostedtoolcache"), 3); root != "" {
			return []string{root}, true
		}
	}
	if root := linuxRuntimeRootBelow(cleaned, "/opt/hostedtoolcache", 3); root != "" {
		return []string{root}, true
	}
	if filepath.Dir(cleaned) == "/usr/local/bin" && strings.HasPrefix(strings.ToLower(filepath.Base(cleaned)), "python") {
		version := strings.TrimPrefix(strings.ToLower(filepath.Base(cleaned)), "python")
		stdlib := filepath.Join("/usr/local/lib", "python"+version)
		if dirExists(stdlib) {
			return []string{cleaned, stdlib}, true
		}
	}
	return nil, false
}

func linuxRuntimeRootBelow(path, prefix string, depth int) string {
	if depth <= 0 || !linuxPathWithin(prefix, path) {
		return ""
	}
	relative, err := filepath.Rel(prefix, path)
	if err != nil {
		return ""
	}
	components := strings.Split(relative, string(filepath.Separator))
	if len(components) <= depth {
		return ""
	}
	return filepath.Join(prefix, filepath.Join(components[:depth]...))
}

func linuxSupportedRuntimeName(name string) bool {
	name = strings.ToLower(name)
	switch name {
	case "python", "python3", "node", "nodejs", "go", "gofmt", "npm", "npx", "corepack", "pnpm", "yarn", "pip", "pip3":
		return true
	default:
		return strings.HasPrefix(name, "python3.") || strings.HasPrefix(name, "pip3.")
	}
}

func linuxSystemReadOnlyMounts() []linuxReadOnlyMount {
	var mounts []linuxReadOnlyMount
	for _, path := range []string{
		"/bin", "/sbin", "/lib", "/lib64", "/usr/bin", "/usr/sbin", "/usr/lib", "/usr/lib64", "/usr/libexec",
		"/nix/store", "/usr/share/zoneinfo", "/usr/share/locale", "/usr/share/ca-certificates",
		"/etc/ssl/certs", "/etc/pki/tls/certs", "/etc/ca-certificates",
	} {
		if mount, ok := linuxResolvedReadOnlyMount(path, true); ok {
			mounts = append(mounts, mount)
		}
	}
	for _, path := range []string{
		"/etc/hosts", "/etc/resolv.conf", "/etc/nsswitch.conf", "/etc/passwd", "/etc/group",
		"/etc/protocols", "/etc/services", "/etc/host.conf", "/etc/gai.conf", "/etc/localtime",
		"/etc/timezone", "/etc/ld.so.cache", "/etc/ld.so.conf", "/etc/os-release", "/etc/ca-certificates.conf",
	} {
		if mount, ok := linuxResolvedReadOnlyMount(path, false); ok {
			mounts = append(mounts, mount)
		}
	}
	return linuxUniqueMounts(mounts)
}

func linuxResolvedReadOnlyMount(target string, wantDirectory bool) (linuxReadOnlyMount, bool) {
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return linuxReadOnlyMount{}, false
	}
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() != wantDirectory || !wantDirectory && !info.Mode().IsRegular() {
		return linuxReadOnlyMount{}, false
	}
	return linuxReadOnlyMount{source: filepath.Clean(resolved), target: filepath.Clean(target)}, true
}

func linuxUniqueMounts(mounts []linuxReadOnlyMount) []linuxReadOnlyMount {
	seen := make(map[string]struct{}, len(mounts))
	result := make([]linuxReadOnlyMount, 0, len(mounts))
	for _, mount := range mounts {
		if _, exists := seen[mount.target]; exists {
			continue
		}
		seen[mount.target] = struct{}{}
		result = append(result, mount)
	}
	sort.SliceStable(result, func(i, j int) bool {
		leftDepth := strings.Count(result[i].target, string(filepath.Separator))
		rightDepth := strings.Count(result[j].target, string(filepath.Separator))
		if leftDepth == rightDepth {
			return result[i].target < result[j].target
		}
		return leftDepth < rightDepth
	})
	return result
}

func linuxMountParentDirectories(mounts []linuxReadOnlyMount) []string {
	seen := make(map[string]struct{})
	for _, mount := range mounts {
		for current := filepath.Dir(mount.target); current != string(filepath.Separator) && current != "."; current = filepath.Dir(current) {
			seen[current] = struct{}{}
		}
	}
	directories := make([]string, 0, len(seen))
	for dir := range seen {
		directories = append(directories, dir)
	}
	sort.Slice(directories, func(i, j int) bool {
		leftDepth := strings.Count(directories[i], string(filepath.Separator))
		rightDepth := strings.Count(directories[j], string(filepath.Separator))
		if leftDepth == rightDepth {
			return directories[i] < directories[j]
		}
		return leftDepth < rightDepth
	})
	return directories
}

func linuxPathWithin(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func linuxUniquePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if !filepath.IsAbs(path) {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	return result
}

func cleanLinuxMountPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return filepath.Clean(p)
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
