// Package sandbox 提供面向本地代码执行的进程、文件和网络隔离能力。
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const maxCrossPlatformProcessLimit = 1<<31 - 1

var (
	// ErrUnsupportedNetworkPolicy 表示当前运行环境无法落实所请求的网络策略。
	ErrUnsupportedNetworkPolicy = errors.New("sandbox: requested network policy is not enforceable")
	// ErrInvalidCapabilityContract 表示调用方没有提供完整、有效的能力合同。
	ErrInvalidCapabilityContract = errors.New("sandbox: invalid required capability contract")
	// ErrRequiredCapabilitiesUnavailable 表示执行前无法证明调用方要求的全部沙箱能力。
	ErrRequiredCapabilitiesUnavailable = errors.New("sandbox: required capabilities are unavailable")
	// ErrInvalidContext 表示执行调用没有提供有效上下文。
	ErrInvalidContext = errors.New("sandbox: context must not be nil")
	// ErrSandboxClosed 表示沙箱已经开始关闭，不能再接受任何新操作。
	ErrSandboxClosed = errors.New("sandbox: closed")
	// ErrOutputDrainTimeout 表示根进程结束后标准输出流未在边界内收敛。
	ErrOutputDrainTimeout = errors.New("sandbox: output drain timed out")
	// ErrProcessReapTimeout 表示根进程未在边界内完成 Wait 回收。
	ErrProcessReapTimeout = errors.New("sandbox: process reap timed out")
	// ErrPOSIXExecutionUnsettled 表示前一次 POSIX 执行仍由沙箱持有并等待回收。
	ErrPOSIXExecutionUnsettled = errors.New("sandbox: previous POSIX execution is not settled")
	// ErrPOSIXExecutionCapacity 表示单个 POSIX 沙箱的并发执行所有权已达到固定上界。
	ErrPOSIXExecutionCapacity = errors.New("sandbox: POSIX execution capacity is exhausted")
	// ErrProcessGroupSurvivedRoot 表示根进程退出时同一受控进程组仍有存活成员。
	ErrProcessGroupSurvivedRoot = errors.New("sandbox: process group survived root exit")
	// ErrProcessGroupSettlement 表示受控进程组未在边界内被证明为空。
	ErrProcessGroupSettlement = errors.New("sandbox: process group did not settle")
)

// NetworkMode 精确定义载荷获得的网络视图。
type NetworkMode bool

const (
	// NetworkDisabled 要求后端阻断载荷的 IP 网络访问。
	NetworkDisabled NetworkMode = false
	// NetworkHost 要求载荷继承宿主网络视图，不附加目的地址过滤。
	NetworkHost NetworkMode = true
)

// String 返回稳定的英文网络模式名称。
func (mode NetworkMode) String() string {
	if mode == NetworkHost {
		return "host"
	}
	return "disabled"
}

// ExecutionProfile 选择与载荷信任级别匹配的进程派生策略。
type ExecutionProfile uint8

const (
	// ExecutionProfileUntrusted 是安全零值，要求后端提供完整进程生命周期收容。
	ExecutionProfileUntrusted ExecutionProfile = iota
	// ExecutionProfileTrustedBuild 仅用于固定可信构建工具，允许平台接受无法收容的后代。
	ExecutionProfileTrustedBuild
)

// String 返回稳定的英文执行配置名称。
func (profile ExecutionProfile) String() string {
	switch profile {
	case ExecutionProfileUntrusted:
		return "untrusted"
	case ExecutionProfileTrustedBuild:
		return "trusted-build"
	default:
		return fmt.Sprintf("unknown(%d)", uint8(profile))
	}
}

func executionProfileRequiredCapabilities(profile ExecutionProfile) (CapabilitySet, error) {
	switch profile {
	case ExecutionProfileUntrusted:
		return CapabilityProcessContainment, nil
	case ExecutionProfileTrustedBuild:
		return CapabilityProcessCreation, nil
	default:
		return 0, fmt.Errorf("%w: execution profile %d is invalid", ErrInvalidCapabilityContract, uint8(profile))
	}
}

func processCreationFitsBudget(maxProcesses int) bool {
	return maxProcesses == 0 || maxProcesses >= 2
}

// CapabilitySet 使用位集合表达执行前要求和后端已经证明的能力。
// 同一个集合模型同时用于协商双方，避免为每项能力维护重复布尔状态。
type CapabilitySet uint16

const (
	// CapabilityFilesystem 表示 deny-by-default 文件系统隔离。
	CapabilityFilesystem CapabilitySet = 1 << iota
	// CapabilityNetwork 表示后端能精确落实所选 NetworkMode。
	CapabilityNetwork
	// CapabilityMemory 表示当前配置的正值内存上限在载荷启动前已经落实。
	CapabilityMemory
	// CapabilityProcesses 表示当前配置的正值进程数量上限在载荷启动前已经落实。
	CapabilityProcesses
	// CapabilityProcessContainment 表示载荷根进程及其后代始终处于不可逃逸的生命周期边界。
	// 后端可以禁止产生后代，也可以可靠收敛完整后代集合；Exec 只有确认全部退出后才能证明该能力。
	CapabilityProcessContainment
	// CapabilityStorage 表示当前配置的正值工作区存储上限已经实时落实。
	CapabilityStorage
	// CapabilityOutput 表示标准输出和标准错误均具有实时硬上限。
	CapabilityOutput
	// CapabilityProcessCreation 表示所选后端策略允许可信载荷派生子进程。
	// 该能力不等同于 ProcessContainment；两者能否同时提供取决于平台边界。
	CapabilityProcessCreation
	// UntrustedCodeIsolationCapabilities 是执行不可信代码必须显式要求的隔离能力集合。
	// 该集合不承诺抗拒绝服务配额；调用方显式请求资源限额时必须追加对应能力。
	UntrustedCodeIsolationCapabilities = CapabilityFilesystem |
		CapabilityNetwork |
		CapabilityProcessContainment |
		CapabilityOutput
	// TrustedBuildIsolationCapabilities 是可信构建配置的便利要求集合。
	// 调用方仍必须选择 ExecutionProfileTrustedBuild；该集合不承诺后代生命周期收容，
	// 绝不能用于执行不可信产物。
	TrustedBuildIsolationCapabilities = CapabilityFilesystem |
		CapabilityNetwork |
		CapabilityOutput |
		CapabilityProcessCreation

	allSandboxCapabilities = CapabilityFilesystem |
		CapabilityNetwork |
		CapabilityMemory |
		CapabilityProcesses |
		CapabilityProcessContainment |
		CapabilityStorage |
		CapabilityOutput |
		CapabilityProcessCreation

	resourceLimitCapabilities = CapabilityMemory |
		CapabilityProcesses |
		CapabilityStorage |
		CapabilityOutput
)

// Has 返回集合是否完整包含目标能力集合。
func (set CapabilitySet) Has(capabilities CapabilitySet) bool {
	return capabilities != 0 && set&capabilities == capabilities
}

// Missing 返回要求集合中尚未由可用集合证明的能力。
func (set CapabilitySet) Missing(available CapabilitySet) CapabilitySet {
	return set &^ available
}

// String 返回稳定、可审计的英文能力名称列表。
func (set CapabilitySet) String() string {
	names := make([]string, 0, 8)
	for _, item := range []struct {
		capability CapabilitySet
		name       string
	}{
		{CapabilityFilesystem, "filesystem"},
		{CapabilityNetwork, "network"},
		{CapabilityMemory, "memory"},
		{CapabilityProcesses, "processes"},
		{CapabilityProcessContainment, "process-containment"},
		{CapabilityStorage, "storage"},
		{CapabilityOutput, "output"},
		{CapabilityProcessCreation, "process-creation"},
	} {
		if set.Has(item.capability) {
			names = append(names, item.name)
		}
	}
	if unknown := set &^ allSandboxCapabilities; unknown != 0 {
		names = append(names, fmt.Sprintf("unknown(0x%x)", uint16(unknown)))
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ",")
}

func validateCapabilitySet(set CapabilitySet) error {
	if unknown := set &^ allSandboxCapabilities; unknown != 0 {
		return fmt.Errorf("sandbox capabilities contain unknown bits: 0x%x", uint16(unknown))
	}
	return nil
}

func requireSandboxCapabilities(required, available CapabilitySet) error {
	if err := validateCapabilitySet(required); err != nil {
		return err
	}
	if err := validateCapabilitySet(available); err != nil {
		return err
	}
	missing := required.Missing(available)
	if missing == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrRequiredCapabilitiesUnavailable, missing)
}

func requestedResourceCapabilities(cfg Config) CapabilitySet {
	var required CapabilitySet
	if cfg.MaxOutputBytes > 0 || cfg.MaxStderrBytes > 0 {
		required |= CapabilityOutput
	}
	if cfg.MaxMemoryBytes > 0 {
		required |= CapabilityMemory
	}
	if cfg.MaxProcesses > 0 {
		required |= CapabilityProcesses
	}
	if cfg.MaxWorkspaceBytes > 0 || cfg.MaxArtifactBytes > 0 {
		required |= CapabilityStorage
	}
	return required
}

func validateRequiredCapabilityContract(cfg Config) error {
	if err := validateCapabilitySet(cfg.RequiredCapabilities); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCapabilityContract, err)
	}
	if cfg.RequiredCapabilities == 0 {
		return fmt.Errorf("%w: required capabilities must not be empty", ErrInvalidCapabilityContract)
	}
	configured := requestedResourceCapabilities(cfg)
	declared := cfg.RequiredCapabilities & resourceLimitCapabilities
	missing := configured.Missing(declared)
	if missing != 0 {
		return fmt.Errorf("%w: requested resource limits require %s", ErrInvalidCapabilityContract, missing)
	}
	unconfigured := declared.Missing(configured)
	if unconfigured != 0 {
		return fmt.Errorf("%w: resource capabilities require configured positive limits: %s", ErrInvalidCapabilityContract, unconfigured)
	}
	return nil
}

// Config 沙箱配置
type Config struct {
	Workspace   string   `yaml:"workspace"`    // 工作区目录 (可读写)
	Timeout     int      `yaml:"timeout"`      // 超时秒数，默认 60
	DeniedPaths []string `yaml:"denied_paths"` // 禁止访问的路径
	// ReadablePaths 额外授予「只读」访问的宿主路径（在 Workspace 之外）。
	// 用途：用户经数据连接器等显式授权的本地目录，需让沙箱内代码 (code_exec) 能读到。
	// 语义：deny-default 沙箱里为每个路径追加只读放行（darwin: file-read* subpath）；
	// 不授予写权限（写仅限 Workspace）。DeniedPaths 的 deny 规则写在放行之后、保持优先。
	ReadablePaths []string `yaml:"readable_paths"`
	// Network 只接受两种明确语义：disabled 阻断 IP 网络，host 继承完整宿主网络。
	// 无法证明所选语义的平台必须在载荷启动前拒绝，不能静默映射为近似能力。
	Network NetworkMode `yaml:"network"`
	// ExecutionProfile 独立选择进程派生策略；零值是不允许策略降级的不可信载荷配置。
	ExecutionProfile ExecutionProfile `yaml:"execution_profile"`
	// RequiredCapabilities 声明本次载荷启动前必须证明的非空能力集合。
	// 它只追加单调能力断言，不能改变 ExecutionProfile 对应的平台隔离策略。
	// 不可信代码从 UntrustedCodeIsolationCapabilities 开始；显式资源配额还要追加对应能力。
	RequiredCapabilities CapabilitySet `yaml:"required_capabilities"`

	// MaxOutputBytes 与 MaxStderrBytes 为零时使用安全的有界输出默认值。
	// Memory、Processes 与 Storage 类限额为零时表示未请求且不设置上限；正值必须声明对应能力。
	MaxOutputBytes    int64 `yaml:"max_output_bytes"`
	MaxStderrBytes    int64 `yaml:"max_stderr_bytes"`
	MaxWorkspaceBytes int64 `yaml:"max_workspace_bytes"`
	MaxArtifactBytes  int64 `yaml:"max_artifact_bytes"`
	MaxMemoryBytes    int64 `yaml:"max_memory_bytes"`
	// MaxProcesses 是包含载荷根进程在内的同时存活进程总预算。
	// 只有能够证明该树级上界的平台才会报告 CapabilityProcesses。
	MaxProcesses int `yaml:"max_processes"`

	workspaceIdentity sandboxPathIdentity
}

// LimitStatus 标记某项资源限制在当前平台/后端是否真实生效
type LimitStatus string

const (
	// LimitStatusNotRequested 表示调用方没有为本次执行请求该项可选资源配额。
	LimitStatusNotRequested LimitStatus = "not_requested"
	// LimitStatusEnforced 表示资源限制已被当前后端真实执行。
	LimitStatusEnforced LimitStatus = "enforced"
	// LimitStatusUnsupported 表示当前平台或后端无法执行该资源限制。
	LimitStatusUnsupported LimitStatus = "unsupported"
)

func requestedLimitStatus(requested bool, capability LimitStatus) LimitStatus {
	if !requested {
		return LimitStatusNotRequested
	}
	return capability
}

// LimitReport 记录本次执行结束后的实际状态，仅用于结果审计；执行准入只使用
// RequiredCapabilities 与执行前 CapabilitySet，不能依赖本报告事后拒载。
type LimitReport struct {
	Network   LimitStatus
	Memory    LimitStatus
	Processes LimitStatus
	// ProcessContainment 表示根进程及其后代在整个执行期间不可逃逸，且返回前已经全部退出。
	// 该能力独立于 Processes 数量限制，不得用 RLIMIT_NPROC 等数量上限替代。
	ProcessContainment LimitStatus
	Storage            LimitStatus // MaxWorkspaceBytes/MaxArtifactBytes 的实时硬上限
	Output             LimitStatus // stdout/stderr 有界缓冲
	// Filesystem 文件系统隔离(deny-by-default containment)的实际强度:
	//   enforced    — 强隔离(darwin Seatbelt / linux bubblewrap / windows ACL);
	//   unsupported — 无 OS 级文件系统隔离(basic 后端)。
	// unsupported 信号供上层(code_exec)决策是否拒载机密任务。
	Filesystem LimitStatus
}

// ExecResult 沙箱执行结果
type ExecResult struct {
	Stdout          string
	Stderr          string
	ExitCode        int
	StdoutBytes     int64
	StderrBytes     int64
	StdoutTruncated bool
	StderrTruncated bool
	// Limits 逐项报告本次执行中资源限制的实际执行状态。
	// 平台不支持的项(如 darwin 无法下调内存 rlimit)标 unsupported,
	// 由调用方作为能力缺口显式上报, 不许静默假装已生效。
	Limits LimitReport
}

// Sandbox 仅负责结构化命令的隔离执行与生命周期收敛；语言构建和源码管理由调用方编排。
type Sandbox interface {
	// Exec 在沙箱内执行命令
	Exec(ctx context.Context, command Command) (*ExecResult, error)

	// Close 等待已经进入的操作完成，再确定性释放沙箱持有的资源。
	Close() error
}

// CapabilityReporter 在执行前公开当前后端可以证明的能力集合。
type CapabilityReporter interface {
	Capabilities(ctx context.Context) (CapabilitySet, error)
}

type sandboxCapabilitySource interface {
	sandboxCapabilities(ctx context.Context) (CapabilitySet, error)
}

// sandboxRetryableCloser 仅由关闭失败后仍安全持有资源、允许调用方再次收敛的后端实现。
type sandboxRetryableCloser interface {
	sandboxCloseRetryable()
}

type capabilitySandbox struct {
	backend   Sandbox
	cfg       Config
	lifecycle sandboxLifecycle
}

// sandboxLifecycle 以引用计数跟踪已经进入的操作。
// closing 在线性化点先置位；等待活动操作时不持有互斥锁，因此新操作无需等待
// 旧操作结束即可稳定返回 ErrSandboxClosed。
type sandboxLifecycle struct {
	mu        sync.Mutex
	closing   bool
	active    int
	drained   chan struct{}
	closeOnce sync.Once
	closeErr  error
	// closeStartOnce 保证普通关闭与可重试关闭共用同一个拒绝新操作、等待活动操作的线性化点。
	closeStartOnce sync.Once
	retryCloseMu   sync.Mutex
	retryComplete  bool
	// beforeCloseOnce 仅供同包并发测试在调用 closeOnce.Do 前取得确定性回执。
	// 生产环境保持 nil；如需测试注入，必须在并发使用生命周期之前完成设置且不再修改。
	beforeCloseOnce func()
}

func (lifecycle *sandboxLifecycle) begin() (func(), error) {
	lifecycle.mu.Lock()
	if lifecycle.closing {
		lifecycle.mu.Unlock()
		return nil, ErrSandboxClosed
	}
	lifecycle.active++
	lifecycle.mu.Unlock()
	return lifecycle.end, nil
}

func (lifecycle *sandboxLifecycle) end() {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	lifecycle.active--
	if lifecycle.closing && lifecycle.active == 0 && lifecycle.drained != nil {
		close(lifecycle.drained)
		lifecycle.drained = nil
	}
}

func (lifecycle *sandboxLifecycle) close(closeBackend func() error) error {
	if lifecycle.beforeCloseOnce != nil {
		lifecycle.beforeCloseOnce()
	}
	lifecycle.closeOnce.Do(func() {
		lifecycle.startClosing()
		lifecycle.closeErr = closeSandboxBackend(closeBackend)
	})
	return lifecycle.closeErr
}

func (lifecycle *sandboxLifecycle) closeRetryable(closeBackend func() error) error {
	lifecycle.startClosing()
	lifecycle.retryCloseMu.Lock()
	defer lifecycle.retryCloseMu.Unlock()
	if lifecycle.retryComplete {
		return nil
	}
	lifecycle.closeErr = closeSandboxBackend(closeBackend)
	if lifecycle.closeErr == nil {
		lifecycle.retryComplete = true
	}
	return lifecycle.closeErr
}

func (lifecycle *sandboxLifecycle) startClosing() {
	lifecycle.closeStartOnce.Do(func() {
		lifecycle.mu.Lock()
		lifecycle.closing = true
		if lifecycle.active > 0 {
			lifecycle.drained = make(chan struct{})
		}
		drained := lifecycle.drained
		lifecycle.mu.Unlock()
		if drained != nil {
			<-drained
		}
	})
}

const closeBackendPanicMessage = "sandbox: backend close panicked"

// closeBackendPanicError 保存后端 Close 的原始 panic 值，但错误文本不调用该值的
// Error、String 或 Format 方法，避免恢复路径再次 panic。
type closeBackendPanicError struct {
	value any
	cause error
}

func (err *closeBackendPanicError) Error() string {
	return closeBackendPanicMessage
}

func (err *closeBackendPanicError) Unwrap() error {
	return err.cause
}

func (err *closeBackendPanicError) PanicValue() any {
	return err.value
}

func closeSandboxBackend(closeBackend func() error) (err error) {
	normalReturn := false
	defer func() {
		recovered := recover()
		if normalReturn {
			return
		}
		panicErr := &closeBackendPanicError{value: recovered}
		if cause, ok := recovered.(error); ok {
			panicErr.cause = cause
		}
		err = panicErr
	}()
	err = closeBackend()
	normalReturn = true
	return err
}

func (lifecycle *sandboxLifecycle) isClosing() bool {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	return lifecycle.closing
}

func (s *capabilitySandbox) Capabilities(ctx context.Context) (CapabilitySet, error) {
	release, err := s.lifecycle.begin()
	if err != nil {
		return 0, err
	}
	defer release()
	return s.capabilities(ctx)
}

func (s *capabilitySandbox) capabilities(ctx context.Context) (CapabilitySet, error) {
	if err := validateExecContext(ctx); err != nil {
		return 0, err
	}
	source, ok := s.backend.(sandboxCapabilitySource)
	if !ok {
		return 0, nil
	}
	available, err := source.sandboxCapabilities(ctx)
	if err != nil {
		return 0, err
	}
	if err := validateCapabilitySet(available); err != nil {
		return 0, err
	}
	return available, nil
}

func (s *capabilitySandbox) Exec(ctx context.Context, requested Command) (*ExecResult, error) {
	release, lifecycleErr := s.lifecycle.begin()
	if lifecycleErr != nil {
		return nil, lifecycleErr
	}
	defer release()
	if err := validateExecContext(ctx); err != nil {
		return nil, err
	}
	command, err := snapshotSandboxCommandPaths(s.cfg, requested)
	if err != nil {
		return nil, err
	}
	if err := revalidateSandboxExecutionPaths(command); err != nil {
		return nil, err
	}
	available, capabilityErr := s.capabilities(ctx)
	if capabilityErr != nil {
		return nil, capabilityErr
	}
	profileRequired, profileErr := executionProfileRequiredCapabilities(s.cfg.ExecutionProfile)
	if profileErr != nil {
		return nil, profileErr
	}
	required := s.cfg.RequiredCapabilities | profileRequired
	if capabilityErr := requireSandboxCapabilities(required, available); capabilityErr != nil {
		return nil, capabilityErr
	}
	return s.backend.Exec(ctx, command)
}

// Close 等待所有已经进入的操作完成。普通后端只关闭一次；显式实现可重试合同的
// 后端在失败后仍拒绝新操作，但允许后续 Close 继续收敛其保留资源。
func (s *capabilitySandbox) Close() error {
	if _, ok := s.backend.(sandboxRetryableCloser); ok {
		return s.lifecycle.closeRetryable(s.backend.Close)
	}
	return s.lifecycle.close(s.backend.Close)
}

// New 创建当前平台的沙箱实例
func New(cfg Config) (Sandbox, error) {
	var err error
	cfg, err = validateSandboxConfigSemantics(cfg)
	if err != nil {
		return nil, err
	}

	workspace, workspaceIdentity, err := snapshotSandboxWorkspace(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	cfg.Workspace = workspace
	cfg.workspaceIdentity = workspaceIdentity
	if cfg.ReadablePaths, err = normalizeSandboxPaths("readable path", cfg.ReadablePaths); err != nil {
		return nil, err
	}
	if cfg.DeniedPaths, err = normalizeSandboxPaths("denied path", cfg.DeniedPaths); err != nil {
		return nil, err
	}
	backend, err := newPlatformSandbox(cfg)
	if err != nil {
		return nil, err
	}
	return &capabilitySandbox{backend: backend, cfg: cfg}, nil
}

// validateSandboxConfigSemantics 在任何文件系统副作用之前完成纯配置语义校验。
func validateSandboxConfigSemantics(cfg Config) (Config, error) {
	if cfg.Workspace == "" {
		return Config{}, fmt.Errorf("sandbox workspace is required")
	}
	if cfg.Timeout < 0 {
		return Config{}, fmt.Errorf("sandbox timeout must not be negative")
	}
	if uint64(cfg.Timeout) > uint64((1<<63-1)/int64(time.Second)) {
		return Config{}, fmt.Errorf("sandbox timeout exceeds the duration limit")
	}
	if cfg.MaxOutputBytes < 0 || cfg.MaxStderrBytes < 0 || cfg.MaxWorkspaceBytes < 0 ||
		cfg.MaxArtifactBytes < 0 || cfg.MaxMemoryBytes < 0 || cfg.MaxProcesses < 0 {
		return Config{}, fmt.Errorf("sandbox resource limits must not be negative")
	}
	if cfg.MaxProcesses > maxCrossPlatformProcessLimit {
		return Config{}, fmt.Errorf("sandbox max processes exceeds the cross-platform limit")
	}
	if _, err := executionProfileRequiredCapabilities(cfg.ExecutionProfile); err != nil {
		return Config{}, err
	}
	if cfg.ExecutionProfile == ExecutionProfileTrustedBuild &&
		cfg.MaxProcesses > 0 && !processCreationFitsBudget(cfg.MaxProcesses) {
		return Config{}, fmt.Errorf("%w: trusted-build execution profile requires MaxProcesses to be at least 2", ErrInvalidCapabilityContract)
	}
	workspacePath, err := absoluteSandboxPath("workspace", cfg.Workspace)
	if err != nil {
		return Config{}, err
	}
	if isFilesystemRoot(workspacePath) {
		return Config{}, fmt.Errorf("sandbox workspace must not be a filesystem root")
	}
	if err := validateSandboxPathInputs("readable path", cfg.ReadablePaths); err != nil {
		return Config{}, err
	}
	if err := validateSandboxPathInputs("denied path", cfg.DeniedPaths); err != nil {
		return Config{}, err
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60
	}
	if cfg.MaxOutputBytes == 0 {
		cfg.MaxOutputBytes = 64 * 1024
	}
	if cfg.MaxStderrBytes == 0 {
		cfg.MaxStderrBytes = 64 * 1024
	}
	// 输出零值先归一为安全正值，再参与资源能力合同的双向校验。
	if err := validateRequiredCapabilityContract(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// AvailableCapabilities 返回当前沙箱后端在载荷启动前能够证明的能力集合。
func AvailableCapabilities(ctx context.Context, sandboxInstance Sandbox) (CapabilitySet, error) {
	if err := validateExecContext(ctx); err != nil {
		return 0, err
	}
	if sandboxInstance == nil {
		return 0, fmt.Errorf("sandbox instance is required")
	}
	if reporter, ok := sandboxInstance.(CapabilityReporter); ok {
		return reporter.Capabilities(ctx)
	}
	if source, ok := sandboxInstance.(sandboxCapabilitySource); ok {
		available, err := source.sandboxCapabilities(ctx)
		if err != nil {
			return 0, err
		}
		if err := validateCapabilitySet(available); err != nil {
			return 0, err
		}
		return available, nil
	}
	return 0, nil
}

func normalizeSandboxWorkspace(rawPath string) (string, error) {
	path, _, err := snapshotSandboxWorkspace(rawPath)
	return path, err
}

func snapshotSandboxWorkspace(rawPath string) (string, sandboxPathIdentity, error) {
	path, err := absoluteSandboxPath("workspace", rawPath)
	if err != nil {
		return "", sandboxPathIdentity{}, err
	}
	if isFilesystemRoot(path) {
		return "", sandboxPathIdentity{}, fmt.Errorf("sandbox workspace must not be a filesystem root")
	}
	if mkdirErr := os.MkdirAll(path, 0o700); mkdirErr != nil {
		return "", sandboxPathIdentity{}, fmt.Errorf("create sandbox workspace: %w", mkdirErr)
	}
	identity, err := snapshotSandboxDirectoryIdentity("workspace", path)
	if err != nil {
		return "", sandboxPathIdentity{}, err
	}
	return path, identity, nil
}

func normalizeSandboxPaths(field string, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	normalized := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for index, rawPath := range paths {
		if rawPath == "" {
			return nil, fmt.Errorf("sandbox %s at index %d is empty", field, index)
		}
		expanded := expandPath(rawPath)
		if !filepath.IsAbs(expanded) {
			return nil, fmt.Errorf("sandbox %s %q must be absolute", field, rawPath)
		}
		path, err := absoluteSandboxPath(field, expanded)
		if err != nil {
			return nil, err
		}
		if isFilesystemRoot(path) {
			return nil, fmt.Errorf("sandbox %s must not be a filesystem root", field)
		}
		path, err = resolveExistingPathPrefix(path)
		if err != nil {
			return nil, fmt.Errorf("resolve sandbox %s %q: %w", field, rawPath, err)
		}
		key := path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, path)
	}
	return normalized, nil
}

func validateSandboxPathInputs(field string, paths []string) error {
	for index, rawPath := range paths {
		if rawPath == "" {
			return fmt.Errorf("sandbox %s at index %d is empty", field, index)
		}
		expanded := expandPath(rawPath)
		if !filepath.IsAbs(expanded) {
			return fmt.Errorf("sandbox %s %q must be absolute", field, rawPath)
		}
		path, err := absoluteSandboxPath(field, expanded)
		if err != nil {
			return err
		}
		if isFilesystemRoot(path) {
			return fmt.Errorf("sandbox %s must not be a filesystem root", field)
		}
	}
	return nil
}

func absoluteSandboxPath(field, rawPath string) (string, error) {
	expanded := expandPath(rawPath)
	if strings.ContainsAny(expanded, "\x00\r\n\"") || (runtime.GOOS == "darwin" && strings.Contains(expanded, `\`)) {
		return "", fmt.Errorf("sandbox %s contains unsupported characters", field)
	}
	path, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("resolve sandbox %s: %w", field, err)
	}
	return filepath.Clean(path), nil
}

func resolveExistingPathPrefix(path string) (string, error) {
	current := filepath.Clean(path)
	missing := make([]string, 0)
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(path), nil
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func isFilesystemRoot(path string) bool {
	volume := filepath.VolumeName(path)
	root := volume + string(filepath.Separator)
	return filepath.Clean(path) == filepath.Clean(root)
}

// expandPath 展开当前用户家目录的波浪号前缀。
func expandPath(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

func validateExecContext(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalidContext
	}
	return nil
}

type boundedBuffer struct {
	mu        sync.Mutex
	limit     int64
	total     int64
	truncated bool
	buf       []byte
}

// boundedBufferSnapshot 是一次锁内取得的不可变输出视图。
type boundedBufferSnapshot struct {
	Text      string
	BytesSeen int64
	Truncated bool
}

func newBoundedBuffer(limit int64) *boundedBuffer {
	if limit <= 0 {
		limit = 64 * 1024
	}
	return &boundedBuffer{limit: limit}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	const maxInt64 = int64(^uint64(0) >> 1)
	if int64(n) > maxInt64-b.total {
		b.total = maxInt64
		b.truncated = true
	} else {
		b.total += int64(n)
	}
	remaining := b.limit - int64(len(b.buf))
	if remaining > 0 {
		if int64(len(p)) > remaining {
			b.buf = append(b.buf, p[:remaining]...)
			b.truncated = true
		} else {
			b.buf = append(b.buf, p...)
		}
	} else if n > 0 {
		b.truncated = true
	}
	return n, nil
}

// Snapshot 在同一临界区返回文本、总字节数和截断状态。
func (b *boundedBuffer) Snapshot() boundedBufferSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	return boundedBufferSnapshot{
		Text:      string(b.buf),
		BytesSeen: b.total,
		Truncated: b.truncated || b.total > b.limit,
	}
}

func (b *boundedBuffer) String() string   { return b.Snapshot().Text }
func (b *boundedBuffer) BytesSeen() int64 { return b.Snapshot().BytesSeen }
func (b *boundedBuffer) Truncated() bool  { return b.Snapshot().Truncated }

// withTimeout 依据 cfg.Timeout(秒)为执行派生一个截止时间。
//
// 设计动机: Config.Timeout 字段承诺"超时秒数, 默认 60", 但历史实现仅 windows
// 路径使用了它, darwin/linux/basic 三个 POSIX 路径完全无视该字段, 形成跨平台
// 行为不一致的"死配置"。本函数统一兜底: 当 timeoutSec > 0 且调用方传入的 ctx
// 没有更早的 deadline 时, 派生一个 timeoutSec 秒后触发的 deadline,
// 使 cfg.Timeout 真正生效并强制终止超时进程。
//
// 调用方必须 defer 调用返回的 cancel 以释放计时器资源。
func withTimeout(ctx context.Context, timeoutSec int) (context.Context, context.CancelFunc) {
	if timeoutSec <= 0 {
		return ctx, func() {}
	}
	var limit time.Duration
	if uint64(timeoutSec) > uint64((1<<63-1)/int64(time.Second)) {
		// 正常入口会在任何副作用前拒绝该配置；包内直接调用仍使用不溢出的有限上界。
		limit = time.Duration(1<<63 - 1)
	} else {
		limit = time.Duration(timeoutSec) * time.Second
	}
	// 若调用方 ctx 已有更早(或相同)的 deadline, 则尊重调用方, 不再缩短。
	if dl, ok := ctx.Deadline(); ok && time.Until(dl) <= limit {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, limit)
}
