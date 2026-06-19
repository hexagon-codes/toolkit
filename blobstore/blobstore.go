// Package blobstore 提供内容寻址（content-addressed）的本地文件 blob 存储。
//
// 核心特性（零业务/AI 依赖的通用件）：
//   - SHA-256 内容哈希命名：同内容天然去重，哈希文件名天然防路径穿越
//   - 按 YYYYMM 分子目录落盘：{root}/{yyyymm}/{hash}.{ext}
//   - 原子写入：唯一临时文件 + rename，并发/多进程共享目录安全
//   - 远程 URL 拉取落盘（用于 Provider 返回的临时过期 URL）
//   - Open 带路径穿越防护
//
// 典型用途：把生成的图像/视频等大二进制内容落盘，业务侧只持久化返回的相对路径
// （而非内嵌 base64 或依赖会过期的远程 URL）。访问由上层 file server 按相对路径提供。
package blobstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hexagon-codes/toolkit/net/httpx"
)

// Store 内容寻址 blob 存储。线程安全（依赖底层文件系统）。
type Store struct {
	root  string       // blob 根目录的绝对路径
	httpc *http.Client // 用于下载远程 URL
}

// NewStore 创建存储；root 是 blob 根目录。会自动 mkdir。
func NewStore(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", root, err)
	}
	return &Store{
		root: root,
		// 远程下载可能较慢（如视频）
		httpc: httpx.RawClient(httpx.WithRawTimeout(5 * time.Minute)),
	}, nil
}

// Root 返回存储根目录（用于 file server 配置）。
func (s *Store) Root() string { return s.root }

// SaveBytes 把字节流落盘并返回相对路径（如 "202604/abc...png"）。
//
// ext 应不带点（"png" / "mp4"）。同样内容的文件复用同一份磁盘存储。
func (s *Store) SaveBytes(data []byte, ext string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("empty data")
	}
	if ext == "" {
		ext = "bin"
	}
	ext = strings.TrimPrefix(ext, ".")

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	subdir := time.Now().Format("200601") // YYYYMM
	relPath := filepath.Join(subdir, hash+"."+ext)
	abs := filepath.Join(s.root, relPath)

	// 已存在（同内容）则跳过写入
	if _, err := os.Stat(abs); err == nil {
		return filepath.ToSlash(relPath), nil
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	// 原子写入：唯一 tmp + rename，避免并发写入者（或多进程共享目录）互相覆盖
	// 对方仍在写的 tmp。CreateTemp 保证文件名唯一，rename 是 POSIX 原子操作。
	tmp, err := os.CreateTemp(filepath.Dir(abs), filepath.Base(abs)+".tmp.*")
	if err != nil {
		return "", fmt.Errorf("create tmp: %w", err)
	}
	tmpPath := tmp.Name()
	// 写 + 正确 close；失败路径下 best-effort 清理
	cleanupTmp := func() { _ = os.Remove(tmpPath) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanupTmp()
		return "", fmt.Errorf("write tmp %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		cleanupTmp()
		return "", fmt.Errorf("close tmp %s: %w", tmpPath, err)
	}
	// Rename 成功即视为落盘；若另一并发写者先完成，此处 Rename 覆盖，内容等价（SHA-256 一致）
	if err := os.Rename(tmpPath, abs); err != nil {
		cleanupTmp()
		return "", fmt.Errorf("rename %s→%s: %w", tmpPath, abs, err)
	}
	return filepath.ToSlash(relPath), nil
}

// SaveFromURL 下载远程 URL 并落盘，返回相对路径。
// 用于内容只在临时过期 URL 上可得的场景（如视频 Provider 给的 24h 过期 URL）。
func (s *Store) SaveFromURL(ctx context.Context, url, ext string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("empty url")
	}
	rctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(rctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	resp, err := s.httpc.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("download HTTP %d: %s", resp.StatusCode, url)
	}
	// 限制 200MB（普通 5-10s 视频不超过 50MB）
	body, err := io.ReadAll(io.LimitReader(resp.Body, 200<<20))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	return s.SaveBytes(body, ext)
}

// Open 安全打开存储文件（防路径穿越）。
//
// relPath 必须是 SaveBytes 返回的相对路径形式（forward slash + 子目录/哈希名）。
func (s *Store) Open(relPath string) (*os.File, error) {
	relPath = filepath.FromSlash(strings.TrimLeft(relPath, "/"))
	abs, err := filepath.Abs(filepath.Join(s.root, relPath))
	if err != nil {
		return nil, err
	}
	rootAbs, err := filepath.Abs(s.root)
	if err != nil {
		return nil, err
	}
	// 防止 ../../../etc/passwd 穿越
	if !strings.HasPrefix(abs, rootAbs+string(filepath.Separator)) && abs != rootAbs {
		return nil, fmt.Errorf("path escape blocked: %s", relPath)
	}
	return os.Open(abs)
}
