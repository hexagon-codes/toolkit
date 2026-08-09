package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBugProofLinuxUnshareBackendMustApplyFilesystemPolicy(t *testing.T) {
	body := mustFunctionBody(t, "sandbox_linux.go", "func (s *linuxSandbox) unshareArgs")

	required := []string{"ReadablePaths", "DeniedPaths"}
	for _, token := range required {
		if !strings.Contains(body, token) {
			t.Errorf("linux unshare backend does not apply %s; body:\n%s", token, body)
		}
	}
}

func TestBugProofLinuxBwrapMustNotBindHostTmpReadWrite(t *testing.T) {
	body := mustFunctionBody(t, "sandbox_linux.go", "func (s *linuxSandbox) bwrapArgs")

	if strings.Contains(body, `"--bind", "/tmp", "/tmp"`) {
		t.Fatalf("linux bwrap backend bind-mounts host /tmp read-write; want private tmpfs/dir instead. body:\n%s", body)
	}
}

func TestBugProofLinuxBwrapDistinguishesDeniedFilesAndDirectories(t *testing.T) {
	body := mustFunctionBody(t, "sandbox_linux.go", "func (s *linuxSandbox) bwrapArgs")
	for _, required := range []string{"os.Stat", `"--ro-bind", "/dev/null"`, `"--tmpfs"`} {
		if !strings.Contains(body, required) {
			t.Fatalf("linux bwrap deny policy is missing %q; body:\n%s", required, body)
		}
	}
}

func TestNewCreatesWorkspaceAndRejectsDangerousConfiguration(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "nested", "workspace")
	sandboxInstance, err := New(Config{Workspace: workspace})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if sandboxInstance == nil {
		t.Fatal("New() returned a nil sandbox")
	}
	info, err := os.Stat(workspace)
	if err != nil || !info.IsDir() {
		t.Fatalf("workspace was not created as a directory: info=%v err=%v", info, err)
	}

	tests := []Config{
		{Workspace: string(filepath.Separator)},
		{Workspace: t.TempDir(), Timeout: -1},
		{Workspace: t.TempDir(), DeniedPaths: []string{`/tmp/x") (allow network*) (`}},
		{Workspace: t.TempDir(), ReadablePaths: []string{"relative/path"}},
	}
	for _, config := range tests {
		if _, err := New(config); err == nil {
			t.Fatalf("New(%+v) returned nil error", config)
		}
	}
}

func TestSandboxExecRejectsNilContextWithoutPanic(t *testing.T) {
	sandboxInstance, err := New(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Exec(nil, ...) panicked: %v", recovered)
		}
	}()
	if _, err := sandboxInstance.Exec(nil, "true", nil); !errors.Is(err, ErrInvalidContext) { //nolint:staticcheck // 必须覆盖公开 API 的 nil context 防御边界。
		t.Fatalf("Exec(nil, ...) error = %v, want ErrInvalidContext", err)
	}
}

func TestBugProofResourceLimitFieldsMustBeEnforced(t *testing.T) {
	refs := referencesOutsideFile(t, []string{"MaxWorkspaceBytes", "MaxArtifactBytes"}, "sandbox.go")

	for _, field := range []string{"MaxWorkspaceBytes", "MaxArtifactBytes"} {
		if len(refs[field]) == 0 {
			t.Errorf("%s is only defaulted/documented and has no enforcement reference outside sandbox.go", field)
		}
	}
}

func TestBugProofPosixMemoryAndProcessLimitsMustBeEnforced(t *testing.T) {
	files := []string{"sandbox_basic.go", "sandbox_darwin.go", "sandbox_linux.go", "exec_posix.go"}
	tokens := []string{"MaxMemoryBytes", "MaxProcesses"}

	for _, token := range tokens {
		var hits []string
		for _, file := range files {
			if strings.Contains(mustReadSandboxSource(t, file), token) {
				hits = append(hits, file)
			}
		}
		if len(hits) == 0 {
			t.Errorf("%s has no POSIX enforcement reference in %s", token, strings.Join(files, ", "))
		}
	}
}

func TestBugProofWindowsExecMustReturnRealExitCode(t *testing.T) {
	body := mustFunctionBody(t, "sandbox_windows.go", "func (s *windowsSandbox) Exec")

	if !strings.Contains(body, ".ExitCode()") {
		t.Fatalf("windows Exec does not use os.ProcessState.ExitCode(), so non-zero child exits are collapsed to 0/1. body:\n%s", body)
	}
}

func TestBugProofWindowsExecMustPropagateLifecycleErrors(t *testing.T) {
	body := mustFunctionBody(t, "sandbox_windows.go", "func (s *windowsSandbox) Exec")

	if strings.Contains(body, "_ = proc.Kill()") {
		t.Fatalf("windows Exec discards the process termination result; body:\n%s", body)
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "<-done" {
			t.Fatalf("windows Exec discards the process wait result; body:\n%s", body)
		}
	}
	for _, required := range []string{"killErr := proc.Kill()", "wait.err", "sandbox exec wait failed"} {
		if !strings.Contains(body, required) {
			t.Fatalf("windows Exec does not propagate %q; body:\n%s", required, body)
		}
	}
}

func mustFunctionBody(t *testing.T, filename, signature string) string {
	t.Helper()
	src := mustReadSandboxSource(t, filename)
	start := strings.Index(src, signature)
	if start < 0 {
		t.Fatalf("signature %q not found in %s", signature, filename)
	}
	open := strings.Index(src[start:], "{")
	if open < 0 {
		t.Fatalf("function %q has no opening brace in %s", signature, filename)
	}
	open += start

	depth := 0
	for i := open; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[open+1 : i]
			}
		}
	}
	t.Fatalf("function %q has no closing brace in %s", signature, filename)
	return ""
}

func referencesOutsideFile(t *testing.T, tokens []string, excludedFile string) map[string][]string {
	t.Helper()
	refs := make(map[string][]string, len(tokens))
	dir := sandboxSourceDir(t)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read sandbox dir: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == excludedFile || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src := mustReadSandboxSource(t, name)
		for _, token := range tokens {
			if strings.Contains(src, token) {
				refs[token] = append(refs[token], name)
			}
		}
	}
	return refs
}

func mustReadSandboxSource(t *testing.T, filename string) string {
	t.Helper()
	path := filepath.Join(sandboxSourceDir(t), filename)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}
	return string(b)
}

func sandboxSourceDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}
