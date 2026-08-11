//go:build windows

package sandbox

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// enforceWindowsWorkspaceLimits 仅通过 os.Root 和已审计句柄统计工作区。
// 该检查不能阻止运行中的瞬时增长，因此能力报告必须保持 Unsupported。
func enforceWindowsWorkspaceLimits(workspace *windowsWorkspace, cfg Config) error {
	if workspace == nil || workspace.root == nil {
		return fmt.Errorf("Windows workspace is not initialized")
	}
	var total uint64
	return workspace.walk(func(relativePath string, file *os.File, identity windowsFileIdentity) error {
		allowedSIDs, err := privateWindowsWorkspaceSIDs(workspace.ownerSID, workspace.appContainerSID)
		if err != nil {
			return err
		}
		appPresent, err := auditPrivateWindowsHandle(
			file,
			identity,
			workspace.ownerSID,
			allowedSIDs,
			workspace.appContainerSID,
		)
		if err != nil {
			return fmt.Errorf("audit Windows workspace entry %q: %w", relativePath, err)
		}
		if !appPresent {
			return fmt.Errorf("audit Windows workspace entry %q: stable AppContainer identity is missing", relativePath)
		}
		if identity.attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
			return nil
		}
		if cfg.MaxArtifactBytes > 0 && identity.size > uint64(cfg.MaxArtifactBytes) {
			return fmt.Errorf(
				"%w: sandbox MaxArtifactBytes exceeded: %s is %d bytes > %d",
				ErrStorageLimitExceeded,
				relativePath,
				identity.size,
				cfg.MaxArtifactBytes,
			)
		}
		if ^uint64(0)-total < identity.size {
			return fmt.Errorf("%w: sandbox workspace size overflow", ErrStorageLimitExceeded)
		}
		total += identity.size
		if cfg.MaxWorkspaceBytes > 0 && total > uint64(cfg.MaxWorkspaceBytes) {
			return fmt.Errorf(
				"%w: sandbox MaxWorkspaceBytes exceeded: %s has at least %d bytes > %d",
				ErrStorageLimitExceeded,
				workspace.canonicalPath,
				total,
				cfg.MaxWorkspaceBytes,
			)
		}
		return nil
	})
}
