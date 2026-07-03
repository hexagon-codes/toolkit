//go:build linux

package sandbox

import (
	"errors"
	"os/exec"
	"testing"
)

// bug-20260702 linux 集成: 校验真实 linuxSandboxRunner 把机密配置的 fail-closed 决策
// 落到实处(在 CI linux 上跑; 本地 darwin 由纯函数单测覆盖)。
//
// 前置: bubblewrap 必须缺席才能触发弱兜底路径; CI 装了 bwrap 的常态下本测试跳过,
// 由 selectLinuxSandboxBackend 纯函数单测保证决策正确性。
func TestBug20260702_LinuxRunnerFailsClosedWhenOnlyUnshareAndConfidential(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err == nil {
		t.Skip("bubblewrap 可用, 强隔离路径生效, 无需触发弱兜底 fail-closed")
	}
	unshare, err := exec.LookPath("unshare")
	if err != nil {
		t.Skip("unshare 亦不可用, 无弱兜底路径可测")
	}
	if !linuxUnshareBackendUsable(unshare) {
		t.Skip("unshare 二进制存在但当前 runner 禁止 user namespace, 无弱兜底路径可测")
	}

	s := &linuxSandbox{cfg: Config{
		Workspace:   t.TempDir(),
		DeniedPaths: []string{"/etc/shadow"}, // 机密性要求 → confidential=true
	}}

	_, _, _, containment, err := s.linuxSandboxRunner("true", nil)
	if err == nil {
		t.Fatalf("机密配置下仅剩 unshare 弱兜底, linuxSandboxRunner 必须 fail-closed 拒绝")
	}
	if !errors.Is(err, ErrFilesystemContainmentUnavailable) {
		t.Fatalf("fail-closed 错误必须命中 ErrFilesystemContainmentUnavailable, got: %v", err)
	}
	if containment != LimitStatusWeak {
		t.Fatalf("拒绝时应报告弱隔离降级信号 weak, got %q", containment)
	}
}

// 无机密要求时弱兜底仍可执行, 且 ExecResult.Limits.Filesystem 报 weak。
func TestBug20260702_LinuxUnshareExecReportsWeakFilesystem(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err == nil {
		t.Skip("bubblewrap 可用, 走 enforced 强隔离路径")
	}
	unshare, err := exec.LookPath("unshare")
	if err != nil {
		t.Skip("unshare 不可用")
	}
	if !linuxUnshareBackendUsable(unshare) {
		t.Skip("unshare 二进制存在但当前 runner 禁止 user namespace")
	}

	s := &linuxSandbox{cfg: Config{Workspace: t.TempDir()}} // 无 DeniedPaths → 非机密
	_, _, _, containment, err := s.linuxSandboxRunner("true", nil)
	if err != nil {
		t.Fatalf("非机密配置下 unshare 兜底应允许执行, got err=%v", err)
	}
	if containment != LimitStatusWeak {
		t.Fatalf("unshare 兜底文件系统隔离必须标 weak, got %q", containment)
	}
}
