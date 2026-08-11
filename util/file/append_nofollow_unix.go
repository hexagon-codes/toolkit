//go:build darwin || linux

package file

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func openAppendFileNoFollow(dir, base string, perm os.FileMode) (file *os.File, err error) {
	// 父目录句柄固定路径身份，O_NOFOLLOW 原子拒绝最终符号链接。
	parent, err := os.Open(dir) // #nosec G304 -- 路径是通用文件 API 的显式调用参数。
	if err != nil {
		return nil, fmt.Errorf("open parent directory: %w", err)
	}
	defer func() {
		if closeErr := parent.Close(); closeErr != nil {
			if file != nil {
				closeErr = errors.Join(closeErr, file.Close())
				file = nil
			}
			err = errors.Join(err, fmt.Errorf("close parent directory: %w", closeErr))
		}
	}()

	fd, err := unix.Openat(
		int(parent.Fd()),
		base,
		unix.O_APPEND|unix.O_CLOEXEC|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_WRONLY,
		uint32(perm.Perm()),
	)
	if err != nil {
		return nil, &os.PathError{Op: "openat", Path: filepath.Join(dir, base), Err: err}
	}
	file = os.NewFile(uintptr(fd), filepath.Join(dir, base))
	if file == nil {
		return nil, errors.Join(errors.New("create append file handle"), unix.Close(fd))
	}
	return file, nil
}
