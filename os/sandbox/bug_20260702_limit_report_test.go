package sandbox

import (
	"context"
	"runtime"
	"testing"
)

// bug-20260702 正向契约: 各平台 ExecResult.Limits 的限额报告必须与实际行为一致。
//
// 验收标准(与 hexclaw 侧消费契约钉死):
//   - darwin: Memory=unsupported(内核拒绝下调内存 rlimit), Processes=enforced;
//   - linux: Memory=enforced, Processes=enforced;
//   - windows: Job Object 真实执行 → Memory/Processes 均 enforced;
//   - 所有平台: Storage(walk 检查)/Output(有界缓冲)恒 enforced。
func TestBug20260702_LimitReportMatchesPlatformCapability(t *testing.T) {
	sb, err := New(Config{Workspace: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	command, args := "/bin/sh", []string{"-c", "true"}
	if runtime.GOOS == "windows" {
		command, args = "cmd", []string{"/c", "exit 0"}
	}
	res, err := sb.Exec(context.Background(), command, args)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}

	wantMemory := LimitStatusEnforced
	if runtime.GOOS == "darwin" {
		wantMemory = LimitStatusUnsupported
	}
	if res.Limits.Memory != wantMemory {
		t.Errorf("Limits.Memory on %s = %q, want %q", runtime.GOOS, res.Limits.Memory, wantMemory)
	}
	if res.Limits.Processes != LimitStatusEnforced {
		t.Errorf("Limits.Processes on %s = %q, want enforced", runtime.GOOS, res.Limits.Processes)
	}
	if res.Limits.Storage != LimitStatusEnforced {
		t.Errorf("Limits.Storage = %q, want enforced (walk 检查恒生效)", res.Limits.Storage)
	}
	if res.Limits.Output != LimitStatusEnforced {
		t.Errorf("Limits.Output = %q, want enforced (有界缓冲恒生效)", res.Limits.Output)
	}
}

// 支持的平台上限额报告与实际行为一致: Output 报 enforced 时, 超限输出必须真被截断。
func TestBug20260702_LimitReportOutputEnforcedMatchesBehavior(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows 无 /bin/sh, 输出截断行为由 POSIX 平台验证")
	}
	sb, err := New(Config{
		Workspace:      t.TempDir(),
		MaxOutputBytes: 16,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res, err := sb.Exec(context.Background(), "/bin/sh", []string{"-c", "printf '%0.s0' $(seq 1 4096)"})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Limits.Output != LimitStatusEnforced {
		t.Fatalf("Limits.Output = %q, want enforced", res.Limits.Output)
	}
	if !res.StdoutTruncated {
		t.Fatalf("报告 Output=enforced 但 4096 字节输出未被 16 字节上限截断, 报告与行为不一致")
	}
	if int64(len(res.Stdout)) > 16 {
		t.Fatalf("stdout 保留长度 %d 超过 MaxOutputBytes=16", len(res.Stdout))
	}
}
