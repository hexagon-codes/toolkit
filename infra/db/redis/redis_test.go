package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func setupMiniRedis(t *testing.T) (*miniredis.Miniredis, *Client) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}

	cfg := DefaultConfig(ModeSingle, mr.Addr())
	cfg.DialTimeout = time.Second
	client, err := New(context.Background(), cfg)
	if err != nil {
		mr.Close()
		t.Fatalf("create redis client: %v", err)
	}

	t.Cleanup(func() {
		_ = client.Close()
		mr.Close()
	})
	return mr, client
}

func TestNewOpensHealthyConnection(t *testing.T) {
	_, client := setupMiniRedis(t)
	if client == nil || client.UniversalClient == nil {
		t.Fatal("New returned a nil client")
	}
	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health() error = %v", err)
	}
}

func TestNewRejectsNilContext(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	client, err := New(nil, DefaultConfig(ModeSingle, mr.Addr())) //nolint:staticcheck // Deliberately verify the nil-context guard.
	if !errors.Is(err, ErrInvalidContext) {
		if client != nil {
			_ = client.Close()
		}
		t.Fatalf("New(nil, config) error = %v, want errors.Is(ErrInvalidContext)", err)
	}
	if client != nil {
		_ = client.Close()
		t.Fatal("New(nil, config) returned a client")
	}
}

func TestNewUsesCallerContextForProbe(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client, err := New(ctx, DefaultConfig(ModeSingle, mr.Addr()))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("New(canceled context) error = %v, want context.Canceled", err)
	}
	if client != nil {
		_ = client.Close()
		t.Fatal("New(canceled context) returned a client")
	}
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	client, err := New(context.Background(), Config{})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("New() error = %v, want errors.Is(ErrInvalidConfig)", err)
	}
	if client != nil {
		_ = client.Close()
		t.Fatal("New() returned a client for invalid config")
	}
}

func TestNewRejectsIncompleteACLCredentials(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	cfg := DefaultConfig(ModeSingle, mr.Addr())
	cfg.DataCredentials = Credentials{Password: "password-only-is-not-supported"}
	client, err := New(context.Background(), cfg)
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("New() error = %v, want errors.Is(ErrInvalidCredentials)", err)
	}
	if client != nil {
		_ = client.Close()
		t.Fatal("New() returned a client for incomplete ACL credentials")
	}
}

func TestNewAuthenticatesNamedACLUser(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	mr.RequireUserAuth("application", "correct-secret")

	cfg := DefaultConfig(ModeSingle, mr.Addr())
	cfg.DataCredentials = Credentials{Username: "application", Password: "correct-secret"}
	client, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()

	if err := client.Set(context.Background(), "acl-key", "value", 0).Err(); err != nil {
		t.Fatalf("authenticated Set() error = %v", err)
	}
}

func TestNewRejectsWrongNamedACLCredentials(t *testing.T) {
	tests := []struct {
		name        string
		credentials Credentials
	}{
		{
			name:        "wrong username",
			credentials: Credentials{Username: "wrong-user", Password: "correct-secret"},
		},
		{
			name:        "wrong password",
			credentials: Credentials{Username: "application", Password: "wrong-secret"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mr, err := miniredis.Run()
			if err != nil {
				t.Fatalf("start miniredis: %v", err)
			}
			defer mr.Close()
			mr.RequireUserAuth("application", "correct-secret")

			cfg := DefaultConfig(ModeSingle, mr.Addr())
			cfg.DataCredentials = test.credentials
			client, err := New(context.Background(), cfg)
			if err == nil {
				if client != nil {
					_ = client.Close()
				}
				t.Fatal("New() error = nil")
			}
			if client != nil {
				_ = client.Close()
				t.Fatal("New() returned a client with invalid ACL credentials")
			}
		})
	}
}

func TestNewCreatesIndependentClientLifecycles(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	cfg := DefaultConfig(ModeSingle, mr.Addr())
	first, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("first New() error = %v", err)
	}
	second, err := New(context.Background(), cfg)
	if err != nil {
		_ = first.Close()
		t.Fatalf("second New() error = %v", err)
	}
	defer second.Close()

	if first.UniversalClient == second.UniversalClient {
		t.Fatal("New reused a process-global Redis client")
	}
	if err := first.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := second.Set(context.Background(), "independent", "alive", 0).Err(); err != nil {
		t.Fatalf("closing first client affected second client: %v", err)
	}
}

func TestHealthRejectsNilContext(t *testing.T) {
	_, client := setupMiniRedis(t)
	if err := client.Health(nil); !errors.Is(err, ErrInvalidContext) { //nolint:staticcheck // Deliberately verify the nil-context guard.
		t.Fatalf("Health(nil) error = %v, want errors.Is(ErrInvalidContext)", err)
	}
}

func TestHealthWithNilClient(t *testing.T) {
	var client *Client
	if err := client.Health(context.Background()); err == nil {
		t.Fatal("nil Client.Health() error = nil")
	}
}

func TestGetWithDefaultDistinguishesMissingKeyFromFailure(t *testing.T) {
	_, client := setupMiniRedis(t)
	ctx := context.Background()
	if err := client.Set(ctx, "present", "value", 0).Err(); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, err := client.GetWithDefault(ctx, "present", "fallback")
	if err != nil || got != "value" {
		t.Fatalf("GetWithDefault(present) = %q, %v; want value, nil", got, err)
	}
	got, err = client.GetWithDefault(ctx, "missing", "fallback")
	if err != nil || got != "fallback" {
		t.Fatalf("GetWithDefault(missing) = %q, %v; want fallback, nil", got, err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	got, err = client.GetWithDefault(canceled, "present", "fallback")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("GetWithDefault(canceled) = %q, %v; want context.Canceled", got, err)
	}
}

func TestClientValueConvenienceMethods(t *testing.T) {
	_, client := setupMiniRedis(t)
	ctx := context.Background()

	if err := client.MSetValues(ctx, "one", "1", "two", "2"); err != nil {
		t.Fatalf("MSetValues() error = %v", err)
	}
	values, err := client.MGetValues(ctx, "one", "two", "missing")
	if err != nil {
		t.Fatalf("MGetValues() error = %v", err)
	}
	if len(values) != 3 || values[0] != "1" || values[1] != "2" || values[2] != nil {
		t.Fatalf("MGetValues() = %#v", values)
	}
	count, err := client.ExistsCount(ctx, "one", "two", "missing")
	if err != nil || count != 2 {
		t.Fatalf("ExistsCount() = %d, %v; want 2, nil", count, err)
	}
	if err := client.DeleteKeys(ctx, "one", "two"); err != nil {
		t.Fatalf("DeleteKeys() error = %v", err)
	}
}

func TestClientConditionalAndIncrementConvenienceMethods(t *testing.T) {
	_, client := setupMiniRedis(t)
	ctx := context.Background()

	ok, err := client.SetNX(ctx, "conditional", "first", time.Minute)
	if err != nil || !ok {
		t.Fatalf("first SetNX() = %t, %v; want true, nil", ok, err)
	}
	ok, err = client.SetNXEx(ctx, "conditional", "second", time.Minute)
	if err != nil || ok {
		t.Fatalf("second SetNXEx() = %t, %v; want false, nil", ok, err)
	}

	value, err := client.IncrByWithExpire(ctx, "counter", 5, time.Minute)
	if err != nil || value != 5 {
		t.Fatalf("IncrByWithExpire() = %d, %v; want 5, nil", value, err)
	}
	ttl, err := client.GetTTL(ctx, "counter")
	if err != nil || ttl <= 0 {
		t.Fatalf("GetTTL(counter) = %s, %v; want positive TTL", ttl, err)
	}
}

func TestClientExpirationConvenienceMethods(t *testing.T) {
	_, client := setupMiniRedis(t)
	ctx := context.Background()

	if err := client.SetWithExpire(ctx, "expiring", "value", time.Minute); err != nil {
		t.Fatalf("SetWithExpire() error = %v", err)
	}
	if ttl, err := client.GetTTL(ctx, "expiring"); err != nil || ttl <= 0 {
		t.Fatalf("GetTTL(expiring) = %s, %v; want positive TTL", ttl, err)
	}

	if err := client.Set(ctx, "expire-at", "value", 0).Err(); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := client.SetExpireAt(ctx, "expire-at", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SetExpireAt() error = %v", err)
	}
	if ttl, err := client.GetTTL(ctx, "expire-at"); err != nil || ttl <= 0 || ttl > time.Hour {
		t.Fatalf("GetTTL(expire-at) = %s, %v; want (0, 1h]", ttl, err)
	}
}

func TestClientCloseAndStatsNilSafety(t *testing.T) {
	var client *Client
	if err := client.Close(); err != nil {
		t.Fatalf("nil Client.Close() error = %v", err)
	}
	if stats := client.Stats(); stats != nil {
		t.Fatalf("nil Client.Stats() = %#v, want nil", stats)
	}

	_, live := setupMiniRedis(t)
	if stats := live.Stats(); stats == nil {
		t.Fatal("live Client.Stats() = nil")
	}
}
