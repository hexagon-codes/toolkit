//go:build windows

package sandbox

import (
	"strings"
	"testing"
	"unsafe"
)

func TestWindowsLifecycleDoesNotDiscardCleanupErrors(t *testing.T) {
	tests := []struct {
		filename  string
		forbidden []string
	}{
		{
			filename: "win_acl.go",
			forbidden: []string{
				"_ = policy.restoreACL()",
				"_, _ = syscall.LocalFree",
			},
		},
		{
			filename: "win_launcher.go",
			forbidden: []string{
				"_ = aclPolicy.restoreACL()",
			},
		},
	}

	for _, test := range tests {
		source := mustReadSandboxSource(t, test.filename)
		for _, forbidden := range test.forbidden {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s discards cleanup error with %q", test.filename, forbidden)
			}
		}
	}
}

func TestWindowsLauncherUsesExactInheritedHandleList(t *testing.T) {
	launcher := mustReadSandboxSource(t, "win_launcher.go")
	for _, required := range []string{
		"PROC_THREAD_ATTRIBUTE_HANDLE_LIST",
		"EXTENDED_STARTUPINFO_PRESENT",
		"runtime.KeepAlive(inheritedHandles)",
		"windows.Handle(stdinR.value())",
		"windows.Handle(stdoutW.value())",
		"windows.Handle(stderrW.value())",
	} {
		if !strings.Contains(launcher, required) {
			t.Errorf("win_launcher.go is missing %q", required)
		}
	}
}

func TestWindowsJobAccountingInformationABI(t *testing.T) {
	if got := unsafe.Sizeof(jobObjectBasicAccountingInformation{}); got != 48 {
		t.Fatalf("job accounting information size = %d, want 48", got)
	}
}
