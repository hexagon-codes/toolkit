package ip

import (
	"context"
	"errors"
	"testing"
	"time"
)

type typedNilContext struct{}

func (*typedNilContext) Deadline() (time.Time, bool) { panic("typed nil context used") }
func (*typedNilContext) Done() <-chan struct{}       { panic("typed nil context used") }
func (*typedNilContext) Err() error                  { panic("typed nil context used") }
func (*typedNilContext) Value(any) any               { panic("typed nil context used") }

func TestIsPublicRejectsNonGlobalUnicastAddresses(t *testing.T) {
	for _, address := range []string{
		"224.0.0.1",
		"239.255.255.250",
		"255.255.255.255",
		"ff02::1",
	} {
		if IsPublic(address) {
			t.Errorf("IsPublic(%q) = true, want false", address)
		}
	}
}

func TestIsInRangeRejectsMixedAddressFamilies(t *testing.T) {
	tests := []struct {
		address string
		start   string
		end     string
	}{
		{"192.168.1.5", "::1", "ffff::"},
		{"2001:db8::5", "192.168.1.1", "192.168.1.10"},
		{"192.168.1.5", "192.168.1.1", "2001:db8::10"},
	}
	for _, test := range tests {
		if IsInRange(test.address, test.start, test.end) {
			t.Errorf("IsInRange(%q, %q, %q) = true, want false", test.address, test.start, test.end)
		}
	}
}

func TestMaskRejectsInvalidPrefixLengths(t *testing.T) {
	for _, test := range []struct {
		address string
		prefix  int
	}{
		{"192.168.1.1", -1},
		{"192.168.1.1", 33},
		{"2001:db8::1", -1},
		{"2001:db8::1", 129},
	} {
		if got := Mask(test.address, test.prefix); got != "" {
			t.Errorf("Mask(%q, %d) = %q, want empty string", test.address, test.prefix, got)
		}
	}
}

func TestRequestHelpersAcceptNilRequest(t *testing.T) {
	for name, call := range map[string]func() string{
		"FromRequest":       func() string { return FromRequest(nil) },
		"FromRequestDirect": func() string { return FromRequestDirect(nil) },
		"FromRequestWithTrustedProxies": func() string {
			return FromRequestWithTrustedProxies(nil, []string{"127.0.0.1"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("request helper panicked: %v", recovered)
				}
			}()
			if got := call(); got != "" {
				t.Fatalf("request helper = %q, want empty string", got)
			}
		})
	}
}

func TestNetworkHelpersRejectNilContext(t *testing.T) {
	tests := map[string]func() error{
		"GetLocalIP": func() error {
			//lint:ignore SA1012 需要验证公开 API 对 nil context 的错误合同。
			//nolint:staticcheck // 需要验证公开 API 对 nil context 的错误合同。
			_, err := GetLocalIP(nil)
			return err
		},
		"ResolveHost": func() error {
			//lint:ignore SA1012 需要验证公开 API 对 nil context 的错误合同。
			//nolint:staticcheck // 需要验证公开 API 对 nil context 的错误合同。
			_, err := ResolveHost(nil, "localhost")
			return err
		},
		"ReverseLookup": func() error {
			//lint:ignore SA1012 需要验证公开 API 对 nil context 的错误合同。
			//nolint:staticcheck // 需要验证公开 API 对 nil context 的错误合同。
			_, err := ReverseLookup(nil, "127.0.0.1")
			return err
		},
	}
	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("network helper panicked: %v", recovered)
				}
			}()
			if err := call(); !errors.Is(err, ErrNilContext) {
				t.Fatalf("expected ErrNilContext, got %v", err)
			}
		})
	}
}

func TestNetworkHelpersRejectTypedNilContext(t *testing.T) {
	var concrete *typedNilContext
	var ctx context.Context = concrete
	tests := map[string]func() error{
		"GetLocalIP": func() error {
			_, err := GetLocalIP(ctx)
			return err
		},
		"ResolveHost": func() error {
			_, err := ResolveHost(ctx, "localhost")
			return err
		},
		"ReverseLookup": func() error {
			_, err := ReverseLookup(ctx, "127.0.0.1")
			return err
		},
	}
	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("network helper panicked: %v", recovered)
				}
			}()
			if err := call(); !errors.Is(err, ErrNilContext) {
				t.Fatalf("expected ErrNilContext, got %v", err)
			}
		})
	}
}
