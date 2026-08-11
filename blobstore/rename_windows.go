//go:build windows

package blobstore

import (
	"errors"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// replaceRootFile 在 Windows 上保留 os.Root 相对路径原子替换，并对瞬时占用
// （Windows Defender 等以无 FILE_SHARE_DELETE 方式短暂打开目标文件）做有界
// 指数退避重试。这是 Go 官方对 Windows rename 的既有结论
// （golang/go#36163：Windows 不保证原子 rename 成功，需重试瞬时错误）。
func replaceRootFile(root *os.Root, source, target string) error {
	const maxAttempts = 6
	delay := 10 * time.Millisecond
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		lastErr = root.Rename(source, target)
		if lastErr == nil {
			return nil
		}
		if !errors.Is(lastErr, syscall.EACCES) && !errors.Is(lastErr, windows.ERROR_SHARING_VIOLATION) {
			return lastErr
		}
		if attempt < maxAttempts-1 {
			time.Sleep(delay)
			delay *= 2
		}
	}
	return lastErr
}
