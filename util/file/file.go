package file

import (
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
	return os.ReadFile(path)
}

// ReadString 读取文件内容为字符串
func ReadString(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Write 写入文件内容
//
// 注意：使用权限 0644（所有者可读写，其他人只读）
// 如需写入敏感文件（如密钥、凭证），请使用 WriteWithPerm 并设置 0600
func Write(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

// WriteWithPerm 写入文件内容（自定义权限）
//
// 示例：
//
//	file.WriteWithPerm("secret.key", data, 0600)  // 仅所有者可读写
func WriteWithPerm(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

// WriteString 写入字符串到文件
//
// 注意：使用权限 0644，敏感文件请使用 WriteStringWithPerm
func WriteString(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// WriteStringWithPerm 写入字符串到文件（自定义权限）
func WriteStringWithPerm(path, content string, perm os.FileMode) error {
	return os.WriteFile(path, []byte(content), perm)
}

// Append 追加内容到文件
//
// 注意：使用权限 0644，敏感文件请使用 AppendWithPerm
func Append(path string, data []byte) error {
	return AppendWithPerm(path, data, 0644)
}

// AppendWithPerm 追加内容到文件（自定义权限）
func AppendWithPerm(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

// AppendString 追加字符串到文件
func AppendString(path, content string) error {
	return Append(path, []byte(content))
}

// Copy 复制文件
func Copy(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// Move 移动文件
func Move(src, dst string) error {
	return os.Rename(src, dst)
}

// Remove 删除文件或目录
func Remove(path string) error {
	return os.RemoveAll(path)
}

// MkdirAll 创建多级目录
func MkdirAll(path string) error {
	return os.MkdirAll(path, 0755)
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
	return filepath.Walk(root, fn)
}
