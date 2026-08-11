package file

import (
	"crypto/rand"
	"encoding/hex"
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
}

type atomicWriteDirectory interface {
	CreateTemp(prefix string) (atomicWriteFile, string, error)
	Lstat(name string) (os.FileInfo, error)
	Rename(oldName, newName string) error
	Remove(name string) error
	VerifyBound() error
	Sync() error
	Close() error
}

type atomicWriteOps struct {
	openParent func(path string) (atomicWriteDirectory, error)
}

type rootAtomicWriteDirectory struct {
	path     string
	root     *os.Root
	identity os.FileInfo
}

func atomicReplace(path string, perm os.FileMode, populate func(io.Writer) error) error {
	return atomicReplaceWithOps(path, perm, populate, atomicWriteOps{
		openParent: openRootAtomicWriteDirectory,
	})
}

func openRootAtomicWriteDirectory(path string) (atomicWriteDirectory, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("open parent directory: %w", err)
	}
	identity, err := root.Stat(".")
	if err != nil {
		return nil, errors.Join(fmt.Errorf("stat pinned parent directory: %w", err), root.Close())
	}
	directory := &rootAtomicWriteDirectory{path: path, root: root, identity: identity}
	if err := directory.VerifyBound(); err != nil {
		return nil, errors.Join(err, root.Close())
	}
	return directory, nil
}

func (d *rootAtomicWriteDirectory) CreateTemp(prefix string) (atomicWriteFile, string, error) {
	const maxAttempts = 128
	var randomBytes [16]byte
	for range maxAttempts {
		if _, err := rand.Read(randomBytes[:]); err != nil {
			return nil, "", fmt.Errorf("generate temporary file name: %w", err)
		}
		name := prefix + hex.EncodeToString(randomBytes[:])
		file, err := d.root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return nil, "", fmt.Errorf("create temporary file: %w", err)
		}
		return file, name, nil
	}
	return nil, "", fmt.Errorf("create temporary file: %w", os.ErrExist)
}

func (d *rootAtomicWriteDirectory) Lstat(name string) (os.FileInfo, error) {
	return d.root.Lstat(name)
}

func (d *rootAtomicWriteDirectory) Rename(oldName, newName string) error {
	return d.root.Rename(oldName, newName)
}

func (d *rootAtomicWriteDirectory) Remove(name string) error {
	return d.root.Remove(name)
}

func (d *rootAtomicWriteDirectory) VerifyBound() error {
	current, err := os.Stat(d.path)
	if err != nil {
		return fmt.Errorf("verify parent directory identity: %w", err)
	}
	if !os.SameFile(d.identity, current) {
		return errors.New("parent directory changed during file operation")
	}
	return nil
}

func (d *rootAtomicWriteDirectory) Sync() error {
	return syncRootDirectory(d.root)
}

func (d *rootAtomicWriteDirectory) Close() error {
	return d.root.Close()
}

func atomicReplaceWithOps(path string, perm os.FileMode, populate func(io.Writer) error, ops atomicWriteOps) (err error) {
	_, dir, base, err := atomicDestination(path)
	if err != nil {
		return err
	}
	if permissionErr := validateFilePermission(perm); permissionErr != nil {
		return permissionErr
	}
	if populate == nil {
		return errors.New("populate function must not be nil")
	}
	if ops.openParent == nil {
		return errors.New("open parent operation must not be nil")
	}

	parent, err := ops.openParent(dir)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := parent.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close parent directory: %w", closeErr))
		}
	}()

	temp, tempName, err := parent.CreateTemp("." + base + ".tmp-")
	if err != nil {
		return err
	}
	tempOpen := true
	renamed := false
	defer func() {
		if tempOpen {
			if closeErr := temp.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close temporary file: %w", closeErr))
			}
		}
		if !renamed {
			if removeErr := parent.Remove(tempName); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
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
	if err := parent.VerifyBound(); err != nil {
		return err
	}
	if destinationInfo, lstatErr := parent.Lstat(base); lstatErr == nil {
		if destinationInfo.Mode()&os.ModeSymlink != 0 {
			return errors.New("destination file must not be a symbolic link")
		}
	} else if !errors.Is(lstatErr, os.ErrNotExist) {
		return fmt.Errorf("inspect destination file: %w", lstatErr)
	}
	if err := parent.Rename(tempName, base); err != nil {
		return fmt.Errorf("replace destination file: %w", err)
	}
	renamed = true
	if err := parent.Sync(); err != nil {
		return fmt.Errorf("sync parent directory after replacing destination: %w", err)
	}
	if err := parent.VerifyBound(); err != nil {
		return fmt.Errorf("verify parent directory after replacing destination: %w", err)
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
