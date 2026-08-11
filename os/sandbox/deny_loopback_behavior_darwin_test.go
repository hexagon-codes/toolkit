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

// host/disabled 网络模式必须由 Seatbelt 真实执行，不能只验证 profile 字符串。
func TestDarwinNetworkModesControlLocalhost(t *testing.T) {
	python := requireSandboxTools(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "REACHED-LOCAL-SERVER")
	}))
	defer srv.Close()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	code := fmt.Sprintf(`
import socket
try:
    connection = socket.create_connection(("127.0.0.1", %s), timeout=3)
    connection.close()
    print("CONNECTED")
except OSError:
    print("BLOCKED")
`, port)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	denied := newDarwinSandbox(Config{Network: NetworkDisabled, Timeout: 10, Workspace: t.TempDir()})
	res, err := denied.Exec(ctx, Command{Path: python, Args: []string{"-c", code}})
	if err != nil {
		t.Fatalf("denied Exec() error = %v", err)
	}
	if got := strings.TrimSpace(res.Stdout); res.ExitCode != 0 || got != "BLOCKED" {
		t.Fatalf("NetworkDisabled result: exit=%d stdout=%q stderr=%q", res.ExitCode, res.Stdout, res.Stderr)
	}

	open := newDarwinSandbox(Config{Network: NetworkHost, Timeout: 10, Workspace: t.TempDir()})
	res2, err := open.Exec(ctx, Command{Path: python, Args: []string{"-c", code}})
	if err != nil {
		t.Fatalf("open Exec() error = %v", err)
	}
	if got := strings.TrimSpace(res2.Stdout); res2.ExitCode != 0 || got != "CONNECTED" {
		t.Fatalf("NetworkHost result: exit=%d stdout=%q stderr=%q", res2.ExitCode, res2.Stdout, res2.Stderr)
	}
}
