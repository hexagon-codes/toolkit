//go:build windows

package file

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func openAppendFileNoFollow(dir, base string, _ os.FileMode) (*os.File, error) {
	path, err := windowsExtendedPath(filepath.Join(dir, base))
	if err != nil {
		return nil, fmt.Errorf("resolve append path: %w", err)
	}
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("encode append path: %w", err)
	}

	// 只授予追加、检查属性和按句柄更新只读属性所需的权限，不授予普通数据覆盖权限。
	access := uint32(windows.FILE_APPEND_DATA |
		windows.FILE_READ_ATTRIBUTES |
		windows.FILE_READ_EA |
		windows.FILE_WRITE_ATTRIBUTES |
		windows.FILE_WRITE_EA |
		windows.STANDARD_RIGHTS_READ |
		windows.STANDARD_RIGHTS_WRITE |
		windows.SYNCHRONIZE)
	handle, err := windows.CreateFile(
		pathPointer,
		access,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: filepath.Join(dir, base), Err: err}
	}

	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &information); err != nil {
		return nil, errors.Join(
			fmt.Errorf("inspect append destination: %w", err),
			windows.CloseHandle(handle),
		)
	}
	if information.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return nil, errors.Join(
			errors.New("append destination must not be a reparse point"),
			windows.CloseHandle(handle),
		)
	}
	if information.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return nil, errors.Join(
			errors.New("append destination must be a file"),
			windows.CloseHandle(handle),
		)
	}

	file := os.NewFile(uintptr(handle), filepath.Join(dir, base))
	if file == nil {
		return nil, errors.Join(errors.New("create append file handle"), windows.CloseHandle(handle))
	}
	return file, nil
}

func windowsExtendedPath(path string) (string, error) {
	fullPath, err := windows.FullPath(path)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(fullPath, `\\?\`) || strings.HasPrefix(fullPath, `\??\`) {
		return fullPath, nil
	}
	if strings.HasPrefix(fullPath, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(fullPath, `\\`), nil
	}
	return `\\?\` + fullPath, nil
}
