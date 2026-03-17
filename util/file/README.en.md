[中文](README.md) | English

# File Utility

A convenient toolkit that simplifies file operations, encapsulating common file and directory operations.

## Features

- ✅ File/directory existence and attribute checks
- ✅ File reading and writing (supports strings and bytes)
- ✅ File append operations
- ✅ File copy and move
- ✅ Directory creation and traversal
- ✅ Path operations (join, absolute path)
- ✅ Zero external dependencies

## Quick Start

### File Checks

```go
import "github.com/everyday-items/toolkit/util/file"

// Check if file exists
if file.Exists("/path/to/file.txt") {
    fmt.Println("File exists")
}

// Check if it's a file
if file.IsFile("/path/to/file.txt") {
    fmt.Println("It's a file")
}

// Check if it's a directory
if file.IsDir("/path/to/dir") {
    fmt.Println("It's a directory")
}

// Check if file is empty
isEmpty, _ := file.IsEmpty("/path/to/file.txt")
```

### File Read/Write

```go
// Read file content
content, err := file.ReadString("/path/to/file.txt")
if err != nil {
    log.Fatal(err)
}
fmt.Println(content)

// Read as byte array
data, err := file.Read("/path/to/file.bin")

// Write string to file
err = file.WriteString("/path/to/output.txt", "Hello, World!")

// Write bytes to file
err = file.Write("/path/to/output.bin", []byte{0x01, 0x02})

// Append content to file
err = file.AppendString("/path/to/log.txt", "New log entry\n")
```

### File Information

```go
// Get file size
size, err := file.Size("/path/to/file.txt")
fmt.Printf("File size: %d bytes\n", size)

// Get file extension
ext := file.Ext("/path/to/file.txt")      // ".txt"
ext = file.ExtWithoutDot("/path/to/file.txt")  // "txt"

// Get filename
name := file.Name("/path/to/file.txt")         // "file.txt"
name = file.NameWithoutExt("/path/to/file.txt") // "file"

// Get file's directory
dir := file.Dir("/path/to/file.txt")  // "/path/to"
```

### File Operations

```go
// Copy file
err := file.Copy("/path/to/source.txt", "/path/to/dest.txt")

// Move file
err := file.Move("/path/to/old.txt", "/path/to/new.txt")

// Delete file or directory
err := file.Remove("/path/to/file.txt")
```

### Directory Operations

```go
// Create nested directories
err := file.MkdirAll("/path/to/nested/dir")

// List all files in directory (excluding subdirectories)
files, err := file.ListFiles("/path/to/dir")
for _, f := range files {
    fmt.Println(f)
}

// List all subdirectories
dirs, err := file.ListDirs("/path/to/dir")

// Recursively traverse directory
err = file.Walk("/path/to/dir", func(path string, info os.FileInfo, err error) error {
    if err != nil {
        return err
    }
    fmt.Println(path)
    return nil
})
```

### Path Operations

```go
// Join paths
path := file.Join("path", "to", "file.txt")
// Output: "path/to/file.txt"

// Get absolute path
absPath, err := file.Abs("./relative/path")
// Output: "/full/path/to/relative/path"
```

## API Reference

### Check Functions

```go
// Exists checks if a file or directory exists
Exists(path string) bool

// IsFile checks if path is a file
IsFile(path string) bool

// IsDir checks if path is a directory
IsDir(path string) bool

// IsEmpty checks if a file is empty
IsEmpty(path string) (bool, error)
```

### Read Functions

```go
// Read reads file content as byte array
Read(path string) ([]byte, error)

// ReadString reads file content as string
ReadString(path string) (string, error)
```

### Write Functions

```go
// Write writes byte array to file
Write(path string, data []byte) error

// WriteString writes string to file
WriteString(path, content string) error

// Append appends byte array to file
Append(path string, data []byte) error

// AppendString appends string to file
AppendString(path, content string) error
```

### Info Functions

```go
// Size gets file size in bytes
Size(path string) (int64, error)

// Ext gets file extension (including dot)
Ext(path string) string

// ExtWithoutDot gets file extension (without dot)
ExtWithoutDot(path string) string

// Name gets filename (including extension)
Name(path string) string

// NameWithoutExt gets filename (without extension)
NameWithoutExt(path string) string

// Dir gets the directory containing the file
Dir(path string) string
```

### Operation Functions

```go
// Copy copies a file
Copy(src, dst string) error

// Move moves a file
Move(src, dst string) error

// Remove deletes a file or directory
Remove(path string) error
```

### Directory Functions

```go
// MkdirAll creates nested directories
MkdirAll(path string) error

// ListFiles lists all files in a directory (excluding subdirectories)
ListFiles(dir string) ([]string, error)

// ListDirs lists all subdirectories
ListDirs(dir string) ([]string, error)

// Walk recursively traverses a directory
Walk(root string, fn func(path string, info os.FileInfo, err error) error) error
```

### Path Functions

```go
// Join joins path elements
Join(elem ...string) string

// Abs gets the absolute path
Abs(path string) (string, error)
```

## Use Cases

### 1. Config File Reading

```go
func LoadConfig(path string) (*Config, error) {
    // Check if config file exists
    if !file.Exists(path) {
        return nil, fmt.Errorf("config file not found: %s", path)
    }

    // Read config file
    content, err := file.ReadString(path)
    if err != nil {
        return nil, err
    }

    // Parse config
    var config Config
    if err := json.Unmarshal([]byte(content), &config); err != nil {
        return nil, err
    }

    return &config, nil
}
```

### 2. Log File Append

```go
func AppendLog(logFile, message string) error {
    // Format log entry
    timestamp := time.Now().Format("2006-01-02 15:04:05")
    logEntry := fmt.Sprintf("[%s] %s\n", timestamp, message)

    // Append to log file
    return file.AppendString(logFile, logEntry)
}

// Usage
AppendLog("/var/log/app.log", "Application started")
```

### 3. File Backup

```go
func BackupFile(filePath string) error {
    // Check if file exists
    if !file.Exists(filePath) {
        return fmt.Errorf("file not found: %s", filePath)
    }

    // Generate backup filename
    timestamp := time.Now().Format("20060102150405")
    dir := file.Dir(filePath)
    name := file.NameWithoutExt(filePath)
    ext := file.Ext(filePath)
    backupPath := file.Join(dir, fmt.Sprintf("%s_%s%s", name, timestamp, ext))

    // Copy file
    return file.Copy(filePath, backupPath)
}
```

### 4. Temporary File Cleanup

```go
func CleanTempFiles(tempDir string, maxAge time.Duration) error {
    cutoff := time.Now().Add(-maxAge)

    return file.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }

        // Skip directories
        if info.IsDir() {
            return nil
        }

        // Delete expired files
        if info.ModTime().Before(cutoff) {
            log.Printf("Removing expired file: %s", path)
            return file.Remove(path)
        }

        return nil
    })
}

// Usage: clean temporary files older than 7 days
CleanTempFiles("/tmp/app", 7*24*time.Hour)
```

### 5. File Upload Handling

```go
func HandleFileUpload(c *gin.Context) {
    // Receive uploaded file
    uploadedFile, err := c.FormFile("file")
    if err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    // Validate file extension
    ext := file.ExtWithoutDot(uploadedFile.Filename)
    allowedExts := []string{"jpg", "png", "pdf"}
    if !contains(allowedExts, ext) {
        c.JSON(400, gin.H{"error": "file type not allowed"})
        return
    }

    // Generate unique filename
    filename := fmt.Sprintf("%s.%s", uuid.New().String(), ext)
    uploadDir := "/var/uploads"
    destPath := file.Join(uploadDir, filename)

    // Ensure directory exists
    if err := file.MkdirAll(uploadDir); err != nil {
        c.JSON(500, gin.H{"error": "failed to create upload directory"})
        return
    }

    // Save file
    if err := c.SaveUploadedFile(uploadedFile, destPath); err != nil {
        c.JSON(500, gin.H{"error": "failed to save file"})
        return
    }

    c.JSON(200, gin.H{"filename": filename})
}
```

### 6. Batch File Processing

```go
func ProcessImagesInDirectory(dir string) error {
    // List all image files
    files, err := file.ListFiles(dir)
    if err != nil {
        return err
    }

    for _, filePath := range files {
        // Only process image files
        ext := file.ExtWithoutDot(filePath)
        if ext != "jpg" && ext != "png" {
            continue
        }

        // Process image
        log.Printf("Processing: %s", file.Name(filePath))
        if err := processImage(filePath); err != nil {
            log.Printf("Failed to process %s: %v", filePath, err)
        }
    }

    return nil
}
```

### 7. Data Export

```go
func ExportToCSV(data [][]string, outputPath string) error {
    // Ensure output directory exists
    outputDir := file.Dir(outputPath)
    if err := file.MkdirAll(outputDir); err != nil {
        return err
    }

    // Generate CSV content
    var buffer bytes.Buffer
    writer := csv.NewWriter(&buffer)
    writer.WriteAll(data)
    writer.Flush()

    // Write to file
    return file.Write(outputPath, buffer.Bytes())
}

// Usage
data := [][]string{
    {"Name", "Age", "City"},
    {"Alice", "30", "New York"},
    {"Bob", "25", "London"},
}
ExportToCSV(data, "/exports/users.csv")
```

### 8. Disk Space Check

```go
func GetDirectorySize(dir string) (int64, error) {
    var totalSize int64

    err := file.Walk(dir, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }

        if !info.IsDir() {
            totalSize += info.Size()
        }

        return nil
    })

    return totalSize, err
}

// Usage
size, err := GetDirectorySize("/var/data")
fmt.Printf("Directory size: %.2f MB\n", float64(size)/(1024*1024))
```

## File Permissions

Default file permissions:
- Create file: `0644` (rw-r--r--)
- Create directory: `0755` (rwxr-xr-x)

## Error Handling

All functions return `error` on failure:

```go
// Read non-existent file
content, err := file.ReadString("/nonexistent.txt")
if err != nil {
    // Handle error
    log.Printf("Failed to read file: %v", err)
}

// Write to read-only directory
err = file.WriteString("/readonly/file.txt", "data")
if err != nil {
    log.Printf("Failed to write file: %v", err)
}
```

## Performance

```
Read():         depends on file size (1MB ≈ 5ms)
Write():        depends on file size (1MB ≈ 5ms)
Exists():       < 1ms
Copy():         depends on file size (1MB ≈ 10ms)
Walk():         depends on file count (1000 files ≈ 100ms)
```

## Notes

1. **Path Separators**:
   - Automatically handles path separators for different operating systems
   - Uses `filepath` package to ensure cross-platform compatibility

2. **File Permissions**:
   - Default permissions: file 0644, directory 0755
   - Use the `os` package for custom permissions

3. **Concurrency Safety**:
   - Functions themselves are stateless and can be called concurrently
   - However, file I/O itself does not guarantee atomicity

4. **Large File Handling**:
   - `Read()` reads the entire file into memory
   - Use streaming for large files

5. **Error Handling**:
   - All operations can fail
   - Always check the returned `error`

6. **File Overwriting**:
   - `Write()` overwrites existing files
   - `Append()` appends to the end of the file

## Dependencies

```bash
# Zero external dependencies, uses only standard library
import (
    "io"
    "os"
    "path/filepath"
    "strings"
)
```

## Extension Suggestions

For more advanced file operations, consider:
- `github.com/spf13/afero` - File system abstraction (supports in-memory file systems)
- `github.com/otiai10/copy` - Advanced file copying
- `github.com/fsnotify/fsnotify` - File monitoring
