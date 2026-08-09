// Package file 提供常用文件系统操作。
package file

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Exists 判断文件或目录是否存在
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || os.IsExist(err)
}

// IsFile 判断是否为文件
func IsFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// IsDir 判断是否为目录
func IsDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// Size 获取文件大小（字节）
func Size(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// Ext 获取文件扩展名（包含.）
func Ext(path string) string {
	return filepath.Ext(path)
}

// ExtWithoutDot 获取文件扩展名（不包含.）
func ExtWithoutDot(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimPrefix(ext, ".")
}

// Name 获取文件名（包含扩展名）
func Name(path string) string {
	return filepath.Base(path)
}

// NameWithoutExt 获取文件名（不包含扩展名）
func NameWithoutExt(path string) string {
	name := filepath.Base(path)
	ext := filepath.Ext(name)
	return strings.TrimSuffix(name, ext)
}

// Dir 获取文件所在目录
func Dir(path string) string {
	return filepath.Dir(path)
}

// Read 读取文件内容
func Read(path string) ([]byte, error) {
	return os.ReadFile(path) // #nosec G304 -- 路径是通用文件 API 的显式调用参数。
}

// ReadString 读取文件内容为字符串
func ReadString(path string) (string, error) {
	data, err := Read(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Write 原子替换文件内容
//
// 默认权限为 0600，仅文件所有者可读写。Darwin/Linux 会依次完成临时文件
// 写入、权限设置、文件 Sync、关闭、同目录 rename 和父目录 Sync。Windows 会在
// rename 前完成文件 Sync，但标准库不保证 rename 原子性，也不提供可移植的目录同步。
// 如果父目录 Sync 失败，函数会返回错误，但目标文件此时已经完成替换。
func Write(path string, data []byte) error {
	return WriteWithPerm(path, data, 0o600)
}

// WriteWithPerm 使用自定义权限原子替换文件内容
//
// 示例：
//
//	file.WriteWithPerm("secret.key", data, 0600)  // 仅所有者可读写
func WriteWithPerm(path string, data []byte, perm os.FileMode) error {
	return atomicReplace(path, perm, func(w io.Writer) error {
		return writeAll(w, data)
	})
}

// WriteString 原子替换文件字符串内容
//
// 默认权限为 0600，仅文件所有者可读写。
func WriteString(path, content string) error {
	return Write(path, []byte(content))
}

// WriteStringWithPerm 使用自定义权限原子替换文件字符串内容
func WriteStringWithPerm(path, content string, perm os.FileMode) error {
	return WriteWithPerm(path, []byte(content), perm)
}

// Append 直接追加内容到文件，不提供原子替换或崩溃持久性保证
//
// 默认权限为 0600，仅文件所有者可读写。
func Append(path string, data []byte) error {
	return AppendWithPerm(path, data, 0o600)
}

// AppendWithPerm 追加内容到文件（自定义权限）
func AppendWithPerm(path string, data []byte, perm os.FileMode) error {
	if path == "" {
		return errors.New("file path must not be empty")
	}
	if err := validateFilePermission(perm); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, perm) // #nosec G304 -- 路径是通用文件 API 的显式调用参数。
	if err != nil {
		return err
	}
	if chmodErr := f.Chmod(perm); chmodErr != nil {
		return errors.Join(chmodErr, f.Close())
	}
	_, writeErr := f.Write(data)
	return errors.Join(writeErr, f.Close())
}

// AppendString 追加字符串到文件
func AppendString(path, content string) error {
	return Append(path, []byte(content))
}

// Copy 原子复制普通文件
//
// 源符号链接按 os.Stat 语义解析；目标符号链接会被替换而不会被跟随。目标默认仅向
// 所有者开放，并保留源文件的所有者执行位。持久化与平台边界和 Write 相同。
func Copy(src, dst string) (err error) {
	if src == "" {
		return errors.New("source path must not be empty")
	}
	if dst == "" {
		return errors.New("destination path must not be empty")
	}
	sourceFile, err := os.Open(src) // #nosec G304 -- 源路径是通用文件 API 的显式调用参数。
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, sourceFile.Close()) }()

	// 获取源文件权限
	srcInfo, err := sourceFile.Stat()
	if err != nil {
		return err
	}
	if !srcInfo.Mode().IsRegular() {
		return fmt.Errorf("source must be a regular file: %q", src)
	}
	if dstInfo, statErr := os.Stat(dst); statErr == nil {
		if os.SameFile(srcInfo, dstInfo) {
			return errors.New("source and destination refer to the same file")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}

	destMode := os.FileMode(0o600) | srcInfo.Mode().Perm()&0o100
	return atomicReplace(dst, destMode, func(w io.Writer) error {
		_, copyErr := io.Copy(w, sourceFile)
		return copyErr
	})
}

// Move 移动文件
func Move(src, dst string) error {
	return os.Rename(src, dst)
}

// Remove 删除文件或目录
func Remove(path string) error {
	if path == "" {
		return errors.New("path must not be empty")
	}
	return os.RemoveAll(path)
}

// MkdirAll 创建多级目录
func MkdirAll(path string) error {
	return os.MkdirAll(path, 0o750)
}

// IsEmpty 判断文件是否为空
func IsEmpty(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.Size() == 0, nil
}

// Join 连接路径
func Join(elem ...string) string {
	return filepath.Join(elem...)
}

// Abs 获取绝对路径
func Abs(path string) (string, error) {
	return filepath.Abs(path)
}

// ListFiles 列出目录下的所有文件（不包含子目录）
func ListFiles(dir string) ([]string, error) {
	var files []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}

	return files, nil
}

// ListDirs 列出目录下的所有子目录
func ListDirs(dir string) ([]string, error) {
	var dirs []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(dir, entry.Name()))
		}
	}

	return dirs, nil
}

// Walk 递归遍历目录
func Walk(root string, fn func(path string, info os.FileInfo, err error) error) error {
	if fn == nil {
		return errors.New("walk callback must not be nil")
	}
	return filepath.Walk(root, fn)
}
