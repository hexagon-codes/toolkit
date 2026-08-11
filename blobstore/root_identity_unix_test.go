//go:build darwin || linux

package blobstore

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenedRootFilesystemRootDetectionUsesResolvedIdentity(t *testing.T) {
	linkPath := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(string(os.PathSeparator), linkPath); err != nil {
		t.Fatalf("create root symlink: %v", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(linkPath)
	if err != nil {
		t.Fatalf("resolve root symlink: %v", err)
	}
	openedRoot, err := os.OpenRoot(linkPath)
	if err != nil {
		t.Fatalf("open root symlink: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := openedRoot.Close(); closeErr != nil {
			t.Errorf("close opened root: %v", closeErr)
		}
	})

	isRoot, err := openedRootIsFilesystemRoot(openedRoot, resolvedPath)
	if err != nil {
		t.Fatalf("detect filesystem root: %v", err)
	}
	if !isRoot {
		t.Fatal("opened filesystem root was not detected from its resolved identity")
	}
}

func TestNewStoreRejectsFilesystemRootSymlinkWithoutLeakingHandle(t *testing.T) {
	linkPath := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(string(os.PathSeparator), linkPath); err != nil {
		t.Fatalf("create root symlink: %v", err)
	}
	before := countOpenFileDescriptors(t)
	for range 64 {
		if store, err := NewStore(linkPath); err == nil {
			registerStoreCleanup(t, store)
			t.Fatal("NewStore accepted a symlink to the filesystem root")
		}
	}
	after := countOpenFileDescriptors(t)
	if after != before {
		t.Fatalf("open file descriptors changed from %d to %d", before, after)
	}
}

func countOpenFileDescriptors(t *testing.T) int {
	t.Helper()
	directory := "/proc/self/fd"
	if runtime.GOOS == "darwin" {
		directory = "/dev/fd"
	}
	dir, err := os.Open(directory)
	if err != nil {
		t.Fatalf("open file descriptor directory: %v", err)
	}
	entries, readErr := dir.Readdirnames(-1)
	closeErr := dir.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read file descriptor directory: %v", errors.Join(readErr, closeErr))
	}
	return len(entries)
}
