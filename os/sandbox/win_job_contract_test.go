package sandbox

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWindowsJobLimitSourceSeparatesOptionalLimits(t *testing.T) {
	source := windowsJobLimitFunctionSource(t)

	for _, required := range []string{
		"if memoryLimitBytes < 0",
		"if maxProcesses < 0",
		"LimitFlags = jobObjectLimitKillOnClose",
		"if memoryLimitBytes > 0",
		"if maxProcesses > 0",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("windowsJobLimitInformation source does not contain %q", required)
		}
	}

	for _, forbidden := range []string{
		"memoryLimitBytes <= 0",
		"maxProcesses <= 0",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("windowsJobLimitInformation source contains zero-value rejection %q", forbidden)
		}
	}
}

func TestSandboxConfigRejectsNegativeWindowsJobLimits(t *testing.T) {
	for _, test := range []struct {
		name   string
		config Config
	}{
		{name: "memory", config: Config{MaxMemoryBytes: -1}},
		{name: "processes", config: Config{MaxProcesses: -1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := test.config
			config.Workspace = filepath.Join(t.TempDir(), "workspace")
			_, err := validateSandboxConfigSemantics(config)
			if err == nil || !strings.Contains(err.Error(), "sandbox resource limits must not be negative") {
				t.Fatalf("validateSandboxConfigSemantics() error = %v, want negative resource limit error", err)
			}
		})
	}
}

// windowsJobLimitFunctionSource 返回目标函数的规范化源码，避免静态合同受空白格式影响。
func windowsJobLimitFunctionSource(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve Windows Job contract test path")
	}
	path := filepath.Join(filepath.Dir(filename), "win_syscall.go")
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse Windows Job source %s: %v", path, err)
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "windowsJobLimitInformation" {
			continue
		}
		var formatted bytes.Buffer
		if err := format.Node(&formatted, fileSet, function); err != nil {
			t.Fatalf("format windowsJobLimitInformation source: %v", err)
		}
		return formatted.String()
	}
	t.Fatal("windowsJobLimitInformation source is missing")
	return ""
}
