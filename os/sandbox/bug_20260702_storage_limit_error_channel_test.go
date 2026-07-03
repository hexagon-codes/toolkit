//go:build !windows

package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	sb, err := New(Config{
		Workspace:         ws,
		Network:           true,
		MaxWorkspaceBytes: 5, // 执行会写 10 字节 → 后置检查违规
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// 执行本身成功(exit 0), 但产物把工作区撑过 MaxWorkspaceBytes → 后置违规
	res, execErr := sb.Exec(context.Background(), "/bin/sh", []string{"-c", "printf 0123456789 > out.bin"})
	if execErr == nil {
		t.Fatalf("产物超限应返回错误, got nil (res=%+v)", res)
	}
	if !errors.Is(execErr, ErrStorageLimitExceeded) {
		t.Fatalf("存储限额违规必须命中 ErrStorageLimitExceeded 哨兵, got: %v", execErr)
	}
	if strings.Contains(execErr.Error(), "sandbox unavailable") {
		t.Fatalf("存储限额违规不是后端故障, 不得误报 sandbox unavailable: %v", execErr)
	}
	if res == nil {
		t.Fatalf("后置违规必须保留并返回 ExecResult((res, err) 形态), got nil")
	}
	if res.ExitCode != 0 {
		t.Fatalf("执行本身应成功(exit 0), got %d, stderr=%q", res.ExitCode, res.Stderr)
	}
}

// 前置违规: 执行前工作区已超限 → (nil, err) 形态, 但错误同样携带哨兵。
func TestBug20260702_StorageLimitPreCheckCarriesSentinel(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "big.bin"), []byte("0123456789"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	sb, err := New(Config{
		Workspace:         ws,
		MaxWorkspaceBytes: 5,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res, execErr := sb.Exec(context.Background(), "/bin/sh", []string{"-c", "true"})
	if execErr == nil {
		t.Fatalf("前置超限应返回错误, got nil")
	}
	if !errors.Is(execErr, ErrStorageLimitExceeded) {
		t.Fatalf("前置违规错误必须命中 ErrStorageLimitExceeded 哨兵, got: %v", execErr)
	}
	if res != nil {
		t.Fatalf("前置违规未执行任何命令, 应保持 (nil, err) 形态, got %+v", res)
	}
}

// bug-20260702: walk 过程中文件消失(并发 ExecCode 的 defer os.Remove 竞态)不该判为检查失败。
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
		t.Fatalf("walk 期间文件消失应跳过继续, 不该判为检查失败: %v", err)
	}
}
