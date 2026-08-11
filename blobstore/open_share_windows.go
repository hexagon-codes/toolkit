//go:build windows

package blobstore

import (
	"os"

	"golang.org/x/sys/windows"
)

// openWithDeleteShare 以包含 FILE_SHARE_DELETE 的共享模式打开文件。
// Go 的 os.Open 在 Windows 上 share 模式固定为 READ|WRITE（不含 DELETE），
// 保持其打开的句柄会阻止原子替换；此函数用于验证"替换时句柄仍有效"的场景。
func openWithDeleteShare(path string) (*os.File, error) {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pathPointer,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}
