package blobstore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	return s
}

// TestSaveBytes_Basic 基本落盘 + 路径归一化
func TestSaveBytes_Basic(t *testing.T) {
	s := newTestStore(t)
	rel, err := s.SaveBytes([]byte("hello"), "png")
	if err != nil {
		t.Fatalf("SaveBytes: %v", err)
	}
	if !strings.HasSuffix(rel, ".png") {
		t.Errorf("path should end with .png, got %q", rel)
	}
	// yyyymm/{sha256}.png 结构
	parts := strings.Split(rel, "/")
	if len(parts) != 2 {
		t.Errorf("want 2 path components, got %d: %q", len(parts), rel)
	}
	if len(parts[0]) != 6 {
		t.Errorf("want YYYYMM subdir, got %q", parts[0])
	}
}

// TestSaveBytes_ContentAddressDedup 同内容重复写入应返回同一路径 + 不重写文件
func TestSaveBytes_ContentAddressDedup(t *testing.T) {
	s := newTestStore(t)
	rel1, err := s.SaveBytes([]byte("same content"), "png")
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(s.Root(), rel1)
	stat1, _ := os.Stat(abs)

	// 等 10ms 再写入，验证 mtime 是否变化
	time.Sleep(10 * time.Millisecond)
	rel2, err := s.SaveBytes([]byte("same content"), "png")
	if err != nil {
		t.Fatal(err)
	}
	if rel1 != rel2 {
		t.Errorf("same content → different paths: %q vs %q", rel1, rel2)
	}
	stat2, _ := os.Stat(abs)
	if !stat1.ModTime().Equal(stat2.ModTime()) {
		t.Errorf("dedupe should skip write; mtime changed: %v → %v", stat1.ModTime(), stat2.ModTime())
	}
}

// TestSaveBytes_AtomicWrite 原子写入 — 无 .tmp 残留
func TestSaveBytes_AtomicWrite(t *testing.T) {
	s := newTestStore(t)
	_, err := s.SaveBytes([]byte("atomic"), "mp4")
	if err != nil {
		t.Fatal(err)
	}
	// 扫描整个 root，不应残留 .tmp 文件
	_ = filepath.WalkDir(s.Root(), func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".tmp") {
			t.Errorf("atomic write leaked .tmp file: %s", path)
		}
		return nil
	})
}

// TestSaveBytes_Empty 空输入应报错
func TestSaveBytes_Empty(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.SaveBytes(nil, "png"); err == nil {
		t.Error("empty data should error")
	}
	if _, err := s.SaveBytes([]byte{}, "png"); err == nil {
		t.Error("empty data should error")
	}
}

// TestSaveBytes_RejectsUnsafeExtension verifies that an extension can never
// introduce another path component or an OS-dependent path interpretation.
func TestSaveBytes_RejectsUnsafeExtension(t *testing.T) {
	s := newTestStore(t)
	tests := []struct {
		name string
		ext  string
	}{
		{name: "parent traversal", ext: "../../outside"},
		{name: "forward separator", ext: "png/child"},
		{name: "backward separator", ext: `png\\child`},
		{name: "unix absolute", ext: "/tmp/png"},
		{name: "windows absolute", ext: `C:\\temp\\png`},
		{name: "dot component", ext: ".."},
		{name: "unicode confusable", ext: "ｐｎｇ"},
		{name: "unicode letters", ext: "图片"},
		{name: "embedded nul", ext: "png\x00jpg"},
		{name: "too long", ext: strings.Repeat("a", 17)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rel, err := s.SaveBytes([]byte("untrusted extension"), tt.ext); err == nil {
				t.Fatalf("SaveBytes(%q) returned unsafe path %q; want error", tt.ext, rel)
			}
		})
	}
}

func TestSaveBytes_ReturnedPathIsContainedByRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "store")
	s, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, saveErr := s.SaveBytes([]byte("containment"), "../../escaped"); saveErr == nil {
		t.Fatal("path-escaping extension must be rejected")
	}
	if _, statErr := os.Stat(filepath.Join(parent, "escaped")); !os.IsNotExist(statErr) {
		t.Fatalf("extension escaped store root: stat error = %v", statErr)
	}

	rel, err := s.SaveBytes([]byte("containment"), ".PNG")
	if err != nil {
		t.Fatalf("safe extension rejected: %v", err)
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	relToRoot, err := filepath.Rel(root, abs)
	if err != nil {
		t.Fatal(err)
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) || filepath.IsAbs(relToRoot) {
		t.Fatalf("returned path %q escapes root %q", rel, root)
	}
	if !strings.HasSuffix(rel, ".png") {
		t.Fatalf("extension must be canonical lowercase, got %q", rel)
	}
}

func TestSaveBytes_RejectsSymlinkedStorageSubdirectory(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "store")
	s, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside")
	if mkErr := os.Mkdir(outside, 0o755); mkErr != nil {
		t.Fatal(mkErr)
	}
	monthDir := filepath.Join(root, time.Now().Format("200601"))
	if symErr := os.Symlink(outside, monthDir); symErr != nil {
		t.Skipf("symlink unavailable on this platform: %v", symErr)
	}

	if rel, saveErr := s.SaveBytes([]byte("must stay contained"), "png"); saveErr == nil {
		t.Fatalf("SaveBytes followed a symlinked storage directory: %q", rel)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("SaveBytes wrote outside root through symlink: %v", entries)
	}
}

// TestOpen_PathTraversalBlocked 安全：路径穿越必须被拒绝
//
// 这是 Critical 安全测试 — 如果 Open 接受 "../xxx"，
// 任意客户端知道 API 可读取服务器上任何文件。
func TestOpen_PathTraversalBlocked(t *testing.T) {
	s := newTestStore(t)
	// 先写一个合法文件保证 store 可工作
	_, err := s.SaveBytes([]byte("legit"), "png")
	if err != nil {
		t.Fatal(err)
	}

	attempts := []string{
		"../etc/passwd",
		"../../etc/passwd",
		"../../../etc/passwd",
		"202604/../../etc/passwd",
		"/etc/passwd",
		"/../../etc/passwd",
		"./../../../etc/passwd",
	}
	for _, rel := range attempts {
		t.Run(rel, func(t *testing.T) {
			f, err := s.Open(rel)
			if err == nil {
				f.Close()
				t.Errorf("path traversal NOT blocked: %q opened successfully", rel)
			}
			// 错误文案 / 类型不重要，关键是必须错
		})
	}
}

// TestOpen_ValidPath 正常路径应可打开
func TestOpen_ValidPath(t *testing.T) {
	s := newTestStore(t)
	rel, err := s.SaveBytes([]byte("content"), "png")
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Open(rel)
	if err != nil {
		t.Fatalf("Open(%q) failed: %v", rel, err)
	}
	defer f.Close()
	buf := make([]byte, 100)
	n, _ := f.Read(buf)
	if string(buf[:n]) != "content" {
		t.Errorf("content mismatch: got %q want %q", buf[:n], "content")
	}
}

func TestOpen_BlocksEscapingSymlink(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "store")
	s, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink unavailable on this platform: %v", err)
	}

	if f, err := s.Open("link/secret"); err == nil {
		_ = f.Close()
		t.Fatal("Open followed a symlink outside the store root")
	}
}

// TestSaveFromURL_Basic 下载远程 URL → 落盘
func TestSaveFromURL_Basic(t *testing.T) {
	// 模拟 Provider 临时 URL
	body := []byte("fake video bytes")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(body)
	}))
	defer ts.Close()

	s := newTestStore(t)
	rel, err := s.SaveFromURL(context.Background(), ts.URL+"/video.mp4", "mp4")
	if err != nil {
		t.Fatalf("SaveFromURL: %v", err)
	}
	if !strings.HasSuffix(rel, ".mp4") {
		t.Errorf("want .mp4, got %q", rel)
	}
	// 读回验证
	f, err := s.Open(rel)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	stat, _ := f.Stat()
	if stat.Size() != int64(len(body)) {
		t.Errorf("size mismatch: got %d want %d", stat.Size(), len(body))
	}
}

// TestSaveFromURL_HTTPError 4xx/5xx 应报错，不留空文件
func TestSaveFromURL_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	s := newTestStore(t)
	if _, err := s.SaveFromURL(context.Background(), ts.URL, "mp4"); err == nil {
		t.Error("404 should error")
	}
	// 扫描确认无文件落盘
	var count int
	_ = filepath.WalkDir(s.Root(), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			count++
		}
		return nil
	})
	if count > 0 {
		t.Errorf("failed download left %d files", count)
	}
}

// TestSaveFromURL_EmptyURL 空 URL 应拒绝
func TestSaveFromURL_EmptyURL(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.SaveFromURL(context.Background(), "", "mp4"); err == nil {
		t.Error("empty URL should error")
	}
}

// TestSaveBytes_ConcurrentSameContent 并发写相同内容 — 所有路径应相同，不应 tmp 名冲突
func TestSaveBytes_ConcurrentSameContent(t *testing.T) {
	s := newTestStore(t)
	const N = 32
	data := []byte("concurrent same content")
	results := make(chan string, N)
	errs := make(chan error, N)
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			rel, err := s.SaveBytes(data, "png")
			if err != nil {
				errs <- err
				return
			}
			results <- rel
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		t.Errorf("concurrent write failed: %v", err)
	}
	var first string
	count := 0
	for rel := range results {
		count++
		if first == "" {
			first = rel
		} else if rel != first {
			t.Errorf("concurrent same content → different paths: %q vs %q", rel, first)
		}
	}
	if count != N {
		t.Errorf("want %d results, got %d", N, count)
	}

	// 验证 root 下不应有 .tmp. 残留
	_ = filepath.WalkDir(s.Root(), func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		if strings.Contains(filepath.Base(path), ".tmp.") {
			t.Errorf("tmp leaked after concurrent writes: %s", path)
		}
		return nil
	})
}

// TestSaveBytes_ConcurrentDifferentContent 并发写不同内容应得到 N 个不同路径，无残留
func TestSaveBytes_ConcurrentDifferentContent(t *testing.T) {
	s := newTestStore(t)
	const N = 32
	var wg sync.WaitGroup
	paths := make(chan string, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			data := []byte("unique content " + string(rune('A'+i)))
			rel, err := s.SaveBytes(data, "png")
			if err != nil {
				t.Errorf("write %d failed: %v", i, err)
				return
			}
			paths <- rel
		}(i)
	}
	wg.Wait()
	close(paths)

	seen := make(map[string]bool)
	for p := range paths {
		if seen[p] {
			t.Errorf("duplicate path under different content: %s", p)
		}
		seen[p] = true
	}
	if len(seen) != N {
		t.Errorf("want %d unique paths, got %d", N, len(seen))
	}
}
