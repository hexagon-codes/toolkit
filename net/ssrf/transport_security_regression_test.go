package ssrf

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type staticConn struct {
	remote net.Addr
}

type staticRoundTripper struct{}

func (staticRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("default transport must not be used")
}

func (*staticConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (*staticConn) Write(buffer []byte) (int, error) { return len(buffer), nil }
func (*staticConn) Close() error                     { return nil }
func (*staticConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *staticConn) RemoteAddr() net.Addr           { return c.remote }
func (*staticConn) SetDeadline(time.Time) error      { return nil }
func (*staticConn) SetReadDeadline(time.Time) error  { return nil }
func (*staticConn) SetWriteDeadline(time.Time) error { return nil }

func TestGuardedTransportPinsTheValidatedAddress(t *testing.T) {
	t.Parallel()

	var resolved atomic.Int32
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		resolved.Add(1)
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	}
	var dialedAddress string
	dial := func(_ context.Context, _, address string) (net.Conn, error) {
		dialedAddress = address
		return &staticConn{remote: &net.TCPAddr{IP: net.ParseIP("8.8.8.8"), Port: 443}}, nil
	}
	transport, err := newGuardedTransport(lookup, dial)
	if err != nil {
		t.Fatal(err)
	}

	connection, err := transport.dialContext(context.Background(), "tcp", "example.com:443")
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if resolved.Load() != 1 {
		t.Fatalf("resolver calls = %d, want 1", resolved.Load())
	}
	if dialedAddress != "8.8.8.8:443" {
		t.Fatalf("dial address = %q, want pinned literal address", dialedAddress)
	}
}

func TestGuardedTransportFallsBackAcrossValidatedPublicAddresses(t *testing.T) {
	t.Parallel()

	var dialed []string
	transport, err := newGuardedTransport(
		func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{
				netip.MustParseAddr("8.8.8.8"),
				netip.MustParseAddr("1.1.1.1"),
			}, nil
		},
		func(_ context.Context, _, address string) (net.Conn, error) {
			dialed = append(dialed, address)
			if strings.HasPrefix(address, "8.8.8.8:") {
				return nil, errors.New("first candidate unavailable")
			}
			return &staticConn{remote: &net.TCPAddr{IP: net.ParseIP("1.1.1.1"), Port: 443}}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := transport.dialContext(context.Background(), "tcp", "example.com:443")
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	want := []string{"8.8.8.8:443", "1.1.1.1:443"}
	if strings.Join(dialed, ",") != strings.Join(want, ",") {
		t.Fatalf("dialed addresses = %v, want %v", dialed, want)
	}
}

func TestGuardedTransportRevalidatesDNSForEveryNewConnection(t *testing.T) {
	t.Parallel()

	var lookups atomic.Int32
	var dials atomic.Int32
	transport, err := newGuardedTransport(
		func(context.Context, string, string) ([]netip.Addr, error) {
			if lookups.Add(1) == 1 {
				return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
			}
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		},
		func(context.Context, string, string) (net.Conn, error) {
			dials.Add(1)
			return &staticConn{remote: &net.TCPAddr{IP: net.ParseIP("8.8.8.8"), Port: 443}}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	first, err := transport.dialContext(context.Background(), "tcp", "example.com:443")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := transport.dialContext(context.Background(), "tcp", "example.com:443")
	if second != nil || !errors.Is(err, ErrBlocked) {
		t.Fatalf("second dial = (%v, %v), want DNS rebinding rejection", second, err)
	}
	if lookups.Load() != 2 || dials.Load() != 1 {
		t.Fatalf("lookups = %d, dials = %d, want 2 and 1", lookups.Load(), dials.Load())
	}
}

func TestGuardedTransportRejectsTrailingDotDialHostname(t *testing.T) {
	t.Parallel()

	var resolved atomic.Bool
	transport, err := newGuardedTransport(
		func(context.Context, string, string) ([]netip.Addr, error) {
			resolved.Store(true)
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		},
		func(context.Context, string, string) (net.Conn, error) {
			return &staticConn{remote: &net.TCPAddr{IP: net.ParseIP("8.8.8.8"), Port: 443}}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := transport.dialContext(context.Background(), "tcp", "Example.COM.:443")
	if connection != nil || !errors.Is(err, ErrBlocked) || !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("dialContext() = (%v, %v), want non-canonical host rejection", connection, err)
	}
	if resolved.Load() {
		t.Fatal("resolver was called for a non-canonical trailing-dot host")
	}
}

func TestGuardedTransportRejectsNilConnectionFromDialer(t *testing.T) {
	t.Parallel()

	transport, err := newGuardedTransport(
		publicLookupNetIP,
		func(context.Context, string, string) (net.Conn, error) { return nil, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("dialContext() panicked: %v", recovered)
		}
	}()
	connection, err := transport.dialContext(context.Background(), "tcp", "example.com:443")
	if connection != nil || !errors.Is(err, ErrInvalidTransport) {
		t.Fatalf("dialContext() = (%v, %v), want (nil, ErrInvalidTransport)", connection, err)
	}
}

func TestGuardedTransportRejectsPeerDifferentFromPinnedAddress(t *testing.T) {
	t.Parallel()

	transport, err := newGuardedTransport(
		publicLookupNetIP,
		func(context.Context, string, string) (net.Conn, error) {
			return &staticConn{remote: &net.TCPAddr{IP: net.ParseIP("1.1.1.1"), Port: 443}}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := transport.dialContext(context.Background(), "tcp", "example.com:443")
	if connection != nil || !errors.Is(err, ErrBlocked) {
		t.Fatalf("dialContext() = (%v, %v), want (nil, ErrBlocked)", connection, err)
	}
}

func TestGuardedTransportRejectsUnsafeAnswerBeforeDial(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		addresses []netip.Addr
	}{
		{name: "private", addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")}},
		{name: "mixed public and private", addresses: []netip.Addr{
			netip.MustParseAddr("8.8.8.8"),
			netip.MustParseAddr("127.0.0.1"),
		}},
		{name: "IPv4-mapped loopback", addresses: []netip.Addr{netip.MustParseAddr("::ffff:127.0.0.1")}},
		{name: "empty answer"},
		{name: "invalid answer", addresses: []netip.Addr{{}}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			var dialed atomic.Bool
			transport, err := newGuardedTransport(
				func(context.Context, string, string) ([]netip.Addr, error) {
					return append([]netip.Addr(nil), test.addresses...), nil
				},
				func(context.Context, string, string) (net.Conn, error) {
					dialed.Store(true)
					return nil, errors.New("dial must not be called")
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			connection, err := transport.dialContext(context.Background(), "tcp", "example.com:443")
			if connection != nil || !errors.Is(err, ErrBlocked) {
				t.Fatalf("dial = (%v, %v), want (nil, ErrBlocked)", connection, err)
			}
			if dialed.Load() {
				t.Fatal("unsafe DNS answer reached the dialer")
			}
		})
	}
}

func TestGuardedTransportRejectsUnsafeConnectedPeer(t *testing.T) {
	t.Parallel()

	transport, err := newGuardedTransport(
		func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		},
		func(context.Context, string, string) (net.Conn, error) {
			return &staticConn{remote: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 443}}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := transport.dialContext(context.Background(), "tcp", "example.com:443")
	if connection != nil || !errors.Is(err, ErrBlocked) {
		t.Fatalf("dial = (%v, %v), want (nil, ErrBlocked)", connection, err)
	}
}

func TestGuardedTransportDisablesProxyAndTLSDialBypasses(t *testing.T) {
	t.Parallel()

	transport, err := NewTransport()
	if err != nil {
		t.Fatal(err)
	}
	defer transport.CloseIdleConnections()
	if transport.transport.Proxy != nil {
		t.Fatal("guarded transport retained a proxy function")
	}
	//lint:ignore SA1019 需要验证旧 TLS 拨号入口未形成安全绕过。
	if transport.transport.DialTLS != nil || transport.transport.DialTLSContext != nil {
		t.Fatal("guarded transport retained a TLS dial bypass")
	}
}

func TestNewTransportDoesNotDependOnMutableHTTPDefaultTransport(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = staticRoundTripper{}
	t.Cleanup(func() { http.DefaultTransport = original })

	transport, err := NewTransport()
	if err != nil {
		t.Fatalf("NewTransport() error = %v, want an independent transport", err)
	}
	transport.CloseIdleConnections()
}

func TestGuardedClientValidatesRedirectTargets(t *testing.T) {
	t.Parallel()

	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	for _, target := range []string{
		"http://localhost/secret",
		"file://example.com/secret",
		"https://user:secret@example.com/path",
	} {
		request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		if err := client.CheckRedirect(request, nil); !errors.Is(err, ErrBlocked) {
			t.Fatalf("redirect to %q error = %v, want ErrBlocked", target, err)
		}
	}
}

func TestGuardedTransportRoundTripRejectsAmbiguousURLBeforeDial(t *testing.T) {
	t.Parallel()

	transport, err := newGuardedTransport(publicLookupNetIP, func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("dial must not be called")
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"https://example.com/path",
		http.NoBody,
	)
	if err != nil {
		t.Fatal(err)
	}
	request.URL.User = url.UserPassword("user", "secret")
	response, err := transport.RoundTrip(request)
	if response != nil || !errors.Is(err, ErrBlocked) || !strings.Contains(err.Error(), "user information") {
		t.Fatalf("RoundTrip() = (%v, %v), want blocked user information", response, err)
	}
}
