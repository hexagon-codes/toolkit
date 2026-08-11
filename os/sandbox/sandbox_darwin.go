//go:build darwin

package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const darwinRuntimeDirectory = posixRuntimeDirectory

var (
	// 系统运行时只读路径不包含用户可写的全局临时目录和 var 数据树。
	darwinSystemReadSubpaths = []string{
		"/System/Library",
		"/System/Volumes/Preboot/Cryptexes/OS",
		"/bin",
		"/sbin",
		"/usr/bin",
		"/usr/sbin",
		"/usr/lib",
		"/usr/libexec",
		"/usr/share",
		"/Library/Apple",
		"/Library/Frameworks",
		"/private/var/db/timezone",
	}
	darwinSystemExecSubpaths = []string{
		"/System/Library",
		"/System/Volumes/Preboot/Cryptexes/OS",
		"/bin",
		"/sbin",
		"/usr/bin",
		"/usr/sbin",
		"/usr/libexec",
		"/Library/Apple",
		"/Library/Frameworks",
	}
	darwinSystemExecutableMapSubpaths = []string{
		"/System/Library",
		"/System/Volumes/Preboot/Cryptexes/OS",
		"/bin",
		"/sbin",
		"/usr/bin",
		"/usr/sbin",
		"/usr/lib",
		"/usr/libexec",
		"/Library/Apple",
		"/Library/Frameworks",
	}
	darwinSystemReadLiterals = []string{
		"/dev/null",
		"/dev/random",
		"/dev/urandom",
		"/dev/zero",
		"/etc/group",
		"/etc/hosts",
		"/etc/passwd",
		"/etc/protocols",
		"/etc/resolv.conf",
		"/etc/services",
		"/private/etc/group",
		"/private/etc/hosts",
		"/private/etc/passwd",
		"/private/etc/protocols",
		"/private/etc/resolv.conf",
		"/private/etc/services",
		"/private/var/select/sh",
		"/var/select/sh",
	}
	darwinSystemWriteDataLiterals = []string{
		"/dev/null",
	}
)

// darwinSandbox 使用 macOS Seatbelt 提供 deny-default 的 no-child 单进程沙箱。
// Seatbelt 先于载荷环境和载荷程序生效；同一 PID 可以 exec，但不能派生后代。
type darwinSandbox struct {
	cfg        Config
	executions posixExecutionRegistry
	// afterCommandPlan 仅由同包确定性安全测试在构造后、首次使用前设置。
	afterCommandPlan func()
}

func newPlatformSandbox(cfg Config) (Sandbox, error) {
	if !isSafeSeatbeltPath(cfg.Workspace) {
		return nil, fmt.Errorf("sandbox workspace cannot be represented safely in a macOS profile")
	}
	for _, path := range append(append([]string(nil), cfg.ReadablePaths...), cfg.DeniedPaths...) {
		if !isSafeSeatbeltPath(path) {
			return nil, fmt.Errorf("sandbox path cannot be represented safely in a macOS profile")
		}
	}
	return newDarwinSandbox(cfg), nil
}

func newDarwinSandbox(cfg Config) *darwinSandbox {
	cfg.Workspace = darwinCanonicalPath(cfg.Workspace)
	return &darwinSandbox{cfg: cfg}
}

// generateSBPL 生成 Seatbelt Profile Language 策略
func (s *darwinSandbox) generateSBPL() string {
	return s.generateSBPLWithRuntimePaths(nil)
}

func (s *darwinSandbox) generateSBPLWithRuntimePaths(runtimePaths []string) string {
	workspace := darwinCanonicalPath(s.cfg.Workspace)
	systemReadLiterals := darwinResolvedSystemReadLiterals()
	runtimePaths = darwinUniquePaths(runtimePaths)

	var sb strings.Builder
	sb.WriteString("(version 1)\n")
	sb.WriteString("(deny default)\n")

	// 默认 no-child 策略只允许同一 PID exec；可信构建策略显式允许工具链派生子进程。
	sb.WriteString("(allow process-exec)\n")
	if s.cfg.ExecutionProfile == ExecutionProfileTrustedBuild {
		sb.WriteString("(allow process-fork)\n")
	} else {
		sb.WriteString("(deny process-fork)\n")
	}
	sb.WriteString("(allow sysctl-read)\n")
	sb.WriteString("(allow signal (target same-sandbox))\n")

	// 允许 dyld 将可执行映像 mmap 进内存。
	// macOS 26+ 在 (deny default) 下若不显式授予 file-map-executable,
	// dyld 加载共享缓存/可执行段时会被 SIGABRT, 任何二进制都无法启动。
	sb.WriteString("(allow file-map-executable)\n")

	// 允许读取根目录 inode 本身。
	// 枚举式 file-read* 子路径只授予子项访问权, 不含根目录 "/" 自身;
	// 而 dyld 在路径解析阶段需要 stat/read "/", 缺失会导致进程在
	// dyld 阶段 SIGABRT (macOS 26+ 上整个沙箱不可用的真正根因)。
	sb.WriteString("(allow file-read* (literal \"/\"))\n")
	sb.WriteString("(allow file-read* (literal \"/private\"))\n")

	// 仅放行系统二进制、动态链接库、框架和时区数据；var/tmp 不得广域放行。
	sb.WriteString("(allow file-read*\n")
	for _, path := range darwinSystemReadSubpaths {
		fmt.Fprintf(&sb, "  (subpath \"%s\")\n", path)
	}
	for _, path := range runtimePaths {
		if isSafeSeatbeltPath(path) {
			fmt.Fprintf(&sb, "  (literal \"%s\")\n", path)
			fmt.Fprintf(&sb, "  (subpath \"%s\")\n", path)
		}
	}
	sb.WriteString(")\n")
	for _, path := range systemReadLiterals {
		fmt.Fprintf(&sb, "(allow file-read* (literal \"%s\"))\n", path)
	}
	// 部分运行时会把标准流定向到空设备，仅授予该设备数据写入。
	for _, path := range darwinSystemWriteDataLiterals {
		fmt.Fprintf(&sb, "(allow file-write-data (literal \"%s\"))\n", path)
	}
	metadataPaths := make([]string, 0, len(darwinSystemReadSubpaths)+len(systemReadLiterals)+len(runtimePaths)+1+len(s.cfg.ReadablePaths))
	metadataPaths = append(metadataPaths, darwinSystemReadSubpaths...)
	metadataPaths = append(metadataPaths, systemReadLiterals...)
	metadataPaths = append(metadataPaths, runtimePaths...)
	metadataPaths = append(metadataPaths, workspace)
	metadataPaths = append(metadataPaths, s.cfg.ReadablePaths...)
	for _, path := range darwinPathAncestors(metadataPaths) {
		fmt.Fprintf(&sb, "(allow file-read-metadata (literal \"%s\"))\n", path)
	}

	// 工作区读写
	fmt.Fprintf(&sb, "(allow file-read* (subpath \"%s\"))\n", workspace)
	fmt.Fprintf(&sb, "(allow file-write* (subpath \"%s\"))\n", workspace)

	// 额外授权目录：只读放行（用户经数据连接器等显式授权的本地目录，让 code_exec 能读到）。
	// 仅 file-read*（不授写），且写在 DeniedPaths 的 deny 之前——后写的 deny 规则在 seatbelt 里优先生效。
	// 安全：路径来自连接器自由文本，必须先过 isSafeSeatbeltPath——含 `"`/`\`/换行的路径会终止/损坏
	// SBPL 字面量（轻则整张 profile 失效搞瘫所有 code_exec，重则注入 (allow network*) 之类逃逸沙箱），
	// 非绝对路径写进 subpath 也无意义。非法者跳过，绝不污染 profile。
	for _, readable := range s.cfg.ReadablePaths {
		expanded := expandPath(readable)
		if !isSafeSeatbeltPath(expanded) {
			continue
		}
		fmt.Fprintf(&sb, "(allow file-read* (subpath \"%s\"))\n", expanded)
	}

	// 明确拒绝的路径
	for _, denied := range s.cfg.DeniedPaths {
		expanded := expandPath(denied)
		if !isSafeSeatbeltPath(expanded) {
			continue
		}
		fmt.Fprintf(&sb, "(deny file-read* (subpath \"%s\"))\n", expanded)
		fmt.Fprintf(&sb, "(deny file-write* (subpath \"%s\"))\n", expanded)
	}

	// 基础执行与可执行映射能力由后置补集 deny 收敛到同一组受控路径。
	writeDarwinExecutableBoundaries(&sb, workspace, runtimePaths)

	// 网络控制
	if s.cfg.Network {
		sb.WriteString("(allow network*)\n")
	} else {
		sb.WriteString("(deny network*)\n")
	}

	return sb.String()
}

// Exec 在 Seatbelt 沙箱内执行命令
func (s *darwinSandbox) Exec(ctx context.Context, requested Command) (*ExecResult, error) {
	if err := validateExecContext(ctx); err != nil {
		return nil, err
	}
	if err := s.executions.ensureReady(); err != nil {
		return nil, err
	}
	// 应用 cfg.Timeout: 调用方 ctx 无更早 deadline 时按配置强制超时。
	ctx, cancel := withTimeout(ctx, s.cfg.Timeout)
	defer cancel()
	if err := checkPOSIXPreparationContext(ctx, "prepare macOS execution"); err != nil {
		return nil, err
	}

	command, err := prepareSandboxCommand(s.cfg, requested, func() ([]string, error) {
		return darwinExecEnv(s.cfg.Workspace, os.Environ())
	})
	if err != nil {
		return nil, err
	}
	if contextErr := checkPOSIXPreparationContext(ctx, "prepare macOS execution"); contextErr != nil {
		return nil, contextErr
	}
	commandPlan := darwinCommandExecutionPlanContext(ctx, command, s.cfg.Workspace)
	if commandPlan.err != nil {
		return nil, commandPlan.err
	}
	if s.afterCommandPlan != nil {
		s.afterCommandPlan()
	}
	if contextErr := checkPOSIXPreparationContext(ctx, "prepare macOS execution"); contextErr != nil {
		return nil, contextErr
	}
	guard, err := newDarwinExecutionGuardFromPlanContext(ctx, s.cfg.Workspace, commandPlan)
	if err != nil {
		return nil, err
	}
	runner, err := s.darwinSeatbeltRunnerFromPlan(commandPlan)
	if err != nil {
		return nil, err
	}

	return s.executions.runBoundedPreparedCommandWithPreflight(
		ctx,
		runner,
		s.cfg,
		s.darwinExecutionCapabilities(),
		func(ctx context.Context) error { return guard.RevalidateContext(ctx) },
	)
}

func (s *darwinSandbox) darwinExecutionCapabilities() posixExecutionCapabilities {
	if s.cfg.ExecutionProfile == ExecutionProfileTrustedBuild {
		// Seatbelt 仍约束文件和网络，但原生 Darwin 无法原子确认任意后代集合已经清空。
		return posixExecutionCapabilities{
			Filesystem:          LimitStatusEnforced,
			Network:             LimitStatusEnforced,
			Processes:           LimitStatusUnsupported,
			ProcessContainment:  LimitStatusUnsupported,
			processGroupInspect: inspectDarwinProcessGroup,
		}
	}
	return posixExecutionCapabilities{
		Filesystem: LimitStatusEnforced,
		Network:    LimitStatusEnforced,
		// Seatbelt 在载荷开始前拒绝 process-fork，因此单次执行的真实进程数上限为一。
		// Processes 表示该数量边界；ProcessContainment 表示不存在后代且根进程可被跟踪收敛。
		// 这里不依赖 RLIMIT_NPROC，也不把进程组近似保证混入任一状态。
		Processes:           LimitStatusEnforced,
		ProcessContainment:  LimitStatusEnforced,
		processGroupInspect: inspectDarwinProcessGroup,
	}
}

func (s *darwinSandbox) sandboxCapabilities(ctx context.Context) (CapabilitySet, error) {
	if err := validateExecContext(ctx); err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("inspect macOS sandbox capabilities: %w", err)
	}
	executionCapabilities := s.darwinExecutionCapabilities()
	memory, processes, capabilityErr := posixResourceLimitCapabilitiesContext(ctx, executionCapabilities)
	if capabilityErr != nil {
		return 0, capabilityErr
	}
	available := CapabilityFilesystem | CapabilityNetwork | CapabilityOutput
	if s.cfg.MaxMemoryBytes > 0 && memory == LimitStatusEnforced {
		available |= CapabilityMemory
	}
	if s.cfg.MaxProcesses > 0 && processes == LimitStatusEnforced {
		available |= CapabilityProcesses
	}
	if executionCapabilities.ProcessContainment == LimitStatusEnforced {
		available |= CapabilityProcessContainment
	}
	if s.cfg.ExecutionProfile == ExecutionProfileTrustedBuild && processCreationFitsBudget(s.cfg.MaxProcesses) {
		available |= CapabilityProcessCreation
	}
	return available, nil
}

func (s *darwinSandbox) darwinSeatbeltRunner(command Command) (Command, error) {
	commandPlan := darwinCommandExecutionPlan(command, s.cfg.Workspace)
	return s.darwinSeatbeltRunnerFromPlan(commandPlan)
}

func (s *darwinSandbox) darwinSeatbeltRunnerFromPlan(commandPlan darwinCommandPlan) (Command, error) {
	if commandPlan.err != nil {
		return Command{}, commandPlan.err
	}
	payloadCommand, err := darwinNoChildPayloadCommand(commandPlan.command)
	if err != nil {
		return Command{}, err
	}
	runtimePaths := append([]string(nil), commandPlan.readPaths...)
	sbpl := s.generateSBPLWithRuntimePaths(runtimePaths)
	sandboxExec, err := darwinSandboxExecPath()
	if err != nil {
		return Command{}, err
	}

	sandboxArgs := make([]string, 0, 3+len(payloadCommand.Args))
	sandboxArgs = append(sandboxArgs, "-p", sbpl, payloadCommand.Path)
	sandboxArgs = append(sandboxArgs, payloadCommand.Args...)

	return Command{
		Path: sandboxExec,
		Args: sandboxArgs,
		Dir:  payloadCommand.Dir,
		Env:  payloadCommand.Env,
	}, nil
}

func darwinNoChildPayloadCommand(command Command) (Command, error) {
	environmentLauncher, err := resolveTrustedPOSIXLauncher("macOS sandbox environment launcher", []string{"/usr/bin/env"})
	if err != nil {
		return Command{}, err
	}
	args := make([]string, 0, len(command.Env)+len(command.Args)+3)
	// sandbox-exec 只继承下方固定环境；载荷环境由受约束的 env 在 Seatbelt 内安装。
	args = append(args, "-i", "--")
	args = append(args, command.Env...)
	args = append(args, command.Path)
	args = append(args, command.Args...)
	return Command{
		Path: environmentLauncher,
		Args: args,
		Dir:  command.Dir,
		Env: []string{
			"HOME=/var/empty",
			"LANG=C",
			"LC_ALL=C",
			"PATH=/usr/bin:/bin",
		},
	}, nil
}

func darwinExecEnv(workspace string, env []string) ([]string, error) {
	return buildPOSIXSandboxEnv(workspace, env, darwinSandboxPath())
}

func darwinSandboxPath() string {
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
			if err != nil || !darwinSystemExecutable(resolved) && darwinRuntimeRoot(name, resolved) == "" {
				continue
			}
			paths = append(paths, filepath.Dir(resolved))
			break
		}
	}
	paths = append(paths, "/usr/bin", "/bin", "/usr/sbin", "/sbin")
	return strings.Join(darwinUniquePaths(paths), string(os.PathListSeparator))
}

func darwinSandboxExecPath() (string, error) {
	return resolveTrustedPOSIXLauncher("sandbox-exec", []string{"/usr/bin/sandbox-exec"})
}

type darwinCommandPlan struct {
	command         Command
	commandIdentity darwinPathIdentity
	readPaths       []string
	err             error
}

func darwinCommandExecutionPlan(command Command, workspace string) darwinCommandPlan {
	return darwinCommandExecutionPlanContext(context.Background(), command, workspace)
}

func darwinCommandExecutionPlanContext(ctx context.Context, command Command, workspace string) darwinCommandPlan {
	workspace = darwinCanonicalPath(workspace)
	plan := darwinCommandPlan{command: command}
	if err := checkPOSIXPreparationContext(ctx, "resolve macOS command plan"); err != nil {
		plan.err = err
		return plan
	}
	resolved, err := resolvePOSIXCommandExecutable(command.Path, command.Dir)
	if err != nil {
		plan.err = fmt.Errorf("resolve sandbox command %q", command.Path)
		return plan
	}
	plan.command.Path = resolved
	plan.commandIdentity, err = captureDarwinPathIdentity(resolved)
	if err != nil {
		plan.err = err
		return plan
	}
	plan.commandIdentity.freezeData = true
	if err := checkPOSIXPreparationContext(ctx, "resolve macOS command plan"); err != nil {
		plan.err = err
		return plan
	}

	trusted := darwinPathWithin(workspace, resolved) || darwinSystemExecutable(resolved)
	if !trusted {
		root := darwinRuntimeRoot(command.Path, resolved)
		if root == "" || darwinTemporaryOrVariablePath(resolved) {
			plan.err = fmt.Errorf("sandbox command %q is outside the workspace and trusted runtime roots", command.Path)
			return plan
		}
		plan.readPaths = append(plan.readPaths, resolved, root)
	}

	interpreters, shebangErr := inspectPOSIXShebangCommands(resolved)
	if shebangErr != nil {
		plan.err = fmt.Errorf("inspect sandbox command %q shebang: %w", command.Path, shebangErr)
		return plan
	}
	for _, interpreter := range interpreters {
		if err := checkPOSIXPreparationContext(ctx, "inspect macOS command interpreters"); err != nil {
			plan.err = err
			return plan
		}
		interpreterPath, interpreterErr := resolvePOSIXShebangExecutable(interpreter, command.Dir, command.Env)
		if interpreterErr != nil {
			plan.err = fmt.Errorf("sandbox command %q uses an unavailable interpreter %q", command.Path, interpreter)
			return plan
		}
		if darwinPathWithin(workspace, interpreterPath) || darwinSystemExecutable(interpreterPath) {
			continue
		}
		root := darwinRuntimeRoot(interpreter, interpreterPath)
		if root == "" || darwinTemporaryOrVariablePath(interpreterPath) {
			plan.err = fmt.Errorf("sandbox command %q uses an untrusted interpreter %q", command.Path, interpreter)
			return plan
		}
		plan.readPaths = append(plan.readPaths, interpreterPath, root)
	}
	plan.readPaths = darwinUniquePaths(plan.readPaths)
	return plan
}

func darwinCommandReadPaths(command Command, workspace string) []string {
	return darwinCommandExecutionPlan(command, workspace).readPaths
}

func darwinPathAncestors(paths []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(paths)*2)
	for _, path := range paths {
		if !isSafeSeatbeltPath(path) {
			continue
		}
		for current := filepath.Clean(path); current != string(filepath.Separator); current = filepath.Dir(current) {
			if _, exists := seen[current]; exists {
				continue
			}
			seen[current] = struct{}{}
			result = append(result, current)
		}
	}
	return result
}

func darwinCanonicalPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil && isSafeSeatbeltPath(resolved) {
		return resolved
	}
	return path
}

func darwinResolvedSystemReadLiterals() []string {
	paths := append([]string(nil), darwinSystemReadLiterals...)
	for _, path := range darwinSystemReadLiterals {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil && isSafeSeatbeltPath(resolved) {
			paths = append(paths, resolved)
		}
	}
	return darwinUniquePaths(paths)
}

func darwinUniquePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if !isSafeSeatbeltPath(path) {
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

func darwinSystemExecutable(executable string) bool {
	for _, root := range darwinSystemExecSubpaths {
		if darwinPathWithin(root, executable) {
			return true
		}
	}
	return false
}

func darwinRuntimeRoot(command, executable string) string {
	name := filepath.Base(command)
	root, runtimeOnly := darwinInstalledRuntimeRoot(executable)
	if root == "" || runtimeOnly && !darwinSupportedRuntimeName(name) {
		return ""
	}
	return root
}

func darwinInstalledRuntimeRoot(executable string) (root string, runtimeOnly bool) {
	cleaned := filepath.Clean(executable)
	for _, cellar := range []string{"/opt/homebrew/Cellar", "/usr/local/Cellar"} {
		if root := darwinRuntimeRootBelow(cleaned, cellar, 2); root != "" {
			return root, false
		}
	}
	if darwinPathWithin("/usr/local/go", cleaned) {
		return "/usr/local/go", false
	}
	if cleaned == "/usr/local/bin/node" || cleaned == "/usr/local/bin/nodejs" {
		return cleaned, true
	}
	for _, modules := range []string{"/opt/homebrew/lib/node_modules", "/usr/local/lib/node_modules"} {
		if root := darwinRuntimeRootBelow(cleaned, modules, 1); root != "" {
			return root, true
		}
	}

	home, err := os.UserHomeDir()
	if err == nil {
		home = darwinCanonicalPath(home)
		if root := darwinRuntimeRootBelow(cleaned, filepath.Join(home, ".pyenv", "versions"), 1); root != "" {
			return root, true
		}
		if root := darwinRuntimeRootBelow(cleaned, filepath.Join(home, ".nvm", "versions", "node"), 1); root != "" {
			return root, true
		}
		toolchains := filepath.Join(home, "go", "pkg", "mod", "golang.org")
		if root := darwinRuntimeRootBelow(cleaned, toolchains, 1); root != "" && strings.HasPrefix(filepath.Base(root), "toolchain@") {
			return root, true
		}
		if root := darwinRuntimeRootBelow(cleaned, filepath.Join(home, "hostedtoolcache"), 3); root != "" {
			return root, true
		}
	}
	if root := darwinRuntimeRootBelow(cleaned, "/opt/hostedtoolcache", 3); root != "" {
		return root, true
	}
	return "", false
}

func darwinRuntimeRootBelow(path, prefix string, depth int) string {
	if depth <= 0 || !darwinPathWithin(prefix, path) {
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

func darwinSupportedRuntimeName(name string) bool {
	name = strings.ToLower(name)
	switch name {
	case "python", "python3", "node", "nodejs", "go", "gofmt", "npm", "npx", "corepack", "pnpm", "yarn", "pip", "pip3":
		return true
	default:
		return strings.HasPrefix(name, "python3.") || strings.HasPrefix(name, "pip3.")
	}
}

func writeDarwinExecutableBoundaries(sb *strings.Builder, workspace string, runtimePaths []string) {
	processPaths := make([]string, 0, len(darwinSystemExecSubpaths)+len(runtimePaths)+1)
	processPaths = append(processPaths, darwinSystemExecSubpaths...)
	processPaths = append(processPaths, workspace)
	processPaths = append(processPaths, runtimePaths...)
	writeDarwinPathComplementDeny(sb, "process-exec*", processPaths)

	mapPaths := make([]string, 0, len(darwinSystemExecutableMapSubpaths)+len(runtimePaths)+1)
	mapPaths = append(mapPaths, darwinSystemExecutableMapSubpaths...)
	mapPaths = append(mapPaths, workspace)
	mapPaths = append(mapPaths, runtimePaths...)
	writeDarwinPathComplementDeny(sb, "file-map-executable", mapPaths)
}

func writeDarwinPathComplementDeny(sb *strings.Builder, operation string, allowed []string) {
	// 后置补集拒绝保留受控启动能力，同时阻止当前工作区之外的载荷执行或映射。
	fmt.Fprintf(sb, "(deny %s (require-not (require-any\n", operation)
	for _, path := range darwinUniquePaths(allowed) {
		fmt.Fprintf(sb, "  (literal \"%s\")\n", path)
		fmt.Fprintf(sb, "  (subpath \"%s\")\n", path)
	}
	sb.WriteString(")))\n")
}

func darwinPathWithin(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func darwinTemporaryOrVariablePath(path string) bool {
	for _, root := range []string{"/tmp", "/private/tmp", "/var", "/private/var"} {
		if darwinPathWithin(root, path) {
			return true
		}
	}
	return false
}

// isSafeSeatbeltPath 判断一个路径能否安全写进 SBPL 的 (subpath "...") 字面量。
//
// 要求：① 绝对路径（subpath 只对绝对路径有意义）② 不含会破坏/注入 SBPL 字符串的字符——
// 双引号 `"`（终止字面量→注入）、反斜杠 `\`（SBPL 转义引导；macOS 路径分隔是 `/`，正常路径不含 `\`）、
// 换行/回车/空字符（截断 profile）。非法路径直接拒绝（跳过放行），宁可少授权也不污染整张 profile。
func isSafeSeatbeltPath(p string) bool {
	if p == "" || !strings.HasPrefix(p, "/") {
		return false
	}
	return !strings.ContainsAny(p, "\"\\\n\r\x00")
}
