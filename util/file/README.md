# File 文件操作工具

简化文件操作的便捷工具包，封装常用文件和目录操作。

## 特性

- ✅ 文件/目录判断和属性获取
- ✅ 文件读写（支持字符串和字节）
- ✅ 文件追加操作
- ✅ 文件复制和移动
- ✅ 目录创建和遍历
- ✅ 路径操作（连接、绝对路径）
- ✅ 零外部依赖

## 快速开始

### 文件判断

```go
import "github.com/everyday-items/toolkit/util/file"

// 判断文件是否存在
if file.Exists("/path/to/file.txt") {
    fmt.Println("File exists")
}

// 判断是否为文件
if file.IsFile("/path/to/file.txt") {
    fmt.Println("It's a file")
}

// 判断是否为目录
if file.IsDir("/path/to/dir") {
    fmt.Println("It's a directory")
}

// 判断文件是否为空
isEmpty, _ := file.IsEmpty("/path/to/file.txt")
```

### 文件读写

```go
// 读取文件内容
content, err := file.ReadString("/path/to/file.txt")
if err != nil {
    log.Fatal(err)
}
fmt.Println(content)

// 读取为字节数组
data, err := file.Read("/path/to/file.bin")

// 写入字符串到文件
err = file.WriteString("/path/to/output.txt", "Hello, World!")

// 写入字节到文件
err = file.Write("/path/to/output.bin", []byte{0x01, 0x02})

// 追加内容到文件
err = file.AppendString("/path/to/log.txt", "New log entry\n")
```

### 文件信息

```go
// 获取文件大小
size, err := file.Size("/path/to/file.txt")
fmt.Printf("File size: %d bytes\n", size)

// 获取文件扩展名
ext := file.Ext("/path/to/file.txt")      // ".txt"
ext = file.ExtWithoutDot("/path/to/file.txt")  // "txt"

// 获取文件名
name := file.Name("/path/to/file.txt")         // "file.txt"
name = file.NameWithoutExt("/path/to/file.txt") // "file"

// 获取文件所在目录
dir := file.Dir("/path/to/file.txt")  // "/path/to"
```

### 文件操作

```go
// 复制文件
err := file.Copy("/path/to/source.txt", "/path/to/dest.txt")

// 移动文件
err := file.Move("/path/to/old.txt", "/path/to/new.txt")

// 删除文件或目录
err := file.Remove("/path/to/file.txt")
```

### 目录操作

```go
// 创建多级目录
err := file.MkdirAll("/path/to/nested/dir")

// 列出目录下的所有文件（不包含子目录）
files, err := file.ListFiles("/path/to/dir")
for _, f := range files {
    fmt.Println(f)
}

// 列出目录下的所有子目录
dirs, err := file.ListDirs("/path/to/dir")

// 递归遍历目录
err = file.Walk("/path/to/dir", func(path string, info os.FileInfo, err error) error {
    if err != nil {
        return err
    }
    fmt.Println(path)
    return nil
})
```

### 路径操作

```go
// 连接路径
path := file.Join("path", "to", "file.txt")
// 输出: "path/to/file.txt"

// 获取绝对路径
absPath, err := file.Abs("./relative/path")
// 输出: "/full/path/to/relative/path"
```

## API 文档

### 判断函数

```go
// Exists 判断文件或目录是否存在
Exists(path string) bool

// IsFile 判断是否为文件
IsFile(path string) bool

// IsDir 判断是否为目录
IsDir(path string) bool

// IsEmpty 判断文件是否为空
IsEmpty(path string) (bool, error)
```

### 读取函数

```go
// Read 读取文件内容（字节数组）
Read(path string) ([]byte, error)

// ReadString 读取文件内容为字符串
ReadString(path string) (string, error)
```

### 写入函数

```go
// Write 写入文件内容（字节数组）
Write(path string, data []byte) error

// WriteString 写入字符串到文件
WriteString(path, content string) error

// Append 追加内容到文件（字节数组）
Append(path string, data []byte) error

// AppendString 追加字符串到文件
AppendString(path, content string) error
```

### 信息函数

```go
// Size 获取文件大小（字节）
Size(path string) (int64, error)

// Ext 获取文件扩展名（包含.）
Ext(path string) string

// ExtWithoutDot 获取文件扩展名（不包含.）
ExtWithoutDot(path string) string

// Name 获取文件名（包含扩展名）
Name(path string) string

// NameWithoutExt 获取文件名（不包含扩展名）
NameWithoutExt(path string) string

// Dir 获取文件所在目录
Dir(path string) string
```

### 操作函数

```go
// Copy 复制文件
Copy(src, dst string) error

// Move 移动文件
Move(src, dst string) error

// Remove 删除文件或目录
Remove(path string) error
```

### 目录函数

```go
// MkdirAll 创建多级目录
MkdirAll(path string) error

// ListFiles 列出目录下的所有文件（不包含子目录）
ListFiles(dir string) ([]string, error)

// ListDirs 列出目录下的所有子目录
ListDirs(dir string) ([]string, error)

// Walk 递归遍历目录
Walk(root string, fn func(path string, info os.FileInfo, err error) error) error
```

### 路径函数

```go
// Join 连接路径
Join(elem ...string) string

// Abs 获取绝对路径
Abs(path string) (string, error)
```

## 使用场景

### 1. 配置文件读取

```go
func LoadConfig(path string) (*Config, error) {
    // 检查配置文件是否存在
    if !file.Exists(path) {
        return nil, fmt.Errorf("config file not found: %s", path)
    }

    // 读取配置文件
    content, err := file.ReadString(path)
    if err != nil {
        return nil, err
    }

    // 解析配置
    var config Config
    if err := json.Unmarshal([]byte(content), &config); err != nil {
        return nil, err
    }

    return &config, nil
}
```

### 2. 日志文件追加

```go
func AppendLog(logFile, message string) error {
    // 格式化日志
    timestamp := time.Now().Format("2006-01-02 15:04:05")
    logEntry := fmt.Sprintf("[%s] %s\n", timestamp, message)

    // 追加到日志文件
    return file.AppendString(logFile, logEntry)
}

// 使用
AppendLog("/var/log/app.log", "Application started")
```

### 3. 文件备份

```go
func BackupFile(filePath string) error {
    // 检查文件是否存在
    if !file.Exists(filePath) {
        return fmt.Errorf("file not found: %s", filePath)
    }

    // 生成备份文件名
    timestamp := time.Now().Format("20060102150405")
    dir := file.Dir(filePath)
    name := file.NameWithoutExt(filePath)
    ext := file.Ext(filePath)
    backupPath := file.Join(dir, fmt.Sprintf("%s_%s%s", name, timestamp, ext))

    // 复制文件
    return file.Copy(filePath, backupPath)
}
```

### 4. 临时文件清理

```go
func CleanTempFiles(tempDir string, maxAge time.Duration) error {
    cutoff := time.Now().Add(-maxAge)

    return file.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }

        // 跳过目录
        if info.IsDir() {
            return nil
        }

        // 删除过期文件
        if info.ModTime().Before(cutoff) {
            log.Printf("Removing expired file: %s", path)
            return file.Remove(path)
        }

        return nil
    })
}

// 使用：清理7天前的临时文件
CleanTempFiles("/tmp/app", 7*24*time.Hour)
```

### 5. 文件上传处理

```go
func HandleFileUpload(c *gin.Context) {
    // 接收上传文件
    uploadedFile, err := c.FormFile("file")
    if err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }

    // 验证文件扩展名
    ext := file.ExtWithoutDot(uploadedFile.Filename)
    allowedExts := []string{"jpg", "png", "pdf"}
    if !contains(allowedExts, ext) {
        c.JSON(400, gin.H{"error": "file type not allowed"})
        return
    }

    // 生成唯一文件名
    filename := fmt.Sprintf("%s.%s", uuid.New().String(), ext)
    uploadDir := "/var/uploads"
    destPath := file.Join(uploadDir, filename)

    // 确保目录存在
    if err := file.MkdirAll(uploadDir); err != nil {
        c.JSON(500, gin.H{"error": "failed to create upload directory"})
        return
    }

    // 保存文件
    if err := c.SaveUploadedFile(uploadedFile, destPath); err != nil {
        c.JSON(500, gin.H{"error": "failed to save file"})
        return
    }

    c.JSON(200, gin.H{"filename": filename})
}
```

### 6. 批量文件处理

```go
func ProcessImagesInDirectory(dir string) error {
    // 列出所有图片文件
    files, err := file.ListFiles(dir)
    if err != nil {
        return err
    }

    for _, filePath := range files {
        // 只处理图片文件
        ext := file.ExtWithoutDot(filePath)
        if ext != "jpg" && ext != "png" {
            continue
        }

        // 处理图片
        log.Printf("Processing: %s", file.Name(filePath))
        if err := processImage(filePath); err != nil {
            log.Printf("Failed to process %s: %v", filePath, err)
        }
    }

    return nil
}
```

### 7. 数据导出

```go
func ExportToCSV(data [][]string, outputPath string) error {
    // 确保输出目录存在
    outputDir := file.Dir(outputPath)
    if err := file.MkdirAll(outputDir); err != nil {
        return err
    }

    // 生成 CSV 内容
    var buffer bytes.Buffer
    writer := csv.NewWriter(&buffer)
    writer.WriteAll(data)
    writer.Flush()

    // 写入文件
    return file.Write(outputPath, buffer.Bytes())
}

// 使用
data := [][]string{
    {"Name", "Age", "City"},
    {"Alice", "30", "New York"},
    {"Bob", "25", "London"},
}
ExportToCSV(data, "/exports/users.csv")
```

### 8. 磁盘空间检查

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

// 使用
size, err := GetDirectorySize("/var/data")
fmt.Printf("Directory size: %.2f MB\n", float64(size)/(1024*1024))
```

## 文件权限

默认文件权限：
- 创建文件：`0644` (rw-r--r--)
- 创建目录：`0755` (rwxr-xr-x)

## 错误处理

所有函数在失败时返回 `error`：

```go
// 读取不存在的文件
content, err := file.ReadString("/nonexistent.txt")
if err != nil {
    // 处理错误
    log.Printf("Failed to read file: %v", err)
}

// 写入只读目录
err = file.WriteString("/readonly/file.txt", "data")
if err != nil {
    log.Printf("Failed to write file: %v", err)
}
```

## 性能

```
Read():         根据文件大小（1MB 约 5ms）
Write():        根据文件大小（1MB 约 5ms）
Exists():       < 1ms
Copy():         根据文件大小（1MB 约 10ms）
Walk():         根据文件数量（1000个文件约 100ms）
```

## 注意事项

1. **路径分隔符**：
   - 自动处理不同操作系统的路径分隔符
   - 使用 `filepath` 包保证跨平台兼容

2. **文件权限**：
   - 默认权限：文件 0644，目录 0755
   - 如需自定义权限，使用 `os` 包

3. **并发安全**：
   - 函数本身无状态，可并发调用
   - 但文件 I/O 本身不保证原子性

4. **大文件处理**：
   - `Read()` 会将整个文件读入内存
   - 大文件建议使用流式处理

5. **错误处理**：
   - 所有操作都可能失败
   - 始终检查返回的 `error`

6. **文件覆盖**：
   - `Write()` 会覆盖现有文件
   - `Append()` 会追加到文件末尾

## 依赖

```bash
# 零外部依赖，仅使用标准库
import (
    "io"
    "os"
    "path/filepath"
    "strings"
)
```

## 扩展建议

如需更高级的文件操作，可考虑：
- `github.com/spf13/afero` - 文件系统抽象（支持内存文件系统）
- `github.com/otiai10/copy` - 高级文件复制
- `github.com/fsnotify/fsnotify` - 文件监控
