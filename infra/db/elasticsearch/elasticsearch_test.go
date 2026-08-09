package elasticsearch

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConfigValidationAndOptions(t *testing.T) {
	if err := (&Config{}).Validate(); !errors.Is(err, ErrEmptyAddrs) {
		t.Fatalf("Validate() error = %v, want ErrEmptyAddrs", err)
	}
	cfg := DefaultConfig().Apply(
		WithAddresses("https://search.internal:9200"),
		WithBasicAuth("reader", "secret"),
		WithAPIKey("api-secret"),
		WithMaxRetries(5),
		WithTimeout(7*time.Second),
		WithCACert("ca.pem"),
		WithDebugLogger(true),
		WithCompression(true),
		nil,
	)
	if cfg.MaxRetries != 5 || cfg.RequestTimeout != 7*time.Second || !cfg.EnableDebugLogger || !cfg.CompressRequestBody {
		t.Fatalf("Apply() result = %+v", cfg)
	}
	if got := cfg.String(); strings.Contains(got, "secret") {
		t.Fatalf("Config.String() leaked a credential: %q", got)
	}
}

func TestNewUsesVerifiedTLSAndParsesResponses(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Elastic-Product", "Elasticsearch")
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodHead {
			writer.WriteHeader(http.StatusOK)
			return
		}
		switch request.URL.Path {
		case "/":
			_, _ = writer.Write([]byte(`{"name":"node-a","cluster_name":"cluster-a","cluster_uuid":"uuid-a","version":{"number":"8.0.0"}}`))
		case "/_cluster/health":
			_, _ = writer.Write([]byte(`{"cluster_name":"cluster-a","status":"green","number_of_nodes":1}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	certificatePath := filepath.Join(t.TempDir(), "ca.pem")
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(certificatePath, certificate, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig().Apply(WithAddresses(server.URL), WithCACert(certificatePath))
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if client.transport == nil || client.transport.TLSClientConfig == nil {
		t.Fatal("New() did not retain the configured HTTP transport")
	}
	if client.transport.TLSClientConfig.MinVersion != tls.VersionTLS12 || client.transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("TLS config = %+v, want verified TLS 1.2+", client.transport.TLSClientConfig)
	}
	cfg.Addresses[0] = "https://mutated.invalid"
	snapshot := client.Config()
	snapshot.Addresses[0] = "https://snapshot.invalid"
	if client.Config().Addresses[0] != server.URL {
		t.Fatal("client configuration aliases caller or snapshot slices")
	}
	info, err := client.InfoParsed(context.Background())
	if err != nil || info.ClusterName != "cluster-a" {
		t.Fatalf("InfoParsed() = (%+v, %v)", info, err)
	}
	health, err := client.HealthParsed(context.Background())
	if err != nil || health.Status != "green" {
		t.Fatalf("HealthParsed() = (%+v, %v)", health, err)
	}
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := client.Info(context.Background()); !errors.Is(err, ErrAlreadyClosed) {
		t.Fatalf("Info() after Close error = %v", err)
	}
}

func TestGlobalLifecycleIsConcurrentSafe(t *testing.T) {
	if err := Reset(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Reset() })
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Elastic-Product", "Elasticsearch")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"name":"node-a","cluster_name":"cluster-a","version":{"number":"8.0.0"}}`))
	}))
	defer server.Close()
	cfg := DefaultConfig().Apply(WithAddresses(server.URL))

	const workers = 24
	errorsChannel := make(chan error, workers)
	done := make(chan struct{}, workers)
	for index := 0; index < workers; index++ {
		go func(index int) {
			if index%3 == 0 {
				errorsChannel <- Close()
			} else {
				errorsChannel <- Init(cfg)
				_ = GetClient()
			}
			done <- struct{}{}
		}(index)
	}
	for range workers {
		<-done
	}
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil && !errors.Is(err, ErrAlreadyClosed) {
			t.Fatalf("concurrent lifecycle error = %v", err)
		}
	}
}

func TestConfigCloneDoesNotAliasSlices(t *testing.T) {
	original := DefaultConfig()
	clone := cloneConfig(original)
	clone.Addresses[0] = "http://changed:9200"
	clone.RetryOnStatus[0] = 500
	if original.Addresses[0] == clone.Addresses[0] || original.RetryOnStatus[0] == 500 {
		t.Fatal("cloneConfig() aliases mutable configuration fields")
	}
}

func TestNewClosesClientWhenConnectionVerificationFails(t *testing.T) {
	connectionClosed := make(chan struct{})
	var closeOnce sync.Once
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Elastic-Product", "Elasticsearch")
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte(`{"error":"unavailable"}`))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closeOnce.Do(func() { close(connectionClosed) })
		}
	}
	server.Start()
	defer server.Close()

	client, err := New(DefaultConfig().Apply(WithAddresses(server.URL)))
	if client != nil || err == nil {
		t.Fatalf("New() = (%v, %v), want nil client and verification error", client, err)
	}
	select {
	case <-connectionClosed:
	case <-time.After(time.Second):
		t.Fatal("New() returned without closing the failed client's connection")
	}
}

func TestClientCloseClosesIdleConnections(t *testing.T) {
	type connectionContextKey struct{}
	connections := make(chan net.Conn, 2)
	stateChanged := make(chan struct{}, 16)
	states := make(map[net.Conn]http.ConnState)
	var statesMu sync.Mutex
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connections <- request.Context().Value(connectionContextKey{}).(net.Conn)
		writer.Header().Set("X-Elastic-Product", "Elasticsearch")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"name":"node-a","cluster_name":"cluster-a","version":{"number":"8.0.0"}}`))
	}))
	server.Config.ConnContext = func(ctx context.Context, connection net.Conn) context.Context {
		return context.WithValue(ctx, connectionContextKey{}, connection)
	}
	server.Config.ConnState = func(connection net.Conn, state http.ConnState) {
		statesMu.Lock()
		states[connection] = state
		statesMu.Unlock()
		select {
		case stateChanged <- struct{}{}:
		default:
		}
	}
	server.Start()
	defer server.Close()

	client, err := New(DefaultConfig().Apply(WithAddresses(server.URL)))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	<-connections
	if _, err := client.InfoParsed(context.Background()); err != nil {
		t.Fatalf("InfoParsed() error = %v", err)
	}
	targetConnection := <-connections
	waitForConnectionState := func(want http.ConnState) bool {
		deadline := time.NewTimer(time.Second)
		defer deadline.Stop()
		for {
			statesMu.Lock()
			got := states[targetConnection]
			statesMu.Unlock()
			if got == want {
				return true
			}
			select {
			case <-stateChanged:
			case <-deadline.C:
				return false
			}
		}
	}
	if !waitForConnectionState(http.StateIdle) {
		t.Fatal("connection did not become idle before Close")
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !waitForConnectionState(http.StateClosed) {
		t.Fatal("Close() did not close the underlying idle connection")
	}
}
