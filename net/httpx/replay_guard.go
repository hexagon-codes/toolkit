package httpx

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync/atomic"

	"golang.org/x/net/http2"
)

type transportAttemptKey struct{}

type transportAttempt struct {
	writeStarted  atomic.Bool
	writeComplete atomic.Bool
}

// TransportReplayError 表示传输层在下一次请求写入前阻止了内部重放。
// Trace 字段仅是 httptrace 事件，不证明业务字节已被对端接收。
type TransportReplayError struct {
	Protocol                  string
	RequestWriteTraceStarted  bool
	RequestWriteTraceComplete bool
}

func (e *TransportReplayError) Error() string {
	return "HTTP transport replay was blocked before the next request"
}

func blockTransportReplay(req *http.Request, protocol string) error {
	attempt, _ := req.Context().Value(transportAttemptKey{}).(*transportAttempt)
	if attempt == nil || !attempt.writeStarted.Load() {
		return nil
	}
	_, idempotencyKey := req.Header["Idempotency-Key"]
	_, alternateKey := req.Header["X-Idempotency-Key"]
	// H1 非可重放写请求再次进入 Proxy 时，标准 Transport 已确认上次写入零字节。
	// WroteRequest 可能先于连接 Flush；不能将其成功事件视为网络写入成功。
	if protocol == "http1" && replayGuardWriteMethod(req.Method) && !idempotencyKey && !alternateKey {
		return nil
	}
	return &TransportReplayError{
		Protocol:                  protocol,
		RequestWriteTraceStarted:  true,
		RequestWriteTraceComplete: attempt.writeComplete.Load(),
	}
}

func replayGuardWriteMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

type replayGuardHTTP2Pool struct{ http2.ClientConnPool }

func (p replayGuardHTTP2Pool) GetClientConn(req *http.Request, address string) (*http2.ClientConn, error) {
	if err := blockTransportReplay(req, "http2"); err != nil {
		return nil, err
	}
	return p.ClientConnPool.GetClientConn(req, address)
}

type replayGuardTransport struct {
	transport       *http.Transport
	http2IdleCloser *http2.Transport
}

func (t *replayGuardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, ErrInvalidRequest
	}
	attempt := &transportAttempt{}
	trace := &httptrace.ClientTrace{
		WroteHeaders: func() { attempt.writeStarted.Store(true) },
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			attempt.writeStarted.Store(true)
			if info.Err == nil {
				attempt.writeComplete.Store(true)
			}
		},
	}
	ctx := context.WithValue(req.Context(), transportAttemptKey{}, attempt)
	return t.transport.RoundTrip(req.WithContext(httptrace.WithClientTrace(ctx, trace)))
}

func (t *replayGuardTransport) CloseIdleConnections() {
	t.transport.CloseIdleConnections()
	if t.http2IdleCloser != nil {
		// 包装后的 pool 不转发 http2 私有关闭接口，显式关闭原 pool 的空闲连接。
		t.http2IdleCloser.CloseIdleConnections()
	}
}

// NewReplayGuardTransport 显式阻止标准 HTTP/1.1 和 TLS HTTP/2 传输内部的请求重放。
// 它克隆 base 的配置并拥有新连接池，不修改或共享 base 的已有连接池；调用方通过
// http.Client.CloseIdleConnections 关闭返回传输的空闲连接。客户端重定向和应用重试
// 仍由调用方决定；H1 已确认零字节的连接恢复不算第二次业务发送。
// 自定义 HTTP/2 处理器和明文 HTTP/2 不在此包装器的支持范围内。
func NewReplayGuardTransport(base *http.Transport) (http.RoundTripper, error) {
	if base == nil {
		return nil, fmt.Errorf("%w: transport is required", ErrInvalidRawClientConfig)
	}
	transport := base.Clone()
	if transport.Protocols != nil && transport.Protocols.UnencryptedHTTP2() {
		return nil, fmt.Errorf("%w: replay observation does not support unencrypted HTTP/2", ErrInvalidRawClientConfig)
	}
	originalProxy := transport.Proxy
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		// 返回错误发生在获取下一连接之前，不依赖取消与 ready 连接之间的竞争。
		if err := blockTransportReplay(req, "http1"); err != nil {
			return nil, err
		}
		if originalProxy != nil {
			return originalProxy(req)
		}
		return nil, nil
	}
	guard := &replayGuardTransport{transport: transport}
	if !replayGuardUsesHTTP2(transport) {
		return guard, nil
	}
	http2Transport, err := http2.ConfigureTransports(transport)
	if err != nil {
		return nil, fmt.Errorf("configure replay-observed HTTP/2 transport: %w", err)
	}
	if http2Transport.ConnPool == nil {
		return nil, fmt.Errorf("HTTP/2 transport does not expose its connection pool")
	}
	originalPool := http2Transport.ConnPool
	http2Transport.ConnPool = replayGuardHTTP2Pool{ClientConnPool: originalPool}
	guard.http2IdleCloser = &http2.Transport{ConnPool: originalPool}
	return guard, nil
}

func replayGuardUsesHTTP2(transport *http.Transport) bool {
	if transport.Protocols != nil {
		return transport.Protocols.HTTP2()
	}
	if transport.TLSNextProto != nil {
		_, enabled := transport.TLSNextProto["h2"]
		return enabled
	}
	return transport.ForceAttemptHTTP2 || transport.Dial == nil && transport.DialContext == nil &&
		transport.DialTLS == nil && transport.DialTLSContext == nil && transport.TLSClientConfig == nil
}
