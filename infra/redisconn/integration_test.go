package redisconn

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestOpenAgainstExternalNamedACL(t *testing.T) {
	addr := os.Getenv("REDISCONN_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDISCONN_TEST_ADDR to run against an external Redis")
	}
	username := os.Getenv("REDISCONN_TEST_USERNAME")
	password := os.Getenv("REDISCONN_TEST_PASSWORD")
	if username == "" || password == "" {
		t.Fatal("REDISCONN_TEST_USERNAME and REDISCONN_TEST_PASSWORD must both be non-empty")
	}

	factory := mustFactory(t, Config{
		Mode:            ModeSingle,
		Addrs:           []string{addr},
		DataCredentials: Credentials{Username: username, Password: password},
		MaxRetries:      -1,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := factory.Open(ctx)
	if err != nil {
		t.Fatalf("Open() against external named ACL Redis error = %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	key := fmt.Sprintf("redisconn:external-acl:%d", time.Now().UnixNano())
	if err := client.Set(ctx, key, "verified", time.Minute).Err(); err != nil {
		t.Fatalf("authenticated SET against external Redis error = %v", err)
	}
	t.Cleanup(func() { _ = client.Del(context.Background(), key).Err() })
	if value, err := client.Get(ctx, key).Result(); err != nil || value != "verified" {
		t.Fatalf("authenticated GET against external Redis = (%q, %v), want verified", value, err)
	}
}

func TestOpenAuthenticatesWithNamedACLStaticCredentials(t *testing.T) {
	server := miniredis.RunT(t)
	server.RequireUserAuth("service-user", "correct-password")

	factory := mustFactory(t, Config{
		Mode:            ModeSingle,
		Addrs:           []string{server.Addr()},
		DataCredentials: Credentials{Username: "service-user", Password: "correct-password"},
		MaxRetries:      -1,
		MinRetryBackoff: -1,
		MaxRetryBackoff: -1,
	})
	client, err := factory.Open(context.Background())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if err := client.Set(context.Background(), "acl-key", "value", 0).Err(); err != nil {
		t.Fatalf("authenticated SET failed: %v", err)
	}
}

func TestOpenRejectsWrongNamedACLCredentialsAndClosesClient(t *testing.T) {
	tests := []struct {
		name        string
		credentials Credentials
	}{
		{
			name:        "wrong username",
			credentials: Credentials{Username: "wrong-user-must-not-leak", Password: "correct-password"},
		},
		{
			name:        "wrong password",
			credentials: Credentials{Username: "service-user", Password: "wrong-password-must-not-leak"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := miniredis.RunT(t)
			server.RequireUserAuth("service-user", "correct-password")
			factory := mustFactory(t, Config{
				Mode:            ModeSingle,
				Addrs:           []string{server.Addr()},
				DataCredentials: test.credentials,
				MaxRetries:      -1,
			})

			client, err := factory.Open(context.Background())
			if err == nil {
				if client != nil {
					_ = client.Close()
				}
				t.Fatal("Open() error = nil, want authentication failure")
			}
			if client != nil {
				t.Fatal("Open() returned a client after Ping failed")
			}
			for _, secret := range []string{"wrong-user-must-not-leak", "wrong-password-must-not-leak", "correct-password"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("Open() leaked credential %q: %q", secret, err)
				}
			}
			if server.TotalConnectionCount() == 0 {
				t.Fatal("Open() did not attempt a Redis connection")
			}
			assertEventuallyNoConnections(t, server)
		})
	}
}

func TestOpenAuthenticatesWithDynamicNamedACLCredentials(t *testing.T) {
	server := miniredis.RunT(t)
	server.RequireUserAuth("rotating-user", "rotating-password")
	factory := mustFactory(t, Config{
		Mode:  ModeSingle,
		Addrs: []string{server.Addr()},
		CredentialsProvider: func(context.Context) (Credentials, error) {
			return Credentials{Username: "rotating-user", Password: "rotating-password"}, nil
		},
	})

	client, err := factory.Open(context.Background())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
}

func TestOpenReadsDynamicCredentialsAgainForEachClient(t *testing.T) {
	server := miniredis.RunT(t)
	credentials := Credentials{Username: "first-user", Password: "first-password"}
	server.RequireUserAuth(credentials.Username, credentials.Password)
	factory := mustFactory(t, Config{
		Mode:  ModeSingle,
		Addrs: []string{server.Addr()},
		CredentialsProvider: func(context.Context) (Credentials, error) {
			return credentials, nil
		},
	})

	firstClient, err := factory.Open(context.Background())
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	if closeErr := firstClient.Close(); closeErr != nil {
		t.Fatalf("first Close() error = %v", closeErr)
	}
	assertEventuallyNoConnections(t, server)

	server.RequireUserAuth("first-user", "")
	credentials = Credentials{Username: "second-user", Password: "second-password"}
	server.RequireUserAuth(credentials.Username, credentials.Password)
	secondClient, err := factory.Open(context.Background())
	if err != nil {
		t.Fatalf("second Open() after credential rotation error = %v", err)
	}
	t.Cleanup(func() { _ = secondClient.Close() })
}

func TestOpenRejectsNilContextWithoutConnecting(t *testing.T) {
	server := miniredis.RunT(t)
	factory := mustFactory(t, Config{Mode: ModeSingle, Addrs: []string{server.Addr()}})

	client, err := factory.Open(nil) //nolint:staticcheck // Deliberately verify the nil-context guard.
	if client != nil {
		_ = client.Close()
		t.Fatal("Open(nil) returned a client")
	}
	if !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("Open(nil) error = %v, want ErrInvalidContext", err)
	}
	if server.TotalConnectionCount() != 0 {
		t.Fatal("Open(nil) attempted a Redis connection")
	}
}

func TestOpenRejectsInvalidDynamicCredentialsWithoutLeakingThem(t *testing.T) {
	server := miniredis.RunT(t)
	server.RequireUserAuth("service-user", "correct-password")
	factory := mustFactory(t, Config{
		Mode:  ModeSingle,
		Addrs: []string{server.Addr()},
		CredentialsProvider: func(context.Context) (Credentials, error) {
			return Credentials{Password: "runtime-password-must-not-leak"}, nil
		},
	})

	client, err := factory.Open(context.Background())
	if client != nil {
		_ = client.Close()
		t.Fatal("Open() returned a client for incomplete dynamic credentials")
	}
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Open() error = %v, want ErrInvalidCredentials", err)
	}
	if strings.Contains(err.Error(), "runtime-password-must-not-leak") {
		t.Fatalf("Open() leaked dynamic credential: %q", err)
	}
	assertEventuallyNoConnections(t, server)
}

func assertEventuallyNoConnections(t *testing.T, server *miniredis.Miniredis) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for server.CurrentConnectionCount() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := server.CurrentConnectionCount(); got != 0 {
		t.Fatalf("Open() left %d connection(s) open after failure or Close", got)
	}
}
