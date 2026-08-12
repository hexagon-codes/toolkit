//go:build unix

package hostprocess

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestHostProcessLifecycleHelper(t *testing.T) {
	switch os.Getenv("TOOLKIT_PROCESS_HELPER") {
	case "child":
		if ready := os.Getenv("TOOLKIT_PROCESS_CHILD_READY"); ready != "" {
			signal.Ignore(syscall.SIGTERM)
			if err := os.WriteFile(ready, []byte("ready"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "root":
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		child := exec.CommandContext(t.Context(), exe, "-test.run=^TestHostProcessLifecycleHelper$")
		for _, value := range os.Environ() {
			if !strings.HasPrefix(value, "TOOLKIT_PROCESS_HELPER=") {
				child.Env = append(child.Env, value)
			}
		}
		child.Env = append(child.Env, "TOOLKIT_PROCESS_HELPER=child")
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprintln(os.Stdout, child.Process.Pid); err != nil {
			t.Fatal(err)
		}
		var gate [1]byte
		_, _ = os.Stdin.Read(gate[:])
		os.Exit(0)
	}
}

func TestHostProcessExitCancellationOwnsDescendants(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	input, gate, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer gate.Close()
	output, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	childReady := filepath.Join(t.TempDir(), "child-ready")
	type outcome struct {
		result Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := Run(ctx, Spec{Executable: exe, Args: []string{"-test.run=^TestHostProcessLifecycleHelper$"},
			Env: append(os.Environ(), "TOOLKIT_PROCESS_HELPER=root", "TOOLKIT_PROCESS_CHILD_READY="+childReady), Stdin: input, Stdout: output, Stderr: output})
		done <- outcome{result, err}
	}()
	deadline := time.Now().Add(3 * time.Second)
	var childPID int
	for childPID == 0 {
		data, err := os.ReadFile(output.Name())
		if err != nil {
			t.Fatal(err)
		}
		childPID, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		if time.Now().After(deadline) {
			t.Fatal("helper did not start its child")
		}
		time.Sleep(time.Millisecond)
	}
	defer func() { _ = syscall.Kill(childPID, syscall.SIGKILL) }()
	for {
		if _, err := os.Stat(childReady); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not become ready to require group termination")
		}
		time.Sleep(time.Millisecond)
	}
	// 放行根进程退出的同时取消，同一进程组始终属于本次Run。
	if err := gate.Close(); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case got := <-done:
		defer func() { _ = syscall.Kill(-got.result.PID, syscall.SIGKILL) }()
		if !got.result.Started || !got.result.Settled || got.result.BackgroundProcessesRunning || !errors.Is(got.err, context.Canceled) {
			t.Fatalf("result=%+v err=%v", got.result, got.err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("canceled process did not settle")
	}
	deadline = time.Now().Add(time.Second)
	for !errors.Is(syscall.Kill(childPID, 0), syscall.ESRCH) {
		if time.Now().After(deadline) {
			t.Fatal("owned child survived cancellation")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestHostProcessNormalExitRetainsBackgroundProcess(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	input, gate, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer gate.Close()
	output, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type outcome struct {
		result Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := Run(ctx, Spec{Executable: exe, Args: []string{"-test.run=^TestHostProcessLifecycleHelper$"},
			Env: append(os.Environ(), "TOOLKIT_PROCESS_HELPER=root", "TOOLKIT_PROCESS_CHILD_READY="), Stdin: input, Stdout: output, Stderr: output})
		done <- outcome{result, err}
	}()
	deadline := time.Now().Add(3 * time.Second)
	var childPID int
	for childPID == 0 {
		data, err := os.ReadFile(output.Name())
		if err != nil {
			t.Fatal(err)
		}
		childPID, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		if time.Now().After(deadline) {
			t.Fatal("helper did not start its background child")
		}
		time.Sleep(time.Millisecond)
	}
	defer func() { _ = syscall.Kill(childPID, syscall.SIGKILL) }()
	if err := gate.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got.err != nil || !got.result.Started || !got.result.Settled || !got.result.BackgroundProcessesRunning || got.result.ExitCode != 0 {
			t.Fatalf("real background process was hidden: result=%+v err=%v", got.result, got.err)
		}
		if err := syscall.Kill(childPID, 0); err != nil {
			t.Fatalf("normal root exit terminated its child: %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("normal root did not exit")
	}
	if err := syscall.Kill(childPID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for !errors.Is(syscall.Kill(childPID, 0), syscall.ESRCH) {
		if time.Now().After(deadline) {
			t.Fatal("background child cleanup did not complete")
		}
		time.Sleep(time.Millisecond)
	}
}
