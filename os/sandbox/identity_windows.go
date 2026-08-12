//go:build windows

package sandbox

import (
	"fmt"
	"os"
	"time"
	"unsafe"

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

// sandboxIdentityExtraMatches 对比目录创建时间；任一环节取不到创建时间都按
// fail-closed 视为身份已变化（无法证明相同即不可信），避免文件索引复用或
// 创建时间不可读时把替换后的目录误判为原身份。
func sandboxIdentityExtraMatches(identity sandboxPathIdentity, current os.FileInfo) bool {
	if identity.creationTime.IsZero() {
		return false
	}
	currentCreation := sandboxCreationTime(identity.path)
	if currentCreation.IsZero() {
		return false
	}
	return identity.creationTime.Equal(currentCreation)
}

// sandboxPathIsReparsePoint 用内核 reparse tag 判定路径是否为 reparse point。
// 适用于 os.Lstat 因 OPEN_REPARSE_POINT 属性位缺失而误判非目录的场景。
func sandboxPathIsReparsePoint(path string) bool {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
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
		return false
	}
	defer func() {
		if closeErr := windows.CloseHandle(handle); closeErr != nil {
			fmt.Fprintf(os.Stderr, "sandbox: close reparse probe handle: %v\n", closeErr)
		}
	}()
	var tagInfo struct {
		fileAttributes uint32
		reparseTag     uint32
	}
	if err := windows.GetFileInformationByHandleEx(
		handle,
		windows.FileAttributeTagInfo,
		(*byte)(unsafe.Pointer(&tagInfo)),
		uint32(unsafe.Sizeof(tagInfo)),
	); err != nil {
		return false
	}
	return tagInfo.reparseTag != 0
}
