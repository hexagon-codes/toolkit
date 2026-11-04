package httpx

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"sync"
	"time"
)

// TransportReplayError 描述传输重放被阻止时的诊断信息。
// Trace 字段仅是 httptrace 事件，不证明业务字节已被对端接收。
// 单次连接发送路径直接返回底层错误，不需要通过此错误阻止内部重放。
type TransportReplayError struct {
	Protocol                  string
	RequestWriteTraceStarted  bool
	RequestWriteTraceComplete bool
}

func (e *TransportReplayError) Error() string {
	return "HTTP transport replay was blocked before the next request"
}

type replayGuardRoute struct {
	key         string
	connections []*replayGuardConn
	dialing     int
	waiters     int
	changed     chan struct{}
}

type replayGuardConn struct {
	conn      *http.ClientConn
	idleSince time.Time
	timer     *time.Timer
}

type replayGuardTransport struct {
	transport *http.Transport
	mu        sync.Mutex
	routes    map[string]*replayGuardRoute
}

// standardLibraryTLSNextProto 返回标准库在默认 HTTP/2 配置下自动安装的
// TLSNextProto handler。不同 Go 版本安装的 key 集合可能不同，因此运行时
// 用一个未使用的 Transport 取得基准，而不是在代码中硬编码 handler 数量。
var (
	standardLibraryTLSNextProtoOnce sync.Once
	standardLibraryTLSNextProto     map[string]func(string, *tls.Conn) http.RoundTripper
)

func standardLibraryNextProtoHandlers() map[string]func(string, *tls.Conn) http.RoundTripper {
	standardLibraryTLSNextProtoOnce.Do(func() {
		probe := &http.Transport{}
		// Clone 会触发标准库的 onceSetNextProtoDefaults，但不会发起网络请求。
		_ = probe.Clone()
		if len(probe.TLSNextProto) == 0 {
			return
		}
		standardLibraryTLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper, len(probe.TLSNextProto))
		for key, handler := range probe.TLSNextProto {
			standardLibraryTLSNextProto[key] = handler
		}
	})
	return standardLibraryTLSNextProto
}

// hasCustomTLSNextProto 只把真正的自定义处理器视为不支持。标准库会在
// Clone/RoundTrip 时自动写入 bundled h2（某些版本还包含 unencrypted_http2），
// 不能仅凭 TLSNextProto 非空就拒绝；否则使用过或预克隆过的 Transport 都会被误拒。
// 标准 handler 的子集（例如显式删除 unencrypted_http2 以关闭 h2c）同样合法，
// 只要每个 key 和函数指针都来自标准库即可。
func hasCustomTLSNextProto(handlers map[string]func(string, *tls.Conn) http.RoundTripper) bool {
	if len(handlers) == 0 {
		return false
	}
	standard := standardLibraryNextProtoHandlers()
	if len(standard) == 0 {
		return true
	}
	for key, handler := range handlers {
		standardHandler, ok := standard[key]
		if !ok || funcPointer(handler) != funcPointer(standardHandler) {
			return true
		}
	}
	return false
}

func funcPointer(handler func(string, *tls.Conn) http.RoundTripper) uintptr {
	if handler == nil {
		return 0
	}
	return reflect.ValueOf(handler).Pointer()
}

// cloneTransportConfig 复制 base 的导出配置，但不触发 base 的 nextProtoOnce，
// 也不修改 base。直接调用 base.Clone() 会初始化 base 的 HTTP/2 状态并写入
// TLSNextProto，导致构造器对同一 base 非幂等，也不满足“不修改调用方配置”的合同。
func cloneTransportConfig(base *http.Transport) *http.Transport {
	clone := new(http.Transport)
	source := reflect.ValueOf(base).Elem()
	target := reflect.ValueOf(clone).Elem()
	for index := 0; index < source.NumField(); index++ {
		field := source.Type().Field(index)
		if !field.IsExported() {
			continue
		}
		target.Field(index).Set(source.Field(index))
	}

	if base.TLSClientConfig != nil {
		clone.TLSClientConfig = base.TLSClientConfig.Clone()
	}
	if base.HTTP2 != nil {
		clone.HTTP2 = new(http.HTTP2Config)
		*clone.HTTP2 = *base.HTTP2
	}
	if base.Protocols != nil {
		clone.Protocols = new(http.Protocols)
		*clone.Protocols = *base.Protocols
	}
	if base.ProxyConnectHeader != nil {
		clone.ProxyConnectHeader = base.ProxyConnectHeader.Clone()
	}
	if base.TLSNextProto != nil {
		clone.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper, len(base.TLSNextProto))
		for key, handler := range base.TLSNextProto {
			clone.TLSNextProto[key] = handler
		}
	}
	return clone
}

// NewReplayGuardTransport 使用单次连接发送阻止 HTTP/1.1 和 TLS HTTP/2 内部重放。
// 它克隆 base 的拨号、代理、TLS 等配置，并独立管理连接复用、空闲期限及连接数量；
// 构造过程不会修改调用方的 base Transport。
// 重定向与应用重试仍由调用方控制；自定义 HTTP/2 处理器和明文 HTTP/2 不受支持。
// 标准库自动安装的 bundled HTTP/2 以及通过 ForceAttemptHTTP2/Protocols 配置的
// 标准 TLS HTTP/2 可用；旧版 x/net http2.ConfigureTransport 若安装
// TLSNextProto handler 会被拒绝，Go 1.27 的 wrapper 使用 Protocols，可正常使用。
// 调用方通过 http.Client.CloseIdleConnections 关闭空闲连接。
func NewReplayGuardTransport(base *http.Transport) (http.RoundTripper, error) {
	if base == nil {
		return nil, fmt.Errorf("%w: transport is required", ErrInvalidRawClientConfig)
	}
	if hasCustomTLSNextProto(base.TLSNextProto) {
		return nil, fmt.Errorf("%w: replay observation does not support TLS next-protocol handlers; use ForceAttemptHTTP2 or Protocols", ErrInvalidRawClientConfig)
	}
	transport := cloneTransportConfig(base)
	// 标准库自动安装的 unencrypted_http2 handler 会让 http:// 连接有机会
	// 协商明文 HTTP/2；公开合同不支持它，从克隆副本中移除，保持调用方 base 不变。
	if transport.TLSNextProto != nil {
		delete(transport.TLSNextProto, "unencrypted_http2")
	}
	if transport.Protocols != nil && transport.Protocols.UnencryptedHTTP2() {
		return nil, fmt.Errorf("%w: replay observation does not support unencrypted HTTP/2", ErrInvalidRawClientConfig)
	}
	return &replayGuardTransport{transport: transport, routes: make(map[string]*replayGuardRoute)}, nil
}

func (t *replayGuardTransport) RoundTrip(req *http.Request) (_ *http.Response, resultErr error) {
	if req != nil && req.Body != nil {
		req = req.Clone(req.Context())
		body := &replayGuardRequestBody{ReadCloser: req.Body}
		req.Body = body
		defer func() {
			if resultErr != nil {
				resultErr = errors.Join(resultErr, body.Close())
			}
		}()
	}
	if req == nil || req.URL == nil || req.URL.Hostname() == "" || (req.URL.Scheme != "http" && req.URL.Scheme != "https") {
		return nil, ErrInvalidRequest
	}
	if req.Header == nil {
		req = req.Clone(req.Context())
		req.Header = make(http.Header)
	}
	var proxyURL *url.URL
	if t.transport.Proxy != nil {
		var err error
		proxyURL, err = t.transport.Proxy(req)
		if err != nil {
			return nil, err
		}
	}
	port := req.URL.Port()
	if port == "" {
		port = "80"
		if req.URL.Scheme == "https" {
			port = "443"
		}
	}
	address := net.JoinHostPort(req.URL.Hostname(), port)
	key := req.URL.Scheme + "://" + address
	if proxyURL != nil {
		key += "|" + proxyURL.String()
	}
	connection, route, err := t.acquire(req, key, address, proxyURL)
	if err != nil {
		return nil, err
	}
	// ClientConn.RoundTrip 不重新获取连接；一次调用只向该连接提交一次请求。
	response, err := connection.conn.RoundTrip(req)
	if err != nil {
		t.finished(route, connection)
		return nil, err
	}
	if response.Body == nil {
		t.finished(route, connection)
	} else {
		response.Body = &replayGuardBody{ReadCloser: response.Body, finish: func() { t.finished(route, connection) }}
	}
	return response, nil
}

func (t *replayGuardTransport) acquire(req *http.Request, key, address string, proxyURL *url.URL) (*replayGuardConn, *replayGuardRoute, error) {
	for {
		if err := req.Context().Err(); err != nil {
			return nil, nil, err
		}
		t.mu.Lock()
		route := t.routes[key]
		if route == nil {
			route = &replayGuardRoute{key: key, changed: make(chan struct{}, 1)}
			t.routes[key] = route
		}
		for i := 0; i < len(route.connections); {
			connection := route.connections[i]
			if connection.conn.Err() != nil {
				t.remove(route, i)
				continue
			}
			if connection.conn.Reserve() == nil {
				if connection.timer != nil {
					connection.timer.Stop()
					connection.timer = nil
				}
				connection.idleSince = time.Time{}
				t.mu.Unlock()
				return connection, route, nil
			}
			if connection.conn.InFlight() == 0 {
				t.remove(route, i)
				continue
			}
			i++
		}
		limit := t.transport.MaxConnsPerHost
		if limit > 0 && len(route.connections)+route.dialing >= limit {
			route.waiters++
			t.mu.Unlock()
			select {
			case <-req.Context().Done():
				t.mu.Lock()
				route.waiters--
				t.pruneRoute(route)
				t.mu.Unlock()
				return nil, nil, req.Context().Err()
			case <-route.changed:
				t.mu.Lock()
				route.waiters--
				t.pruneRoute(route)
				t.mu.Unlock()
				continue
			}
		}
		route.dialing++
		t.mu.Unlock()
		// 标准连接构造器仅接收地址；把实际请求选出的代理固定到本次拨号配置。
		dialer := t.transport.Clone()
		// ClientConn 不进入 Transport 的连接计数，数量限制由当前池独立维护。
		dialer.MaxConnsPerHost = 0
		dialer.Proxy = func(*http.Request) (*url.URL, error) { return proxyURL, nil }
		conn, err := dialer.NewClientConn(req.Context(), req.URL.Scheme, address)
		t.mu.Lock()
		route.dialing--
		if err != nil {
			t.pruneRoute(route)
			t.notify(route)
			t.mu.Unlock()
			return nil, nil, err
		}
		connection := &replayGuardConn{conn: conn}
		route.connections = append(route.connections, connection)
		// 回调仅通知等待者，不获取池锁，避免 Reserve/Close 的同步回调死锁。
		conn.SetStateHook(func(*http.ClientConn) { t.notify(route) })
		err = conn.Reserve()
		if err != nil {
			t.remove(route, len(route.connections)-1)
		}
		t.notify(route)
		t.mu.Unlock()
		if err != nil {
			return nil, nil, err
		}
		return connection, route, nil
	}
}

func (t *replayGuardTransport) notify(route *replayGuardRoute) {
	select {
	case route.changed <- struct{}{}:
	default:
	}
}

// 调用方持有池锁；等待者退出前仍拥有路由引用，不能提前另建同目标连接池。
func (t *replayGuardTransport) pruneRoute(route *replayGuardRoute) {
	if len(route.connections) == 0 && route.dialing == 0 && route.waiters == 0 && t.routes[route.key] == route {
		delete(t.routes, route.key)
	}
}

// remove 的调用方持有池锁；连接关闭的通知不会反向获取该锁。
func (t *replayGuardTransport) remove(route *replayGuardRoute, index int) {
	connection := route.connections[index]
	if connection.timer != nil {
		connection.timer.Stop()
	}
	_ = connection.conn.Close() //nolint:errcheck // 空闲或失效连接的关闭为尽力清理，接口没有错误返回通道。
	route.connections = append(route.connections[:index], route.connections[index+1:]...)
	t.notify(route)
}

func (t *replayGuardTransport) finished(route *replayGuardRoute, connection *replayGuardConn) {
	t.mu.Lock()
	defer func() {
		t.pruneRoute(route)
		t.mu.Unlock()
	}()
	t.notify(route)
	if connection.conn.InFlight() != 0 {
		return
	}
	connection.idleSince = time.Now()
	if t.transport.DisableKeepAlives {
		for i, candidate := range route.connections {
			if candidate == connection {
				t.remove(route, i)
				return
			}
		}
	}
	t.trimIdle()
	if t.transport.IdleConnTimeout > 0 && connection.conn.Err() == nil {
		if connection.timer != nil {
			connection.timer.Stop()
		}
		connection.timer = time.AfterFunc(t.transport.IdleConnTimeout, func() {
			t.mu.Lock()
			defer func() {
				t.pruneRoute(route)
				t.mu.Unlock()
			}()
			if connection.conn.InFlight() != 0 || time.Since(connection.idleSince) < t.transport.IdleConnTimeout {
				return
			}
			for i, candidate := range route.connections {
				if candidate == connection {
					t.remove(route, i)
					return
				}
			}
		})
	}
}

func (t *replayGuardTransport) trimIdle() {
	idleLimit := t.transport.MaxIdleConnsPerHost
	if idleLimit == 0 {
		idleLimit = http.DefaultMaxIdleConnsPerHost
	}
	total := 0
	for _, route := range t.routes {
		idle := 0
		for i := len(route.connections) - 1; i >= 0; i-- {
			connection := route.connections[i]
			if connection.conn.InFlight() != 0 {
				continue
			}
			idle++
			if idle > idleLimit {
				t.remove(route, i)
			} else {
				total++
			}
		}
		t.pruneRoute(route)
	}
	for t.transport.MaxIdleConns > 0 && total > t.transport.MaxIdleConns {
		var oldest *replayGuardConn
		var oldestRoute *replayGuardRoute
		oldestIndex := 0
		for _, route := range t.routes {
			for i, connection := range route.connections {
				if connection.conn.InFlight() == 0 && (oldest == nil || connection.idleSince.Before(oldest.idleSince)) {
					oldest, oldestRoute, oldestIndex = connection, route, i
				}
			}
		}
		if oldest == nil {
			break
		}
		t.remove(oldestRoute, oldestIndex)
		t.pruneRoute(oldestRoute)
		total--
	}
}

func (t *replayGuardTransport) CloseIdleConnections() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, route := range t.routes {
		for i := len(route.connections) - 1; i >= 0; i-- {
			if route.connections[i].conn.InFlight() == 0 {
				t.remove(route, i)
			}
		}
		t.pruneRoute(route)
	}
}

type replayGuardBody struct {
	io.ReadCloser
	once   sync.Once
	finish func()
}

type replayGuardRequestBody struct {
	io.ReadCloser
	once sync.Once
	err  error
}

func (b *replayGuardRequestBody) Close() error {
	b.once.Do(func() { b.err = b.ReadCloser.Close() })
	return b.err
}

func (b *replayGuardBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		b.once.Do(b.finish)
	}
	return n, err
}

func (b *replayGuardBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.finish)
	return err
}
