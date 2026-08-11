package ssrf

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"
)

type dialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

// Transport 在每次连接时校验并固定 DNS 结果，且不使用环境代理。
type Transport struct {
	transport *http.Transport
	lookup    lookupNetIPFunc
	dial      dialContextFunc
}

// NewTransport 创建具有连接期 SSRF 防护的 HTTP Transport。
func NewTransport() (*Transport, error) {
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	return newGuardedTransport(net.DefaultResolver.LookupNetIP, dialer.DialContext)
}

func newGuardedTransport(lookup lookupNetIPFunc, dial dialContextFunc) (*Transport, error) {
	if lookup == nil {
		return nil, fmt.Errorf("%w: resolver must not be nil", ErrInvalidTransport)
	}
	if dial == nil {
		return nil, fmt.Errorf("%w: dialer must not be nil", ErrInvalidTransport)
	}
	guarded := &Transport{
		lookup: lookup,
		dial:   dial,
	}
	// 使用独立配置，避免继承进程可变的默认代理、拨号器或自定义协议。
	guarded.transport = &http.Transport{
		DialContext:           guarded.dialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	return guarded, nil
}

// RoundTrip 在交给标准传输层前校验 URL 的唯一规范形式。
func (t *Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t == nil || t.transport == nil || t.lookup == nil || t.dial == nil {
		return nil, fmt.Errorf("%w: transport is not initialized", ErrInvalidTransport)
	}
	if request == nil || request.URL == nil {
		return nil, fmt.Errorf("%w: request URL must not be nil", ErrBlocked)
	}
	_, host, err := parseHTTPURL(request.URL.String())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBlocked, err)
	}
	if _, _, err := validateHostWithoutDNS(host); err != nil {
		return nil, err
	}
	return t.transport.RoundTrip(request)
}

// CloseIdleConnections 关闭底层传输层持有的空闲连接。
func (t *Transport) CloseIdleConnections() {
	if t == nil || t.transport == nil {
		return
	}
	t.transport.CloseIdleConnections()
}

func (t *Transport) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if isNilContext(ctx) {
		return nil, ErrInvalidContext
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid dial address %q: %w", ErrBlocked, address, err)
	}
	if err := validateASCIIHost(host); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBlocked, err)
	}

	var candidates []netip.Addr
	if literal, parsed, policyErr := validateHostWithoutDNS(host); policyErr != nil {
		return nil, policyErr
	} else if literal {
		candidates = []netip.Addr{parsed}
	} else {
		resolved, lookupErr := t.lookup(ctx, "ip", host)
		if lookupErr != nil {
			return nil, fmt.Errorf("%w: DNS lookup failed for %q: %w", ErrBlocked, host, lookupErr)
		}
		candidates, err = validateResolvedAddresses(host, resolved)
		if err != nil {
			return nil, err
		}
	}

	var dialErrors error
	for _, candidate := range candidates {
		if network == "tcp4" && !candidate.Is4() || network == "tcp6" && !candidate.Is6() {
			continue
		}
		pinnedAddress := net.JoinHostPort(candidate.String(), port)
		connection, dialErr := t.dial(ctx, network, pinnedAddress)
		if dialErr != nil {
			dialErrors = errors.Join(dialErrors, dialErr)
			continue
		}
		if connection == nil {
			dialErrors = errors.Join(dialErrors, fmt.Errorf("%w: dialer returned a nil connection", ErrInvalidTransport))
			continue
		}
		if peerErr := validateConnectedPeer(connection.RemoteAddr(), candidate); peerErr != nil {
			return nil, errors.Join(peerErr, connection.Close())
		}
		return connection, nil
	}
	if dialErrors == nil {
		return nil, fmt.Errorf("%w: no resolved address matches network %q", ErrBlocked, network)
	}
	return nil, fmt.Errorf("dial %q failed: %w", host, dialErrors)
}

func validateConnectedPeer(address net.Addr, expected netip.Addr) error {
	tcpAddress, ok := address.(*net.TCPAddr)
	if !ok || tcpAddress == nil {
		return fmt.Errorf("%w: connected peer address is unverifiable", ErrBlocked)
	}
	if tcpAddress.Zone != "" {
		return fmt.Errorf("%w: connected peer address has an IPv6 zone", ErrBlocked)
	}
	parsed, ok := netip.AddrFromSlice(tcpAddress.IP)
	if !ok {
		return fmt.Errorf("%w: connected peer address is invalid", ErrBlocked)
	}
	parsed = parsed.Unmap()
	if err := validateResolvedAddress("connected peer", parsed); err != nil {
		return err
	}
	if parsed != expected.Unmap() {
		return fmt.Errorf("%w: connected peer %s differs from pinned address %s", ErrBlocked, parsed, expected)
	}
	return nil
}

// NewClient 创建默认使用受保护 Transport 的 HTTP 客户端。
func NewClient() (*http.Client, error) {
	transport, err := NewTransport()
	if err != nil {
		return nil, err
	}
	client := &http.Client{Transport: transport}
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("%w: too many redirects", ErrBlocked)
		}
		if request == nil || request.URL == nil {
			return fmt.Errorf("%w: redirect URL must not be nil", ErrBlocked)
		}
		_, host, err := parseHTTPURL(request.URL.String())
		if err != nil {
			return fmt.Errorf("%w: %w", ErrBlocked, err)
		}
		if _, _, err := validateHostWithoutDNS(host); err != nil {
			return err
		}
		return nil
	}
	return client, nil
}
