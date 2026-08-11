//go:build windows

package blobstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenedRootFilesystemRootDetectionUsesResolvedWindowsVolume(t *testing.T) {
	volumeRoot := filepath.VolumeName(os.TempDir()) + string(os.PathSeparator)
	resolvedPath, err := filepath.EvalSymlinks(volumeRoot)
	if err != nil {
		t.Fatalf("resolve Windows volume root: %v", err)
	}
	openedRoot, err := os.OpenRoot(volumeRoot)
	if err != nil {
		t.Fatalf("open Windows volume root: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := openedRoot.Close(); closeErr != nil {
			t.Errorf("close opened Windows volume root: %v", closeErr)
		}
	})

	isRoot, err := openedRootIsFilesystemRoot(openedRoot, resolvedPath)
	if err != nil {
		t.Fatalf("detect Windows volume root: %v", err)
	}
	if !isRoot {
		t.Fatal("opened Windows volume root was not detected from its resolved identity")
	}
}
