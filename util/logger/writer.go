package logger

import (
	"io"
	"os"
	"path/filepath"
)

// fileWriter 简单的文件写入器
type fileWriter struct {
	file *os.File
	path string
}

// newFileWriter 创建文件写入器
// 注意：这是一个简单实现，不支持自动轮转
// 如需文件轮转，请使用 lumberjack：
//
//	import "gopkg.in/natefinch/lumberjack.v2"
//	writer := &lumberjack.Logger{
//	    Filename:   "/var/log/app.log",
//	    MaxSize:    100, // MB
//	    MaxBackups: 3,
//	    MaxAge:     7, // days
//	    Compress:   true,
//	}
func newFileWriter(path string, _ *FileConfig) (io.Writer, error) {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	// 打开文件（追加模式）
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
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

// Write 写入到所有输出
func (w *MultiWriter) Write(p []byte) (n int, err error) {
	for _, writer := range w.writers {
		n, err = writer.Write(p)
		if err != nil {
			return
		}
	}
	return len(p), nil
}

// Add 添加写入器
func (w *MultiWriter) Add(writer io.Writer) {
	w.writers = append(w.writers, writer)
}
