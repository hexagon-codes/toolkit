package blobstore

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestBlobstoreTTL_PurgeExpired 验证 TTL + Purge：过期 blob 被清、未过期与无 TTL 的保留。
func TestBlobstoreTTL_PurgeExpired(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	// 已过期（ttl 1ns，等待后必过期）
	expired, _ := s.SaveBytesWithTTL([]byte("expired-content"), "txt", time.Nanosecond)
	// 未过期（长 TTL）
	fresh, _ := s.SaveBytesWithTTL([]byte("fresh-content"), "txt", time.Hour)
	// 无 TTL（永不过期）
	permanent, _ := s.SaveBytes([]byte("permanent-content"), "txt")

	time.Sleep(2 * time.Millisecond)

	n, err := s.Purge(time.Now())
	if err != nil {
		t.Fatalf("Purge error = %v", err)
	}
	if n != 1 {
		t.Fatalf("应清除 1 个过期 blob，实际 %d", n)
	}

	// 过期的应已删
	if _, err := s.Open(expired); err == nil {
		t.Error("过期 blob 应已被清除")
	}
	// 未过期 + 无 TTL 的应保留
	if f, err := s.Open(fresh); err != nil {
		t.Errorf("未过期 blob 不应被清除: %v", err)
	} else {
		f.Close()
	}
	if f, err := s.Open(permanent); err != nil {
		t.Errorf("无 TTL blob 不应被清除: %v", err)
	} else {
		f.Close()
	}
}

// TestBlobstoreTTL_ExpiresAt 验证 ExpiresAt 读取与清除。
func TestBlobstoreTTL_ExpiresAt(t *testing.T) {
	s, _ := NewStore(t.TempDir())

	rel, _ := s.SaveBytes([]byte("x"), "txt")
	if _, ok, _ := s.ExpiresAt(rel); ok {
		t.Fatal("无 TTL 时 ExpiresAt 应 ok=false")
	}

	if err := s.SetTTL(rel, time.Hour); err != nil {
		t.Fatalf("SetTTL error = %v", err)
	}
	exp, ok, err := s.ExpiresAt(rel)
	if err != nil || !ok {
		t.Fatalf("设 TTL 后 ExpiresAt 应 ok=true; ok=%v err=%v", ok, err)
	}
	if time.Until(exp) <= 0 {
		t.Error("过期时刻应在未来")
	}

	// 清除 TTL
	if err := s.SetTTL(rel, 0); err != nil {
		t.Fatalf("清除 TTL error = %v", err)
	}
	if _, ok, _ := s.ExpiresAt(rel); ok {
		t.Error("清除 TTL 后应 ok=false")
	}
}

func TestBlobstoreTTL_RejectsEscapingPathsAndSymlinks(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "store")
	s, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}

	for _, relPath := range []string{
		"../outside",
		"nested/../../outside",
		"/absolute",
		`C:/windows/path`,
		`nested\windows\path`,
	} {
		t.Run(relPath, func(t *testing.T) {
			if setErr := s.SetTTL(relPath, time.Hour); setErr == nil {
				t.Fatalf("SetTTL(%q) 未拒绝越界路径", relPath)
			}
		})
	}

	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "blob"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("当前平台无法创建符号链接: %v", err)
	}
	if err := s.SetTTL("link/blob", time.Hour); err == nil {
		t.Fatal("SetTTL 跟随了指向存储根目录外的符号链接")
	}
	if _, err := os.Stat(filepath.Join(outside, "blob.ttl")); !os.IsNotExist(err) {
		t.Fatalf("SetTTL 在存储根目录外创建了元数据: %v", err)
	}
}

func TestBlobstoreTTL_RequiresExistingBlob(t *testing.T) {
	s := newTestStore(t)
	relPath, err := s.SaveBytes([]byte("existing"), "bin")
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.ToSlash(filepath.Join(filepath.Dir(relPath), "missing.bin"))
	if err := s.SetTTL(missing, time.Hour); err == nil {
		t.Fatal("SetTTL 接受了不存在的 blob")
	}
}

func TestBlobstore_DefaultPermissionsAreOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 不提供 POSIX 权限位语义")
	}

	root := filepath.Join(t.TempDir(), "store")
	s, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	relPath, err := s.SaveBytesWithTTL([]byte("private"), "bin", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string]os.FileMode{
		root: 0o700,
		filepath.Dir(filepath.Join(root, relPath)): 0o700,
		filepath.Join(root, relPath):               0o600,
		filepath.Join(root, relPath) + ttlSuffix:   0o600,
	} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("Stat(%q): %v", path, statErr)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s 权限 = %04o, 期望 %04o", path, got, want)
		}
	}
}
