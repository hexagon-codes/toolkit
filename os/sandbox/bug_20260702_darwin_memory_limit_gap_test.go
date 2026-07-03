//go:build darwin

package sandbox

import (
	"context"
	"testing"
)

// bug-20260702: darwin 内存限额静默失效——「假装已执行」。
//
// darwin 内核对 RLIMIT_AS/DATA/RSS 的现实量级下调一律返回 EINVAL,
// 旧实现在三连失败后 return nil, 把能力缺口吞成「已生效」, 违反 Config
// 注释钉死的契约(不支持的平台限制必须作为能力缺口显式上报, 不许静默假装)。
//
// RED 阶段(旧代码)本测试的前身断言 setPosixMemoryRlimit 在 darwin 必须返回
// 可识别的 gap 信号(旧签名 return nil → FAIL); 修复后签名带 LimitStatus,
// 本最终形态断言: 能力缺口不算错误(不阻断执行), 但状态必须是 unsupported。
func TestBug20260702_DarwinMemoryLimitGapMustBeObservable(t *testing.T) {
	// 256MB 即 New() 的默认 MaxMemoryBytes; darwin 上三资源均 EINVAL, 不会真改动测试进程的 rlimit
	status, err := setPosixMemoryRlimit(256 * 1024 * 1024)
	if err != nil {
		t.Fatalf("能力缺口不该以错误形态阻断执行, got err=%v", err)
	}
	if status != LimitStatusUnsupported {
		t.Fatalf("darwin 上内存 rlimit 三连 EINVAL 必须上报 unsupported(不许假装已执行), got %q", status)
	}
}

// darwin 真机执行链: ExecResult.Limits 必须显式上报「内存限额 unsupported、
// 其余项 enforced」, 供 hexclaw 侧按同一契约消费。
func TestBug20260702_DarwinExecResultReportsMemoryUnsupported(t *testing.T) {
	sb, err := New(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res, err := sb.Exec(context.Background(), "/bin/sh", []string{"-c", "true"})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Limits.Memory != LimitStatusUnsupported {
		t.Fatalf("darwin ExecResult.Limits.Memory 必须是 unsupported, got %q", res.Limits.Memory)
	}
	if res.Limits.Processes != LimitStatusEnforced {
		t.Fatalf("darwin 上 RLIMIT_NPROC 可设置, Limits.Processes 应为 enforced, got %q", res.Limits.Processes)
	}
	if res.Limits.Storage != LimitStatusEnforced {
		t.Fatalf("Storage walk 检查恒生效, got %q", res.Limits.Storage)
	}
	if res.Limits.Output != LimitStatusEnforced {
		t.Fatalf("stdout/stderr 有界缓冲恒生效, got %q", res.Limits.Output)
	}
}
