//go:build windows

package sandbox

import "testing"

// newWindowsTestSandbox 在 TempDir 清理前确定性关闭 Windows 根守卫与工作区资源。
func newWindowsTestSandbox(t *testing.T, cfg Config) Sandbox {
	t.Helper()
	sandboxValue, err := New(cfg)
	if err != nil {
		t.Fatalf("create Windows sandbox: %v", err)
	}
	registerWindowsTestSandboxCleanup(t, sandboxValue)
	return sandboxValue
}

func registerWindowsTestSandboxCleanup(t *testing.T, sandboxValue Sandbox) {
	t.Helper()
	t.Cleanup(func() {
		if err := sandboxValue.Close(); err != nil {
			t.Errorf("close Windows sandbox: %v", err)
		}
	})
}
