package file

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteReplacesDestinationInsteadOfTruncatingItInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if writeErr := Write(path, []byte("new")); writeErr != nil {
		t.Fatal(writeErr)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatal("Write truncated the destination inode instead of replacing it")
	}
	assertFileContent(t, path, "new")
}

func TestWriteWithPermDoesNotFollowDestinationSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation depends on host privileges")
	}

	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "destination.txt")
	if err := os.Symlink(outside, destination); err != nil {
		t.Fatal(err)
	}

	if err := WriteWithPerm(destination, []byte("inside"), 0o640); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, outside, "outside")
	assertFileContent(t, destination, "inside")
	info, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("WriteWithPerm left the destination symlink in place")
	}
}

func TestCopyRejectsSameFileWithoutTruncatingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Copy(path, path); err == nil {
		t.Fatal("Copy accepted identical source and destination")
	}
	assertFileContent(t, path, "keep")
}

func TestCopyDoesNotFollowDestinationSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation depends on host privileges")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(source, []byte("source"), 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "destination.txt")
	if err := os.Symlink(outside, destination); err != nil {
		t.Fatal(err)
	}

	if err := Copy(source, destination); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, outside, "outside")
	assertFileContent(t, destination, "source")
	info, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("Copy left the destination symlink in place")
	}
}

func TestWriteAndAppendRejectNonPermissionModeBits(t *testing.T) {
	dir := t.TempDir()
	invalidPerm := os.FileMode(0o600) | os.ModeDir
	tests := map[string]func(string) error{
		"write": func(path string) error {
			return WriteWithPerm(path, []byte("data"), invalidPerm)
		},
		"write-string": func(path string) error {
			return WriteStringWithPerm(path, "data", invalidPerm)
		},
		"append": func(path string) error {
			return AppendWithPerm(path, []byte("data"), invalidPerm)
		},
	}

	for name, operation := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name)
			if err := operation(path); err == nil {
				t.Fatal("operation accepted file type bits as permissions")
			}
			if Exists(path) {
				t.Fatal("operation created a file for invalid permissions")
			}
		})
	}
}

func TestValidateFilePermissionAcceptsChmodSpecialBits(t *testing.T) {
	perm := os.FileMode(0o600) | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	if err := validateFilePermission(perm); err != nil {
		t.Fatalf("valid chmod bits were rejected: %v", err)
	}
}

func TestWriteAcceptsNilDataAsEmptyContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty")
	if err := Write(path, nil); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, path, "")
}

func TestAtomicWriteCleansTemporaryFileWhenRenameFails(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "destination")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := Write(destination, []byte("data")); err == nil {
		t.Fatal("Write unexpectedly replaced a directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".destination.tmp-") {
			t.Fatalf("temporary file leaked after rename failure: %s", entry.Name())
		}
	}
}

func TestRemoveRejectsEmptyPath(t *testing.T) {
	if err := Remove(""); err == nil {
		t.Fatal("Remove accepted an empty path")
	}
}

func TestWalkRejectsNilCallback(t *testing.T) {
	if err := Walk(t.TempDir(), nil); err == nil {
		t.Fatal("Walk accepted a nil callback")
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}
