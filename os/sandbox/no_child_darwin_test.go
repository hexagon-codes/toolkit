//go:build darwin

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const darwinNoChildNativeSource = `
#include <errno.h>
#include <fcntl.h>
#include <spawn.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>

extern char **environ;

static void write_marker(const char *path) {
    int fd = open(path, O_WRONLY | O_CREAT | O_TRUNC, 0600);
    if (fd >= 0) {
        (void)write(fd, "escaped", 7);
        (void)close(fd);
    }
}

static int blocked(const char *operation, int rounds) {
    printf("BLOCKED:%s:EPERM:%d\n", operation, rounds);
    return 0;
}

int main(int argc, char **argv) {
    if (argc < 2) return 64;
    if (strcmp(argv[1], "direct") == 0) {
        puts("DIRECT");
        return 0;
    }
    if (strcmp(argv[1], "marker") == 0 && argc == 3) {
        usleep(200000);
        write_marker(argv[2]);
        return 0;
    }
    if (strcmp(argv[1], "sleep-marker") == 0 && argc == 4) {
        write_marker(argv[3]);
        sleep(2);
        write_marker(argv[2]);
        return 0;
    }
    if (argc != 4) return 65;

    const char *mode = argv[1];
    const char *marker = argv[2];
    int rounds = atoi(argv[3]);
    if (rounds <= 0) return 67;
    if (strcmp(mode, "fork") == 0 || strcmp(mode, "setsid") == 0) {
        for (int iteration = 0; iteration < rounds; iteration++) {
            errno = 0;
            pid_t pid = fork();
            if (pid < 0) {
                if (errno != EPERM) return 80;
                continue;
            }
            if (pid == 0) {
                if (strcmp(mode, "setsid") == 0) {
                    (void)setsid();
                    (void)close(STDIN_FILENO);
                    (void)close(STDOUT_FILENO);
                    (void)close(STDERR_FILENO);
                }
                usleep(200000);
                write_marker(marker);
                _exit(0);
            }
            (void)waitpid(pid, NULL, 0);
            return 90;
        }
        return blocked(mode, rounds);
    }
    if (strcmp(mode, "vfork") == 0) {
        for (int iteration = 0; iteration < rounds; iteration++) {
            errno = 0;
            pid_t pid = vfork();
            if (pid < 0) {
                if (errno != EPERM) return 81;
                continue;
            }
            if (pid == 0) {
                write_marker(marker);
                _exit(0);
            }
            (void)waitpid(pid, NULL, 0);
            return 91;
        }
        return blocked(mode, rounds);
    }
    if (strcmp(mode, "posix_spawn") == 0) {
        for (int iteration = 0; iteration < rounds; iteration++) {
            pid_t pid = 0;
            char *const child_argv[] = {argv[0], "marker", (char *)marker, NULL};
            int rc = posix_spawn(&pid, argv[0], NULL, NULL, child_argv, environ);
            if (rc != 0) {
                if (rc != EPERM) return 82;
                continue;
            }
            (void)waitpid(pid, NULL, 0);
            return 92;
        }
        return blocked(mode, rounds);
    }
    return 66;
}
`

const (
	darwinNoChildDirectIterations = 100
	darwinNoChildSpawnIterations  = 100
	darwinNoChildWorkerIterations = 100
)

const darwinNoChildGoSource = `
package main

import (
    "errors"
    "fmt"
    "os"
    "os/exec"
    "strconv"
    "syscall"
)

func main() {
    if len(os.Args) < 2 {
        os.Exit(64)
    }
    switch os.Args[1] {
    case "direct":
        fmt.Println("GO-DIRECT")
    case "os-exec":
        if len(os.Args) != 5 {
            os.Exit(65)
        }
        rounds, err := strconv.Atoi(os.Args[4])
        if err != nil || rounds <= 0 {
            os.Exit(66)
        }
        for iteration := 0; iteration < rounds; iteration++ {
            err = exec.Command(os.Args[3], "marker", os.Args[2]).Run()
            if !errors.Is(err, syscall.EPERM) {
                fmt.Fprintf(os.Stderr, "unexpected os/exec result at iteration %d: %v\n", iteration, err)
                os.Exit(90)
            }
        }
        fmt.Printf("BLOCKED:go-os-exec:EPERM:%d\n", rounds)
    default:
        os.Exit(67)
    }
}
`

func TestDarwinNoChildProfileOmitsProcessForkGrant(t *testing.T) {
	profile := newDarwinSandbox(Config{Workspace: t.TempDir()}).generateSBPL()
	if strings.Contains(profile, "(allow process-fork)") {
		t.Fatalf("no-child profile grants process-fork:\n%s", profile)
	}
	if !strings.Contains(profile, "(deny process-fork)") {
		t.Fatalf("no-child profile does not explicitly deny process-fork:\n%s", profile)
	}
}

func TestDarwinNoChildBoundaryAllowsDirectSingleProcessRuntimes(t *testing.T) {
	requireSandboxExec(t)
	workspace := t.TempDir()
	native := buildDarwinNoChildNativeHelper(t, workspace)
	goHelper := buildDarwinNoChildGoHelper(t, workspace)
	tests := []struct {
		name string
		path string
		args []string
		want string
	}{
		{name: "native", path: native, args: []string{"direct"}, want: "DIRECT"},
		{name: "go", path: goHelper, args: []string{"direct"}, want: "GO-DIRECT"},
		{name: "python", path: requireDarwinRuntime(t, "python3"), args: []string{"-c", `print("PYTHON-DIRECT")`}, want: "PYTHON-DIRECT"},
		{name: "node", path: requireDarwinRuntime(t, "node"), args: []string{"-e", `console.log("NODE-DIRECT")`}, want: "NODE-DIRECT"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sandboxInstance, err := New(Config{Workspace: workspace, Timeout: 60, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
			if err != nil {
				t.Fatal(err)
			}
			for iteration := 0; iteration < darwinNoChildDirectIterations; iteration++ {
				result, err := sandboxInstance.Exec(context.Background(), Command{Path: test.path, Args: test.args})
				if err != nil {
					t.Fatalf("iteration %d Exec() error = %v", iteration, err)
				}
				if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != test.want {
					t.Fatalf("iteration %d direct runtime exit=%d stdout=%q stderr=%q", iteration, result.ExitCode, result.Stdout, result.Stderr)
				}
			}
		})
	}
}

func TestDarwinNoChildBoundaryBlocksNativeForkVariants(t *testing.T) {
	requireSandboxExec(t)
	workspace := t.TempDir()
	native := buildDarwinNoChildNativeHelper(t, workspace)
	for _, operation := range []string{"fork", "vfork", "posix_spawn", "setsid"} {
		t.Run(operation, func(t *testing.T) {
			marker := filepath.Join(workspace, operation+"-marker")
			sandboxInstance, err := New(Config{Workspace: workspace, Timeout: 60, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
			if err != nil {
				t.Fatal(err)
			}
			result, err := sandboxInstance.Exec(context.Background(), Command{Path: native, Args: []string{operation, marker, fmt.Sprint(darwinNoChildSpawnIterations)}})
			if err != nil {
				t.Fatalf("Exec() error = %v", err)
			}
			want := fmt.Sprintf("BLOCKED:%s:EPERM:%d", operation, darwinNoChildSpawnIterations)
			if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != want {
				t.Fatalf("%s escape result: exit=%d stdout=%q stderr=%q", operation, result.ExitCode, result.Stdout, result.Stderr)
			}
			assertDarwinMarkerAbsentAfter(t, marker, 400*time.Millisecond)
		})
	}
}

func TestDarwinNoChildBoundaryBlocksLanguageRuntimeSpawns(t *testing.T) {
	requireSandboxExec(t)
	workspace := t.TempDir()
	marker := filepath.Join(workspace, "runtime-spawn-marker")
	native := buildDarwinNoChildNativeHelper(t, workspace)
	goHelper := buildDarwinNoChildGoHelper(t, workspace)
	python := requireDarwinRuntime(t, "python3")
	node := requireDarwinRuntime(t, "node")
	type runtimeSpawnTest struct {
		name string
		path string
		args []string
		want string
	}
	tests := make([]runtimeSpawnTest, 0, 9)
	tests = append(tests,
		runtimeSpawnTest{
			name: "go-os-exec",
			path: goHelper,
			args: []string{"os-exec", marker, native, fmt.Sprint(darwinNoChildSpawnIterations)},
			want: fmt.Sprintf("BLOCKED:go-os-exec:EPERM:%d", darwinNoChildSpawnIterations),
		},
		runtimeSpawnTest{
			name: "python-fork",
			path: python,
			args: []string{"-c", `
import errno
import os
import sys
rounds = int(sys.argv[2])
for iteration in range(rounds):
    try:
        pid = os.fork()
    except OSError as error:
        if error.errno != errno.EPERM:
            raise
        continue
    if pid == 0:
        open(sys.argv[1], "w").write("escaped")
        os._exit(0)
    os.waitpid(pid, 0)
    raise SystemExit(90)
print("BLOCKED:python-fork:EPERM:%d" % rounds)
`, marker, fmt.Sprint(darwinNoChildSpawnIterations)},
			want: fmt.Sprintf("BLOCKED:python-fork:EPERM:%d", darwinNoChildSpawnIterations),
		},
		runtimeSpawnTest{
			name: "python-posix-spawn",
			path: python,
			args: []string{"-c", `
import errno
import os
import sys
rounds = int(sys.argv[3])
for iteration in range(rounds):
    try:
        os.posix_spawn(sys.argv[1], [sys.argv[1], "marker", sys.argv[2]], os.environ)
    except OSError as error:
        if error.errno != errno.EPERM:
            raise
        continue
    raise SystemExit(91)
print("BLOCKED:python-posix-spawn:EPERM:%d" % rounds)
`, native, marker, fmt.Sprint(darwinNoChildSpawnIterations)},
			want: fmt.Sprintf("BLOCKED:python-posix-spawn:EPERM:%d", darwinNoChildSpawnIterations),
		},
		runtimeSpawnTest{
			name: "python-start-new-session",
			path: python,
			args: []string{"-c", `
import errno
import subprocess
import sys
rounds = int(sys.argv[3])
for iteration in range(rounds):
    try:
        subprocess.Popen(
            [sys.argv[1], "marker", sys.argv[2]],
            stdin=subprocess.DEVNULL,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            start_new_session=True,
        )
    except OSError as error:
        if error.errno != errno.EPERM:
            raise
        continue
    raise SystemExit(92)
print("BLOCKED:python-start-new-session:EPERM:%d" % rounds)
`, native, marker, fmt.Sprint(darwinNoChildSpawnIterations)},
			want: fmt.Sprintf("BLOCKED:python-start-new-session:EPERM:%d", darwinNoChildSpawnIterations),
		},
		runtimeSpawnTest{
			name: "node-spawn",
			path: node,
			args: []string{"-e", `
const { spawn } = require("child_process");

function attempt(iteration) {
  return new Promise((resolve, reject) => {
    let child;
    try {
      child = spawn(process.argv[1], ["marker", process.argv[2]]);
    } catch (error) {
      if (error.code === "EPERM") resolve();
      else reject(error);
      return;
    }
    child.once("error", (error) => {
      if (error.code === "EPERM") resolve();
      else reject(error);
    });
    child.once("spawn", () => reject(new Error("spawn escaped at iteration " + iteration)));
  });
}

(async () => {
  const rounds = Number(process.argv[3]);
  for (let iteration = 0; iteration < rounds; iteration++) await attempt(iteration);
  console.log("BLOCKED:node-spawn:EPERM:" + rounds);
})().catch((error) => {
  console.error(error.stack || error);
  process.exit(93);
});
`, native, marker, fmt.Sprint(darwinNoChildSpawnIterations)},
			want: fmt.Sprintf("BLOCKED:node-spawn:EPERM:%d", darwinNoChildSpawnIterations),
		},
		runtimeSpawnTest{
			name: "node-detached",
			path: node,
			args: []string{"-e", `
const { spawn } = require("child_process");

function attempt(iteration) {
  return new Promise((resolve, reject) => {
    let child;
    try {
      child = spawn(process.argv[1], ["marker", process.argv[2]], {
        detached: true,
        stdio: "ignore",
      });
    } catch (error) {
      if (error.code === "EPERM") resolve();
      else reject(error);
      return;
    }
    child.once("error", (error) => {
      if (error.code === "EPERM") resolve();
      else reject(error);
    });
    child.once("spawn", () => reject(new Error("detached spawn escaped at iteration " + iteration)));
  });
}

(async () => {
  const rounds = Number(process.argv[3]);
  for (let iteration = 0; iteration < rounds; iteration++) await attempt(iteration);
  console.log("BLOCKED:node-detached:EPERM:" + rounds);
})().catch((error) => {
  console.error(error.stack || error);
  process.exit(94);
});
`, native, marker, fmt.Sprint(darwinNoChildSpawnIterations)},
			want: fmt.Sprintf("BLOCKED:node-detached:EPERM:%d", darwinNoChildSpawnIterations),
		},
	)
	for _, method := range []string{"fork", "spawn", "forkserver"} {
		name := "python-multiprocessing-" + method
		tests = append(tests, runtimeSpawnTest{
			name: name,
			path: python,
			args: []string{"-c", fmt.Sprintf(`
import multiprocessing
import pathlib
import sys

def write_marker(path):
    pathlib.Path(path).write_text("escaped")

rounds = int(sys.argv[2])
for iteration in range(rounds):
    process = multiprocessing.get_context(%q).Process(target=write_marker, args=(sys.argv[1],))
    try:
        process.start()
    except OSError as error:
        if error.errno != 1:
            raise
        continue
    process.join(1)
    raise SystemExit(97)
print("BLOCKED:%s:EPERM:%%d" %% rounds)
`, method, name), marker, fmt.Sprint(darwinNoChildSpawnIterations)},
			want: fmt.Sprintf("BLOCKED:%s:EPERM:%d", name, darwinNoChildSpawnIterations),
		})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sandboxInstance, err := New(Config{Workspace: workspace, Timeout: 60, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
			if err != nil {
				t.Fatal(err)
			}
			result, err := sandboxInstance.Exec(context.Background(), Command{Path: test.path, Args: test.args})
			if err != nil {
				t.Fatalf("Exec() error = %v", err)
			}
			if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != test.want {
				t.Fatalf("runtime spawn result: exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
			}
		})
	}
	assertDarwinMarkerAbsentAfter(t, marker, 400*time.Millisecond)
}

func TestDarwinNoChildBoundaryAllowsNodeWorkerThreads(t *testing.T) {
	requireSandboxExec(t)
	workspace := t.TempDir()
	sandboxInstance, err := New(Config{Workspace: workspace, Timeout: 30, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	if err != nil {
		t.Fatal(err)
	}
	node := requireDarwinRuntime(t, "node")
	code := fmt.Sprintf(`
const { Worker } = require("worker_threads");

function runWorker(iteration) {
  return new Promise((resolve, reject) => {
    const worker = new Worker("const { parentPort } = require('worker_threads'); parentPort.postMessage('ok');", { eval: true });
    worker.once("message", (message) => {
      if (message !== "ok") reject(new Error("unexpected worker response at " + iteration));
      else resolve();
    });
    worker.once("error", reject);
  });
}

(async () => {
  for (let iteration = 0; iteration < %d; iteration++) await runWorker(iteration);
  console.log("WORKER-THREADS:%d");
})().catch((error) => {
  console.error(error.stack || error);
  process.exit(98);
});
`, darwinNoChildWorkerIterations, darwinNoChildWorkerIterations)
	result, err := sandboxInstance.Exec(context.Background(), Command{Path: node, Args: []string{"-e", code}})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	want := fmt.Sprintf("WORKER-THREADS:%d", darwinNoChildWorkerIterations)
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != want {
		t.Fatalf("worker_threads result: exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
	}
}

func TestDarwinNoChildBoundaryTimeoutAndCancelKillTrackedRoot(t *testing.T) {
	requireSandboxExec(t)
	t.Run("timeout", func(t *testing.T) {
		workspace := t.TempDir()
		native := buildDarwinNoChildNativeHelper(t, workspace)
		marker := filepath.Join(workspace, "timeout-marker")
		ready := filepath.Join(workspace, "timeout-ready")
		sandboxInstance, err := New(Config{Workspace: workspace, Timeout: 1, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
		if err != nil {
			t.Fatal(err)
		}
		result, err := sandboxInstance.Exec(context.Background(), Command{Path: native, Args: []string{"sleep-marker", marker, ready}})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("timeout error = %v", err)
		}
		if result == nil || result.Limits.ProcessContainment != LimitStatusEnforced {
			t.Fatalf("timeout result = %+v", result)
		}
		assertDarwinMarkerAbsentAfter(t, marker, 1300*time.Millisecond)
	})

	t.Run("cancel", func(t *testing.T) {
		workspace := t.TempDir()
		native := buildDarwinNoChildNativeHelper(t, workspace)
		marker := filepath.Join(workspace, "cancel-marker")
		ready := filepath.Join(workspace, "cancel-ready")
		sandboxInstance, err := New(Config{Workspace: workspace, Timeout: 60, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		type execution struct {
			result *ExecResult
			err    error
		}
		done := make(chan execution, 1)
		go func() {
			result, execErr := sandboxInstance.Exec(ctx, Command{Path: native, Args: []string{"sleep-marker", marker, ready}})
			done <- execution{result: result, err: execErr}
		}()
		readyDeadline := time.NewTimer(30 * time.Second)
		defer readyDeadline.Stop()
		readyPoll := time.NewTicker(10 * time.Millisecond)
		defer readyPoll.Stop()
	waitUntilReady:
		for {
			if _, statErr := os.Stat(ready); statErr == nil {
				break waitUntilReady
			} else if !os.IsNotExist(statErr) {
				t.Fatalf("inspect cancel readiness marker: %v", statErr)
			}
			select {
			case got := <-done:
				t.Fatalf("sandbox execution ended before cancel readiness: result=%+v error=%v", got.result, got.err)
			case <-readyPoll.C:
			case <-readyDeadline.C:
				t.Fatal("sandbox execution did not reach cancel readiness within 30s")
			}
		}
		cancel()
		select {
		case got := <-done:
			if !errors.Is(got.err, context.Canceled) {
				t.Fatalf("cancel error = %v", got.err)
			}
			if got.result == nil || got.result.Limits.ProcessContainment != LimitStatusEnforced {
				t.Fatalf("cancel result = %+v", got.result)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("cancel did not terminate the tracked root")
		}
		assertDarwinMarkerAbsentAfter(t, marker, 2100*time.Millisecond)
	})
}

func TestDarwinNoChildBoundaryBlocksDelayedSetsidMarkerForOneHundredRuns(t *testing.T) {
	requireSandboxExec(t)
	workspace := t.TempDir()
	native := buildDarwinNoChildNativeHelper(t, workspace)
	sandboxInstance, err := New(Config{Workspace: workspace, Timeout: 20, RequiredCapabilities: UntrustedCodeIsolationCapabilities})
	if err != nil {
		t.Fatal(err)
	}
	markers := make([]string, 0, 100)
	for iteration := 0; iteration < 100; iteration++ {
		marker := filepath.Join(workspace, fmt.Sprintf("setsid-marker-%03d", iteration))
		markers = append(markers, marker)
		result, err := sandboxInstance.Exec(context.Background(), Command{Path: native, Args: []string{"setsid", marker, "1"}})
		if err != nil {
			t.Fatalf("iteration %d Exec() error = %v", iteration, err)
		}
		if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "BLOCKED:setsid:EPERM:1" {
			t.Fatalf("iteration %d result: exit=%d stdout=%q stderr=%q", iteration, result.ExitCode, result.Stdout, result.Stderr)
		}
	}
	time.Sleep(500 * time.Millisecond)
	for iteration, marker := range markers {
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("iteration %d delayed marker exists: %v", iteration, err)
		}
	}
}

func buildDarwinNoChildNativeHelper(t *testing.T, workspace string) string {
	t.Helper()
	compiler := "/usr/bin/clang"
	if info, err := os.Stat(compiler); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("trusted native compiler is unavailable: %v", err)
	}
	source := filepath.Join(workspace, "no-child-helper.c")
	binary := filepath.Join(workspace, "no-child-helper")
	if err := os.WriteFile(source, []byte(darwinNoChildNativeSource), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), compiler, "-O2", "-Wall", "-Wextra", "-o", binary, source)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("compile native no-child helper: %v: %s", err, output)
	}
	return binary
}

func buildDarwinNoChildGoHelper(t *testing.T, workspace string) string {
	t.Helper()
	goBinary, _ := darwinTestGoToolchain(t)
	source := filepath.Join(workspace, "no-child-helper.go")
	binary := filepath.Join(workspace, "no-child-go-helper")
	if err := os.WriteFile(source, []byte(darwinNoChildGoSource), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), goBinary, "build", "-o", binary, source)
	command.Env = append(os.Environ(), "GOENV=off", "GOTOOLCHAIN=local", "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("compile Go no-child helper: %v: %s", err, output)
	}
	return binary
}

func requireDarwinRuntime(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("required runtime %q is unavailable: %v", name, err)
	}
	return path
}

func assertDarwinMarkerAbsentAfter(t *testing.T, marker string, wait time.Duration) {
	t.Helper()
	time.Sleep(wait)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("delayed marker %q exists: %v", marker, err)
	}
}

// newDarwinTestSandbox 为 Darwin 集成测试显式声明不可信代码最低能力合同。
func newDarwinTestSandbox(config Config) (Sandbox, error) {
	if config.RequiredCapabilities == 0 {
		config.RequiredCapabilities = UntrustedCodeIsolationCapabilities
	}
	return New(config)
}
