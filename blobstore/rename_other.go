//go:build !windows

package blobstore

import "os"

// replaceRootFile 非 Windows 平台直接原子替换（renameat 替换已存在目标）。
func replaceRootFile(root *os.Root, source, target string) error {
	return root.Rename(source, target)
}
