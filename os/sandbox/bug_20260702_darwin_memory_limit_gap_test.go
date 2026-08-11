//go:build darwin

package sandbox

import (
	"context"
	"testing"

	"golang.org/x/sys/unix"
)

// bug-20260702: darwin 内存限额静默失效——「假装已执行」。
//
// darwin 内核对 RLIMIT_AS/DATA/RSS 的现实量级下调一律返回 EINVAL,
// 旧实现在三连失败后 return nil, 把能力缺口吞成「已生效」, 违反 Config
// 注释钉死的契约(不支持的平台限制必须作为能力缺口显式上报, 不许静默假装)。
//
// 能力探测只能在受控辅助子进程内发生，不能瞬时下调宿主测试进程的限制。
func TestBug20260702_DarwinMemoryLimitGapMustBeObservable(t *testing.T) {
	resources := []int{unix.RLIMIT_AS, unix.RLIMIT_DATA, unix.RLIMIT_RSS}
	before := make([]unix.Rlimit, len(resources))
	for index, resource := range resources {
		if err := unix.Getrlimit(resource, &before[index]); err != nil {
			t.Fatal(err)
		}
	}
	status, err := probePosixMemoryLimitCapabilityContext(t.Context())
	if err != nil {
		t.Fatalf("capability gap must not block execution as an error: %v", err)
	}
	if status != LimitStatusUnsupported {
		t.Fatalf("Darwin memory rlimit status = %q, want unsupported", status)
	}
	for index, resource := range resources {
		var after unix.Rlimit
		if err := unix.Getrlimit(resource, &after); err != nil {
			t.Fatal(err)
		}
		if after != before[index] {
			t.Fatalf("host rlimit %d changed during child probe: before=%+v after=%+v", resource, before[index], after)
		}
	}
}

// darwin 真机执行链必须把未请求的可选配额与平台能力分开报告。
func TestBug20260702_DarwinExecResultReportsOptionalLimitsNotRequested(t *testing.T) {
	sb, err := newDarwinTestSandbox(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res, err := sb.Exec(context.Background(), Command{Path: "/usr/bin/true"})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Limits.Memory != LimitStatusNotRequested {
		t.Fatalf("Darwin memory limit status = %q, want not_requested", res.Limits.Memory)
	}
	if res.Limits.Processes != LimitStatusNotRequested {
		t.Fatalf("Darwin process limit status = %q, want not_requested", res.Limits.Processes)
	}
	if res.Limits.ProcessContainment != LimitStatusEnforced {
		t.Fatalf("Darwin process containment status = %q, want enforced", res.Limits.ProcessContainment)
	}
	if res.Limits.Storage != LimitStatusNotRequested {
		t.Fatalf("Darwin storage limit status = %q, want not_requested", res.Limits.Storage)
	}
	if res.Limits.Output != LimitStatusEnforced {
		t.Fatalf("Darwin output limit status = %q, want enforced", res.Limits.Output)
	}
}
