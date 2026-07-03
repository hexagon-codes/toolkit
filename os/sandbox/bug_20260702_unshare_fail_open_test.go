package sandbox

import (
	"errors"
	"testing"
)

// bug-20260702: linux unshare 兜底 fail-open + 注释撒谎。
//
// sandbox_linux.go 旧注释宣称 unshare 后端「较弱但仍 fail-closed」, 但 unshareArgs
// 用 user+mount namespace 却不 pivot_root, 宿主完整挂载视图对载荷仍可见/可写,
// 策略脚本仅掩蔽 DeniedPaths——属 allow-first / fail-open 的弱隔离。叠加上层 project
// 模式默认无 secrets DeniedPaths 时, 无人值守载荷可读 ~/.ssh 等。
//
// 后端选择决策已抽成纯函数 selectLinuxSandboxBackend(与 linuxSandboxRunner 当前
// 行为等价的可测提取), 可在任意平台单测, 无需 linux。RED 断言: 机密配置
// (confidential)下仅剩 unshare 弱兜底时必须 fail-closed 拒绝——旧行为(静默降级到
// unshare)下本断言 FAIL。
func TestBug20260702_UnshareBackendFailsClosedForConfidentialConfig(t *testing.T) {
	// bwrap 缺席、仅 unshare 可用、配置有机密性要求(DeniedPaths 非空 → confidential=true)
	backend, containment, err := selectLinuxSandboxBackend(false, true, true)

	if err == nil {
		t.Fatalf("机密配置下仅剩 unshare 弱兜底必须 fail-closed 拒绝, 而非静默降级(got backend=%d, containment=%q, err=nil)", backend, containment)
	}
	if !errors.Is(err, ErrFilesystemContainmentUnavailable) {
		t.Fatalf("fail-closed 错误必须命中 ErrFilesystemContainmentUnavailable 哨兵, got: %v", err)
	}
	if backend != linuxBackendNone {
		t.Fatalf("fail-closed 时不得返回任何可执行后端, got backend=%d", backend)
	}
}

// 正向契约: 无机密性要求时用 unshare, 但文件系统隔离如实标 weak(降级信号交上层)。
func TestBug20260702_UnshareBackendReportsWeakContainment(t *testing.T) {
	backend, containment, err := selectLinuxSandboxBackend(false, true, false)
	if err != nil {
		t.Fatalf("非机密配置下 unshare 兜底应允许执行, got err=%v", err)
	}
	if backend != linuxBackendUnshare {
		t.Fatalf("应选用 unshare 兜底, got backend=%d", backend)
	}
	if containment != LimitStatusWeak {
		t.Fatalf("unshare 非 deny-by-default, Filesystem 必须标 weak(不许假装 enforced), got %q", containment)
	}
}

// 正向契约: bubblewrap 可用时用它, 文件系统隔离 enforced, 机密配置也不受影响。
func TestBug20260702_BwrapBackendReportsEnforcedContainment(t *testing.T) {
	for _, confidential := range []bool{false, true} {
		backend, containment, err := selectLinuxSandboxBackend(true, true, confidential)
		if err != nil {
			t.Fatalf("bwrap 可用时不应拒绝(confidential=%v), got err=%v", confidential, err)
		}
		if backend != linuxBackendBwrap {
			t.Fatalf("bwrap 可用时应优先选用, got backend=%d", backend)
		}
		if containment != LimitStatusEnforced {
			t.Fatalf("bubblewrap 提供 deny-by-default 强隔离, Filesystem 应为 enforced, got %q", containment)
		}
	}
}

// 边界: 两个后端都不可用时拒绝执行, 且不误报为 containment 问题。
func TestBug20260702_NoBackendAvailableRefuses(t *testing.T) {
	backend, containment, err := selectLinuxSandboxBackend(false, false, false)
	if err == nil {
		t.Fatalf("无任何隔离后端时必须拒绝执行")
	}
	if errors.Is(err, ErrFilesystemContainmentUnavailable) {
		t.Fatalf("「后端缺失」不同于「有弱后端但机密拒载」, 不应命中 containment 哨兵: %v", err)
	}
	if backend != linuxBackendNone {
		t.Fatalf("无后端时不得返回可执行后端, got backend=%d", backend)
	}
	if containment != LimitStatusUnsupported {
		t.Fatalf("无 OS 隔离后端 → Filesystem=unsupported, got %q", containment)
	}
}
