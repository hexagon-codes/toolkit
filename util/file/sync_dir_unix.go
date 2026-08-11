//go:build darwin || linux

package file

import (
	"errors"
	"fmt"
	"os"
)

func syncRootDirectory(root *os.Root) error {
	// 目录文件从已固定的 Root 打开，不再解析调用者提供的父目录路径。
	dir, err := root.Open(".")
	if err != nil {
		return fmt.Errorf("open parent directory for sync: %w", err)
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil {
		syncErr = fmt.Errorf("sync parent directory: %w", syncErr)
	}
	if closeErr != nil {
		closeErr = fmt.Errorf("close parent directory: %w", closeErr)
	}
	return errors.Join(syncErr, closeErr)
}
