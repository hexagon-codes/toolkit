//go:build !windows

package sandbox

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// bug-20260702: 存储限额违规的错误通道混叠。
//
// 现状: enforceSandboxStorageLimits 违规时返回普通 error, 与「后端故障」不可区分;
// linux 侧 Exec 把任何 runBoundedCommand 错误统一包装成 "sandbox unavailable" 并丢弃
// ExecResult —— 一次成功执行仅因产物超限被误报为「沙箱不可用」, stdout/stderr 全丢。
//
// 契约: 违规错误必须命中 ErrStorageLimitExceeded 哨兵; 后置违规必须连同 ExecResult
// 一起返回; 各平台不得把哨兵错误再包装成 backend failed/unavailable。
//
// RED 阶段(旧代码): 违规路径未挂哨兵 → errors.Is 不命中 → FAIL。
func TestBug20260702_StorageLimitViolationCarriesSentinelAndResult(t *testing.T) {
	ws := t.TempDir()
	// 执行本身成功(exit 0), 但产物把工作区撑过 MaxWorkspaceBytes → 后置违规
	res, execErr := runBoundedCommand(
		context.Background(),
		Command{
			Path: "/bin/sh",
			Args: []string{"-c", "printf 0123456789 > out.bin"},
			Dir:  ws,
			Env:  os.Environ(),
		},
		Config{
			Workspace:         ws,
			MaxWorkspaceBytes: 5,
			MaxOutputBytes:    1024,
			MaxStderrBytes:    1024,
		},
		posixExecutionCapabilities{},
	)
	if execErr == nil {
		t.Fatalf("artifact limit violation returned nil error: result=%+v", res)
	}
	if !errors.Is(execErr, ErrStorageLimitExceeded) {
		skipIfSandboxBackendUnavailable(t, execErr)
	}
	if !errors.Is(execErr, ErrStorageLimitExceeded) {
		t.Fatalf("storage limit error = %v, want ErrStorageLimitExceeded", execErr)
	}
	if strings.Contains(execErr.Error(), "sandbox unavailable") {
		t.Fatalf("storage limit violation was misreported as sandbox unavailable: %v", execErr)
	}
	if res == nil {
		t.Fatal("post-execution storage violation returned a nil ExecResult")
		return
	}
	if res.ExitCode != 0 {
		t.Fatalf("payload exit code = %d, want 0; stderr=%q", res.ExitCode, res.Stderr)
	}
}

// 前置违规: 执行前工作区已超限 → (nil, err) 形态, 但错误同样携带哨兵。
func TestBug20260702_StorageLimitPreCheckCarriesSentinel(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "big.bin"), []byte("0123456789"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	err := enforceSandboxStorageLimits(Config{
		Workspace:         ws,
		MaxWorkspaceBytes: 5,
	})
	if err == nil {
		t.Fatal("pre-execution storage violation returned nil error")
	}
	if !errors.Is(err, ErrStorageLimitExceeded) {
		t.Fatalf("pre-execution storage error = %v, want ErrStorageLimitExceeded", err)
	}
}

// bug-20260702：walk 过程中条目因并发执行而消失时不应判为检查失败。
//
// RED 阶段(旧代码): d.Info() 的 ErrNotExist 被当成检查失败上抛 → FAIL。
func TestBug20260702_StorageWalkToleratesConcurrentFileRemoval(t *testing.T) {
	ws := t.TempDir()
	trigger := filepath.Join(ws, "a_trigger.bin")
	victim := filepath.Join(ws, "z_victim.bin")
	for _, p := range []string{trigger, victim} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", p, err)
		}
	}

	// WalkDir 按字典序访问: 访问 a_trigger 时删掉尚未访问的 z_victim,
	// 复现「读目录项之后、lstat 之前文件被并发删除」的竞态窗口。
	storageWalkVisitHook = func(path string) {
		if filepath.Base(path) == "a_trigger.bin" {
			_ = os.Remove(victim)
		}
	}
	defer func() { storageWalkVisitHook = nil }()

	err := enforceSandboxStorageLimits(Config{
		Workspace:         ws,
		MaxWorkspaceBytes: 100,
		MaxArtifactBytes:  100,
	})
	if err != nil {
		t.Fatalf("disappearing file during storage walk returned an error: %v", err)
	}
}

func TestTimeoutPreservesStorageAndProcessWaitErrors(t *testing.T) {
	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := runBoundedCommand(
		ctx,
		Command{
			Path: "/bin/sh",
			Args: []string{"-c", "printf 0123456789 > out.bin; sleep 5"},
			Dir:  workspace,
			Env:  os.Environ(),
		},
		Config{
			Workspace:         workspace,
			MaxWorkspaceBytes: 5,
			MaxOutputBytes:    1024,
			MaxStderrBytes:    1024,
		},
		posixExecutionCapabilities{
			Filesystem:         LimitStatusUnsupported,
			ProcessContainment: LimitStatusUnsupported,
		},
	)
	if result == nil {
		t.Fatal("runBoundedCommand() result = nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrStorageLimitExceeded) {
		t.Fatalf("runBoundedCommand() error = %v, want timeout and storage errors", err)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runBoundedCommand() error = %v, want process wait error", err)
	}
}
