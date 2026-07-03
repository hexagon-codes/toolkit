package sandbox

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrStorageLimitExceeded 标记存储限额违规(区别于「沙箱后端不可用」等基础设施错误)。
//
// 调用方用 errors.Is 判别: 命中哨兵说明执行本身可能已成功、仅产物/工作区超限,
// 各平台 Exec 不得把它再包装成 backend failed/unavailable, 且后置违规必须
// 连同 ExecResult 一起返回((res, err) 形态), 不许丢弃已产生的 stdout/stderr。
var ErrStorageLimitExceeded = errors.New("sandbox storage limit exceeded")

// ErrFilesystemContainmentUnavailable 标记「无法提供 deny-by-default 文件系统隔离」的
// fail-closed 拒绝(区别于后端进程启动失败)。
//
// 触发: linux 上 bubblewrap 缺席、仅剩 unshare 弱兜底, 且配置带机密性要求
// (DeniedPaths 非空)时——unshare 兜底不 pivot_root, 宿主完整挂载视图对载荷仍
// 可见/可写, 属 fail-open 弱隔离, 承载机密任务不安全, 故拒绝执行而非静默降级。
// 上层可 errors.Is 判别并据 ExecResult.Limits.Filesystem 决策。
var ErrFilesystemContainmentUnavailable = errors.New("sandbox filesystem containment unavailable")

// linuxBackend 标识 linux 沙箱选用的隔离后端。
type linuxBackend int

const (
	linuxBackendNone linuxBackend = iota
	linuxBackendBwrap
	linuxBackendUnshare
)

// selectLinuxSandboxBackend 依据后端可用性与配置的机密性要求, 选择 linux 沙箱后端,
// 并如实报告其文件系统隔离强度。
//
// 背景: deny-by-default 的强文件系统隔离仅 bubblewrap 满足; unshare 兜底用
// user+mount namespace 但不 pivot_root 到最小 rootfs, 宿主完整挂载视图对载荷
// 仍可见/可写(策略脚本只掩蔽 DeniedPaths、ro-bind ReadablePaths), 属
// allow-first / fail-open 的弱隔离。
//
// 决策(fail-closed 优先, 降级信号如实上报):
//   - bubblewrap 可用            → bwrap, Filesystem=enforced;
//   - 仅 unshare 可用 且 confidential → 拒绝执行(ErrFilesystemContainmentUnavailable),
//     不静默降级到关不住的弱后端;
//   - 仅 unshare 可用 且 非机密   → unshare, Filesystem=weak(降级信号, 由上层决策);
//   - 都不可用                    → 拒绝执行(无隔离后端), Filesystem=unsupported。
//
// confidential 由调用方是否显式声明受保护路径(DeniedPaths 非空)推断:
// 主动保护特定路径即表明该调用方在意机密性, 弱兜底无法保证 deny-by-default,
// 因此对其 fail-closed。无 DeniedPaths 的调用方若同样需要强隔离, 应读取
// 返回的 Filesystem 信号自行决策(见 ExecResult.Limits.Filesystem)。
func selectLinuxSandboxBackend(bwrapAvailable, unshareAvailable, confidential bool) (linuxBackend, LimitStatus, error) {
	if bwrapAvailable {
		return linuxBackendBwrap, LimitStatusEnforced, nil
	}
	if unshareAvailable {
		if confidential {
			// fail-closed: 弱兜底关不住整个宿主文件系统, 机密任务不静默降级
			return linuxBackendNone, LimitStatusWeak, fmt.Errorf("%w: linux unshare 兜底非 deny-by-default, 机密性配置(DeniedPaths 非空)要求 bubblewrap", ErrFilesystemContainmentUnavailable)
		}
		return linuxBackendUnshare, LimitStatusWeak, nil
	}
	return linuxBackendNone, LimitStatusUnsupported, errors.New("sandbox unavailable: linux requires bubblewrap or unshare")
}

// storageWalkVisitHook 仅供测试注入(模拟 walk 期间文件被并发删除的竞态窗口), 生产恒为 nil。
var storageWalkVisitHook func(path string)

func enforceSandboxStorageLimits(cfg Config) error {
	if cfg.Workspace == "" || (cfg.MaxWorkspaceBytes <= 0 && cfg.MaxArtifactBytes <= 0) {
		return nil
	}

	var total int64
	err := filepath.WalkDir(cfg.Workspace, func(path string, d os.DirEntry, walkErr error) error {
		if storageWalkVisitHook != nil {
			storageWalkVisitHook(path)
		}
		if walkErr != nil {
			// walk 期间条目消失(并发 ExecCode 的 defer os.Remove 竞态)不算检查失败, 跳过继续
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			// 读目录项之后、lstat 之前文件被并发删除: 同上, 跳过继续
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		size := info.Size()
		if cfg.MaxArtifactBytes > 0 && size > cfg.MaxArtifactBytes {
			return fmt.Errorf("%w: sandbox MaxArtifactBytes exceeded: %s is %d bytes > %d", ErrStorageLimitExceeded, path, size, cfg.MaxArtifactBytes)
		}
		total += size
		if cfg.MaxWorkspaceBytes > 0 && total > cfg.MaxWorkspaceBytes {
			return fmt.Errorf("%w: sandbox MaxWorkspaceBytes exceeded: %s has at least %d bytes > %d", ErrStorageLimitExceeded, cfg.Workspace, total, cfg.MaxWorkspaceBytes)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("check sandbox workspace storage limits: %w", err)
	}
	return nil
}
