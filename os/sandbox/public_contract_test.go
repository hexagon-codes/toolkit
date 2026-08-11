package sandbox

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestSandboxExposesOnlyStructuredExecution(t *testing.T) {
	contract := reflect.TypeOf((*Sandbox)(nil)).Elem()
	if _, exists := contract.MethodByName("ExecCode"); exists {
		t.Fatal("Sandbox.ExecCode must be removed; language orchestration belongs to the caller")
	}
}

func TestSandboxStructuredCommandContractIsDocumented(t *testing.T) {
	repositoryRoot := sandboxRepositoryRoot(t)
	for _, contract := range []struct {
		path     string
		required []string
	}{
		{
			path: "README.md",
			required: []string{
				"### 结构化命令沙箱",
				"sandbox.Command{",
				"Env == nil",
				"不负责语言识别",
				"### 威胁模型与能力合同",
				"UntrustedCodeIsolationCapabilities",
				"TrustedBuildIsolationCapabilities",
				"同 UID",
				"最终 preflight",
				"载荷启动前拒绝",
			},
		},
		{
			path: "README.en.md",
			required: []string{
				"### Structured Command Sandbox",
				"sandbox.Command{",
				"Env == nil",
				"does not own language detection",
				"### Threat Model and Capability Contract",
				"UntrustedCodeIsolationCapabilities",
				"TrustedBuildIsolationCapabilities",
				"same UID",
				"final preflight",
				"before payload startup",
			},
		},
		{
			path: "CHANGELOG.md",
			required: []string{
				"**BREAKING** `os/sandbox`",
				"Exec(ctx, Command)",
				"Sandbox.ExecCode",
				"LimitStatusWeak",
				"RequiredCapabilities == 0",
				"UntrustedCodeIsolationCapabilities",
				"TrustedBuildIsolationCapabilities",
				"**BREAKING** `util/hash`",
				"MD5",
				"SHA1",
			},
		},
	} {
		t.Run(contract.path, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(repositoryRoot, contract.path))
			if err != nil {
				t.Fatalf("read contract document %s: %v", contract.path, err)
			}
			for _, required := range contract.required {
				if !strings.Contains(string(body), required) {
					t.Errorf("%s is missing structured sandbox contract marker %q", contract.path, required)
				}
			}
		})
	}
	if _, err := os.Stat(filepath.Join(repositoryRoot, "examples", "os", "sandbox", "main.go")); err != nil {
		t.Fatalf("structured sandbox example is missing: %v", err)
	}
}

func TestRemovedCompatibilityExportsRemainAbsent(t *testing.T) {
	repositoryRoot := sandboxRepositoryRoot(t)
	for _, contract := range []struct {
		path      string
		forbidden []string
	}{
		{path: filepath.Join("os", "sandbox"), forbidden: []string{
			"LimitStatusWeak",
			"NetPolicy",
			"NewDefaultPolicy",
			"CapabilityProcessTree",
			"UntrustedCodeMinimumCapabilities",
		}},
		{path: filepath.Join("util", "retry"), forbidden: []string{"RetryIf"}},
		{path: filepath.Join("util", "hash"), forbidden: []string{"MD5", "MD5Bytes", "SHA1", "SHA1Bytes"}},
	} {
		t.Run(contract.path, func(t *testing.T) {
			declared := packageLevelDeclarations(t, filepath.Join(repositoryRoot, contract.path))
			for _, name := range contract.forbidden {
				if source, exists := declared[name]; exists {
					t.Errorf("forbidden compatibility export %s remains in %s", name, source)
				}
			}
		})
	}
}

func sandboxRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve sandbox contract test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

// packageLevelDeclarations 只枚举非测试源码的包级声明，结构体字段不属于公共构造函数。
func packageLevelDeclarations(t *testing.T, directory string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read package directory %s: %v", directory, err)
	}
	declarations := make(map[string]string)
	fileSet := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(directory, name)
		file, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse package source %s: %v", path, err)
		}
		for _, declaration := range file.Decls {
			switch typed := declaration.(type) {
			case *ast.FuncDecl:
				declarations[typed.Name.Name] = path
			case *ast.GenDecl:
				for _, specification := range typed.Specs {
					switch item := specification.(type) {
					case *ast.TypeSpec:
						declarations[item.Name.Name] = path
					case *ast.ValueSpec:
						for _, declaredName := range item.Names {
							declarations[declaredName.Name] = path
						}
					}
				}
			}
		}
	}
	return declarations
}
