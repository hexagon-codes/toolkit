package blobstore

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ttlSuffix 是 TTL sidecar 文件后缀：blob 旁存 "<blob>.ttl"，内容为过期时刻 UnixNano。
//
// 为何用 sidecar 而非中央索引：blobstore 是内容寻址 + 原子写 + 多进程共享目录的无状态
// 设计，per-blob sidecar 同样无状态、原子、可并发，且 Purge 只需遍历不需加载全量索引。
const ttlSuffix = ".ttl"

// SetTTL 为已落盘的相对路径设置存活时长；ttl<=0 表示清除过期（永不过期）。
//
// 注意：blobstore 内容寻址会让相同内容共享同一相对路径，故 TTL 以"路径"为粒度
// （同内容的多个逻辑产物共享一个过期时间，取最后一次 SetTTL）。
func (s *Store) SetTTL(relPath string, ttl time.Duration) error {
	blobPath, err := s.safeJoin(relPath)
	if err != nil {
		return err
	}
	sidecar := blobPath + ttlSuffix
	if ttl <= 0 {
		if err := os.Remove(sidecar); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("blobstore: clear ttl: %w", err)
		}
		return nil
	}
	expiry := time.Now().Add(ttl).UnixNano()
	if err := os.WriteFile(sidecar, []byte(strconv.FormatInt(expiry, 10)), 0o644); err != nil {
		return fmt.Errorf("blobstore: write ttl: %w", err)
	}
	return nil
}

// SaveBytesWithTTL 落盘并设置存活时长，返回相对路径。ttl<=0 等价于 SaveBytes（不过期）。
func (s *Store) SaveBytesWithTTL(data []byte, ext string, ttl time.Duration) (string, error) {
	relPath, err := s.SaveBytes(data, ext)
	if err != nil {
		return "", err
	}
	if ttl > 0 {
		if err := s.SetTTL(relPath, ttl); err != nil {
			return relPath, err
		}
	}
	return relPath, nil
}

// ExpiresAt 返回相对路径的过期时刻；ok=false 表示无 TTL（永不过期）。
func (s *Store) ExpiresAt(relPath string) (t time.Time, ok bool, err error) {
	blobPath, err := s.safeJoin(relPath)
	if err != nil {
		return time.Time{}, false, err
	}
	b, err := os.ReadFile(blobPath + ttlSuffix)
	if os.IsNotExist(err) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("blobstore: read ttl: %w", err)
	}
	ns, perr := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if perr != nil {
		return time.Time{}, false, fmt.Errorf("blobstore: parse ttl: %w", perr)
	}
	return time.Unix(0, ns), true, nil
}

// Purge 删除所有在 now 时刻已过期的 blob（及其 TTL sidecar），返回删除的 blob 数。
//
// 无 sidecar 的 blob（未设 TTL）永不删除。适合后台定时清理或启动时一次性清理。
func (s *Store) Purge(now time.Time) (int, error) {
	purged := 0
	walkErr := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ttlSuffix) {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil // sidecar 读失败则跳过，不阻断清理
		}
		ns, perr := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
		if perr != nil {
			return nil
		}
		if now.UnixNano() < ns {
			return nil // 未过期
		}
		blobPath := strings.TrimSuffix(path, ttlSuffix)
		if rmErr := os.Remove(blobPath); rmErr != nil && !os.IsNotExist(rmErr) {
			return fmt.Errorf("blobstore: purge blob %s: %w", blobPath, rmErr)
		}
		_ = os.Remove(path) // 删 sidecar，失败不阻断
		purged++
		return nil
	})
	if walkErr != nil {
		return purged, fmt.Errorf("blobstore: purge walk: %w", walkErr)
	}
	return purged, nil
}

// safeJoin 把相对路径安全拼接到 root，防路径穿越（与 Open 同款防护）。
func (s *Store) safeJoin(relPath string) (string, error) {
	clean := filepath.Clean(relPath)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("blobstore: illegal relPath %q", relPath)
	}
	full := filepath.Join(s.root, clean)
	if !strings.HasPrefix(full, filepath.Clean(s.root)+string(os.PathSeparator)) && full != filepath.Clean(s.root) {
		return "", fmt.Errorf("blobstore: relPath escapes root: %q", relPath)
	}
	return full, nil
}
