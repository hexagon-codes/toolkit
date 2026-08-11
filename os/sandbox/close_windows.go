//go:build windows

package sandbox

import "errors"

const windowsQuarantineReclaimAttempts = 3

// Close 等待 Windows 后端当前执行结束，再释放 os.Root 与根身份守卫句柄。
// 公共 capabilitySandbox 的关闭屏障保证此方法只调用一次并缓存完整错误链。
func (s *windowsSandbox) Close() error {
	if s == nil {
		return nil
	}
	s.execMu.Lock()
	defer s.execMu.Unlock()
	quarantineConfirmed, quarantineErr := s.quarantine.reclaim(
		windowsQuarantineReclaimAttempts,
		windowsJobTerminationLimit,
	)
	if !quarantineConfirmed {
		return errors.Join(quarantineErr, errWindowsProcessLifecycleUnconfirmed)
	}
	if s.workspace == nil {
		return quarantineErr
	}
	workspaceErr := s.workspace.close()
	if workspaceErr == nil {
		s.workspace = nil
	}
	return errors.Join(quarantineErr, workspaceErr)
}
