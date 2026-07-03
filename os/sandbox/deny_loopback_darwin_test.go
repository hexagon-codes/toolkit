//go:build darwin

package sandbox

import (
	"context"
	"strings"
	"testing"
)

// GO-1/GO-2 根修：DenyLoopback 在 Network=true 时禁本机回环、放行外网。
func TestDarwinDenyLoopbackProfile(t *testing.T) {
	s := &darwinSandbox{cfg: Config{Network: true, DenyLoopback: true, Workspace: t.TempDir()}}
	sbpl := s.generateSBPL()
	if !strings.Contains(sbpl, "(allow network*)") {
		t.Fatal("Network=true 应仍放行外网")
	}
	if !strings.Contains(sbpl, "(deny network-outbound (remote ip \"localhost:*\"))") {
		t.Fatalf("DenyLoopback=true 应含回环 deny 规则，profile=\n%s", sbpl)
	}
	// deny 必须在 allow 之后（seatbelt last-match-wins）
	if strings.Index(sbpl, "(deny network-outbound") < strings.Index(sbpl, "(allow network*)") {
		t.Fatal("deny 回环规则必须写在 allow network* 之后才生效")
	}

	// DenyLoopback=false 不加限制（向后兼容）
	s2 := &darwinSandbox{cfg: Config{Network: true, Workspace: t.TempDir()}}
	if strings.Contains(s2.generateSBPL(), "deny network-outbound (remote ip") {
		t.Fatal("DenyLoopback=false 不应加回环限制")
	}
}

// 真机 seatbelt 语法验证：profile 必须能被 sandbox-exec 接受（语法错会挂掉整个 code_exec）。
func TestDarwinDenyLoopbackSeatbeltSyntaxValid(t *testing.T) {
	s := &darwinSandbox{cfg: Config{Network: true, DenyLoopback: true, Timeout: 10, Workspace: t.TempDir()}}
	// 跑一个最小命令：seatbelt profile 语法无效时 sandbox-exec 直接非零退出报 profile 错。
	res, err := s.Exec(context.Background(), "/bin/echo", []string{"ok"})
	if err != nil {
		t.Fatalf("带 DenyLoopback 的 seatbelt profile 应语法有效可执行: %v", err)
	}
	if !strings.Contains(res.Stdout, "ok") {
		t.Fatalf("sandbox-exec 未正常执行，stdout=%q stderr=%q", res.Stdout, res.Stderr)
	}
}
