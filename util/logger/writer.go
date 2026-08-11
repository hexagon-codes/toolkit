package logger

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// fileWriter 简单的文件写入器
type fileWriter struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	closed   bool
	closeErr error
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
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.file == nil {
		return 0, os.ErrClosed
	}
	n, err = w.file.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	return n, err
}

// Close 关闭文件
func (w *fileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return w.closeErr
	}
	w.closed = true
	if w.file == nil {
		return nil
	}
	w.closeErr = w.file.Close()
	w.file = nil
	return w.closeErr
}

// Sync 同步文件
func (w *fileWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return w.closeErr
	}
	if w.file == nil {
		return nil
	}
	return w.file.Sync()
}

type writerSnapshot struct {
	writers []io.Writer
}

// MultiWriter 多输出写入器
type MultiWriter struct {
	addMu   sync.Mutex
	writeMu sync.Mutex
	writers atomic.Pointer[writerSnapshot]
}

// NewMultiWriter 创建多输出写入器
func NewMultiWriter(writers ...io.Writer) *MultiWriter {
	w := &MultiWriter{}
	w.writers.Store(&writerSnapshot{writers: append([]io.Writer(nil), writers...)})
	return w
}

// Write 写入到所有输出，任何一个 writer 出错时立即返回
func (w *MultiWriter) Write(p []byte) (n int, err error) {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()

	snapshot := w.writers.Load()
	if snapshot == nil {
		return len(p), nil
	}
	for _, writer := range snapshot.writers {
		n, err = writeSafely(writer, p)
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
	w.addMu.Lock()
	defer w.addMu.Unlock()

	current := w.writers.Load()
	var writers []io.Writer
	if current != nil {
		writers = make([]io.Writer, len(current.writers), len(current.writers)+1)
		copy(writers, current.writers)
	}
	writers = append(writers, writer)
	w.writers.Store(&writerSnapshot{writers: writers})
}

func writeSafely(writer io.Writer, p []byte) (n int, err error) {
	if writer == nil {
		return 0, errors.New("logger: writer is nil")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			n = 0
			err = recoveredError("logger: writer panicked", recovered)
		}
	}()
	n, err = writer.Write(p)
	if n < 0 || n > len(p) {
		return 0, fmt.Errorf("logger: writer returned invalid byte count %d", n)
	}
	return n, err
}
