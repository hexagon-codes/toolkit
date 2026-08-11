package file

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExists(t *testing.T) {
	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "test_exists_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	if !Exists(tmpPath) {
		t.Error("Exists should return true for existing file")
	}

	if Exists("/nonexistent/path/file.txt") {
		t.Error("Exists should return false for nonexistent file")
	}
}

func TestIsFile(t *testing.T) {
	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "test_isfile_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "test_isfile_dir")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if !IsFile(tmpPath) {
		t.Error("IsFile should return true for file")
	}

	if IsFile(tmpDir) {
		t.Error("IsFile should return false for directory")
	}

	if IsFile("/nonexistent/path") {
		t.Error("IsFile should return false for nonexistent path")
	}
}

func TestIsDir(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "test_isdir")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "test_isdir_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	if !IsDir(tmpDir) {
		t.Error("IsDir should return true for directory")
	}

	if IsDir(tmpPath) {
		t.Error("IsDir should return false for file")
	}

	if IsDir("/nonexistent/path") {
		t.Error("IsDir should return false for nonexistent path")
	}
}

func TestSize(t *testing.T) {
	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "test_size_*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()

	content := "hello world"
	tmpFile.WriteString(content)
	tmpFile.Close()
	defer os.Remove(tmpPath)

	size, err := Size(tmpPath)
	if err != nil {
		t.Fatalf("Size failed: %v", err)
	}

	if size != int64(len(content)) {
		t.Errorf("Size = %d, want %d", size, len(content))
	}

	_, err = Size("/nonexistent/path")
	if err == nil {
		t.Error("Size should return error for nonexistent file")
	}
}

func TestExt(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"file.txt", ".txt"},
		{"file.tar.gz", ".gz"},
		{"file", ""},
		{"/path/to/file.go", ".go"},
	}

	for _, tt := range tests {
		if got := Ext(tt.path); got != tt.want {
			t.Errorf("Ext(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestExtWithoutDot(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"file.txt", "txt"},
		{"file.tar.gz", "gz"},
		{"file", ""},
		{"/path/to/file.go", "go"},
	}

	for _, tt := range tests {
		if got := ExtWithoutDot(tt.path); got != tt.want {
			t.Errorf("ExtWithoutDot(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"file.txt", "file.txt"},
		{"/path/to/file.go", "file.go"},
		{"/path/to/", "to"},
	}

	for _, tt := range tests {
		if got := Name(tt.path); got != tt.want {
			t.Errorf("Name(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestNameWithoutExt(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"file.txt", "file"},
		{"/path/to/file.go", "file"},
		{"file", "file"},
	}

	for _, tt := range tests {
		if got := NameWithoutExt(tt.path); got != tt.want {
			t.Errorf("NameWithoutExt(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestDir(t *testing.T) {
	root := string(os.PathSeparator)
	tests := []struct {
		path string
		want string
	}{
		{filepath.Join(root, "path", "to", "file.txt"), filepath.Join(root, "path", "to")},
		{"file.txt", "."},
	}

	for _, tt := range tests {
		if got := Dir(tt.path); got != tt.want {
			t.Errorf("Dir(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestReadWrite(t *testing.T) {
	tmpDir, mkdirErr := os.MkdirTemp("", "test_readwrite")
	if mkdirErr != nil {
		t.Fatalf("failed to create temp dir: %v", mkdirErr)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "test.txt")
	content := []byte("hello world")

	// Test Write
	if err := Write(filePath, content); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Test Read
	data, err := Read(filePath)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if string(data) != string(content) {
		t.Errorf("Read = %q, want %q", string(data), string(content))
	}
}

func TestReadWriteString(t *testing.T) {
	tmpDir, mkdirErr := os.MkdirTemp("", "test_readwritestring")
	if mkdirErr != nil {
		t.Fatalf("failed to create temp dir: %v", mkdirErr)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "test.txt")
	content := "hello world"

	// Test WriteString
	if err := WriteString(filePath, content); err != nil {
		t.Fatalf("WriteString failed: %v", err)
	}

	// Test ReadString
	data, err := ReadString(filePath)
	if err != nil {
		t.Fatalf("ReadString failed: %v", err)
	}

	if data != content {
		t.Errorf("ReadString = %q, want %q", data, content)
	}
}

func TestAppend(t *testing.T) {
	tmpDir, mkdirErr := os.MkdirTemp("", "test_append")
	if mkdirErr != nil {
		t.Fatalf("failed to create temp dir: %v", mkdirErr)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "test.txt")

	// Write initial content
	if err := Write(filePath, []byte("hello")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Append
	if err := Append(filePath, []byte(" world")); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Read and verify
	data, err := ReadString(filePath)
	if err != nil {
		t.Fatalf("ReadString failed: %v", err)
	}

	if data != "hello world" {
		t.Errorf("After append, content = %q, want %q", data, "hello world")
	}
}

func TestCopy(t *testing.T) {
	tmpDir, mkdirErr := os.MkdirTemp("", "test_copy")
	if mkdirErr != nil {
		t.Fatalf("failed to create temp dir: %v", mkdirErr)
	}
	defer os.RemoveAll(tmpDir)

	srcPath := filepath.Join(tmpDir, "src.txt")
	dstPath := filepath.Join(tmpDir, "dst.txt")
	content := "hello world"

	// Write source
	if err := WriteString(srcPath, content); err != nil {
		t.Fatalf("WriteString failed: %v", err)
	}

	// Copy
	if err := Copy(srcPath, dstPath); err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	// Verify
	data, err := ReadString(dstPath)
	if err != nil {
		t.Fatalf("ReadString failed: %v", err)
	}

	if data != content {
		t.Errorf("After copy, content = %q, want %q", data, content)
	}
}

func TestMove(t *testing.T) {
	tmpDir, mkdirErr := os.MkdirTemp("", "test_move")
	if mkdirErr != nil {
		t.Fatalf("failed to create temp dir: %v", mkdirErr)
	}
	defer os.RemoveAll(tmpDir)

	srcPath := filepath.Join(tmpDir, "src.txt")
	dstPath := filepath.Join(tmpDir, "dst.txt")
	content := "hello world"

	// Write source
	if err := WriteString(srcPath, content); err != nil {
		t.Fatalf("WriteString failed: %v", err)
	}

	// Move
	if err := Move(srcPath, dstPath); err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	// Verify source no longer exists
	if Exists(srcPath) {
		t.Error("Source file should not exist after move")
	}

	// Verify destination
	data, err := ReadString(dstPath)
	if err != nil {
		t.Fatalf("ReadString failed: %v", err)
	}

	if data != content {
		t.Errorf("After move, content = %q, want %q", data, content)
	}
}

func TestMkdirAll(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test_mkdirall")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	newDir := filepath.Join(tmpDir, "a", "b", "c")

	if err := MkdirAll(newDir); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	if !IsDir(newDir) {
		t.Error("MkdirAll should create nested directories")
	}
}

func TestDefaultPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 不提供 POSIX 权限位语义")
	}

	dir := filepath.Join(t.TempDir(), "private", "nested")
	if err := MkdirAll(dir); err != nil {
		t.Fatal(err)
	}
	for name, write := range map[string]func(string) error{
		"bytes":  func(path string) error { return Write(path, []byte("data")) },
		"string": func(path string) error { return WriteString(path, "data") },
		"append": func(path string) error { return Append(path, []byte("data")) },
	} {
		path := filepath.Join(dir, name)
		if err := write(path); err != nil {
			t.Fatalf("%s 写入失败: %v", name, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("%s 权限 = %04o, 期望 0600", name, got)
		}
	}

	source := filepath.Join(dir, "source")
	if err := os.WriteFile(source, []byte("data"), 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dir, "destination")
	if err := Copy(source, destination); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("复制目标权限 = %04o, 期望 0700", got)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got&0o027 != 0 {
		t.Errorf("目录权限 = %04o，包含组写入或其他用户权限", got)
	}
}

func TestIsEmpty(t *testing.T) {
	tmpDir, mkdirErr := os.MkdirTemp("", "test_isempty")
	if mkdirErr != nil {
		t.Fatalf("failed to create temp dir: %v", mkdirErr)
	}
	defer os.RemoveAll(tmpDir)

	// Create empty file
	emptyPath := filepath.Join(tmpDir, "empty.txt")
	if err := WriteString(emptyPath, ""); err != nil {
		t.Fatalf("WriteString failed: %v", err)
	}

	isEmpty, err := IsEmpty(emptyPath)
	if err != nil {
		t.Fatalf("IsEmpty failed: %v", err)
	}
	if !isEmpty {
		t.Error("IsEmpty should return true for empty file")
	}

	// Create non-empty file
	nonEmptyPath := filepath.Join(tmpDir, "nonempty.txt")
	if writeErr := WriteString(nonEmptyPath, "content"); writeErr != nil {
		t.Fatalf("WriteString failed: %v", writeErr)
	}

	isEmpty, err = IsEmpty(nonEmptyPath)
	if err != nil {
		t.Fatalf("IsEmpty failed: %v", err)
	}
	if isEmpty {
		t.Error("IsEmpty should return false for non-empty file")
	}
}

func TestJoin(t *testing.T) {
	result := Join("path", "to", "file.txt")
	expected := filepath.Join("path", "to", "file.txt")

	if result != expected {
		t.Errorf("Join = %q, want %q", result, expected)
	}
}

func TestListFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test_listfiles")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create files
	WriteString(filepath.Join(tmpDir, "file1.txt"), "")
	WriteString(filepath.Join(tmpDir, "file2.txt"), "")
	MkdirAll(filepath.Join(tmpDir, "subdir"))

	files, err := ListFiles(tmpDir)
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("ListFiles returned %d files, want 2", len(files))
	}
}

func TestListDirs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test_listdirs")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create dirs
	MkdirAll(filepath.Join(tmpDir, "dir1"))
	MkdirAll(filepath.Join(tmpDir, "dir2"))
	WriteString(filepath.Join(tmpDir, "file.txt"), "")

	dirs, err := ListDirs(tmpDir)
	if err != nil {
		t.Fatalf("ListDirs failed: %v", err)
	}

	if len(dirs) != 2 {
		t.Errorf("ListDirs returned %d dirs, want 2", len(dirs))
	}
}
