package blobstore

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// ttlSuffix 是 TTL 元数据文件后缀，内容为过期时刻的 UnixNano。
	ttlSuffix = ".blobstore.ttl"
	// maxTTLMetadataBytes 限制元数据大小，避免异常文件造成无界内存占用。
	maxTTLMetadataBytes = 64
)

// SetTTL 为已落盘的相对路径设置存活时长；ttl<=0 表示清除过期时间。
//
// 相同内容共享同一相对路径，因此 TTL 也以路径为粒度，最后一次设置生效。
func (s *Store) SetTTL(relPath string, ttl time.Duration) (err error) {
	root, release, err := s.acquireRoot()
	if err != nil {
		return err
	}
	defer release()
	s.ttlMu.Lock()
	defer s.ttlMu.Unlock()
	lock, err := s.acquireTTLFileLock(root, true)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lock.close()) }()
	return s.setTTL(root, relPath, ttl)
}

func (s *Store) setTTL(root *os.Root, relPath string, ttl time.Duration) (err error) {
	relPath, err = normalizeStorePath(relPath)
	if err != nil {
		return err
	}
	sidecarPath := relPath + ttlSuffix
	if ttl <= 0 {
		if removeErr := root.Remove(sidecarPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("blobstore: clear ttl: %w", removeErr)
		}
		return nil
	}

	info, err := root.Stat(relPath)
	if err != nil {
		return fmt.Errorf("blobstore: stat blob: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("blobstore: ttl target is not a regular file")
	}

	expiryTime := time.Now().Add(ttl)
	if expiryTime.After(time.Unix(0, math.MaxInt64)) {
		return fmt.Errorf("blobstore: ttl expiry exceeds UnixNano range")
	}
	expiry := strconv.FormatInt(expiryTime.UnixNano(), 10)
	if writeErr := writeTTLMetadata(root, sidecarPath, []byte(expiry)); writeErr != nil {
		return fmt.Errorf("blobstore: write ttl: %w", writeErr)
	}
	return nil
}

func writeTTLMetadata(root *os.Root, sidecarPath string, data []byte) (err error) {
	dir := filepath.Dir(sidecarPath)
	tmp, err := createRootTemp(root, dir, filepath.Base(sidecarPath)+".tmp.", "ttl metadata")
	if err != nil {
		return fmt.Errorf("create temporary ttl metadata: %w", err)
	}
	defer func() { err = errors.Join(err, tmp.cleanup()) }()

	if _, err := tmp.file.Write(data); err != nil {
		return fmt.Errorf("write temporary ttl metadata: %w", err)
	}
	if err := tmp.file.Sync(); err != nil {
		return fmt.Errorf("sync temporary ttl metadata: %w", err)
	}
	if err := tmp.close(); err != nil {
		return err
	}
	if err := replaceRootFile(root, tmp.path, sidecarPath); err != nil {
		return fmt.Errorf("replace ttl metadata: %w", err)
	}
	return nil
}

// SaveBytesWithTTL 落盘并设置存活时长，返回相对路径。
func (s *Store) SaveBytesWithTTL(data []byte, ext string, ttl time.Duration) (relPath string, err error) {
	root, release, err := s.acquireRoot()
	if err != nil {
		return "", err
	}
	defer release()
	s.ttlMu.Lock()
	defer s.ttlMu.Unlock()
	lock, err := s.acquireTTLFileLock(root, true)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, lock.close()) }()

	relPath, err = s.saveBytes(root, data, ext)
	if err != nil {
		return "", err
	}
	if err := s.setTTL(root, relPath, ttl); err != nil {
		return relPath, err
	}
	return relPath, nil
}

// ExpiresAt 返回相对路径的过期时刻；ok=false 表示未设置 TTL。
func (s *Store) ExpiresAt(relPath string) (t time.Time, ok bool, err error) {
	root, release, err := s.acquireRoot()
	if err != nil {
		return time.Time{}, false, err
	}
	defer release()
	s.ttlMu.RLock()
	defer s.ttlMu.RUnlock()
	lock, err := s.acquireTTLFileLock(root, false)
	if err != nil {
		return time.Time{}, false, err
	}
	defer func() { err = errors.Join(err, lock.close()) }()

	relPath, err = normalizeStorePath(relPath)
	if err != nil {
		return time.Time{}, false, err
	}
	expiry, err := readTTL(root, relPath+ttlSuffix)
	if os.IsNotExist(err) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("blobstore: read ttl: %w", err)
	}
	return expiry, true, nil
}

// Purge 删除所有在 now 时刻已过期的 blob 及其 TTL 元数据，返回删除的 blob 数。
//
// 未设置 TTL 的 blob 永不删除。损坏或不可读的元数据会汇总为错误，其他条目仍会继续清理。
func (s *Store) Purge(now time.Time) (purged int, err error) {
	root, release, err := s.acquireRoot()
	if err != nil {
		return 0, err
	}
	defer release()
	s.ttlMu.Lock()
	defer s.ttlMu.Unlock()
	lock, err := s.acquireTTLFileLock(root, true)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, lock.close()) }()

	var purgeErr error
	walkErr := fs.WalkDir(root.FS(), ".", func(entryPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entryPath, ttlSuffix) {
			return nil
		}

		sidecarPath, pathErr := normalizeStorePath(entryPath)
		if pathErr != nil {
			purgeErr = errors.Join(purgeErr, pathErr)
			return nil
		}
		expiry, readErr := readTTL(root, sidecarPath)
		if os.IsNotExist(readErr) {
			return nil
		}
		if readErr != nil {
			purgeErr = errors.Join(purgeErr, fmt.Errorf("blobstore: read ttl %s: %w", entryPath, readErr))
			return nil
		}
		if now.Before(expiry) {
			return nil
		}

		blobPath := strings.TrimSuffix(sidecarPath, ttlSuffix)
		removedBlob := false
		if removeErr := root.Remove(blobPath); removeErr == nil {
			removedBlob = true
		} else if !os.IsNotExist(removeErr) {
			purgeErr = errors.Join(purgeErr, fmt.Errorf("blobstore: purge blob %s: %w", entryPath, removeErr))
			return nil
		}
		if removeErr := root.Remove(sidecarPath); removeErr != nil && !os.IsNotExist(removeErr) {
			purgeErr = errors.Join(purgeErr, fmt.Errorf("blobstore: purge ttl %s: %w", entryPath, removeErr))
		}
		if removedBlob {
			purged++
		}
		return nil
	})
	if walkErr != nil {
		purgeErr = errors.Join(purgeErr, fmt.Errorf("blobstore: purge walk: %w", walkErr))
	}
	return purged, purgeErr
}

func readTTL(root *os.Root, sidecarPath string) (_ time.Time, err error) {
	file, err := root.Open(filepath.FromSlash(sidecarPath))
	if err != nil {
		return time.Time{}, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()

	data, err := io.ReadAll(io.LimitReader(file, maxTTLMetadataBytes+1))
	if err != nil {
		return time.Time{}, err
	}
	if len(data) > maxTTLMetadataBytes {
		return time.Time{}, fmt.Errorf("ttl metadata exceeds %d bytes", maxTTLMetadataBytes)
	}
	nanoseconds, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse ttl: %w", err)
	}
	return time.Unix(0, nanoseconds), nil
}
