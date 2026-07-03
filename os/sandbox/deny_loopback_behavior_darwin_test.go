//go:build darwin

package sandbox

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// GO-1/GO-2 行为验证（真机 seatbelt）：DenyLoopback 时沙箱内代码访问本机
// 监听端口必须失败——证明 loopback 真被内核拦住，而非仅 profile 字符串正确。
func TestDarwinDenyLoopbackActuallyBlocksLocalhost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "REACHED-LOCAL-SERVER")
	}))
	defer srv.Close()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	url := "http://127.0.0.1:" + port

	code := "curl -s --max-time 3 " + url + " || echo BLOCKED"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// DenyLoopback=true → 访问本机应被拦（输出不含 REACHED）
	denied := &darwinSandbox{cfg: Config{Network: true, DenyLoopback: true, Timeout: 10, Workspace: t.TempDir()}}
	res, err := denied.Exec(ctx, "/bin/sh", []string{"-c", code})
	if err != nil {
		t.Fatalf("sandbox exec: %v", err)
	}
	if strings.Contains(res.Stdout, "REACHED-LOCAL-SERVER") {
		t.Fatalf("DenyLoopback=true 沙箱竟连通了本机服务（loopback 未被拦）: %q", res.Stdout)
	}

	// 对照：DenyLoopback=false 时能连通（证明拦截确由该开关造成，非环境噪声）
	open := &darwinSandbox{cfg: Config{Network: true, Timeout: 10, Workspace: t.TempDir()}}
	res2, err := open.Exec(ctx, "/bin/sh", []string{"-c", code})
	if err != nil {
		t.Fatalf("sandbox exec (open): %v", err)
	}
	if !strings.Contains(res2.Stdout, "REACHED-LOCAL-SERVER") {
		t.Skipf("[env] 对照组未连通本机（curl 缺失/环境限制），无法证明拦截归因；denied 组已拦截 stdout=%q", res2.Stdout)
	}
}
