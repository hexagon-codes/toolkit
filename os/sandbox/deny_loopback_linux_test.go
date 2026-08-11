//go:build linux

package sandbox

import (
	"slices"
	"testing"
)

// Linux 网络合同只有禁用网络与继承宿主网络两种精确语义，不提供近似的回环过滤。
func TestLinuxNetworkModesMapExactlyToNetworkNamespaces(t *testing.T) {
	workspace := t.TempDir()
	if err := initializePOSIXRuntimeDirectories(workspace); err != nil {
		t.Fatal(err)
	}
	command := linuxPreparedTestCommand(t, "/usr/bin/true", workspace)

	tests := []struct {
		name           string
		mode           NetworkMode
		wantUnshareNet bool
	}{
		{name: "disabled", mode: NetworkDisabled, wantUnshareNet: true},
		{name: "host", mode: NetworkHost, wantUnshareNet: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend, err := newPlatformSandbox(Config{Workspace: workspace, Network: test.mode})
			if err != nil {
				t.Fatal(err)
			}
			linuxBackend, ok := backend.(*linuxSandbox)
			if !ok {
				t.Fatalf("newPlatformSandbox() type = %T, want *linuxSandbox", backend)
			}
			arguments, err := linuxBackend.bwrapArgs(command)
			if err != nil {
				t.Fatal(err)
			}
			if got := slices.Contains(arguments, "--unshare-net"); got != test.wantUnshareNet {
				t.Fatalf("--unshare-net present = %t, want %t", got, test.wantUnshareNet)
			}
		})
	}
}
