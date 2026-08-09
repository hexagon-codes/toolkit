//go:build linux

package sandbox

import (
	"errors"
	"testing"
)

func TestLinuxRejectsUnenforceableLoopbackPolicy(t *testing.T) {
	_, err := newPlatformSandbox(Config{
		Workspace:    t.TempDir(),
		Network:      true,
		DenyLoopback: true,
	})
	if !errors.Is(err, ErrUnsupportedNetworkPolicy) {
		t.Fatalf("expected ErrUnsupportedNetworkPolicy, got %v", err)
	}
}
