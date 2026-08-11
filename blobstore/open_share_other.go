//go:build !windows

package blobstore

import "os"

// openWithDeleteShare 非 Windows 平台无共享模式概念，等价 os.Open。
func openWithDeleteShare(path string) (*os.File, error) {
	return os.Open(path)
}
