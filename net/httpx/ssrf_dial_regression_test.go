package httpx

import (
	"context"
	"errors"
	"testing"
)

func TestSSRFSafeDialerRejectsBlockedAddressBeforeConnect(t *testing.T) {
	dialer := newSSRFSafeDialer()
	if dialer.ControlContext == nil {
		t.Fatal("SSRF-safe dialer has no pre-connect control")
	}

	for _, address := range []string{"127.0.0.1:80", "[::1]:443", "169.254.169.254:80"} {
		t.Run(address, func(t *testing.T) {
			err := dialer.ControlContext(context.Background(), "tcp", address, nil)
			if !errors.Is(err, ErrSSRFBlocked) {
				t.Fatalf("ControlContext(%q) error = %v, want ErrSSRFBlocked", address, err)
			}
		})
	}

	for _, address := range []string{"8.8.8.8:443", "[2606:4700:4700::1111]:443"} {
		t.Run(address, func(t *testing.T) {
			if err := dialer.ControlContext(context.Background(), "tcp", address, nil); err != nil {
				t.Fatalf("ControlContext(%q) error = %v, want nil", address, err)
			}
		})
	}
}
