package clickhouse

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"testing"
	"time"

	drivercfg "github.com/ClickHouse/clickhouse-go/v2"
)

func TestConfigValidationAndOptions(t *testing.T) {
	if err := (&Config{Database: "default"}).Validate(); !errors.Is(err, ErrEmptyAddrs) {
		t.Fatalf("Validate() error = %v, want ErrEmptyAddrs", err)
	}
	if err := (&Config{Addrs: []string{"localhost:9000"}}).Validate(); !errors.Is(err, ErrEmptyDatabase) {
		t.Fatalf("Validate() error = %v, want ErrEmptyDatabase", err)
	}

	cfg := DefaultConfig().Apply(
		WithAddrs("db.internal:9440"),
		WithDatabase("analytics"),
		WithAuth("reader", "secret"),
		WithMaxOpenConns(17),
		WithMaxIdleConns(7),
		WithDialTimeout(4*time.Second),
		WithCompression("zstd"),
		WithTLS(),
		nil,
	)
	cfg.ReadTimeout = 9 * time.Second

	opts := buildClickHouseOptions(cfg)
	if opts.ReadTimeout != cfg.ReadTimeout {
		t.Fatalf("ReadTimeout = %v, want %v", opts.ReadTimeout, cfg.ReadTimeout)
	}
	if opts.Compression == nil || opts.Compression.Method != drivercfg.CompressionZSTD {
		t.Fatalf("Compression = %+v, want ZSTD", opts.Compression)
	}
	if opts.TLS == nil || opts.TLS.MinVersion != tls.VersionTLS12 || opts.TLS.InsecureSkipVerify {
		t.Fatalf("TLS config = %+v, want verified TLS 1.2+", opts.TLS)
	}
	if got := cfg.String(); got == "" || strings.Contains(got, "secret") {
		t.Fatalf("Config.String() leaked a credential: %q", got)
	}
}

func TestNewRejectsNilContext(t *testing.T) {
	var nilContext context.Context
	_, err := New(nilContext, DefaultConfig())
	if err == nil || !strings.Contains(err.Error(), "context must not be nil") {
		t.Fatalf("New(nil, ...) error = %v", err)
	}
}

func TestConfigCloneDoesNotAliasMutableFields(t *testing.T) {
	original := DefaultConfig()
	clone := cloneConfig(original)
	clone.Addrs[0] = "changed:9000"
	clone.Settings["max_execution_time"] = 1
	if original.Addrs[0] == clone.Addrs[0] || original.Settings["max_execution_time"] == 1 {
		t.Fatal("cloneConfig() aliases mutable configuration fields")
	}
}
