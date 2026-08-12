//go:build windows

package sandbox

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// sandboxCreationTime 返回路径对应目录的创建时间。NTFS 上目录被替换必然产生
// 新的创建时间，而沙箱对 DACL/integrity 的修改不会改变它，因此可作为
// "文件索引可能被复用"场景下的可靠身份判据。
func sandboxCreationTime(path string) time.Time {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return time.Time{}
	}
	handle, err := windows.CreateFile(
		pathPointer,
		windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windowsReparseFlag,
		0,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: read creation time for %q: %v\n", path, err)
		return time.Time{}
	}
	defer func() {
		if closeErr := windows.CloseHandle(handle); closeErr != nil {
			fmt.Fprintf(os.Stderr, "sandbox: close identity handle: %v\n", closeErr)
		}
	}()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: read creation time info for %q: %v\n", path, err)
		return time.Time{}
	}
	return time.Unix(0, info.CreationTime.Nanoseconds())
}

// sandboxIdentityExtraMatches 对比目录创建时间；无法读取创建时间时视为一致
// （保持 os.SameFile 的基础判据）。
func sandboxIdentityExtraMatches(identity sandboxPathIdentity, current os.FileInfo) bool {
	if identity.creationTime.IsZero() {
		return true
	}
	currentCreation := sandboxCreationTime(identity.path)
	return currentCreation.IsZero() || identity.creationTime.Equal(currentCreation)
}
