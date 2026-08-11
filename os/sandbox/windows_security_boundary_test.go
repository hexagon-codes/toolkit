//go:build windows

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

type windowsExecOutcome struct {
	result *ExecResult
	err    error
}

func TestWindows_NetworkDisabledSocketMatrix(t *testing.T) {
	tests := []struct {
		name    string
		network string
		listen  func() (string, func(), error)
	}{
		{name: "TCP IPv4 loopback", network: "tcp4", listen: windowsTCP4Fixture},
		{name: "TCP IPv6 loopback", network: "tcp6", listen: windowsTCP6Fixture},
		{name: "UDP IPv4 loopback", network: "udp4", listen: windowsUDP4Fixture},
		{name: "UDP IPv6 loopback", network: "udp6", listen: windowsUDP6Fixture},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			address, closeFixture, err := test.listen()
			if err != nil {
				t.Fatalf("create %s network fixture: %v", test.network, err)
			}
			t.Cleanup(closeFixture)

			workspacePath := t.TempDir()
			sandboxValue := newWindowsTestSandbox(t, Config{
				Workspace:            workspacePath,
				Timeout:              10,
				Network:              NetworkDisabled,
				RequiredCapabilities: UntrustedCodeIsolationCapabilities,
			})
			payloadPath := installWindowsTestPayload(t, workspacePath, "sandbox-network-disabled-payload.exe")
			result, execErr := sandboxValue.Exec(context.Background(), Command{
				Path: payloadPath,
				Args: []string{
					"-test.run=^TestWindows_NetworkDisabledPayload$",
					"-test.count=1",
					"--",
					test.network,
					address,
				},
			})
			if execErr != nil {
				t.Fatalf("execute %s NetworkDisabled payload: %v", test.network, execErr)
			}
			if result == nil || result.ExitCode != 0 {
				t.Fatalf("%s NetworkDisabled result = %+v, want exit code 0", test.network, result)
			}
			want := "NETWORK_DENIED:" + test.network + ":10013"
			if !strings.Contains(result.Stdout, want) {
				t.Fatalf("%s network denial = %q, want %q; stderr=%q", test.network, result.Stdout, want, result.Stderr)
			}
			if strings.Contains(result.Stdout, "NETWORK_ALLOWED") || result.Limits.Network != LimitStatusEnforced {
				t.Fatalf("%s network boundary was not enforced: result=%+v", test.network, result)
			}
		})
	}
}

func TestWindows_NetworkDisabledPayload(t *testing.T) {
	if !strings.EqualFold(filepath.Base(os.Args[0]), "sandbox-network-disabled-payload.exe") {
		return
	}
	arguments := argumentsAfterDoubleDash(os.Args)
	if len(arguments) != 2 {
		fmt.Print("NETWORK_FIXTURE_INVALID")
		return
	}
	networkName, address := arguments[0], arguments[1]
	connection, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(context.Background(), networkName, address)
	if err != nil {
		printWindowsNetworkResult(networkName, err)
		return
	}
	defer connection.Close()
	if strings.HasPrefix(networkName, "udp") {
		if err := connection.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
			fmt.Printf("NETWORK_FIXTURE_ERROR:%s:set-deadline:%v", networkName, err)
			return
		}
		if _, err := connection.Write([]byte("network-disabled-probe")); err != nil {
			printWindowsNetworkResult(networkName, err)
			return
		}
	}
	fmt.Printf("NETWORK_ALLOWED:%s", networkName)
}

func TestWindowsNetworkHostRejectedBeforeExecution(t *testing.T) {
	workspacePath := t.TempDir()
	markerPath := filepath.Join(workspacePath, "network-host-executed")
	_, err := New(Config{
		Workspace:            workspacePath,
		Network:              NetworkHost,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities,
	})
	if !errors.Is(err, ErrUnsupportedNetworkPolicy) {
		t.Fatalf("NetworkHost error = %v, want ErrUnsupportedNetworkPolicy", err)
	}
	if _, statErr := os.Stat(markerPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("NetworkHost rejection observed an unexpected execution marker: %v", statErr)
	}
}

func TestWindows_ContextCanceledTerminatesProcessContainment(t *testing.T) {
	workspacePath := t.TempDir()
	sandboxValue := newWindowsTestSandbox(t, Config{
		Workspace:            workspacePath,
		Timeout:              30,
		MaxProcesses:         8,
		RequiredCapabilities: UntrustedCodeIsolationCapabilities | CapabilityProcesses,
	})
	payloadPath := installWindowsTestPayload(t, workspacePath, "sandbox-context-cancel-payload.exe")
	pidsPath := filepath.Join(workspacePath, "cancel-process-containment.pids")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan windowsExecOutcome, 1)
	go func() {
		result, execErr := sandboxValue.Exec(ctx, Command{
			Path: payloadPath,
			Args: []string{
				"-test.run=^TestWindows_ContextCanceledRootPayload$",
				"-test.count=1",
				"--",
				pidsPath,
			},
		})
		done <- windowsExecOutcome{result: result, err: execErr}
	}()

	waitForWindowsCancellationFixture(t, pidsPath, done, 8*time.Second)
	pids := readWindowsCancellationPIDs(t, pidsPath)
	cancel()

	var got windowsExecOutcome
	select {
	case got = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("canceled Windows Exec did not converge")
	}
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("canceled Windows Exec error = %v, want context.Canceled", got.err)
	}
	if got.result == nil || got.result.Limits.ProcessContainment != LimitStatusEnforced {
		t.Fatalf("canceled Windows Exec result = %+v, want enforced process tree", got.result)
	}
	for _, pid := range pids {
		assertWindowsProcessExited(t, pid)
	}
}

// TestWindows_ContextCanceledRootPayload 创建一个真实后代并在就绪后等待取消。
func TestWindows_ContextCanceledRootPayload(t *testing.T) {
	if !strings.EqualFold(filepath.Base(os.Args[0]), "sandbox-context-cancel-payload.exe") {
		return
	}
	arguments := argumentsAfterDoubleDash(os.Args)
	if len(arguments) != 1 {
		return
	}
	childReadyPath := "cancel-child.ready"
	child := exec.Command(
		os.Args[0],
		"-test.run=^TestWindows_ContextCanceledChildPayload$",
		"-test.count=1",
		"--",
		childReadyPath,
	) // #nosec G204 -- 仅重启工作区内已冻结的测试载荷。
	if err := child.Start(); err != nil {
		return
	}
	childDone := make(chan error, 1)
	go func() {
		childDone <- child.Wait()
		close(childDone)
	}()
	if ready, _ := waitForWindowsPayloadOutcome(childReadyPath, childDone, 5*time.Second); !ready {
		killWindowsPayloadChildren([]*exec.Cmd{child}, []<-chan error{childDone})
		return
	}
	content := fmt.Sprintf("%d\n%d\n", os.Getpid(), child.Process.Pid)
	if err := os.WriteFile(arguments[0], []byte(content), 0o600); err != nil {
		killWindowsPayloadChildren([]*exec.Cmd{child}, []<-chan error{childDone})
		return
	}
	time.Sleep(30 * time.Second)
}

// TestWindows_ContextCanceledChildPayload 保持后代存活，供取消后的 Job 收敛验证使用。
func TestWindows_ContextCanceledChildPayload(t *testing.T) {
	if !strings.EqualFold(filepath.Base(os.Args[0]), "sandbox-context-cancel-payload.exe") {
		return
	}
	arguments := argumentsAfterDoubleDash(os.Args)
	if len(arguments) != 1 {
		return
	}
	if err := os.WriteFile(arguments[0], []byte("ready"), 0o600); err != nil {
		return
	}
	time.Sleep(30 * time.Second)
}

func windowsTCP4Fixture() (string, func(), error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	return listener.Addr().String(), func() { _ = listener.Close() }, nil
}

func windowsTCP6Fixture() (string, func(), error) {
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		return "", nil, err
	}
	return listener.Addr().String(), func() { _ = listener.Close() }, nil
}

func windowsUDP4Fixture() (string, func(), error) {
	connection, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	return connection.LocalAddr().String(), func() { _ = connection.Close() }, nil
}

func windowsUDP6Fixture() (string, func(), error) {
	connection, err := net.ListenPacket("udp6", "[::1]:0")
	if err != nil {
		return "", nil, err
	}
	return connection.LocalAddr().String(), func() { _ = connection.Close() }, nil
}

func printWindowsNetworkResult(networkName string, err error) {
	var errno syscall.Errno
	if errors.As(err, &errno) && errno == windows.WSAEACCES {
		fmt.Printf("NETWORK_DENIED:%s:%d", networkName, uint32(errno))
		return
	}
	fmt.Printf("NETWORK_UNEXPECTED_ERROR:%s:%v", networkName, err)
}

func waitForWindowsCancellationFixture(
	t *testing.T,
	path string,
	done <-chan windowsExecOutcome,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("inspect cancellation fixture: %v", err)
		}
		select {
		case result := <-done:
			t.Fatalf("Windows Exec exited before cancellation: result=%+v err=%v", result.result, result.err)
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("Windows cancellation fixture was not ready within %s", timeout)
		}
	}
}

func readWindowsCancellationPIDs(t *testing.T, path string) []uint32 {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Windows cancellation PID fixture: %v", err)
	}
	lines := strings.Fields(string(content))
	if len(lines) != 2 {
		t.Fatalf("Windows cancellation PID fixture = %q, want two PIDs", content)
	}
	pids := make([]uint32, 0, len(lines))
	for _, line := range lines {
		value, parseErr := strconv.ParseUint(line, 10, 32)
		if parseErr != nil {
			t.Fatalf("parse Windows cancellation PID %q: %v", line, parseErr)
		}
		pids = append(pids, uint32(value))
	}
	return pids
}

func assertWindowsProcessExited(t *testing.T, pid uint32) {
	t.Helper()
	process, err := windows.OpenProcess(
		windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false,
		pid,
	)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return
	}
	if err != nil {
		t.Fatalf("open canceled Windows process %d: %v", pid, err)
	}
	defer windows.CloseHandle(process)
	waitResult, err := windows.WaitForSingleObject(process, 0)
	if err != nil {
		t.Fatalf("inspect canceled Windows process %d: %v", pid, err)
	}
	if waitResult != windows.WAIT_OBJECT_0 {
		t.Fatalf("Windows process %d survived context cancellation", pid)
	}
}
