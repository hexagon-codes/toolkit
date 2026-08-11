//go:build darwin

package sandbox

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestDarwinProfileDoesNotGrantBroadTemporaryOrVariableDirectories(t *testing.T) {
	workspace := darwinCanonicalPath(t.TempDir())
	profile := newDarwinSandbox(Config{Workspace: workspace}).generateSBPL()

	for _, path := range []string{"/tmp", "/private/tmp", "/var", "/private/var"} {
		rule := fmt.Sprintf("(subpath %q)", path)
		if strings.Contains(profile, rule) {
			t.Errorf("default profile must not grant broad access to %s:\n%s", path, profile)
		}
		for _, operation := range []string{"file-read*", "file-write*", "file-read-data", "file-write-data"} {
			literalRule := fmt.Sprintf("(allow %s (literal %q))", operation, path)
			if strings.Contains(profile, literalRule) {
				t.Errorf("default profile must not grant broad %s access to %s", operation, path)
			}
		}
	}

	wantWrite := fmt.Sprintf("(allow file-write* (subpath %q))", workspace)
	var writeRules []string
	for _, line := range strings.Split(profile, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "(allow file-write*") {
			writeRules = append(writeRules, line)
		}
	}
	if len(writeRules) != 1 || writeRules[0] != wantWrite {
		t.Fatalf("default profile write grants = %q, want only %q", writeRules, wantWrite)
	}
	if !strings.Contains(profile, "(allow file-write-data (literal \"/dev/null\"))") {
		t.Fatal("default profile must grant only data writes to /dev/null for runtime standard streams")
	}
	if !strings.Contains(profile, "(subpath \"/private/var/db/timezone\")") {
		t.Fatal("default profile must grant the precise system timezone directory")
	}
	if strings.Contains(profile, "(subpath \"/System\")") {
		t.Fatal("default profile must not grant the macOS Data volume through /System")
	}
}

func TestDarwinExecUsesWorkspacePrivateRuntimeDirectories(t *testing.T) {
	requireSandboxExec(t)

	sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: t.TempDir(), Timeout: 20})
	if err != nil {
		t.Fatal(err)
	}
	darwin := unwrapDarwinSandbox(t, sandboxInstance)

	result, err := sandboxInstance.Exec(context.Background(), Command{
		Path: requireDarwinRuntime(t, "python3"),
		Args: []string{"-c", `
import os
from pathlib import Path

Path(os.environ["TMPDIR"], "probe").write_text("private")
for key in ("HOME", "TMPDIR", "TMP", "TEMP", "XDG_CACHE_HOME"):
    print(os.environ[key])
`},
	})

	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("Exec() exit = %d, stderr = %q", result.ExitCode, result.Stderr)
	}

	runtimeRoot := filepath.Join(darwin.cfg.Workspace, ".sandbox-runtime")
	want := []string{
		filepath.Join(runtimeRoot, "home"),
		filepath.Join(runtimeRoot, "tmp"),
		filepath.Join(runtimeRoot, "tmp"),
		filepath.Join(runtimeRoot, "tmp"),
		filepath.Join(runtimeRoot, "cache"),
	}
	got := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	if len(got) != len(want) {
		t.Fatalf("runtime environment lines = %q, want %q", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("runtime environment[%d] = %q, want %q", index, got[index], want[index])
		}
	}
	for _, dir := range []string{runtimeRoot, want[0], want[1], want[4]} {
		info, statErr := os.Stat(dir)
		if statErr != nil {
			t.Errorf("inspect private runtime directory %q: %v", dir, statErr)
			continue
		}
		if info.Mode().Perm() != 0o700 {
			t.Errorf("private runtime directory %q mode = %o, want 700", dir, info.Mode().Perm())
		}
	}
}

func TestDarwinExecRejectsBroadRuntimeDirectoryPermissions(t *testing.T) {
	workspace := t.TempDir()
	runtimeRoot := filepath.Join(workspace, darwinRuntimeDirectory)
	if err := os.Mkdir(runtimeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runtimeRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := darwinExecEnv(workspace, os.Environ()); err == nil {
		t.Fatal("darwinExecEnv() must reject a runtime directory with broad permissions")
	}
}

func TestDarwinExecEnvUsesMinimalWorkspaceEnvironment(t *testing.T) {
	workspace := t.TempDir()
	env, err := darwinExecEnv(workspace, []string{
		"PATH=/Users/Shared/attacker/bin:/usr/bin",
		"LANG=en_US.UTF-8",
		"LC_CTYPE=UTF-8",
		"TERM=xterm-256color",
		"OPENAI_API_KEY=api-sentinel",
		"SERVICE_TOKEN=token-sentinel",
		"TOP_SECRET=secret-sentinel",
		"HTTPS_PROXY=http://proxy.invalid",
		"https_proxy=http://proxy.invalid",
		"GOPATH=/Users/Shared/attacker/go",
		"GOCACHE=/Users/Shared/attacker/cache",
		"GOMODCACHE=/Users/Shared/attacker/mod",
		"GOTMPDIR=/Users/Shared/attacker/tmp",
		"GOENV=/Users/Shared/attacker/goenv",
		"GOWORK=/Users/Shared/attacker/go.work",
		"NPM_CONFIG_CACHE=/Users/Shared/attacker/npm",
		"PIP_CACHE_DIR=/Users/Shared/attacker/pip",
		"UNLISTED_VALUE=unlisted-sentinel",
	})
	if err != nil {
		t.Fatal(err)
	}
	values := darwinEnvironmentMap(env)
	for _, key := range []string{
		"OPENAI_API_KEY", "SERVICE_TOKEN", "TOP_SECRET", "HTTPS_PROXY", "https_proxy", "UNLISTED_VALUE",
	} {
		if _, exists := values[key]; exists {
			t.Errorf("environment %s must not cross the sandbox boundary", key)
		}
	}
	for key, want := range map[string]string{
		"LANG":                  "en_US.UTF-8",
		"LC_CTYPE":              "UTF-8",
		"TERM":                  "xterm-256color",
		"HOME":                  filepath.Join(workspace, darwinRuntimeDirectory, "home"),
		"TMPDIR":                filepath.Join(workspace, darwinRuntimeDirectory, "tmp"),
		"GOPATH":                filepath.Join(workspace, darwinRuntimeDirectory, "go", "path"),
		"GOCACHE":               filepath.Join(workspace, darwinRuntimeDirectory, "go", "cache"),
		"GOMODCACHE":            filepath.Join(workspace, darwinRuntimeDirectory, "go", "mod"),
		"GOTMPDIR":              filepath.Join(workspace, darwinRuntimeDirectory, "go", "tmp"),
		"GOENV":                 filepath.Join(workspace, darwinRuntimeDirectory, "go", "env"),
		"GOWORK":                "off",
		"GOTOOLCHAIN":           "local",
		"NPM_CONFIG_CACHE":      filepath.Join(workspace, darwinRuntimeDirectory, "cache", "npm"),
		"NPM_CONFIG_USERCONFIG": filepath.Join(workspace, darwinRuntimeDirectory, "config", "npmrc"),
		"PIP_CACHE_DIR":         filepath.Join(workspace, darwinRuntimeDirectory, "cache", "pip"),
		"PIP_CONFIG_FILE":       "/dev/null",
	} {
		if got := values[key]; got != want {
			t.Errorf("environment %s = %q, want %q", key, got, want)
		}
	}
	if path := values["PATH"]; path == "" || strings.Contains(path, "/Users/Shared/attacker") {
		t.Errorf("sandbox PATH is not minimal: %q", path)
	}
}

func TestDarwinSandboxDoesNotExposeHostSecretsOrCaches(t *testing.T) {
	python := requireSandboxTools(t)

	t.Setenv("TOOLKIT_API_KEY_SENTINEL", "api-sentinel")
	t.Setenv("TOOLKIT_TOKEN_SENTINEL", "token-sentinel")
	t.Setenv("TOOLKIT_SECRET_SENTINEL", "secret-sentinel")
	t.Setenv("HTTPS_PROXY", "http://proxy.invalid")
	t.Setenv("GOPATH", "/Users/Shared/host-go-sentinel")
	t.Setenv("NPM_CONFIG_CACHE", "/Users/Shared/host-npm-sentinel")
	t.Setenv("PIP_CACHE_DIR", "/Users/Shared/host-pip-sentinel")

	workspace := t.TempDir()
	sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: workspace, Timeout: 20})
	if err != nil {
		t.Fatal(err)
	}
	code := `
import os
for key in (
    "TOOLKIT_API_KEY_SENTINEL",
    "TOOLKIT_TOKEN_SENTINEL",
    "TOOLKIT_SECRET_SENTINEL",
    "HTTPS_PROXY",
):
    if key in os.environ:
        raise RuntimeError("host environment escaped: " + key)
runtime = os.path.join(os.getcwd(), ".sandbox-runtime")
expected = {
    "GOPATH": os.path.join(runtime, "go", "path"),
    "GOCACHE": os.path.join(runtime, "go", "cache"),
    "GOMODCACHE": os.path.join(runtime, "go", "mod"),
    "GOTMPDIR": os.path.join(runtime, "go", "tmp"),
    "GOENV": os.path.join(runtime, "go", "env"),
    "GOWORK": "off",
    "NPM_CONFIG_CACHE": os.path.join(runtime, "cache", "npm"),
    "PIP_CACHE_DIR": os.path.join(runtime, "cache", "pip"),
}
for key, value in expected.items():
    if os.environ.get(key) != value:
        raise RuntimeError("unexpected isolated environment: " + key)
print("environment-private")
`
	result, err := sandboxInstance.Exec(context.Background(), Command{Path: python, Args: []string{"-c", code}})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "environment-private" {
		t.Fatalf("environment boundary failed: exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
}

func TestDarwinMissingCommandDoesNotReceiveRuntimeReadGrant(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(home, ".toolkit-missing-seatbelt-command")
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("missing command fixture is not absent: %v", err)
	}
	workspace := t.TempDir()
	plan := darwinCommandExecutionPlan(darwinPreparedTestCommand(t, missing, workspace), workspace)
	if plan.err == nil || plan.err.Error() != fmt.Sprintf("resolve sandbox command %q", missing) {
		t.Fatalf("missing command plan error = %v", plan.err)
	}
	if paths := plan.readPaths; len(paths) != 0 {
		t.Fatalf("missing command runtime grants = %q, want none", paths)
	}
}

func TestDarwinCommandPlanFreezesCanonicalTargetAcrossPathReplacement(t *testing.T) {
	workspace := t.TempDir()
	first := filepath.Join(workspace, "first.sh")
	second := filepath.Join(workspace, "second.sh")
	alias := filepath.Join(workspace, "command")
	for path, body := range map[string]string{
		first:  "#!/bin/sh\nprintf first\n",
		second: "#!/bin/sh\nprintf second\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(first, alias); err != nil {
		t.Fatal(err)
	}
	plan := darwinCommandExecutionPlan(darwinPreparedTestCommand(t, alias, workspace), workspace)
	if plan.err != nil {
		t.Fatal(plan.err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, alias); err != nil {
		t.Fatal(err)
	}
	canonicalFirst := darwinCanonicalPath(first)
	if plan.command.Path != canonicalFirst {
		t.Fatalf("frozen command = %q, want %q", plan.command.Path, canonicalFirst)
	}
}

func TestDarwinSandboxExecIgnoresHostPATHPoisoning(t *testing.T) {
	requireSandboxExec(t)
	poisonDirectory := t.TempDir()
	marker := filepath.Join(poisonDirectory, "executed")
	poisonedLauncher := filepath.Join(poisonDirectory, "sandbox-exec")
	poisonedBody := fmt.Sprintf("#!/bin/sh\nprintf poisoned > %q\nexit 99\n", marker)
	if err := os.WriteFile(poisonedLauncher, []byte(poisonedBody), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", poisonDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))

	launcher, err := darwinSandboxExecPath()
	if err != nil {
		t.Fatal(err)
	}
	if launcher == poisonedLauncher {
		t.Fatal("sandbox-exec was resolved from the caller PATH")
	}
	sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: t.TempDir(), Timeout: 20})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sandboxInstance.Exec(context.Background(), Command{Path: "/usr/bin/true"})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("Exec() exit=%d stderr=%q", result.ExitCode, result.Stderr)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("poisoned sandbox-exec was invoked: %v", err)
	}
}

func TestDarwinStructuredCommandRunsDirectGoWithDirectoryAndEnvironment(t *testing.T) {
	requireSandboxExec(t)
	workspace := t.TempDir()
	project := filepath.Join(workspace, "project")
	home := filepath.Join(workspace, "home")
	temporary := filepath.Join(workspace, "tmp")
	cache := filepath.Join(workspace, "go-build")
	for _, directory := range []string{project, home, temporary, cache} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	goMod := filepath.Join(project, "go.mod")
	if err := os.WriteFile(goMod, []byte("module example.com/direct\n\ngo 1.25.12\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	goBinary, goRoot := darwinTestGoToolchain(t)
	sandboxInstance, err := newDarwinTestSandbox(Config{
		Workspace: workspace,
		Timeout:   30,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sandboxInstance.Exec(context.Background(), Command{
		Path: goBinary,
		Args: []string{"env", "GOMOD", "GOROOT", "GOENV", "GOWORK"},
		Dir:  project,
		Env: []string{
			"PATH=" + filepath.Dir(goBinary) + ":/usr/bin:/bin",
			"HOME=" + home,
			"TMPDIR=" + temporary,
			"GOCACHE=" + cache,
			"GOENV=off",
			"GOTOOLCHAIN=local",
			"GOWORK=off",
			"GOROOT=" + goRoot,
			"LANG=C",
		},
	})
	if err != nil {
		t.Fatalf("Exec() error = %v; stderr=%q", err, resultStderr(result))
	}
	if result.ExitCode != 0 {
		t.Fatalf("direct Go env exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	canonicalGoMod := darwinCanonicalPath(goMod)
	if len(lines) < 4 || lines[0] != canonicalGoMod || lines[1] != goRoot || lines[2] != "" || lines[3] != "off" {
		t.Fatalf("direct Go environment = %q", result.Stdout)
	}
}

func TestDarwinNoChildEnvironmentLauncherStartsInsideSeatbelt(t *testing.T) {
	requireSandboxExec(t)

	workspace := darwinCanonicalPath(t.TempDir())
	sandboxInstance := newDarwinSandbox(Config{Workspace: workspace})
	env, err := darwinExecEnv(workspace, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	env = append(env, "TOOLKIT_SANDBOX_TEST_UNTRUSTED=1")
	payload := Command{Path: "/usr/bin/true", Dir: workspace, Env: env}
	runner, err := sandboxInstance.darwinSeatbeltRunner(payload)
	if err != nil {
		t.Fatal(err)
	}
	sandboxExec, err := darwinSandboxExecPath()
	if err != nil {
		t.Fatal(err)
	}
	environmentLauncher, err := resolveTrustedPOSIXLauncher("macOS sandbox environment launcher", []string{"/usr/bin/env"})
	if err != nil {
		t.Fatal(err)
	}
	if runner.Path != sandboxExec || len(runner.Args) < 7 || runner.Args[0] != "-p" || runner.Args[2] != environmentLauncher {
		t.Fatalf("Seatbelt runner = path %q args %q, want trusted environment launcher %q as first sandbox payload", runner.Path, runner.Args, environmentLauncher)
	}
	profile := runner.Args[1]
	if !strings.Contains(profile, "(deny process-fork)") || strings.Contains(profile, "(allow process-fork)") {
		t.Fatalf("Seatbelt profile does not enforce the no-child boundary:\n%s", profile)
	}
	if runner.Args[3] != "-i" || runner.Args[4] != "--" || !containsString(runner.Args, payload.Path) ||
		!containsString(runner.Args, "TOOLKIT_SANDBOX_TEST_UNTRUSTED=1") {
		t.Fatalf("environment launcher argv = %q, want isolated payload environment followed by payload", runner.Args[3:])
	}
	if containsString(runner.Env, "TOOLKIT_SANDBOX_TEST_UNTRUSTED=1") {
		t.Fatalf("environment launcher inherited payload environment before Seatbelt: %q", runner.Env)
	}
	if got := posixEnvironmentMap(runner.Env); got["PATH"] != "/usr/bin:/bin" || got["LC_ALL"] != "C" {
		t.Fatalf("environment launcher environment = %q, want fixed safe environment", runner.Env)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestDarwinStructuredCommandRejectsUnsafeEnvironmentAndOutsideDirectory(t *testing.T) {
	sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: t.TempDir(), Timeout: 20})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sandboxInstance.Exec(context.Background(), Command{
		Path: "/usr/bin/true",
		Env:  []string{"DYLD_INSERT_LIBRARIES=/tmp/inject.dylib"},
	}); err == nil || !strings.Contains(err.Error(), "is not allowed") {
		t.Fatalf("dangerous environment error = %v", err)
	}
	if _, err := sandboxInstance.Exec(context.Background(), Command{
		Path: "/usr/bin/true",
		Dir:  t.TempDir(),
	}); err == nil || !strings.Contains(err.Error(), "must be within the workspace") {
		t.Fatalf("outside directory error = %v", err)
	}
}

func resultStderr(result *ExecResult) string {
	if result == nil {
		return ""
	}
	return result.Stderr
}

// darwinTestGoToolchain 冻结当前测试使用的 Go 二进制，并从该二进制查询配套 GOROOT。
func darwinTestGoToolchain(t *testing.T) (string, string) {
	t.Helper()
	var candidates []string
	if goRoot := os.Getenv("GOROOT"); goRoot != "" {
		candidates = append(candidates, filepath.Join(goRoot, "bin", "go"))
	}
	if goBinary, err := exec.LookPath("go"); err == nil {
		candidates = append(candidates, goBinary)
	}
	var diagnostic string
	for _, candidate := range candidates {
		goBinary, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			diagnostic = err.Error()
			continue
		}
		command := exec.CommandContext(t.Context(), goBinary, "env", "GOROOT")
		output, err := command.CombinedOutput()
		if err != nil {
			diagnostic = fmt.Sprintf("%v: %s", err, output)
			continue
		}
		goRoot := strings.TrimSpace(string(output))
		if !filepath.IsAbs(goRoot) {
			diagnostic = fmt.Sprintf("Go reported a non-absolute GOROOT %q", goRoot)
			continue
		}
		return darwinCanonicalPath(goBinary), darwinCanonicalPath(goRoot)
	}
	t.Skipf("Go toolchain is unavailable: %s", diagnostic)
	return "", ""
}

func TestDarwinResolverUsesExactSystemFilesWithoutBroadVarGrant(t *testing.T) {
	python := requireSandboxTools(t)

	resolverTarget, err := filepath.EvalSymlinks("/private/etc/resolv.conf")
	if err != nil {
		t.Fatalf("resolve system resolver configuration: %v", err)
	}
	sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: t.TempDir(), Timeout: 20})
	if err != nil {
		t.Fatal(err)
	}
	profile := unwrapDarwinSandbox(t, sandboxInstance).generateSBPL()
	wantTargetRule := fmt.Sprintf("(allow file-read* (literal %q))", resolverTarget)
	if !strings.Contains(profile, wantTargetRule) {
		t.Fatalf("resolver target rule is missing: %s", wantTargetRule)
	}

	code := `
import socket
with open("/private/etc/resolv.conf", "rb") as resolver:
    if not resolver.read(1):
        raise RuntimeError("empty resolver configuration")
addresses = socket.getaddrinfo("localhost", 0)
if not addresses:
    raise RuntimeError("localhost did not resolve")
print("resolver-private")
`
	result, err := sandboxInstance.Exec(context.Background(), Command{Path: python, Args: []string{"-c", code}})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "resolver-private" {
		t.Fatalf("resolver boundary failed: exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
}

func TestDarwinControlledDNSResolutionWithoutBlanketServiceGrants(t *testing.T) {
	requireSandboxExec(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}

	var listenConfig net.ListenConfig
	listener, err := listenConfig.ListenPacket(t.Context(), "udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	serverErr := make(chan error, 1)
	go func() {
		buffer := make([]byte, 2048)
		_ = listener.SetDeadline(time.Now().Add(20 * time.Second))
		n, client, readErr := listener.ReadFrom(buffer)
		if readErr != nil {
			serverErr <- readErr
			return
		}
		response, responseErr := darwinControlledDNSResponse(buffer[:n])
		if responseErr == nil {
			_, responseErr = listener.WriteTo(response, client)
		}
		serverErr <- responseErr
	}()

	port := listener.LocalAddr().(*net.UDPAddr).Port
	script := fmt.Sprintf(`
const dns = require("dns");
const resolver = new dns.Resolver();
resolver.setServers(["127.0.0.1:%d"]);
resolver.resolve4("sandbox.test", (error, addresses) => {
  if (error) throw error;
  if (addresses.length !== 1 || addresses[0] !== "127.0.0.42") {
    throw new Error("unexpected controlled DNS response");
  }
  console.log("controlled-dns");
});
`, port)
	sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: t.TempDir(), Timeout: 20, Network: NetworkHost})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sandboxInstance.Exec(context.Background(), Command{Path: node, Args: []string{"-e", script}})
	if err != nil {
		t.Fatalf("Exec() error = %v; stderr=%q", err, resultStderr(result))
	}
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "controlled-dns" {
		t.Fatalf("controlled DNS failed: exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("controlled DNS server failed: %v", err)
	}
}

// darwinControlledDNSResponse 为单个 A 查询构造固定本机响应，测试不依赖公网 DNS。
func darwinControlledDNSResponse(query []byte) ([]byte, error) {
	if len(query) < 17 || query[4] != 0 || query[5] != 1 {
		return nil, fmt.Errorf("controlled DNS query is invalid")
	}
	end := 12
	for {
		if end >= len(query) {
			return nil, fmt.Errorf("controlled DNS question is truncated")
		}
		length := int(query[end])
		end++
		if length == 0 {
			break
		}
		end += length
	}
	end += 4
	if end > len(query) {
		return nil, fmt.Errorf("controlled DNS question is truncated")
	}
	response := append([]byte(nil), query[:end]...)
	response[2], response[3] = 0x81, 0x80
	response[6], response[7] = 0, 1
	response[8], response[9] = 0, 0
	response[10], response[11] = 0, 0
	response = append(response,
		0xc0, 0x0c,
		0x00, 0x01,
		0x00, 0x01,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x04,
		127, 0, 0, 42,
	)
	return response, nil
}

func TestDarwinCommandPathDoesNotResolveThroughHostPATH(t *testing.T) {
	workspace := t.TempDir()
	for _, command := range []string{"python3", "node", "go", "gofmt", "npm", "npx", "pip", "pip3", "tree"} {
		absolute, err := exec.LookPath(command)
		if err != nil {
			continue
		}
		barePlan := darwinCommandExecutionPlan(darwinPreparedTestCommand(t, command, workspace), workspace)
		if barePlan.err == nil {
			t.Errorf("bare command %q unexpectedly resolved through host PATH", command)
		}
		absolutePlan := darwinCommandExecutionPlan(darwinPreparedTestCommand(t, absolute, workspace), workspace)
		if absolutePlan.err != nil {
			t.Errorf("absolute command %q plan error = %v", absolute, absolutePlan.err)
		}
	}
}

func TestDarwinHomebrewRuntimeRootsAreVersionScoped(t *testing.T) {
	tests := map[string]string{
		"Intel":        "/usr/local/Cellar/node/24.1.0",
		"AppleSilicon": "/opt/homebrew/Cellar/python@3.13/3.13.5",
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			executable := filepath.Join(want, "bin", "runtime")
			got, _ := darwinInstalledRuntimeRoot(executable)
			if got != want {
				t.Fatalf("darwinRuntimeRoot(%q) = %q, want %q", executable, got, want)
			}
		})
	}
}

func TestDarwinForgedExternalRuntimePathsFailClosed(t *testing.T) {
	_, workspace, otherWorkspace := newDarwinSharedWorkspacePair(t)
	paths := []string{
		filepath.Join(otherWorkspace, "Cellar", "node", "99.0.0", "bin", "node"),
		filepath.Join(otherWorkspace, ".pyenv", "versions", "3.99.0", "bin", "python3"),
		filepath.Join(otherWorkspace, ".nvm", "versions", "node", "v99.0.0", "bin", "node"),
		filepath.Join(otherWorkspace, "go", "pkg", "mod", "golang.org", "toolchain@v0.0.1-go99.0.0.darwin-amd64", "bin", "go"),
		filepath.Join(otherWorkspace, "node_modules", "npm", "bin", "npm"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf 'FORGED-RUNTIME'\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		if grants := darwinCommandReadPaths(darwinPreparedTestCommand(t, path, workspace), workspace); len(grants) != 0 {
			t.Errorf("forged runtime %q received grants %q", path, grants)
		}
	}
	dataAlias := "/System/Volumes/Data" + paths[0]
	if darwinSystemExecutable(dataAlias) {
		t.Fatalf("Data volume alias was classified as a system executable: %q", dataAlias)
	}
}

func TestDarwinExecSupportsConfiguredRuntimesWithPrivateWorkspace(t *testing.T) {
	requireSandboxExec(t)

	tests := []struct {
		name    string
		command string
		args    []string
		want    string
	}{
		{name: "python", command: "python3", args: []string{"-c", "print('python-private')"}, want: "python-private"},
		{name: "node", command: "node", args: []string{"-e", "console.log('node-private')"}, want: "node-private"},
		{name: "go", command: "go", args: []string{"version"}, want: "go version"},
		{name: "homebrew", command: "tree", args: []string{"--version"}, want: "tree"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			absolute, err := exec.LookPath(test.command)
			if err != nil {
				t.Skipf("%s is unavailable", test.command)
			}
			sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: t.TempDir(), Timeout: 60})
			if err != nil {
				t.Fatal(err)
			}
			result, err := sandboxInstance.Exec(context.Background(), Command{Path: absolute, Args: test.args})
			if err != nil {
				t.Fatalf("Exec() error = %v", err)
			}
			if result.ExitCode != 0 || !strings.Contains(result.Stdout, test.want) {
				t.Fatalf("Exec() exit=%d stdout=%q stderr=%q, want stdout containing %q", result.ExitCode, result.Stdout, result.Stderr, test.want)
			}
		})
	}
}

func TestDarwinDirectInterpreterWrapperRunsWithMinimalRuntimeGrants(t *testing.T) {
	requireSandboxExec(t)

	workspace := t.TempDir()
	wrapper := filepath.Join(workspace, "direct-interpreter-wrapper")
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\nprintf 'direct-wrapper'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: workspace, Timeout: 20})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sandboxInstance.Exec(context.Background(), Command{Path: wrapper})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 || result.Stdout != "direct-wrapper" {
		t.Fatalf("Exec() exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
}

func TestDarwinSupportedToolWrappersRunWithMinimalRuntimeGrants(t *testing.T) {
	requireSandboxExec(t)

	tests := []struct {
		command string
		args    []string
	}{
		{command: "gofmt", args: []string{"-h"}},
		{command: "npm", args: []string{"--version"}},
		{command: "npx", args: []string{"--version"}},
		{command: "pip", args: []string{"--version"}},
		{command: "pip3", args: []string{"--version"}},
	}
	for _, test := range tests {
		t.Run(test.command, func(t *testing.T) {
			absolute, err := exec.LookPath(test.command)
			if err != nil {
				t.Skipf("%s is unavailable", test.command)
			}
			// Apple 的 pip shim 依赖未显式授权的 Developer Toolchain；该表只验证可直接执行的安装包装器。
			if (test.command == "pip" || test.command == "pip3") && filepath.Dir(absolute) == "/usr/bin" {
				t.Skipf("%s is an Apple developer tool shim, not a direct runtime wrapper", absolute)
			}
			sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: t.TempDir(), Timeout: 60})
			if err != nil {
				t.Fatal(err)
			}
			result, err := sandboxInstance.Exec(context.Background(), Command{Path: absolute, Args: test.args})
			if err != nil {
				t.Fatalf("Exec() error = %v", err)
			}
			if result.ExitCode != 0 || strings.TrimSpace(result.Stdout+result.Stderr) == "" {
				t.Fatalf("Exec() exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
			}
		})
	}
}

func TestDarwinConcurrentWorkspacesCannotReadOrWriteEachOther(t *testing.T) {
	requireSandboxExec(t)

	root, rootErr := os.MkdirTemp("/private/tmp", "toolkit-seatbelt-isolation-")
	if rootErr != nil {
		t.Fatalf("create private tmp fixture: %v", rootErr)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })

	workspaces := []string{filepath.Join(root, "first"), filepath.Join(root, "second")}
	for _, workspace := range workspaces {
		if err := os.Mkdir(workspace, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(workspace, "secret.txt"), []byte(filepath.Base(workspace)), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	sandboxes := make([]Sandbox, len(workspaces))
	for index, workspace := range workspaces {
		sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: workspace, Timeout: 20})
		if err != nil {
			t.Fatal(err)
		}
		sandboxes[index] = sandboxInstance
	}
	python := requireDarwinRuntime(t, "python3")

	type outcome struct {
		index  int
		result *ExecResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, len(sandboxes))
	var ready sync.WaitGroup
	ready.Add(len(sandboxes))
	for index := range sandboxes {
		go func(index int) {
			ready.Done()
			<-start
			own := workspaces[index]
			other := workspaces[1-index]
			result, execErr := sandboxes[index].Exec(context.Background(), Command{Path: python, Args: []string{"-c", `
from pathlib import Path
import sys

own = Path(sys.argv[1])
other = Path(sys.argv[2])
if (own / "secret.txt").read_text() != own.name:
    raise SystemExit(68)
(own / "owned.txt").write_text("owned")
try:
    (other / "secret.txt").read_text()
except OSError:
    pass
else:
    raise SystemExit(70)
try:
    (other / "intrusion.txt").write_text("escaped")
except OSError:
    pass
else:
    raise SystemExit(71)
print("isolated")
`, own, other}})

			outcomes <- outcome{index: index, result: result, err: execErr}
		}(index)
	}
	ready.Wait()
	close(start)

	for range sandboxes {
		outcome := <-outcomes
		if outcome.err != nil {
			t.Errorf("workspace %d Exec() error = %v", outcome.index, outcome.err)
			continue
		}
		if outcome.result.ExitCode != 0 || strings.TrimSpace(outcome.result.Stdout) != "isolated" {
			t.Errorf(
				"workspace %d escaped isolation: exit=%d stdout=%q stderr=%q",
				outcome.index,
				outcome.result.ExitCode,
				outcome.result.Stdout,
				outcome.result.Stderr,
			)
		}
	}

	for _, workspace := range workspaces {
		if content, err := os.ReadFile(filepath.Join(workspace, "owned.txt")); err != nil || string(content) != "owned" {
			t.Errorf("workspace-local write in %q = %q, %v", workspace, content, err)
		}
		if _, err := os.Stat(filepath.Join(workspace, "intrusion.txt")); !os.IsNotExist(err) {
			t.Errorf("cross-workspace write created intrusion file in %q: %v", workspace, err)
		}
	}
}

func TestDarwinOtherWorkspaceExecutablesAreDenied(t *testing.T) {
	requireSandboxExec(t)

	root, rootErr := os.MkdirTemp("/private/tmp", "toolkit-seatbelt-exec-isolation-")
	if rootErr != nil {
		t.Fatalf("create private tmp fixture: %v", rootErr)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	workspace := filepath.Join(root, "current")
	otherWorkspace := filepath.Join(root, "other")
	for _, dir := range []string{workspace, otherWorkspace} {
		if mkdirErr := os.Mkdir(dir, 0o700); mkdirErr != nil {
			t.Fatal(mkdirErr)
		}
	}

	script := filepath.Join(otherWorkspace, "payload.sh")
	if writeErr := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'SCRIPT-ESCAPED'\n"), 0o700); writeErr != nil {
		t.Fatal(writeErr)
	}
	executable := filepath.Join(otherWorkspace, "payload-bin")
	testExecutable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(testExecutable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, binary, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name    string
		command string
		args    []string
		marker  string
	}{
		{name: "script", command: script, marker: "SCRIPT-ESCAPED"},
		{
			name:    "mach-o",
			command: executable,
			args:    []string{"-test.list", "^TestDarwinProfileDoesNotGrantBroadTemporaryOrVariableDirectories$"},
			marker:  "TestDarwinProfileDoesNotGrantBroadTemporaryOrVariableDirectories",
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			hostOutput, err := exec.CommandContext(t.Context(), fixture.command, fixture.args...).CombinedOutput()
			if err != nil || !strings.Contains(string(hostOutput), fixture.marker) {
				t.Fatalf("host fixture failed: output=%q error=%v", hostOutput, err)
			}
			sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: workspace, Timeout: 20})
			if err != nil {
				t.Fatal(err)
			}
			profile := unwrapDarwinSandbox(t, sandboxInstance).generateSBPLWithRuntimePaths(nil)
			if !strings.Contains(profile, "(deny process-exec") {
				t.Fatal("profile must apply a process execution allowlist deny")
			}
			result, err := sandboxInstance.Exec(context.Background(), Command{Path: fixture.command, Args: fixture.args})
			if err == nil || !strings.Contains(err.Error(), "outside the workspace and trusted runtime roots") {
				t.Fatalf("other workspace executable error = %v, result = %+v", err, result)
			}
			if result != nil && strings.Contains(result.Stdout+result.Stderr, fixture.marker) {
				t.Fatalf("other workspace executable escaped: stdout=%q stderr=%q", result.Stdout, result.Stderr)
			}
		})
	}
}

func TestDarwinNonTemporaryOtherWorkspaceExecutablesAreDenied(t *testing.T) {
	_, workspace, otherWorkspace := newDarwinSharedWorkspacePair(t)

	script := filepath.Join(otherWorkspace, "Cellar", "fake", "1.0.0", "bin", "payload.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'SCRIPT-ESCAPED'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	machO := filepath.Join(otherWorkspace, "node_modules", "fake", "payload-bin")
	if err := os.MkdirAll(filepath.Dir(machO), 0o700); err != nil {
		t.Fatal(err)
	}
	testExecutable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(testExecutable)
	if err != nil {
		t.Fatal(err)
	}
	if writeErr := os.WriteFile(machO, binary, 0o700); writeErr != nil {
		t.Fatal(writeErr)
	}

	fixtures := []struct {
		name    string
		command string
		args    []string
		marker  string
	}{
		{name: "script-logical", command: script, marker: "SCRIPT-ESCAPED"},
		{name: "script-data-alias", command: "/System/Volumes/Data" + script, marker: "SCRIPT-ESCAPED"},
		{
			name:    "mach-o-logical",
			command: machO,
			args:    []string{"-test.list", "^TestDarwinProfileDoesNotGrantBroadTemporaryOrVariableDirectories$"},
			marker:  "TestDarwinProfileDoesNotGrantBroadTemporaryOrVariableDirectories",
		},
		{
			name:    "mach-o-data-alias",
			command: "/System/Volumes/Data" + machO,
			args:    []string{"-test.list", "^TestDarwinProfileDoesNotGrantBroadTemporaryOrVariableDirectories$"},
			marker:  "TestDarwinProfileDoesNotGrantBroadTemporaryOrVariableDirectories",
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			hostOutput, hostErr := exec.CommandContext(t.Context(), fixture.command, fixture.args...).CombinedOutput()
			if hostErr != nil || !strings.Contains(string(hostOutput), fixture.marker) {
				t.Fatalf("host fixture failed: output=%q error=%v", hostOutput, hostErr)
			}
			sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: workspace, Timeout: 20})
			if err != nil {
				t.Fatal(err)
			}
			result, err := sandboxInstance.Exec(context.Background(), Command{Path: fixture.command, Args: fixture.args})
			if err == nil || !strings.Contains(err.Error(), "outside the workspace and trusted runtime roots") {
				t.Fatalf("non-temporary other workspace executable error = %v, result = %+v", err, result)
			}
			if result != nil && strings.Contains(result.Stdout+result.Stderr, fixture.marker) {
				t.Fatalf("non-temporary other workspace executable escaped: stdout=%q stderr=%q", result.Stdout, result.Stderr)
			}
		})
	}
}

func TestDarwinExecutableMappingIsBoundedInProfile(t *testing.T) {
	profile := newDarwinSandbox(Config{Workspace: t.TempDir()}).generateSBPL()
	if !strings.Contains(profile, "(deny file-map-executable") {
		t.Fatal("profile must apply an executable mapping boundary")
	}
}

func TestDarwinReadableOtherWorkspaceCannotMapExecutable(t *testing.T) {
	requireSandboxExec(t)

	_, workspace, otherWorkspace := newDarwinSharedWorkspacePair(t)
	probe := buildDarwinExecutableMapProbe(t, workspace)
	payload := filepath.Join(otherWorkspace, "readable-payload")
	// 一页普通文件足以触发 file-map-executable，避免把运行时和 Mach-O 签名检查抖动混入边界测试。
	if writeErr := os.WriteFile(payload, make([]byte, 4096), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	dataAlias := "/System/Volumes/Data" + payload
	sandboxInstance, err := newDarwinTestSandbox(Config{
		Workspace:     workspace,
		ReadablePaths: []string{otherWorkspace, "/System/Volumes/Data" + otherWorkspace},
		Timeout:       20,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sandboxInstance.Exec(context.Background(), Command{Path: probe, Args: []string{dataAlias}})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "MAP-DENIED" {
		t.Fatalf("executable mapping boundary failed: exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
}

const darwinExecutableMapProbeSource = `
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <sys/mman.h>
#include <unistd.h>

int main(int argc, char **argv) {
    if (argc != 2) return 64;
    int fd = open(argv[1], O_RDONLY);
    if (fd < 0) return 65;
    void *mapping = mmap(NULL, 4096, PROT_READ | PROT_EXEC, MAP_PRIVATE, fd, 0);
    int saved_errno = errno;
    (void)close(fd);
    if (mapping == MAP_FAILED) {
        if (saved_errno == EPERM || saved_errno == EACCES) {
            puts("MAP-DENIED");
            return 0;
        }
        errno = saved_errno;
        perror("mmap");
        return 66;
    }
    (void)munmap(mapping, 4096);
    puts("MAP-ESCAPED");
    return 0;
}
`

var (
	darwinExecutableMapProbeOnce   sync.Once
	darwinExecutableMapProbeBinary []byte
	darwinExecutableMapProbeErr    error
)

func buildDarwinExecutableMapProbe(t *testing.T, workspace string) string {
	t.Helper()
	darwinExecutableMapProbeOnce.Do(func() {
		buildDirectory, err := os.MkdirTemp("", "toolkit-executable-map-probe-")
		if err != nil {
			darwinExecutableMapProbeErr = err
			return
		}
		defer func() {
			if cleanupErr := os.RemoveAll(buildDirectory); cleanupErr != nil && darwinExecutableMapProbeErr == nil {
				darwinExecutableMapProbeErr = fmt.Errorf("clean executable mapping probe build directory: %w", cleanupErr)
			}
		}()
		source := filepath.Join(buildDirectory, "executable-map-probe.c")
		probe := filepath.Join(buildDirectory, "executable-map-probe")
		if writeErr := os.WriteFile(source, []byte(darwinExecutableMapProbeSource), 0o600); writeErr != nil {
			darwinExecutableMapProbeErr = writeErr
			return
		}
		command := exec.CommandContext(t.Context(), "/usr/bin/clang", "-O2", "-Wall", "-Wextra", "-o", probe, source)
		output, err := command.CombinedOutput()
		if err != nil {
			darwinExecutableMapProbeErr = fmt.Errorf("compile executable mapping probe: %w: %s", err, output)
			return
		}
		controlPayload := filepath.Join(buildDirectory, "host-control-payload")
		if writeErr := os.WriteFile(controlPayload, make([]byte, 4096), 0o600); writeErr != nil {
			darwinExecutableMapProbeErr = writeErr
			return
		}
		controlOutput, controlErr := exec.CommandContext(t.Context(), probe, controlPayload).CombinedOutput()
		if controlErr != nil {
			darwinExecutableMapProbeErr = fmt.Errorf("host executable mapping control failed: output=%q: %w", controlOutput, controlErr)
			return
		}
		if strings.TrimSpace(string(controlOutput)) != "MAP-ESCAPED" {
			darwinExecutableMapProbeErr = fmt.Errorf("host executable mapping control failed: output=%q", controlOutput)
			return
		}
		darwinExecutableMapProbeBinary, darwinExecutableMapProbeErr = os.ReadFile(probe)
	})
	if darwinExecutableMapProbeErr != nil {
		t.Fatal(darwinExecutableMapProbeErr)
	}
	probe := filepath.Join(workspace, "executable-map-probe")
	if err := os.WriteFile(probe, darwinExecutableMapProbeBinary, 0o700); err != nil {
		t.Fatal(err)
	}
	return probe
}

func TestDarwinReadablePathIsReadOnlyInSeatbelt(t *testing.T) {
	requireSandboxExec(t)

	workspace := t.TempDir()
	readable := externalAuthorizedDir(t)
	secret := filepath.Join(readable, "secret.txt")
	if err := os.WriteFile(secret, []byte("readable"), 0o600); err != nil {
		t.Fatal(err)
	}
	sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: workspace, ReadablePaths: []string{readable}, Timeout: 20})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sandboxInstance.Exec(context.Background(), Command{
		Path: requireDarwinRuntime(t, "python3"),
		Args: []string{"-c", `
from pathlib import Path
import sys

readable = Path(sys.argv[1])
workspace = Path(sys.argv[2])
if (readable / "secret.txt").read_text() != "readable":
    raise SystemExit(80)
try:
    (readable / "intrusion.txt").write_text("escaped")
except OSError:
    pass
else:
    raise SystemExit(81)
(workspace / "owned.txt").write_text("owned")
print("read-only")
`, readable, darwinCanonicalPath(workspace)},
	})

	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "read-only" {
		t.Fatalf("read-only boundary failed: exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
	if _, err := os.Stat(filepath.Join(readable, "intrusion.txt")); !os.IsNotExist(err) {
		t.Fatalf("readable path accepted a write: %v", err)
	}
}

func TestDarwinDeniedPathOverridesWorkspaceGrantInSeatbelt(t *testing.T) {
	requireSandboxExec(t)

	workspace := t.TempDir()
	denied := filepath.Join(workspace, "denied")
	if err := os.Mkdir(denied, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(denied, "secret.txt")
	if err := os.WriteFile(secret, []byte("denied"), 0o600); err != nil {
		t.Fatal(err)
	}
	sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: workspace, DeniedPaths: []string{denied}, Timeout: 20})
	if err != nil {
		t.Fatal(err)
	}
	darwin := unwrapDarwinSandbox(t, sandboxInstance)
	result, err := sandboxInstance.Exec(context.Background(), Command{
		Path: requireDarwinRuntime(t, "python3"),
		Args: []string{"-c", `
from pathlib import Path
import sys

denied = Path(sys.argv[1])
workspace = Path(sys.argv[2])
try:
    visible = denied.exists()
except OSError:
    visible = False
if visible:
    raise SystemExit(89)
try:
    (denied / "secret.txt").read_text()
except OSError:
    pass
else:
    raise SystemExit(90)
try:
    (denied / "intrusion.txt").write_text("escaped")
except OSError:
    pass
else:
    raise SystemExit(91)
(workspace / "owned.txt").write_text("owned")
print("denied")
`, darwin.cfg.DeniedPaths[0], darwin.cfg.Workspace},
	})

	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "denied" {
		t.Fatalf("denied path override failed: exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
	if _, err := os.Stat(filepath.Join(denied, "intrusion.txt")); !os.IsNotExist(err) {
		t.Fatalf("denied path accepted a write: %v", err)
	}
}

func unwrapDarwinSandbox(t *testing.T, sandboxInstance Sandbox) *darwinSandbox {
	t.Helper()
	if backend, ok := sandboxInstance.(*darwinSandbox); ok {
		return backend
	}
	wrapper, ok := sandboxInstance.(*capabilitySandbox)
	if !ok {
		t.Fatalf("New() returned unexpected sandbox type %T", sandboxInstance)
	}
	backend, ok := wrapper.backend.(*darwinSandbox)
	if !ok {
		t.Fatalf("sandbox backend = %T, want *darwinSandbox", wrapper.backend)
	}
	return backend
}

func TestDarwinProfileDoesNotGrantBlanketMachOrSystemSocketAccess(t *testing.T) {
	profile := newDarwinSandbox(Config{Workspace: t.TempDir()}).generateSBPL()
	for _, forbidden := range []string{"(allow mach-lookup", "(allow system-socket"} {
		if strings.Contains(profile, forbidden) {
			t.Fatalf("profile contains a blanket service grant %q:\n%s", forbidden, profile)
		}
	}
}

func TestDarwinSandboxCannotSignalHostSibling(t *testing.T) {
	requireSandboxExec(t)

	hostSibling := exec.CommandContext(t.Context(), "/bin/sleep", "30")
	if err := hostSibling.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = hostSibling.Process.Kill()
		_, _ = hostSibling.Process.Wait()
	})

	sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: t.TempDir(), Timeout: 20})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sandboxInstance.Exec(context.Background(), Command{
		Path: requireDarwinRuntime(t, "python3"),
		Args: []string{"-c", `
import os
import sys

try:
    os.kill(int(sys.argv[1]), 15)
except OSError as error:
    print("BLOCKED:%d" % error.errno)
    raise SystemExit(0)
raise SystemExit(90)
`, strconv.Itoa(hostSibling.Process.Pid)},
	})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 || !strings.HasPrefix(strings.TrimSpace(result.Stdout), "BLOCKED:") {
		t.Fatalf("sandbox signal boundary failed: exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
	if err := hostSibling.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("host sibling did not survive denied signal: %v", err)
	}
}

func requireSandboxExec(t *testing.T) {
	t.Helper()
	if _, err := darwinSandboxExecPath(); err != nil {
		t.Skip("sandbox-exec is unavailable")
	}
}

func darwinPreparedTestCommand(t *testing.T, path, workspace string) Command {
	t.Helper()
	env, err := darwinExecEnv(workspace, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	return Command{Path: path, Dir: workspace, Env: env}
}

func newDarwinSharedWorkspacePair(t *testing.T) (root, workspace, otherWorkspace string) {
	t.Helper()
	root, err := os.MkdirTemp("/Users/Shared", "toolkit-seatbelt-shared-")
	if err != nil {
		t.Fatalf("create shared workspace fixture: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	workspace = filepath.Join(root, "current")
	otherWorkspace = filepath.Join(root, "other")
	for _, path := range []string{workspace, otherWorkspace} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return root, workspace, otherWorkspace
}

func darwinEnvironmentMap(env []string) map[string]string {
	result := make(map[string]string, len(env))
	for _, item := range env {
		name, value, ok := strings.Cut(item, "=")
		if ok {
			result[name] = value
		}
	}
	return result
}
