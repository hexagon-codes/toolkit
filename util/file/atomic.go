package file

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type atomicWriteFile interface {
	io.Writer
	Chmod(os.FileMode) error
	Sync() error
	Close() error
	Name() string
}

type atomicWriteOps struct {
	createTemp func(dir, pattern string) (atomicWriteFile, error)
	rename     func(oldPath, newPath string) error
	remove     func(path string) error
	syncDir    func(path string) error
}

func atomicReplace(path string, perm os.FileMode, populate func(io.Writer) error) error {
	return atomicReplaceWithOps(path, perm, populate, atomicWriteOps{
		createTemp: func(dir, pattern string) (atomicWriteFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		rename:  os.Rename,
		remove:  os.Remove,
		syncDir: syncParentDirectory,
	})
}

func atomicReplaceWithOps(path string, perm os.FileMode, populate func(io.Writer) error, ops atomicWriteOps) (err error) {
	cleanPath, dir, base, err := atomicDestination(path)
	if err != nil {
		return err
	}
	if permissionErr := validateFilePermission(perm); permissionErr != nil {
		return permissionErr
	}
	if populate == nil {
		return errors.New("populate function must not be nil")
	}

	temp, err := ops.createTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tempPath := temp.Name()
	tempOpen := true
	renamed := false
	defer func() {
		if tempOpen {
			if closeErr := temp.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close temporary file: %w", closeErr))
			}
		}
		if !renamed {
			if removeErr := ops.remove(tempPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				err = errors.Join(err, fmt.Errorf("remove temporary file: %w", removeErr))
			}
		}
	}()

	if err := populate(temp); err != nil {
		return fmt.Errorf("write temporary file: %w", err)
	}
	// 临时文件创建时保持 0600，写完后再设置最终权限，避免暴露半成品。
	if err := temp.Chmod(perm); err != nil {
		return fmt.Errorf("set temporary file permissions: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync temporary file: %w", err)
	}
	closeErr := temp.Close()
	tempOpen = false
	if closeErr != nil {
		return fmt.Errorf("close temporary file: %w", closeErr)
	}
	if err := ops.rename(tempPath, cleanPath); err != nil {
		return fmt.Errorf("replace destination file: %w", err)
	}
	renamed = true
	if err := ops.syncDir(dir); err != nil {
		return fmt.Errorf("sync parent directory after replacing destination: %w", err)
	}
	return nil
}

func atomicDestination(path string) (cleanPath, dir, base string, err error) {
	if path == "" {
		return "", "", "", errors.New("file path must not be empty")
	}
	if os.IsPathSeparator(path[len(path)-1]) {
		return "", "", "", errors.New("file path must not end with a path separator")
	}
	cleanPath = filepath.Clean(path)
	dir = filepath.Dir(cleanPath)
	base = filepath.Base(cleanPath)
	if base == "." || base == ".." || base == "" {
		return "", "", "", errors.New("file path must name a file")
	}
	return cleanPath, dir, base, nil
}

func validateFilePermission(perm os.FileMode) error {
	const validBits = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	if perm&^validBits != 0 {
		return fmt.Errorf("invalid file permission %v: only chmod permission bits are allowed", perm)
	}
	return nil
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if n < 0 || n > len(data) {
			return errors.New("writer returned an invalid byte count")
		}
		data = data[n:]
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
