package mongodb

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestConfigValidationOptionsAndMasking(t *testing.T) {
	if err := (&Config{Database: "app"}).Validate(); !errors.Is(err, ErrEmptyURI) {
		t.Fatalf("Validate() error = %v, want ErrEmptyURI", err)
	}
	if err := (&Config{URI: "mongodb://localhost:27017"}).Validate(); !errors.Is(err, ErrEmptyDatabase) {
		t.Fatalf("Validate() error = %v, want ErrEmptyDatabase", err)
	}

	cfg := DefaultConfig().Apply(
		WithURI("mongodb://user:highly-secret@db.internal:27017/app?authSource=admin"),
		WithDatabase("app"),
		WithMaxPoolSize(25),
		WithMinPoolSize(3),
		WithConnectTimeout(4*time.Second),
		WithAuth("reader", "another-secret", "admin"),
		WithAppName("toolkit-test"),
		WithReadPreference("secondaryPreferred"),
		nil,
	)
	if got := cfg.String(); strings.Contains(got, "highly-secret") || strings.Contains(got, "user:") {
		t.Fatalf("Config.String() leaked URI credentials: %q", got)
	}

	opts := buildClientOptions(cfg)
	if opts.MaxPoolSize == nil || *opts.MaxPoolSize != 25 || opts.MinPoolSize == nil || *opts.MinPoolSize != 3 {
		t.Fatalf("pool options = max:%v min:%v", opts.MaxPoolSize, opts.MinPoolSize)
	}
	if opts.ReadPreference == nil || opts.ReadPreference.Mode().String() != "secondaryPreferred" {
		t.Fatalf("ReadPreference = %v", opts.ReadPreference)
	}
	if opts.Auth == nil || opts.Auth.Username != "reader" || opts.Auth.Password != "another-secret" {
		t.Fatalf("Auth options = %+v", opts.Auth)
	}
}

func TestNewRejectsNilContext(t *testing.T) {
	var nilContext context.Context
	_, err := New(nilContext, DefaultConfig())
	if err == nil || !strings.Contains(err.Error(), "context must not be nil") {
		t.Fatalf("New(nil, ...) error = %v", err)
	}
}

func TestOptionalTimeoutDoesNotExpireZeroDuration(t *testing.T) {
	ctx, cancel := contextWithOptionalTimeout(context.Background(), 0)
	defer cancel()
	select {
	case <-ctx.Done():
		t.Fatalf("zero timeout context ended immediately: %v", ctx.Err())
	default:
	}
}

func TestConfigCloneDoesNotAliasCompressors(t *testing.T) {
	original := DefaultConfig()
	original.Compressors = []string{"zstd"}
	clone := cloneConfig(original)
	clone.Compressors[0] = "snappy"
	if original.Compressors[0] == clone.Compressors[0] {
		t.Fatal("cloneConfig() aliases Compressors")
	}
}
