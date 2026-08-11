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
// （而非内嵌 base64 或依赖会过期的远程 URL）。若由上层提供文件访问，上层必须独立
// 建立安全目录能力，不能把 Root 返回的路径字符串当作目录身份。
package blobstore

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hexagon-codes/toolkit/net/httpx"
)

// Store 内容寻址 blob 存储。线程安全（依赖底层文件系统）。
type Store struct {
	rootPath       string                    // 构造时根路径的绝对路径快照，仅用于展示与诊断
	root           *os.Root                  // 构造时打开的稳定目录身份，所有存储操作均复用该句柄
	httpc          *http.Client              // 用于下载远程 URL
	ttlMu          sync.RWMutex              // 保证 TTL 更新、读取与清理在单个 Store 内线性化
	lifecycle      sync.RWMutex              // 关闭与在途操作之间的生命周期门禁
	state          storeState                // 关闭失败后保持 closing，允许后续 Close 继续收敛
	closeRoot      func(root *os.Root) error // 注入点仅用于验证关闭错误链，生产固定调用 os.Root.Close
	httpIdleClosed bool                      // HTTP 空闲连接只释放一次，其接口不返回错误
}

type storeState uint8

const (
	storeOpen storeState = iota
	storeClosing
	storeClosed
)

const maxRemoteBlobBytes int64 = 200 << 20

var (
	errRemoteBlobTooLarge = errors.New("blobstore: remote blob exceeds 200 MiB limit")
	// ErrStoreClosed 表示 Store 已开始关闭，不再接受新的存储操作。
	ErrStoreClosed = errors.New("blobstore: store is closed")
)

// NewStore 创建存储；root 是 blob 根目录。会自动 mkdir。
func NewStore(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("blobstore: storage root must not be empty")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve blob root %s: %w", root, err)
	}
	if isFilesystemRoot(rootAbs) {
		return nil, fmt.Errorf("blobstore: storage root must not be a filesystem root")
	}
	if mkdirErr := os.MkdirAll(rootAbs, 0o700); mkdirErr != nil {
		return nil, fmt.Errorf("create blob root %s: %w", rootAbs, mkdirErr)
	}
	resolvedBeforeOpen, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return nil, fmt.Errorf("resolve blob root before open %s: %w", rootAbs, err)
	}
	openedRoot, err := os.OpenRoot(rootAbs)
	if err != nil {
		return nil, fmt.Errorf("open blob root %s: %w", rootAbs, err)
	}
	closeOnError := func(primary error) (*Store, error) {
		return nil, errors.Join(primary, wrapOptionalError("blobstore: close root after construction failure", openedRoot.Close()))
	}
	resolvedAfterOpen, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return closeOnError(fmt.Errorf("resolve blob root after open %s: %w", rootAbs, err))
	}
	isRoot, err := openedRootIsFilesystemRoot(openedRoot, resolvedBeforeOpen, resolvedAfterOpen)
	if err != nil {
		return closeOnError(err)
	}
	if isRoot {
		return closeOnError(errors.New("blobstore: storage root must not resolve to a filesystem root"))
	}
	// 权限变更基于已经打开的目录身份，避免路径在检查与 chmod 之间被换绑。
	if chmodErr := openedRoot.Chmod(".", 0o700); chmodErr != nil {
		return closeOnError(fmt.Errorf("secure blob root permissions %s: %w", rootAbs, chmodErr))
	}
	httpClient, err := httpx.NewRawClient(httpx.WithRawTimeout(5 * time.Minute))
	if err != nil {
		return closeOnError(fmt.Errorf("create blob download client: %w", err))
	}
	return &Store{
		rootPath: rootAbs,
		root:     openedRoot,
		closeRoot: func(root *os.Root) error {
			return root.Close()
		},
		// 远程下载可能较慢（如视频）
		httpc: httpClient,
	}, nil
}

func openedRootIsFilesystemRoot(openedRoot *os.Root, resolvedCandidates ...string) (bool, error) {
	openedInfo, err := openedRoot.Stat(".")
	if err != nil {
		return false, fmt.Errorf("stat opened blob root: %w", err)
	}
	var candidateErr error
	for _, resolvedPath := range resolvedCandidates {
		candidateInfo, statErr := os.Stat(resolvedPath)
		if statErr != nil {
			candidateErr = errors.Join(candidateErr, fmt.Errorf("stat resolved blob root %s: %w", resolvedPath, statErr))
			continue
		}
		// 解析结果仅用于定位卷根，必须先证明它仍指向已经打开的目录身份。
		if !os.SameFile(openedInfo, candidateInfo) {
			candidateErr = errors.Join(candidateErr, fmt.Errorf("resolved blob root identity changed: %s", resolvedPath))
			continue
		}
		volumeRoot := filepath.VolumeName(resolvedPath) + string(os.PathSeparator)
		volumeInfo, statErr := os.Stat(volumeRoot)
		if statErr != nil {
			return false, fmt.Errorf("stat filesystem root %s: %w", volumeRoot, statErr)
		}
		return os.SameFile(openedInfo, volumeInfo), nil
	}
	return false, errors.Join(errors.New("blobstore: storage root identity changed during construction"), candidateErr)
}

func isFilesystemRoot(candidate string) bool {
	clean := filepath.Clean(candidate)
	volume := filepath.VolumeName(clean)
	return clean == volume+string(os.PathSeparator)
}

// Root 返回构造时配置的绝对路径快照，仅用于展示和诊断。
// 路径可能在构造后被换绑，不能把该字符串当作安全文件服务的目录能力。
func (s *Store) Root() string { return s.rootPath }

// Close 等待在途操作结束并关闭稳定根句柄。
// 首次关闭失败会阻止新操作，后续 Close 会继续尝试收敛关闭。
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if s.state == storeClosed {
		return nil
	}
	s.state = storeClosing
	if !s.httpIdleClosed && s.httpc != nil {
		// NewRawClient 为每个 Store 创建专属 Transport；该接口没有错误返回值。
		s.httpc.CloseIdleConnections()
		s.httpIdleClosed = true
	}
	if s.root == nil {
		s.state = storeClosed
		return nil
	}
	closeRoot := s.closeRoot
	if closeRoot == nil {
		closeRoot = func(root *os.Root) error { return root.Close() }
	}
	if err := closeRoot(s.root); err != nil && !errors.Is(err, os.ErrClosed) {
		return fmt.Errorf("blobstore: close store root: %w", err)
	}
	s.root = nil
	s.state = storeClosed
	return nil
}

func (s *Store) acquireRoot() (*os.Root, func(), error) {
	if s == nil {
		return nil, nil, ErrStoreClosed
	}
	s.lifecycle.RLock()
	if s.state != storeOpen || s.root == nil {
		s.lifecycle.RUnlock()
		return nil, nil, ErrStoreClosed
	}
	return s.root, s.lifecycle.RUnlock, nil
}

// SaveBytes 把字节流落盘并返回相对路径（如 "202604/abc...png"）。
//
// ext 应不带点（"png" / "mp4"）。同样内容的文件复用同一份磁盘存储。
func (s *Store) SaveBytes(data []byte, ext string) (string, error) {
	root, release, err := s.acquireRoot()
	if err != nil {
		return "", err
	}
	defer release()
	return s.saveBytes(root, data, ext)
}

func (s *Store) saveBytes(root *os.Root, data []byte, ext string) (relResult string, err error) {
	if len(data) == 0 {
		return "", fmt.Errorf("empty data")
	}
	ext, err = normalizeExtension(ext)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	subdir := time.Now().Format("200601") // YYYYMM
	relPath := filepath.Join(subdir, hash+"."+ext)
	if _, err = containedPath(s.rootPath, relPath); err != nil {
		return "", err
	}
	// 仅在既有文件内容确实匹配地址哈希时复用，损坏文件必须被替换。
	matches, err := storedBlobMatches(root, relPath, sum[:])
	if err != nil {
		return "", fmt.Errorf("verify stored blob: %w", err)
	}
	if matches {
		return filepath.ToSlash(relPath), nil
	}

	if err = root.MkdirAll(subdir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	// 原子写入：唯一 tmp + rename，避免并发写入者（或多进程共享目录）互相覆盖
	// 对方仍在写的 tmp。所有操作都经 os.Root 完成，符号链接不能把文件写出 root。
	tmp, err := createRootTemp(root, subdir, filepath.Base(relPath)+".tmp.", "blob")
	if err != nil {
		return "", fmt.Errorf("create tmp: %w", err)
	}
	defer func() { err = errors.Join(err, tmp.cleanup()) }()
	if _, err := tmp.file.Write(data); err != nil {
		return "", fmt.Errorf("write tmp %s: %w", tmp.path, err)
	}
	if err := tmp.file.Sync(); err != nil {
		return "", fmt.Errorf("sync tmp %s: %w", tmp.path, err)
	}
	if err := tmp.close(); err != nil {
		return "", err
	}
	// Rename 成功即视为原子替换；失败时只接受另一个写入者已经落下相同内容。
	if renameErr := root.Rename(tmp.path, relPath); renameErr != nil {
		matches, matchErr := storedBlobMatches(root, relPath, sum[:])
		if matchErr == nil && matches {
			return filepath.ToSlash(relPath), nil
		}
		return "", errors.Join(
			fmt.Errorf("rename %s→%s: %w", tmp.path, relPath, renameErr),
			wrapOptionalError("verify stored blob after rename failure", matchErr),
		)
	}
	return filepath.ToSlash(relPath), nil
}

func storedBlobMatches(root *os.Root, relPath string, expectedHash []byte) (_ bool, err error) {
	info, err := root.Lstat(relPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}

	file, err := root.Open(relPath)
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false, err
	}
	return bytes.Equal(hash.Sum(nil), expectedHash), nil
}

type rootTempFile struct {
	root *os.Root
	file *os.File
	path string
	kind string
	open bool
}

func createRootTemp(root *os.Root, dir, prefix, kind string) (*rootTempFile, error) {
	for range 10 {
		random := make([]byte, 16)
		if _, err := cryptorand.Read(random); err != nil {
			return nil, err
		}
		name := filepath.Join(dir, prefix+hex.EncodeToString(random))
		f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return &rootTempFile{root: root, file: f, path: name, kind: kind, open: true}, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("temporary filename collisions exhausted")
}

func (t *rootTempFile) close() error {
	if !t.open {
		return nil
	}
	t.open = false
	return wrapOptionalError(fmt.Sprintf("blobstore: close temporary %s %s", t.kind, t.path), t.file.Close())
}

func (t *rootTempFile) cleanup() error {
	closeErr := t.close()
	removeErr := t.root.Remove(t.path)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	return errors.Join(
		closeErr,
		wrapOptionalError(fmt.Sprintf("blobstore: remove temporary %s %s", t.kind, t.path), removeErr),
	)
}

func normalizeExtension(ext string) (string, error) {
	if ext == "" {
		return "bin", nil
	}
	ext = strings.TrimPrefix(ext, ".")
	if ext == "" || len(ext) > 16 {
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

// normalizeStorePath 校验并转换存储 API 使用的规范相对路径。
func normalizeStorePath(relPath string) (string, error) {
	if relPath == "" || strings.ContainsRune(relPath, '\x00') || strings.ContainsRune(relPath, '\\') {
		return "", fmt.Errorf("blobstore: invalid relative path %q", relPath)
	}
	clean := path.Clean(relPath)
	if clean == "." || clean != relPath || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("blobstore: invalid relative path %q", relPath)
	}
	if len(clean) >= 2 && clean[1] == ':' && ((clean[0] >= 'a' && clean[0] <= 'z') || (clean[0] >= 'A' && clean[0] <= 'Z')) {
		return "", fmt.Errorf("blobstore: invalid relative path %q", relPath)
	}
	return filepath.FromSlash(clean), nil
}

// SaveFromURL 下载远程 URL 并落盘，返回相对路径。
// 用于内容只在临时过期 URL 上可得的场景（如视频 Provider 给的 24h 过期 URL）。
func (s *Store) SaveFromURL(ctx context.Context, url, ext string) (relPath string, err error) {
	root, release, err := s.acquireRoot()
	if err != nil {
		return "", err
	}
	defer release()
	if isNilInterface(ctx) {
		return "", fmt.Errorf("blobstore: context must not be nil")
	}
	if url == "" {
		return "", fmt.Errorf("empty url")
	}
	rctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(rctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	resp, err := s.httpc.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer func() {
		err = errors.Join(err, wrapOptionalError("blobstore: close response body", resp.Body.Close()))
	}()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("download HTTP %d: %s", resp.StatusCode, url)
	}
	if resp.ContentLength > maxRemoteBlobBytes {
		return "", errRemoteBlobTooLarge
	}
	// 流式限制下载大小，并额外探测一个字节，禁止把超限响应静默截断后落盘。
	return s.saveStream(rctx, root, newMaxBytesReader(resp.Body, maxRemoteBlobBytes), ext)
}

type maxBytesReader struct {
	reader    io.Reader
	remaining int64
}

func newMaxBytesReader(reader io.Reader, maximum int64) io.Reader {
	return &maxBytesReader{reader: reader, remaining: maximum}
}

func (r *maxBytesReader) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if r.remaining > 0 {
		if int64(len(buffer)) > r.remaining {
			buffer = buffer[:r.remaining]
		}
		n, err := r.reader.Read(buffer)
		r.remaining -= int64(n)
		return n, err
	}

	var probe [1]byte
	n, err := r.reader.Read(probe[:])
	if n > 0 {
		return 0, errRemoteBlobTooLarge
	}
	return 0, err
}

// Open 安全打开存储文件（防路径穿越）。
//
// relPath 必须是 SaveBytes 返回的相对路径形式（forward slash + 子目录/哈希名）。
func (s *Store) Open(relPath string) (*os.File, error) {
	root, release, err := s.acquireRoot()
	if err != nil {
		return nil, err
	}
	defer release()
	relPath, err = normalizeStorePath(relPath)
	if err != nil {
		return nil, err
	}
	f, err := root.Open(relPath)
	if err != nil {
		return nil, fmt.Errorf("open blob: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("stat blob: %w", err), f.Close())
	}
	if !info.Mode().IsRegular() {
		return nil, errors.Join(errors.New("blobstore: blob is not a regular file"), f.Close())
	}
	return f, nil
}
