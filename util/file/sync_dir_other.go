//go:build !darwin && !linux && !windows

package file

func syncParentDirectory(string) error {
	// 其他平台不声明目录持久化保证，文件内容仍会在 rename 前完成 Sync。
	return nil
}
