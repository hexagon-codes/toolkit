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
	cryptorand "crypto/rand"
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
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root %s: %w", root, err)
	}
	return &Store{
		root: rootAbs,
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
	var err error
	ext, err = normalizeExtension(ext)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	subdir := time.Now().Format("200601") // YYYYMM
	relPath := filepath.Join(subdir, hash+"."+ext)
	if _, err := containedPath(s.root, relPath); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return "", fmt.Errorf("open blob root: %w", err)
	}
	defer root.Close()

	// 已存在（同内容）则跳过写入
	if _, err := root.Stat(relPath); err == nil {
		return filepath.ToSlash(relPath), nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat blob: %w", err)
	}

	if err := root.MkdirAll(subdir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	// 原子写入：唯一 tmp + rename，避免并发写入者（或多进程共享目录）互相覆盖
	// 对方仍在写的 tmp。所有操作都经 os.Root 完成，符号链接不能把文件写出 root。
	tmp, tmpPath, err := createRootTemp(root, subdir, filepath.Base(relPath)+".tmp.")
	if err != nil {
		return "", fmt.Errorf("create tmp: %w", err)
	}
	// 写 + 正确 close；失败路径下 best-effort 清理
	cleanupTmp := func() { _ = root.Remove(tmpPath) }
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
	if err := root.Rename(tmpPath, relPath); err != nil {
		// Windows 不允许 Rename 覆盖已存在目标；并发写入同一内容时目标等价。
		if _, statErr := root.Stat(relPath); statErr == nil {
			cleanupTmp()
			return filepath.ToSlash(relPath), nil
		}
		cleanupTmp()
		return "", fmt.Errorf("rename %s→%s: %w", tmpPath, relPath, err)
	}
	return filepath.ToSlash(relPath), nil
}

func createRootTemp(root *os.Root, dir, prefix string) (*os.File, string, error) {
	for range 10 {
		random := make([]byte, 16)
		if _, err := cryptorand.Read(random); err != nil {
			return nil, "", err
		}
		name := filepath.Join(dir, prefix+hex.EncodeToString(random))
		f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return f, name, nil
		}
		if !os.IsExist(err) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("temporary filename collisions exhausted")
}

func normalizeExtension(ext string) (string, error) {
	if ext == "" {
		return "bin", nil
	}
	ext = strings.TrimPrefix(ext, ".")
	if len(ext) == 0 || len(ext) > 16 {
		return "", fmt.Errorf("invalid extension")
	}
	for i := 0; i < len(ext); i++ {
		c := ext[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return "", fmt.Errorf("invalid extension")
		}
	}
	return strings.ToLower(ext), nil
}

func containedPath(root, relPath string) (string, error) {
	abs := filepath.Join(root, relPath)
	relToRoot, err := filepath.Rel(root, abs)
	if err != nil {
		return "", fmt.Errorf("resolve blob path: %w", err)
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) || filepath.IsAbs(relToRoot) {
		return "", fmt.Errorf("blob path escapes root")
	}
	return abs, nil
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
	if _, err := containedPath(s.root, relPath); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, fmt.Errorf("open blob root: %w", err)
	}
	f, err := root.Open(relPath)
	_ = root.Close()
	return f, err
}
