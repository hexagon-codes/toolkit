//go:build windows

package sandbox

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sandboxCreationTime 返回路径对应目录创建时间的 FILETIME 原始值（1601 年起的
// 100ns 单位）。直接用原始值比较，避免 Filetime.Nanoseconds 在 2026 年之后
// 因 int64 溢出损失精度。NTFS 上目录被替换必然产生新的创建时间，而沙箱对
// DACL/integrity 的修改不会改变它，可作为"文件索引可能被复用"场景下的可靠判据。
func sandboxCreationTime(path string) uint64 {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0
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
		return 0
	}
	defer func() {
		if closeErr := windows.CloseHandle(handle); closeErr != nil {
			fmt.Fprintf(os.Stderr, "sandbox: close identity handle: %v\n", closeErr)
		}
	}()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: read creation time info for %q: %v\n", path, err)
		return 0
	}
	return uint64(info.CreationTime.HighDateTime)<<32 | uint64(info.CreationTime.LowDateTime)
}

// sandboxIdentityExtraMatches 对比目录创建时间；任一环节取不到创建时间都按
// fail-closed 视为身份已变化（无法证明相同即不可信），避免文件索引复用或
// 创建时间不可读时把替换后的目录误判为原身份。
func sandboxIdentityExtraMatches(identity sandboxPathIdentity, current os.FileInfo) bool {
	if identity.creationTime == 0 {
		return false
	}
	currentCreation := sandboxCreationTime(identity.path)
	if currentCreation == 0 {
		return false
	}
	if identity.creationTime != currentCreation {
		fmt.Fprintf(os.Stderr, "sandbox: directory identity changed: creation time 0x%x != 0x%x (path %s)\n",
			identity.creationTime, currentCreation, identity.path)
		return false
	}
	return true
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
