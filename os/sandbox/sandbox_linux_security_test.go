//go:build linux

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLinuxBwrapArgsDoNotExposeBroadHostTrees(t *testing.T) {
	workspace := t.TempDir()
	env, err := cleanLinuxEnv(workspace, []string{"LANG=C", "API_KEY=secret"})
	if err != nil {
		t.Fatal(err)
	}
	sandboxInstance := &linuxSandbox{cfg: Config{Workspace: workspace}}
	args, err := sandboxInstance.bwrapArgs(Command{
		Path: "/bin/sh",
		Args: []string{"-c", "exit 0"},
		Dir:  workspace,
		Env:  env,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(args, "--clearenv") {
		t.Fatal("bubblewrap arguments must clear the inherited environment")
	}
	privateTemporary := filepath.Join(workspace, posixRuntimeDirectory, "tmp")
	if source, ok := linuxBwrapMountSource(args, "/tmp"); !ok || source != privateTemporary {
		t.Fatalf("sandbox /tmp source = %q, want workspace-private %q", source, privateTemporary)
	}
	for _, mount := range linuxBwrapMountArguments(args) {
		if linuxBwrapMountExposesBroadHostTree(mount) {
			t.Errorf("bubblewrap exposes broad host tree: source=%q target=%q", mount.source, mount.target)
		}
	}
	if source, ok := linuxBwrapMountSource(args, "/etc/resolv.conf"); ok {
		info, statErr := os.Stat(source)
		if statErr != nil || !info.Mode().IsRegular() {
			t.Errorf("resolver source %q is not a regular file: info=%v error=%v", source, info, statErr)
		}
	}
}

func TestLinuxBroadHostMountDetectionUsesCleanPaths(t *testing.T) {
	tests := []struct {
		name  string
		mount linuxReadOnlyMount
	}{
		{name: "root source", mount: linuxReadOnlyMount{source: "/./", target: "/sandbox/root"}},
		{name: "etc target", mount: linuxReadOnlyMount{source: "/sandbox/etc", target: "//etc//"}},
		{name: "run source", mount: linuxReadOnlyMount{source: "/run/", target: "/sandbox/run"}},
		{name: "opt target", mount: linuxReadOnlyMount{source: "/sandbox/opt", target: "/opt/."}},
		{name: "usr target", mount: linuxReadOnlyMount{source: "/sandbox/usr", target: "/usr/local/.."}},
		{name: "usr local source", mount: linuxReadOnlyMount{source: "/usr/local/", target: "/sandbox/usr-local"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !linuxBwrapMountExposesBroadHostTree(test.mount) {
				t.Fatalf("mount was not rejected after path cleaning: source=%q target=%q", test.mount.source, test.mount.target)
			}
		})
	}

	for _, mount := range []linuxReadOnlyMount{
		{source: "/usr/bin", target: "/usr/bin"},
		{source: "/etc/ssl/certs", target: "/etc/ssl/certs"},
		{source: "/tmp/runtime", target: "/tmp/runtime"},
	} {
		if linuxBwrapMountExposesBroadHostTree(mount) {
			t.Fatalf("precise runtime mount was rejected: source=%q target=%q", mount.source, mount.target)
		}
	}
}

func TestLinuxTemporaryFilesStayInsideWorkspace(t *testing.T) {
	requireUsableLinuxBwrap(t)

	workspace := t.TempDir()
	sandboxInstance, err := New(Config{
		Workspace:            workspace,
		Timeout:              20,
		MaxMemoryBytes:       1 << 62,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities | CapabilityMemory,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sandboxInstance.Exec(context.Background(), Command{
		Path: "/bin/sh",
		Args: []string{"-c", `printf workspace-private > /tmp/proof`},
	})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("temporary write failed: exit=%d stderr=%q", result.ExitCode, result.Stderr)
	}
	proof := filepath.Join(workspace, posixRuntimeDirectory, "tmp", "proof")
	content, err := os.ReadFile(proof)
	if err != nil || string(content) != "workspace-private" {
		t.Fatalf("workspace temporary proof = %q, %v", content, err)
	}
}

func TestLinuxPrivateTemporaryDirectoryCannotBypassDeniedPaths(t *testing.T) {
	workspace := t.TempDir()
	if err := initializePOSIXRuntimeDirectories(workspace); err != nil {
		t.Fatal(err)
	}
	privateTemporary := filepath.Join(workspace, posixRuntimeDirectory, "tmp")
	sandboxInstance := &linuxSandbox{cfg: Config{
		Workspace:   workspace,
		DeniedPaths: []string{privateTemporary},
	}}
	command := linuxPreparedTestCommand(t, "/bin/sh", workspace)
	if _, err := sandboxInstance.bwrapArgs(command); err == nil || !strings.Contains(err.Error(), "conflicts with a denied path") {
		t.Fatalf("denied private temporary directory error = %v", err)
	}
}

func TestLinuxRuntimeLayoutsArePreciseAndMountParentsFirst(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		command    string
		executable string
		wantRoot   string
	}{
		{
			name:       "homebrew-symlink-wrapper",
			command:    "/home/linuxbrew/.linuxbrew/bin/node",
			executable: "/home/linuxbrew/.linuxbrew/Cellar/node/24.1.0/bin/node",
			wantRoot:   "/home/linuxbrew/.linuxbrew/Cellar/node/24.1.0",
		},
		{
			name:       "pyenv",
			command:    filepath.Join(home, ".pyenv", "shims", "python3"),
			executable: filepath.Join(home, ".pyenv", "versions", "3.13.5", "bin", "python3"),
			wantRoot:   filepath.Join(home, ".pyenv", "versions", "3.13.5"),
		},
		{
			name:       "nvm",
			command:    filepath.Join(home, ".nvm", "current", "bin", "node"),
			executable: filepath.Join(home, ".nvm", "versions", "node", "v24.1.0", "bin", "node"),
			wantRoot:   filepath.Join(home, ".nvm", "versions", "node", "v24.1.0"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			paths := linuxRuntimePathsForHost(test.command, test.executable, test.executable)
			if !slices.Contains(paths, test.wantRoot) || !slices.Contains(paths, test.executable) {
				t.Fatalf("runtime paths = %q, want root %q and executable %q", paths, test.wantRoot, test.executable)
			}
			if got := linuxRuntimePathDirectory(test.executable); got != filepath.Dir(test.executable) || got == filepath.Dir(test.command) {
				t.Fatalf("resolved PATH directory = %q, alias directory = %q", got, filepath.Dir(test.command))
			}
			mounts := linuxUniqueMounts([]linuxReadOnlyMount{
				{source: test.executable, target: test.executable},
				{source: test.wantRoot, target: test.wantRoot},
			})
			if len(mounts) != 2 || mounts[0].target != test.wantRoot {
				t.Fatalf("runtime mount order = %+v, want root before executable", mounts)
			}
		})
	}
	if dirExists("/usr/share/nodejs") {
		paths := linuxRuntimePaths("node", "/usr/bin/node")
		if !slices.Contains(paths, "/usr/share/nodejs") {
			t.Fatalf("Debian Node runtime paths = %q, want /usr/share/nodejs", paths)
		}
	}
}

func TestLinuxCommandPathDoesNotResolveThroughHostPATH(t *testing.T) {
	workspace := t.TempDir()
	for _, command := range []string{"python3", "node", "go", "gofmt", "npm", "npx", "pip", "pip3"} {
		absolute, err := exec.LookPath(command)
		if err != nil {
			continue
		}
		if _, err := linuxCommandExecutionPlan(linuxPreparedTestCommand(t, command, workspace), workspace); err == nil {
			t.Errorf("bare command %q unexpectedly resolved through host PATH", command)
		}
		if _, err := linuxCommandExecutionPlan(linuxPreparedTestCommand(t, absolute, workspace), workspace); err != nil {
			t.Errorf("absolute command %q plan error = %v", absolute, err)
		}
	}
}

func TestLinuxForgedExternalRuntimePathsFailClosed(t *testing.T) {
	workspace := t.TempDir()
	otherWorkspace := t.TempDir()
	for _, path := range []string{
		filepath.Join(otherWorkspace, "Cellar", "node", "99.0.0", "bin", "node"),
		filepath.Join(otherWorkspace, ".pyenv", "versions", "3.99.0", "bin", "python3"),
		filepath.Join(otherWorkspace, ".nvm", "versions", "node", "v99.0.0", "bin", "node"),
		filepath.Join(otherWorkspace, "go", "pkg", "mod", "golang.org", "toolchain@v99", "bin", "go"),
		filepath.Join(otherWorkspace, "node_modules", "npm", "bin", "npm"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := linuxCommandExecutionPlan(linuxPreparedTestCommand(t, path, workspace), workspace); err == nil {
			t.Errorf("forged external runtime %q was trusted", path)
		}
	}
}

func TestLinuxWorkspaceScriptsResolveTrustedShebangs(t *testing.T) {
	workspace := t.TempDir()
	for _, test := range []struct {
		name    string
		command string
		body    string
	}{
		{name: "python", command: "python3", body: "print('python-workspace')"},
		{name: "node", command: "node", body: "console.log('node-workspace')"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := exec.LookPath(test.command); err != nil {
				t.Skipf("%s is unavailable", test.command)
			}
			script := filepath.Join(workspace, test.name+"-workspace")
			content := fmt.Sprintf("#!/usr/bin/env %s\n%s\n", test.command, test.body)
			if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
				t.Fatal(err)
			}
			if _, err := linuxCommandExecutionPlan(linuxPreparedTestCommand(t, script, workspace), workspace); err != nil {
				t.Fatalf("workspace shebang plan error = %v", err)
			}
			if bwrap, err := linuxBwrapPath(); err == nil && linuxBwrapBackendUsable(bwrap, false) {
				sandboxInstance, err := New(Config{
					Workspace:            workspace,
					Timeout:              20,
					MaxMemoryBytes:       1 << 62,
					RequiredCapabilities: UntrustedCodeIsolationCapabilities | CapabilityMemory,
				})
				if err != nil {
					t.Fatal(err)
				}
				result, err := sandboxInstance.Exec(context.Background(), Command{Path: script, Args: nil})
				if err != nil {
					t.Fatalf("Exec() error = %v", err)
				}
				want := test.name + "-workspace"
				if result.ExitCode != 0 || !strings.Contains(result.Stdout, want) {
					t.Fatalf("workspace shebang failed: exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
				}
			}
		})
	}
	untrusted := filepath.Join(workspace, "untrusted-workspace")
	if err := os.WriteFile(untrusted, []byte("#!/usr/bin/env toolkit-missing-runtime\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := linuxCommandExecutionPlan(linuxPreparedTestCommand(t, untrusted, workspace), workspace); err == nil {
		t.Fatal("workspace script with an unavailable interpreter must fail closed")
	}
}

func TestLinuxRootBwrapHidesHostUnixSocket(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root Linux Unix socket boundary only")
	}
	requireUsableLinuxBwrapWithoutSkip(t)
	runRoot, err := os.MkdirTemp("/run", "toolkit-bwrap-socket-")
	if err != nil {
		t.Fatalf("create controlled /run fixture: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runRoot) })
	socketPath := filepath.Join(runRoot, "host.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	workspace := t.TempDir()
	probe := copyLinuxTestExecutable(t, workspace)
	sandboxInstance := newLinuxFilesystemIsolationTestSandbox(t, workspace)
	result, err := sandboxInstance.Exec(context.Background(), Command{Path: probe, Args: []string{
		"-test.run=^TestLinuxUnixSocketProbePayload$",
		"-test.v",
		"--",
		socketPath,
	}})

	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, "BLOCKED") || strings.Contains(result.Stdout, "CONNECTED") {
		t.Fatalf("host Unix socket escaped: exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
}

func TestLinuxBwrapHidesExternalWorkspaceWithoutPrivilege(t *testing.T) {
	requireUsableLinuxBwrapWithoutSkip(t)

	workspace := t.TempDir()
	otherWorkspace := t.TempDir()
	payload := filepath.Join(otherWorkspace, "payload.sh")
	if err := os.WriteFile(payload, []byte("#!/bin/sh\nprintf 'ESCAPED'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	sandboxInstance := newLinuxFilesystemIsolationTestSandbox(t, workspace)
	if _, err := sandboxInstance.Exec(context.Background(), Command{Path: payload, Args: nil}); err == nil {
		t.Fatal("external workspace executable did not fail closed")
	}
	result, err := sandboxInstance.Exec(context.Background(), Command{Path: "/bin/sh", Args: []string{"-c", `
other=$1
if test -e "$other/payload.sh"; then exit 81; fi
if (printf escaped >"$other/intrusion") 2>/dev/null; then exit 82; fi
printf isolated
`, "bwrap-isolation", otherWorkspace}})

	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "isolated" {
		t.Fatalf("external workspace escaped: exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
}

func TestLinuxBwrapExecUsesMinimumEnvironment(t *testing.T) {
	requireUsableLinuxBwrap(t)
	for name, value := range map[string]string{
		"OPENAI_API_KEY":   "api-sentinel",
		"SERVICE_TOKEN":    "token-sentinel",
		"TOP_SECRET":       "secret-sentinel",
		"HTTPS_PROXY":      "http://proxy.invalid",
		"GOPATH":           "/host/go",
		"NPM_CONFIG_CACHE": "/host/npm",
		"PIP_CACHE_DIR":    "/host/pip",
	} {
		t.Setenv(name, value)
	}

	workspace := t.TempDir()
	sandboxInstance, err := New(Config{
		Workspace:            workspace,
		Timeout:              20,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sandboxInstance.Exec(context.Background(), Command{Path: "/usr/bin/env", Args: nil})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("environment probe exit=%d stderr=%q", result.ExitCode, result.Stderr)
	}
	values := posixEnvironmentMap(strings.Split(strings.TrimSpace(result.Stdout), "\n"))
	for _, name := range []string{"OPENAI_API_KEY", "SERVICE_TOKEN", "TOP_SECRET", "HTTPS_PROXY"} {
		if _, exists := values[name]; exists {
			t.Errorf("sandbox environment exposed %s", name)
		}
	}
	runtimeRoot := filepath.Join(workspace, posixRuntimeDirectory)
	for _, name := range []string{"HOME", "TMPDIR", "GOPATH", "GOCACHE", "GOMODCACHE", "GOTMPDIR", "NPM_CONFIG_CACHE", "PIP_CACHE_DIR"} {
		if !linuxPathWithin(runtimeRoot, values[name]) {
			t.Errorf("sandbox environment %s=%q is outside workspace runtime", name, values[name])
		}
	}
}

func TestLinuxSandboxLaunchersIgnoreHostPATHPoisoning(t *testing.T) {
	bwrap, err := linuxBwrapPath()
	if err != nil || !linuxBwrapBackendUsable(bwrap, false) {
		t.Skip("usable trusted bubblewrap is unavailable")
	}
	prlimit, err := linuxPrlimitPath()
	if err != nil {
		t.Skip("trusted prlimit is unavailable")
	}

	poisonDirectory := t.TempDir()
	bwrapMarker := filepath.Join(poisonDirectory, "bwrap-executed")
	prlimitMarker := filepath.Join(poisonDirectory, "prlimit-executed")
	writeForwardingLauncher := func(name, target, marker string) {
		t.Helper()
		body := fmt.Sprintf("#!/bin/sh\nprintf poisoned > %q\nexec %q \"$@\"\n", marker, target)
		if err := os.WriteFile(filepath.Join(poisonDirectory, name), []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeForwardingLauncher("bwrap", bwrap, bwrapMarker)
	writeForwardingLauncher("prlimit", prlimit, prlimitMarker)
	t.Setenv("PATH", poisonDirectory+string(os.PathListSeparator)+"/usr/bin:/bin")

	sandboxInstance, err := New(Config{
		Workspace:            t.TempDir(),
		Timeout:              20,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities,
	})
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
	for _, marker := range []string{bwrapMarker, prlimitMarker} {
		if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("PATH-poisoned launcher was invoked: marker=%q error=%v", marker, statErr)
		}
	}
}

func TestLinuxProcessContainmentTerminationIsBackedByPIDNamespace(t *testing.T) {
	requireUsableLinuxBwrap(t)
	setsid, err := exec.LookPath("setsid")
	if err != nil {
		t.Skip("setsid is unavailable")
	}

	for _, mode := range []string{"main-exit", "context-cancel"} {
		t.Run(mode, func(t *testing.T) {
			workspace := t.TempDir()
			marker := filepath.Join(workspace, "escaped")
			ready := filepath.Join(workspace, "ready")
			sandboxInstance, err := New(Config{
				Workspace:            workspace,
				Timeout:              20,
				RequiredCapabilities: UntrustedCodeIsolationCapabilities,
			})
			if err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			parentAction := "exit 0"
			if mode == "context-cancel" {
				parentAction = "while :; do sleep 10; done"
			}
			script := fmt.Sprintf(`
"$1" -f /bin/sh -c 'trap "" HUP INT TERM; sleep 1; printf escaped > "$1"' child "$2" >/dev/null 2>&1 &
printf ready > "$3"
%s
`, parentAction)
			type execution struct {
				result *ExecResult
				err    error
			}
			done := make(chan execution, 1)
			go func() {
				result, execErr := sandboxInstance.Exec(ctx, Command{Path: "/bin/sh", Args: []string{"-c", script, "tree-probe", setsid, marker, ready}})
				done <- execution{result: result, err: execErr}
			}()

			waitForLinuxFixture(t, ready, 5*time.Second)
			if mode == "context-cancel" {
				cancel()
			}

			var executionResult execution
			select {
			case executionResult = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("sandbox execution did not terminate with its PID namespace")
			}
			if mode == "main-exit" && executionResult.err != nil {
				t.Fatalf("Exec() error = %v", executionResult.err)
			}
			if mode == "context-cancel" && !errors.Is(executionResult.err, context.Canceled) {
				t.Fatalf("Exec() error = %v, want context cancellation", executionResult.err)
			}
			if executionResult.result == nil || executionResult.result.Limits.ProcessContainment != LimitStatusEnforced {
				t.Fatalf("ProcessContainment report = %+v, want enforced", executionResult.result)
			}

			time.Sleep(1500 * time.Millisecond)
			if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("detached descendant escaped PID namespace: stat error = %v", statErr)
			}
		})
	}
}

func waitForLinuxFixture(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("fixture %q was not created within %s", path, timeout)
}

func linuxBwrapMountArguments(args []string) []linuxReadOnlyMount {
	var mounts []linuxReadOnlyMount
	for index := 0; index+2 < len(args); index++ {
		if args[index] == "--ro-bind" || args[index] == "--bind" {
			mounts = append(mounts, linuxReadOnlyMount{source: args[index+1], target: args[index+2]})
			index += 2
		}
	}
	return mounts
}

func linuxBwrapMountExposesBroadHostTree(mount linuxReadOnlyMount) bool {
	for _, rawPath := range []string{mount.source, mount.target} {
		switch filepath.Clean(rawPath) {
		case "/", "/etc", "/run", "/opt", "/usr", "/usr/local":
			return true
		}
	}
	return false
}

func linuxBwrapMountSource(args []string, target string) (string, bool) {
	for _, mount := range linuxBwrapMountArguments(args) {
		if mount.target == target {
			return mount.source, true
		}
	}
	return "", false
}

func requireUsableLinuxBwrap(t *testing.T) {
	t.Helper()
	bwrap, err := linuxBwrapPath()
	if err != nil || !linuxBwrapBackendUsable(bwrap, false) {
		t.Skip("usable bubblewrap is unavailable")
	}
}

func requireUsableLinuxBwrapWithoutSkip(t *testing.T) {
	t.Helper()
	bwrap, err := linuxBwrapPath()
	if err != nil {
		t.Fatalf("trusted bubblewrap is required: %v", err)
	}
	if !linuxBwrapBackendUsable(bwrap, false) {
		t.Fatal("trusted bubblewrap is not usable")
	}
}

func newLinuxFilesystemIsolationTestSandbox(t *testing.T, workspace string) Sandbox {
	t.Helper()
	sandboxInstance, err := New(Config{
		Workspace:            workspace,
		Timeout:              20,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := sandboxInstance.Close(); closeErr != nil {
			t.Errorf("Close() error = %v", closeErr)
		}
	})
	return sandboxInstance
}

func linuxPreparedTestCommand(t *testing.T, path, workspace string) Command {
	t.Helper()
	env, err := cleanLinuxEnv(workspace, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	return Command{Path: path, Dir: workspace, Env: env}
}

func TestLinuxUnixSocketProbePayload(t *testing.T) {
	separator := slices.Index(os.Args, "--")
	if separator < 0 || separator+1 >= len(os.Args) {
		return
	}
	connection, err := net.DialTimeout("unix", os.Args[separator+1], time.Second)
	if err != nil {
		fmt.Print("BLOCKED")
		return
	}
	_ = connection.Close()
	fmt.Print("CONNECTED")
}

func copyLinuxTestExecutable(t *testing.T, workspace string) string {
	t.Helper()
	binary := filepath.Join(workspace, "socket-probe")
	testExecutable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(testExecutable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, content, 0o700); err != nil {
		t.Fatal(err)
	}
	return binary
}
