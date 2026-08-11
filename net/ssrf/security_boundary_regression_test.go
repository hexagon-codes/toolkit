package ssrf

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"
)

func publicLookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
}

func TestValidateURLRejectsUnsafeURLStructure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		rawURL string
	}{
		{name: "unsupported scheme", rawURL: "ftp://example.com/file"},
		{name: "userinfo", rawURL: "https://user:secret@example.com/path"},
		{name: "fragment", rawURL: "https://example.com/path#section"},
		{name: "unicode host", rawURL: "https://例子.测试/path"},
		{name: "decimal IPv4", rawURL: "http://2130706433/path"},
		{name: "short IPv4", rawURL: "http://127.1/path"},
		{name: "octal IPv4", rawURL: "http://0177.0.0.1/path"},
		{name: "hexadecimal IPv4", rawURL: "http://0x7f000001/path"},
		{name: "uppercase hexadecimal IPv4", rawURL: "http://0X7F000001/path"},
		{name: "empty port", rawURL: "http://example.com:/path"},
		{name: "leading-zero port", rawURL: "http://example.com:080/path"},
		{name: "trailing-dot host", rawURL: "http://example.com./path"},
		{name: "IPv6 zone", rawURL: "http://[fe80::1%25en0]/path"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if err := validateURLWithResolver(context.Background(), test.rawURL, publicLookupNetIP); err == nil {
				t.Fatalf("validateURLWithResolver(%q) error = nil, want rejection", test.rawURL)
			}
		})
	}
}

func TestValidateURLAcceptsCanonicalIDNAlabel(t *testing.T) {
	t.Parallel()

	var resolvedHost string
	lookup := func(_ context.Context, _, host string) ([]netip.Addr, error) {
		resolvedHost = host
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	if err := validateURLWithResolver(context.Background(), "https://xn--fsqu00a.xn--0zwm56d/path", lookup); err != nil {
		t.Fatal(err)
	}
	if resolvedHost != "xn--fsqu00a.xn--0zwm56d" {
		t.Fatalf("resolved host = %q, want canonical ASCII IDNA host", resolvedHost)
	}
}

func TestValidateURLRejectsMappedLoopbackLiteralWithoutDNS(t *testing.T) {
	t.Parallel()

	called := false
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		called = true
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	err := validateURLWithResolver(context.Background(), "http://[::ffff:127.0.0.1]/", lookup)
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("validation error = %v, want ErrBlocked", err)
	}
	if called {
		t.Fatal("resolver was called for an IPv4-mapped literal")
	}
}

func TestValidateURLRejectsNonCanonicalTrailingDotHosts(t *testing.T) {
	t.Parallel()

	called := false
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		called = true
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	for _, rawURL := range []string{
		"http://example.com./",
		"http://localhost./",
		"http://subdomain.localhost./",
		"http://metadata.google.internal./",
	} {
		if err := validateURLWithResolver(context.Background(), rawURL, lookup); !errors.Is(err, ErrInvalidURL) {
			t.Fatalf("validateURLWithResolver(%q) error = %v, want ErrInvalidURL", rawURL, err)
		}
	}
	if called {
		t.Fatal("resolver was called for a non-canonical trailing-dot host")
	}
}

func TestValidateURLFailsClosedOnInvalidDNSAnswers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		lookup lookupNetIPFunc
	}{
		{name: "empty answer", lookup: func(context.Context, string, string) ([]netip.Addr, error) { return nil, nil }},
		{name: "invalid answer", lookup: func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{{}}, nil
		}},
		{name: "mixed valid and invalid answer", lookup: func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8"), {}}, nil
		}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if err := validateURLWithResolver(context.Background(), "https://example.com/path", test.lookup); err == nil {
				t.Fatal("validation error = nil, want fail-closed DNS rejection")
			}
		})
	}
}

func TestValidateURLRejectsAnyUnsafeAddressInDNSAnswer(t *testing.T) {
	t.Parallel()

	unsafeAddresses := []string{
		"100.64.0.1",
		"192.0.2.1",
		"198.18.0.1",
		"224.0.0.1",
		"240.0.0.1",
		"64:ff9b::7f00:1",
		"2001:db8::1",
		"ff02::1",
	}
	for _, unsafeAddress := range unsafeAddresses {
		unsafeAddress := unsafeAddress
		t.Run(unsafeAddress, func(t *testing.T) {
			lookup := func(context.Context, string, string) ([]netip.Addr, error) {
				return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr(unsafeAddress)}, nil
			}
			if err := validateURLWithResolver(context.Background(), "https://example.com/path", lookup); err == nil {
				t.Fatalf("mixed DNS answer containing %s was accepted", unsafeAddress)
			}
		})
	}
}

func TestValidateLocalURLRejectsNonHTTPAndAmbiguousAuthority(t *testing.T) {
	t.Parallel()

	for _, rawURL := range []string{
		"file://localhost/tmp/model",
		"ftp://127.0.0.1/model",
		"http://user:secret@localhost/model",
		"http://localhost/model#fragment",
	} {
		if err := ValidateLocalURL(rawURL); err == nil {
			t.Fatalf("ValidateLocalURL(%q) error = nil, want rejection", rawURL)
		}
	}
}

func TestValidateLocalURLRejectsLocalhostSubdomains(t *testing.T) {
	t.Parallel()

	if err := ValidateLocalURL("http://service.localhost/api"); !errors.Is(err, ErrBlocked) {
		t.Fatalf("ValidateLocalURL() error = %v, want ErrBlocked", err)
	}
}

func TestValidateURLPreservesResolverError(t *testing.T) {
	t.Parallel()

	sentinel := fmt.Errorf("resolver failed")
	err := validateURLWithResolver(context.Background(), "https://example.com/path", func(context.Context, string, string) ([]netip.Addr, error) {
		return nil, sentinel
	})
	if err == nil || !strings.Contains(err.Error(), sentinel.Error()) {
		t.Fatalf("validation error = %v, want resolver failure", err)
	}
}

func TestValidateURLRejectsNilContextBeforeResolver(t *testing.T) {
	t.Parallel()

	called := false
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		called = true
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	//lint:ignore SA1012 需要验证公开安全 API 的 nil context 合同。
	//nolint:staticcheck // 需要验证公开安全 API 的 nil context 合同。
	err := validateURLWithResolver(nil, "https://example.com/path", lookup)
	if !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("validation error = %v, want ErrInvalidContext", err)
	}
	if called {
		t.Fatal("resolver was called for a nil context")
	}
}
