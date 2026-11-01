package httpx

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

func TestReplayGuardReusesAndClosesConnections(t *testing.T) {
	for _, test := range []struct {
		name        string
		http2       bool
		idleLimit   int
		reusedDials int32
	}{
		{name: "http1", reusedDials: 1},
		{name: "http2", http2: true, reusedDials: 1},
		{name: "http1-idle-disabled", idleLimit: -1, reusedDials: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			useHTTP2 := test.http2
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if useHTTP2 && r.ProtoMajor != 2 {
					t.Errorf("protocol = %s, want HTTP/2", r.Proto)
				}
				if _, err := io.WriteString(w, "ok"); err != nil {
					t.Error(err)
				}
			}))
			server.EnableHTTP2 = useHTTP2
			server.StartTLS()
			defer server.Close()
			baseTransport, ok := server.Client().Transport.(*http.Transport)
			if !ok {
				t.Fatal("unexpected test transport")
			}
			base := baseTransport.Clone()
			base.ForceAttemptHTTP2 = useHTTP2
			base.MaxIdleConnsPerHost = test.idleLimit
			var dials atomic.Int32
			dialer := &net.Dialer{}
			base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				dials.Add(1)
				return dialer.DialContext(ctx, network, address)
			}
			transport, err := NewReplayGuardTransport(base)
			if err != nil {
				t.Fatal(err)
			}
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			defer client.CloseIdleConnections()
			get := func() {
				t.Helper()
				response, err := replayGuardGet(t, client, server.URL)
				if err != nil {
					t.Fatal(err)
				}
				body, readErr := io.ReadAll(response.Body)
				closeErr := response.Body.Close()
				if readErr != nil || closeErr != nil || string(body) != "ok" {
					t.Fatalf("body=%q read=%v close=%v", body, readErr, closeErr)
				}
			}
			get()
			get()
			if got := dials.Load(); got != test.reusedDials {
				t.Fatalf("dials=%d, want %d", got, test.reusedDials)
			}
			client.CloseIdleConnections()
			get()
			if got := dials.Load(); got != test.reusedDials+1 {
				t.Fatalf("dials=%d after close, want %d", got, test.reusedDials+1)
			}
		})
	}
}

func TestReplayGuardDoesNotReplayHTTP1Request(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 2 {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Error("HTTP/1 hijacking unavailable")
				return
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			if err := conn.Close(); err != nil {
				t.Error(err)
			}
			return
		}
		if _, err := io.WriteString(w, "ok"); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	transport, err := NewReplayGuardTransport(&http.Transport{})
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	response, err := replayGuardGet(t, client, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, copyErr := io.Copy(io.Discard, response.Body); copyErr != nil {
		t.Fatal(copyErr)
	}
	if closeErr := response.Body.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	response, err = replayGuardGet(t, client, server.URL)
	if response != nil {
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}
	if err == nil {
		t.Fatal("request accepted then disconnected: want an error, not a replay")
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests=%d, want warmup plus one attempt", got)
	}
}

func TestReplayGuardDoesNotReplayHTTP2RefusedStream(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("unexpected HTTP/1 request")
	}))
	server.EnableHTTP2 = true
	server.Config.TLSNextProto = map[string]func(*http.Server, *tls.Conn, http.Handler){
		"h2": func(_ *http.Server, conn *tls.Conn, _ http.Handler) {
			defer func() {
				if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
					t.Error(err)
				}
			}()
			if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Error(err)
				return
			}
			preface := make([]byte, len(http2.ClientPreface))
			if _, err := io.ReadFull(conn, preface); err != nil {
				return
			}
			framer := http2.NewFramer(conn, conn)
			if err := framer.WriteSettings(); err != nil {
				return
			}
			for {
				frame, err := framer.ReadFrame()
				if err != nil {
					return
				}
				switch f := frame.(type) {
				case *http2.SettingsFrame:
					if !f.IsAck() {
						if err := framer.WriteSettingsAck(); err != nil {
							return
						}
					}
				case *http2.HeadersFrame:
					requests.Add(1)
					if err := framer.WriteRSTStream(f.StreamID, http2.ErrCodeRefusedStream); err != nil {
						return
					}
				}
			}
		},
	}
	server.StartTLS()
	defer server.Close()
	baseTransport, ok := server.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatal("unexpected test transport")
	}
	base := baseTransport.Clone()
	base.ForceAttemptHTTP2 = true
	transport, err := NewReplayGuardTransport(base)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	response, err := replayGuardGet(t, client, server.URL)
	if response != nil {
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}
	if err == nil {
		t.Fatal("REFUSED_STREAM must be returned without replay")
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests=%d, want exactly one attempt", got)
	}
}

func TestReplayGuardProxyUsesActualRequest(t *testing.T) {
	var selected atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "origin.invalid" {
			t.Errorf("proxy target=%s", r.URL.Host)
		}
		if _, err := io.WriteString(w, "proxy"); err != nil {
			t.Error(err)
		}
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := NewReplayGuardTransport(&http.Transport{Proxy: func(r *http.Request) (*url.URL, error) {
		if r.Method != http.MethodPost || r.Header.Get("X-Route") != "proxy" {
			t.Errorf("proxy received synthetic request: %s %v", r.Method, r.Header)
		}
		selected.Add(1)
		return proxyURL, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://origin.invalid/path", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Route", "proxy")
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	if closeErr := response.Body.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil || string(body) != "proxy" || selected.Load() != 1 {
		t.Fatalf("body=%q err=%v selections=%d", body, err, selected.Load())
	}
}

func TestReplayGuardConnectionLimitAndActiveClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "2")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("HTTP flushing unavailable")
			return
		}
		flusher.Flush()
		if _, err := io.WriteString(w, "ok"); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	transport, err := NewReplayGuardTransport(&http.Transport{MaxConnsPerHost: 1})
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	response, err := replayGuardGet(t, client, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	blocked, err := client.Do(req)
	if blocked != nil {
		if closeErr := blocked.Body.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("connection limit error=%v, want deadline exceeded", err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "ok" {
		t.Fatalf("active response interrupted: body=%q err=%v", body, err)
	}
}

func replayGuardGet(t *testing.T, client *http.Client, address string) (*http.Response, error) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	return client.Do(request)
}

func TestReplayGuardIdleExpiryReleasesRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	transport, err := NewReplayGuardTransport(&http.Transport{IdleConnTimeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: transport, Timeout: time.Second}
	defer client.CloseIdleConnections()
	response, err := replayGuardGet(t, client, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := io.Copy(io.Discard, response.Body)
	if err := errors.Join(readErr, response.Body.Close()); err != nil {
		t.Fatal(err)
	}
	pool := transport.(*replayGuardTransport)
	deadline := time.Now().Add(time.Second)
	for {
		pool.mu.Lock()
		empty := len(pool.routes) == 0
		pool.mu.Unlock()
		if empty {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("idle route was not released")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestReplayGuardQueuedRequestSurvivesRouteCleanup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	transport, err := NewReplayGuardTransport(&http.Transport{MaxConnsPerHost: 1, DisableKeepAlives: true})
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: transport, Timeout: time.Second}
	defer client.CloseIdleConnections()
	first, err := replayGuardGet(t, client, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Body.Close()
	second := make(chan error, 1)
	go func() {
		request, requestErr := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
		if requestErr != nil {
			second <- requestErr
			return
		}
		response, err := client.Do(request)
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			err = errors.Join(readErr, response.Body.Close())
			if err == nil && string(body) != "ok" {
				err = errors.New("unexpected response body")
			}
		}
		second <- err
	}()
	pool := transport.(*replayGuardTransport)
	deadline := time.Now().Add(time.Second)
	for {
		pool.mu.Lock()
		waiting := false
		for _, route := range pool.routes {
			waiting = waiting || route.waiters == 1
		}
		pool.mu.Unlock()
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second request did not enter the queue")
		}
		time.Sleep(time.Millisecond)
	}
	if err := first.Body.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-second:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued request did not complete")
	}
	client.CloseIdleConnections()
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if len(pool.routes) != 0 {
		t.Fatal("completed routes were retained")
	}
}
