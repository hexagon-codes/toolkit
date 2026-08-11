//go:build !windows

package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildPOSIXSandboxEnvDropsHostSecretsAndIsolatesToolCaches(t *testing.T) {
	workspace := t.TempDir()
	env, err := buildPOSIXSandboxEnv(workspace, []string{
		"LANG=en_US.UTF-8",
		"TERM=xterm-256color",
		"OPENAI_API_KEY=api-sentinel",
		"SERVICE_TOKEN=token-sentinel",
		"TOP_SECRET=secret-sentinel",
		"HTTPS_PROXY=http://proxy.invalid",
		"LD_PRELOAD=/host/inject.so",
		"GOPATH=/host/go",
		"NPM_CONFIG_CACHE=/host/npm",
		"PIP_CACHE_DIR=/host/pip",
		"UNLISTED_VALUE=unlisted-sentinel",
	}, "/usr/bin:/bin")
	if err != nil {
		t.Fatal(err)
	}
	values := posixEnvironmentMap(env)
	for _, key := range []string{
		"OPENAI_API_KEY", "SERVICE_TOKEN", "TOP_SECRET", "HTTPS_PROXY", "LD_PRELOAD", "UNLISTED_VALUE",
	} {
		if _, exists := values[key]; exists {
			t.Errorf("environment %s must not cross the sandbox boundary", key)
		}
	}
	base := filepath.Join(workspace, posixRuntimeDirectory)
	for key, want := range map[string]string{
		"PATH":             "/usr/bin:/bin",
		"LANG":             "en_US.UTF-8",
		"TERM":             "xterm-256color",
		"HOME":             filepath.Join(base, "home"),
		"TMPDIR":           filepath.Join(base, "tmp"),
		"GOPATH":           filepath.Join(base, "go", "path"),
		"GOCACHE":          filepath.Join(base, "go", "cache"),
		"GOMODCACHE":       filepath.Join(base, "go", "mod"),
		"GOTMPDIR":         filepath.Join(base, "go", "tmp"),
		"GOENV":            filepath.Join(base, "go", "env"),
		"GOWORK":           "off",
		"GOTOOLCHAIN":      "local",
		"NPM_CONFIG_CACHE": filepath.Join(base, "cache", "npm"),
		"PIP_CACHE_DIR":    filepath.Join(base, "cache", "pip"),
	} {
		if got := values[key]; got != want {
			t.Errorf("environment %s = %q, want %q", key, got, want)
		}
	}
	for _, relative := range posixRuntimeRelativeDirectories() {
		info, statErr := os.Stat(filepath.Join(workspace, relative))
		if statErr != nil {
			t.Errorf("inspect runtime directory %q: %v", relative, statErr)
			continue
		}
		if !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Errorf("runtime directory %q mode = %v, want private 0700 directory", relative, info.Mode())
		}
	}
}

func TestCleanBasicEnvUsesTheCommonMinimumPolicy(t *testing.T) {
	workspace := t.TempDir()
	env, err := cleanBasicEnv(workspace, []string{
		"PATH=/host/bin",
		"API_KEY=api-sentinel",
		"HTTP_PROXY=http://proxy.invalid",
		"LANG=C",
	})
	if err != nil {
		t.Fatal(err)
	}
	values := posixEnvironmentMap(env)
	if _, exists := values["API_KEY"]; exists {
		t.Fatal("basic environment exposed API_KEY")
	}
	if _, exists := values["HTTP_PROXY"]; exists {
		t.Fatal("basic environment exposed HTTP_PROXY")
	}
	if values["LANG"] != "C" {
		t.Fatalf("basic LANG = %q, want C", values["LANG"])
	}
	if !strings.HasPrefix(values["HOME"], filepath.Join(workspace, posixRuntimeDirectory)) {
		t.Fatalf("basic HOME is outside workspace runtime: %q", values["HOME"])
	}
}

func posixEnvironmentMap(env []string) map[string]string {
	result := make(map[string]string, len(env))
	for _, item := range env {
		name, value, ok := strings.Cut(item, "=")
		if ok {
			result[name] = value
		}
	}
	return result
}
