//go:build darwin || linux

package file

import (
	"errors"
	"fmt"
	"os"
)

func syncParentDirectory(path string) error {
	dir, err := os.Open(path) // #nosec G304 -- 路径来自调用者明确指定的目标文件父目录。
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
