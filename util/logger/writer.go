package logger

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// fileWriter 简单的文件写入器
type fileWriter struct {
	file *os.File
	path string
}

// newFileWriter 创建文件写入器。
// 注意：这是一个简单实现，不支持自动轮转；需要轮转时可使用 lumberjack：
//
//	import "gopkg.in/natefinch/lumberjack.v2"
//	writer := &lumberjack.Logger{
//	    Filename:   "/var/log/app.log",
//	    MaxSize:    100, // 单位为 MB
//	    MaxBackups: 3,
//	    MaxAge:     7, // 单位为天
//	    Compress:   true,
//	}
func newFileWriter(path string, _ *FileConfig) (io.Writer, error) {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}

	// 打开文件（追加模式）
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304 -- 路径来自日志文件配置，是该 API 的显式输入。
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		return nil, errors.Join(err, file.Close())
	}

	return &fileWriter{
		file: file,
		path: path,
	}, nil
}

// Write 实现 io.Writer 接口
func (w *fileWriter) Write(p []byte) (n int, err error) {
	return w.file.Write(p)
}

// Close 关闭文件
func (w *fileWriter) Close() error {
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// Sync 同步文件
func (w *fileWriter) Sync() error {
	if w.file != nil {
		return w.file.Sync()
	}
	return nil
}

// MultiWriter 多输出写入器
type MultiWriter struct {
	writers []io.Writer
}

// NewMultiWriter 创建多输出写入器
func NewMultiWriter(writers ...io.Writer) *MultiWriter {
	return &MultiWriter{writers: writers}
}

// Write 写入到所有输出，任何一个 writer 出错时立即返回
func (w *MultiWriter) Write(p []byte) (n int, err error) {
	for _, writer := range w.writers {
		n, err = writer.Write(p)
		if err != nil {
			return
		}
		if n != len(p) {
			err = io.ErrShortWrite
			return
		}
	}
	return len(p), nil
}

// Add 添加写入器
func (w *MultiWriter) Add(writer io.Writer) {
	w.writers = append(w.writers, writer)
}
