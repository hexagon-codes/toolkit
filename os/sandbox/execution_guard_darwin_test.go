//go:build darwin

package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwinExecUsesSingleCanonicalCommandPlanAcrossSymlinkSwap(t *testing.T) {
	requireSandboxExec(t)
	workspace := t.TempDir()
	commandA := filepath.Join(workspace, "command-a")
	commandB := filepath.Join(workspace, "command-b")
	markerA := filepath.Join(workspace, "marker-a")
	markerB := filepath.Join(workspace, "marker-b")
	if err := os.WriteFile(commandA, []byte("#!/bin/sh\nprintf a > marker-a\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(commandB, []byte("#!/bin/sh\nprintf b > marker-b\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(workspace, "command")
	if err := os.Symlink(commandA, alias); err != nil {
		t.Fatal(err)
	}

	sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: workspace, Timeout: 20})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := sandboxInstance.Close(); closeErr != nil {
			t.Errorf("Close() error = %v", closeErr)
		}
	})
	backend := unwrapDarwinSandbox(t, sandboxInstance)
	planned := make(chan struct{})
	release := make(chan struct{})
	defer closeDarwinTestBarrier(release)
	backend.afterCommandPlan = func() {
		close(planned)
		<-release
	}

	type outcome struct {
		result *ExecResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, execErr := sandboxInstance.Exec(context.Background(), Command{Path: alias})
		done <- outcome{result: result, err: execErr}
	}()
	<-planned
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(commandB, alias); err != nil {
		t.Fatal(err)
	}
	closeDarwinTestBarrier(release)
	got := <-done
	if got.err != nil {
		t.Fatalf("Exec() error = %v; result=%+v", got.err, got.result)
	}
	if content, err := os.ReadFile(markerA); err != nil || string(content) != "a" {
		t.Fatalf("planned command marker = %q, %v", content, err)
	}
	if _, err := os.Stat(markerB); !os.IsNotExist(err) {
		t.Fatalf("swapped command executed: %v", err)
	}
}

func TestDarwinExecRejectsCanonicalTargetReplacementAfterPlanFreeze(t *testing.T) {
	requireSandboxExec(t)
	workspace := t.TempDir()
	commandPath := filepath.Join(workspace, "command-a")
	markerOriginal := filepath.Join(workspace, "marker-original")
	markerReplacement := filepath.Join(workspace, "marker-replacement")
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nprintf original > marker-original\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(workspace, "command")
	if err := os.Symlink(commandPath, alias); err != nil {
		t.Fatal(err)
	}
	sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: workspace, Timeout: 20})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := sandboxInstance.Close(); closeErr != nil {
			t.Errorf("Close() error = %v", closeErr)
		}
	})
	backend := unwrapDarwinSandbox(t, sandboxInstance)
	planned := make(chan struct{})
	release := make(chan struct{})
	defer closeDarwinTestBarrier(release)
	backend.afterCommandPlan = func() {
		close(planned)
		<-release
	}

	done := make(chan error, 1)
	go func() {
		_, execErr := sandboxInstance.Exec(context.Background(), Command{Path: alias})
		done <- execErr
	}()
	<-planned
	if err := os.Rename(commandPath, commandPath+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nprintf replacement > marker-replacement\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	closeDarwinTestBarrier(release)
	execErr := <-done
	if execErr == nil || !strings.Contains(execErr.Error(), "identity changed") {
		t.Fatalf("Exec() error = %v, want frozen identity rejection", execErr)
	}
	for _, marker := range []string{markerOriginal, markerReplacement} {
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("rejected command created marker %q: %v", marker, err)
		}
	}
}

func closeDarwinTestBarrier(barrier chan struct{}) {
	select {
	case <-barrier:
	default:
		close(barrier)
	}
}

func TestDarwinExecutionGuardRejectsWorkspaceHardlinkBeforePayload(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(root, "sentinel")
	if err := os.WriteFile(sentinel, []byte("protected"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(sentinel, filepath.Join(workspace, "host-link")); err != nil {
		t.Fatal(err)
	}
	sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: workspace, Timeout: 20})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sandboxInstance.Exec(context.Background(), Command{
		Path: requireDarwinRuntime(t, "python3"),
		Args: []string{"-c", `
from pathlib import Path
Path("host-link").write_text("escaped")
Path("payload-ran").write_text("ran")
`},
	})
	if err == nil || !strings.Contains(err.Error(), "multiple hard links") {
		t.Fatalf("Exec() error = %v, want hardlink rejection", err)
	}
	content, readErr := os.ReadFile(sentinel)
	if readErr != nil || string(content) != "protected" {
		t.Fatalf("sentinel = %q, %v", content, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(workspace, "payload-ran")); !os.IsNotExist(statErr) {
		t.Fatalf("payload ran before hardlink rejection: %v", statErr)
	}
}

func TestDarwinExecutionGuardRejectsWorkspacePathAndDirectoryReplacement(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, workspace, commandPath, commandDir string)
	}{
		{
			name: "workspace",
			mutate: func(t *testing.T, workspace, _, _ string) {
				replacement := workspace + "-replacement"
				if err := os.Mkdir(replacement, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(workspace, workspace+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, workspace); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "command path",
			mutate: func(t *testing.T, _, commandPath, _ string) {
				replacement := commandPath + "-replacement"
				if err := os.WriteFile(replacement, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, commandPath); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "command directory",
			mutate: func(t *testing.T, _, _, commandDir string) {
				replacement := commandDir + "-replacement"
				if err := os.Mkdir(replacement, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(commandDir, commandDir+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, commandDir); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := filepath.Join(t.TempDir(), "workspace")
			commandDir := filepath.Join(workspace, "project")
			if err := os.MkdirAll(commandDir, 0o700); err != nil {
				t.Fatal(err)
			}
			commandPath := filepath.Join(workspace, "payload")
			if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			guard, err := newDarwinExecutionGuard(workspace, Command{Path: commandPath, Dir: commandDir})
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, workspace, commandPath, commandDir)
			if err := guard.Revalidate(); err == nil || !strings.Contains(err.Error(), "identity changed") {
				t.Fatalf("Revalidate() error = %v, want identity change rejection", err)
			}
		})
	}
}

func TestDarwinNoChildCapabilitiesAreEnforced(t *testing.T) {
	sandboxInstance, err := newDarwinTestSandbox(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	reporter, ok := sandboxInstance.(CapabilityReporter)
	if !ok {
		t.Fatalf("required-capability sandbox = %T, want CapabilityReporter", sandboxInstance)
	}
	available, err := reporter.Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !available.Has(CapabilityProcessContainment) {
		t.Errorf("macOS no-child capabilities = %s, want process-containment", available)
	}
	if available.Has(CapabilityProcesses) {
		t.Errorf("macOS no-child capabilities = %s, must not report an unrequested process limit", available)
	}
	if available.Has(CapabilityMemory) || available.Has(CapabilityStorage) {
		t.Fatalf("macOS capabilities overstate unsupported limits: %s", available)
	}
	result, err := sandboxInstance.Exec(context.Background(), Command{Path: "/usr/bin/true"})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.Limits.Processes != LimitStatusNotRequested || result.Limits.ProcessContainment != LimitStatusEnforced {
		t.Fatalf("macOS no-child limit report = %+v", result.Limits)
	}
}
