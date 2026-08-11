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

// ErrFilesystemContainmentUnavailable 表示当前平台后端无法提供 deny-by-default
// 文件系统隔离。调用方使用 errors.Is 判别并拒绝承载不可信或机密任务。
var ErrFilesystemContainmentUnavailable = errors.New("sandbox filesystem containment unavailable")

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
			// walk 期间条目因并发执行而消失不算检查失败，跳过继续。
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
