//go:build darwin || linux

package blobstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type replaceTempWithNonEmptyDirectoryReader struct {
	rootPath  string
	primary   error
	delivered bool
}

func (r *replaceTempWithNonEmptyDirectoryReader) Read(buffer []byte) (int, error) {
	if !r.delivered {
		r.delivered = true
		return copy(buffer, "x"), nil
	}
	entries, err := os.ReadDir(r.rootPath)
	if err != nil {
		return 0, err
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".stream.") {
			continue
		}
		tempPath := filepath.Join(r.rootPath, entry.Name())
		if err := os.Remove(tempPath); err != nil {
			return 0, err
		}
		if err := os.Mkdir(tempPath, 0o700); err != nil {
			return 0, err
		}
		if err := os.WriteFile(filepath.Join(tempPath, "child"), []byte("block cleanup"), 0o600); err != nil {
			return 0, err
		}
		return 0, r.primary
	}
	return 0, errors.New("stream temporary file not found")
}

func TestSaveStreamJoinsCopyAndCleanupErrors(t *testing.T) {
	store := newTestStore(t)
	copyErr := errors.New("stream copy failed")
	reader := &replaceTempWithNonEmptyDirectoryReader{
		rootPath: store.Root(),
		primary:  copyErr,
	}

	_, err := store.SaveStream(t.Context(), reader, "bin")
	if !errors.Is(err, copyErr) {
		t.Fatalf("SaveStream error = %v, want copy failure", err)
	}
	if err == nil || !strings.Contains(err.Error(), "remove temporary stream") {
		t.Fatalf("SaveStream error = %v, want joined cleanup failure", err)
	}
}
