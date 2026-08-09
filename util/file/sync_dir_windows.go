//go:build windows

package file

func syncParentDirectory(string) error {
	// Windows 标准库没有可移植的目录同步能力，文件内容仍会在 rename 前完成 Sync。
	return nil
}
