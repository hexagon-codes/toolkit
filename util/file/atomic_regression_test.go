package file

import (
	"io"
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

	previous, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := previous.Close(); closeErr != nil {
			t.Errorf("close previous destination: %v", closeErr)
		}
	})
	before, err := previous.Stat()
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
	oldContent, err := io.ReadAll(previous)
	if err != nil {
		t.Fatalf("read previous destination: %v", err)
	}
	if string(oldContent) != "old" {
		t.Fatalf("previous destination content = %q, want %q", oldContent, "old")
	}
	assertFileContent(t, path, "new")
}

func TestWriteWithPermRejectsDestinationSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "destination.txt")
	if err := os.Symlink(outside, destination); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("create Windows symlink: %v", err)
		}
		t.Fatal(err)
	}

	if err := WriteWithPerm(destination, []byte("inside"), 0o640); err == nil {
		t.Fatal("WriteWithPerm accepted a destination symlink")
	}
	assertFileContent(t, outside, "outside")
	info, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("WriteWithPerm replaced the destination symlink")
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

func TestCopyRejectsDestinationSymlink(t *testing.T) {
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
		if runtime.GOOS == "windows" {
			t.Skipf("create Windows symlink: %v", err)
		}
		t.Fatal(err)
	}

	if err := Copy(source, destination); err == nil {
		t.Fatal("Copy accepted a destination symlink")
	}
	assertFileContent(t, outside, "outside")
	info, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("Copy replaced the destination symlink")
	}
}

func TestAtomicReplaceRejectsParentRenameAndCleansPinnedTemporaryFile(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "parent")
	moved := filepath.Join(base, "moved")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(parent, "state.json")
	err := atomicReplace(destination, 0o600, func(w io.Writer) error {
		if _, writeErr := w.Write([]byte("new")); writeErr != nil {
			return writeErr
		}
		return os.Rename(parent, moved)
	})
	if err == nil {
		t.Fatal("atomicReplace accepted a renamed parent directory")
	}
	assertNoAtomicTemporaryFiles(t, moved, ".state.json.tmp-")
	if _, statErr := os.Stat(filepath.Join(moved, "state.json")); !os.IsNotExist(statErr) {
		t.Fatalf("destination was published after parent rename: %v", statErr)
	}
}

func TestAtomicReplaceRejectsParentSymlinkSwapAndCleansPinnedTemporaryFile(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "parent")
	moved := filepath.Join(base, "moved")
	attacker := filepath.Join(base, "attacker")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(attacker, 0o700); err != nil {
		t.Fatal(err)
	}
	attackerDestination := filepath.Join(attacker, "state.json")
	if err := os.WriteFile(attackerDestination, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(parent, "state.json")
	err := atomicReplace(destination, 0o600, func(w io.Writer) error {
		if _, writeErr := w.Write([]byte("new")); writeErr != nil {
			return writeErr
		}
		if renameErr := os.Rename(parent, moved); renameErr != nil {
			return renameErr
		}
		if symlinkErr := os.Symlink(attacker, parent); symlinkErr != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("create Windows directory symlink: %v", symlinkErr)
			}
			return symlinkErr
		}
		return nil
	})
	if err == nil {
		t.Fatal("atomicReplace accepted a rebound parent directory")
	}
	assertFileContent(t, attackerDestination, "outside")
	assertNoAtomicTemporaryFiles(t, moved, ".state.json.tmp-")
	if _, statErr := os.Stat(filepath.Join(moved, "state.json")); !os.IsNotExist(statErr) {
		t.Fatalf("destination was published after parent symlink swap: %v", statErr)
	}
}

func TestAppendWithPermRejectsDestinationSymlinkWithoutChangingTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target.txt")
	if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "append.txt")
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("create Windows symlink: %v", err)
		}
		t.Fatal(err)
	}

	appendErr := AppendWithPerm(link, []byte("unsafe"), 0o644)
	if appendErr == nil {
		t.Error("AppendWithPerm accepted a destination symlink")
	}
	assertFileContent(t, target, "outside")
	targetInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := targetInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("target permissions = %04o, want 0600", got)
	}
	linkInfo, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatal("AppendWithPerm replaced the destination symlink")
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

func assertNoAtomicTemporaryFiles(t *testing.T, dir, prefix string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			t.Fatalf("temporary file leaked after parent rebinding: %s", entry.Name())
		}
	}
}
