package sandbox

import (
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
