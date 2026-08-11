package redisconn

import (
	"context"
	"crypto/tls"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	redis "github.com/redis/go-redis/v9"
)

func TestNewFactoryValidatesNormalizesAndSnapshotsConfig(t *testing.T) {
	invalidFactory, err := NewFactory(Config{
		Mode:            ModeSingle,
		Addrs:           []string{"redis:6379"},
		DataCredentials: Credentials{Password: "password-only-must-not-leak"},
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("NewFactory() error = %v, want ErrInvalidCredentials", err)
	}
	if invalidFactory != nil {
		t.Fatal("NewFactory() returned a factory for invalid config")
	}
	if strings.Contains(err.Error(), "password-only-must-not-leak") {
		t.Fatalf("NewFactory() leaked a credential: %q", err)
	}

	addrs := []string{"redis.internal:6379"}
	tlsConfig := &tls.Config{ServerName: "redis.internal", NextProtos: []string{"redis"}}
	factory, err := NewFactory(Config{Mode: ModeSingle, Addrs: addrs, TLSConfig: tlsConfig})
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}

	addrs[0] = "mutated:6379"
	tlsConfig.ServerName = "mutated.invalid"
	tlsConfig.NextProtos[0] = "mutated"
	options := factory.singleOptions()
	if options.Addr != "redis.internal:6379" {
		t.Fatalf("Addr = %q, want immutable factory snapshot", options.Addr)
	}
	if options.TLSConfig.ServerName != "redis.internal" || options.TLSConfig.NextProtos[0] != "redis" {
		t.Fatalf("TLSConfig = %+v, want immutable factory snapshot", options.TLSConfig)
	}
	if options.MaxRetries != 3 || options.DialTimeout != 5*time.Second || options.PoolTimeout != 4*time.Second {
		t.Fatalf(
			"factory defaults = retries=%d dial=%s pool_timeout=%s, want 3/5s/4s",
			options.MaxRetries,
			options.DialTimeout,
			options.PoolTimeout,
		)
	}
}

func TestOpenFailurePreservesPingAndCloseErrors(t *testing.T) {
	pingErr := errors.New("ping failed")
	closeErr := errors.New("close failed")

	err := openFailure(pingErr, closeErr)
	if !errors.Is(err, pingErr) {
		t.Fatalf("openFailure() lost the PING error: %v", err)
	}
	if !errors.Is(err, closeErr) {
		t.Fatalf("openFailure() lost the client-close error: %v", err)
	}

	err = openFailure(pingErr, nil)
	if !errors.Is(err, pingErr) {
		t.Fatalf("openFailure() without a close error lost the PING error: %v", err)
	}
}

func TestFactoryMapsSingleOptions(t *testing.T) {
	cfg := completeConfig(ModeSingle)
	cfg.Addrs = []string{"single.redis:6379"}
	cfg.DB = 4
	cfg.SentinelCredentials = Credentials{}
	factory := mustFactory(t, cfg)

	options := factory.singleOptions()
	if options.Addr != cfg.Addrs[0] {
		t.Fatalf("Addr = %q, want %q", options.Addr, cfg.Addrs[0])
	}
	if options.DB != cfg.DB {
		t.Fatalf("DB = %d, want %d", options.DB, cfg.DB)
	}
	assertCommonOptionFields(t, options, cfg)
	assertStaticDataCredentials(t, options.Username, options.Password, cfg.DataCredentials)
	assertIndependentTLSClone(t, options.TLSConfig, factory.singleOptions().TLSConfig, cfg.TLSConfig)
}

func TestFactoryMapsClusterOptions(t *testing.T) {
	cfg := completeConfig(ModeCluster)
	cfg.Addrs = []string{"cluster-1.redis:6379", "cluster-2.redis:6379"}
	cfg.DB = 0
	cfg.SentinelCredentials = Credentials{}
	factory := mustFactory(t, cfg)

	options := factory.clusterOptions()
	if !reflect.DeepEqual(options.Addrs, cfg.Addrs) {
		t.Fatalf("Addrs = %v, want %v", options.Addrs, cfg.Addrs)
	}
	if options.MaxRedirects != cfg.MaxRedirects {
		t.Fatalf("MaxRedirects = %d, want %d", options.MaxRedirects, cfg.MaxRedirects)
	}
	if options.MaxRetries != cfg.MaxRetries {
		t.Fatalf("MaxRetries = %d, want independent per-node retry setting %d", options.MaxRetries, cfg.MaxRetries)
	}
	options.Addrs[0] = "mutated:6379"
	if got := factory.clusterOptions().Addrs[0]; got != cfg.Addrs[0] {
		t.Fatalf("cluster options share Addrs backing array: got %q", got)
	}
	assertCommonOptionFields(t, options, cfg)
	assertStaticDataCredentials(t, options.Username, options.Password, cfg.DataCredentials)
	assertIndependentTLSClone(t, options.TLSConfig, factory.clusterOptions().TLSConfig, cfg.TLSConfig)
}

func TestFactoryMapsSentinelOptions(t *testing.T) {
	cfg := completeConfig(ModeSentinel)
	cfg.Addrs = []string{"sentinel-1.redis:26379", "sentinel-2.redis:26379"}
	factory := mustFactory(t, cfg)

	options := factory.failoverOptions()
	if options.MasterName != cfg.MasterName {
		t.Fatalf("MasterName = %q, want %q", options.MasterName, cfg.MasterName)
	}
	if !reflect.DeepEqual(options.SentinelAddrs, cfg.Addrs) {
		t.Fatalf("SentinelAddrs = %v, want %v", options.SentinelAddrs, cfg.Addrs)
	}
	if options.DB != cfg.DB {
		t.Fatalf("DB = %d, want %d", options.DB, cfg.DB)
	}
	assertCommonOptionFields(t, options, cfg)
	assertStaticDataCredentials(t, options.Username, options.Password, cfg.DataCredentials)
	assertStaticDataCredentials(t, options.SentinelUsername, options.SentinelPassword, cfg.SentinelCredentials)
	options.SentinelAddrs[0] = "mutated:26379"
	if got := factory.failoverOptions().SentinelAddrs[0]; got != cfg.Addrs[0] {
		t.Fatalf("failover options share SentinelAddrs backing array: got %q", got)
	}
	assertIndependentTLSClone(t, options.TLSConfig, factory.failoverOptions().TLSConfig, cfg.TLSConfig)
}

func TestFactoryAdaptsContextCredentialsProviderForEveryMode(t *testing.T) {
	type contextKey struct{}
	wantContextValue := "request-scope"
	provider := CredentialsProvider(func(ctx context.Context) (Credentials, error) {
		if got := ctx.Value(contextKey{}); got != wantContextValue {
			return Credentials{}, errors.New("context was not forwarded")
		}
		return Credentials{Username: "dynamic-user", Password: "dynamic-password"}, nil
	})

	tests := []struct {
		name   string
		config Config
		get    func(*Factory) func(context.Context) (string, string, error)
	}{
		{
			name:   "single",
			config: Config{Mode: ModeSingle, Addrs: []string{"redis:6379"}, CredentialsProvider: provider},
			get: func(factory *Factory) func(context.Context) (string, string, error) {
				options := factory.singleOptions()
				if options.Username != "" || options.Password != "" {
					t.Fatal("provider-backed options unexpectedly contain static credentials")
				}
				return options.CredentialsProviderContext
			},
		},
		{
			name:   "cluster",
			config: Config{Mode: ModeCluster, Addrs: []string{"redis:6379"}, CredentialsProvider: provider},
			get: func(factory *Factory) func(context.Context) (string, string, error) {
				return factory.clusterOptions().CredentialsProviderContext
			},
		},
		{
			name: "sentinel",
			config: Config{
				Mode:                ModeSentinel,
				Addrs:               []string{"sentinel:26379"},
				MasterName:          "primary",
				CredentialsProvider: provider,
			},
			get: func(factory *Factory) func(context.Context) (string, string, error) {
				return factory.failoverOptions().CredentialsProviderContext
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapted := test.get(mustFactory(t, test.config))
			if adapted == nil {
				t.Fatal("CredentialsProviderContext is nil")
			}
			ctx := context.WithValue(context.Background(), contextKey{}, wantContextValue)
			username, password, err := adapted(ctx)
			if err != nil {
				t.Fatalf("provider error = %v", err)
			}
			if username != "dynamic-user" || password != "dynamic-password" {
				t.Fatal("provider did not return the expected named ACL credential pair")
			}
		})
	}
}

func TestCredentialsProviderRuntimeValidationAndSafeErrors(t *testing.T) {
	providerCause := errors.New("vault failure includes provider-secret")
	tests := []struct {
		name      string
		provider  CredentialsProvider
		wantError error
		secrets   []string
	}{
		{
			name: "provider failure",
			provider: func(context.Context) (Credentials, error) {
				return Credentials{Username: "ignored-user", Password: "ignored-password"}, providerCause
			},
			wantError: ErrCredentialsProvider,
			secrets:   []string{"provider-secret", "ignored-user", "ignored-password"},
		},
		{
			name: "password only",
			provider: func(context.Context) (Credentials, error) {
				return Credentials{Password: "dynamic-password-only"}, nil
			},
			wantError: ErrInvalidCredentials,
			secrets:   []string{"dynamic-password-only"},
		},
		{
			name: "username only",
			provider: func(context.Context) (Credentials, error) {
				return Credentials{Username: "dynamic-username-only"}, nil
			},
			wantError: ErrInvalidCredentials,
			secrets:   []string{"dynamic-username-only"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory := mustFactory(t, Config{
				Mode:                ModeSingle,
				Addrs:               []string{"redis:6379"},
				CredentialsProvider: test.provider,
			})
			username, password, err := factory.singleOptions().CredentialsProviderContext(context.Background())
			if username != "" || password != "" {
				t.Fatal("provider failure returned non-empty credentials")
			}
			if !errors.Is(err, test.wantError) {
				t.Fatalf("provider error = %v, want errors.Is(%v)", err, test.wantError)
			}
			if test.name == "provider failure" && !errors.Is(err, providerCause) {
				t.Fatalf("provider error = %v, want original provider cause in the error chain", err)
			}
			for _, secret := range test.secrets {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("provider error leaked %q: %q", secret, err)
				}
			}
		})
	}

	factory := mustFactory(t, Config{
		Mode:  ModeSingle,
		Addrs: []string{"redis:6379"},
		CredentialsProvider: func(context.Context) (Credentials, error) {
			return Credentials{}, nil
		},
	})
	username, password, err := factory.singleOptions().CredentialsProviderContext(context.Background())
	if err != nil || username != "" || password != "" {
		t.Fatalf("empty dynamic credentials did not produce an unauthenticated result: %v", err)
	}
}

func TestFactoryNewClientUsesConfiguredTopology(t *testing.T) {
	tests := []struct {
		name       string
		config     Config
		assertType func(*testing.T, redis.UniversalClient)
	}{
		{
			name:   "single",
			config: Config{Mode: ModeSingle, Addrs: []string{"redis:6379"}},
			assertType: func(t *testing.T, client redis.UniversalClient) {
				if _, ok := client.(*redis.Client); !ok {
					t.Fatalf("NewClient() type = %T, want *redis.Client", client)
				}
			},
		},
		{
			name:   "cluster",
			config: Config{Mode: ModeCluster, Addrs: []string{"redis:6379"}},
			assertType: func(t *testing.T, client redis.UniversalClient) {
				if _, ok := client.(*redis.ClusterClient); !ok {
					t.Fatalf("NewClient() type = %T, want *redis.ClusterClient", client)
				}
			},
		},
		{
			name: "sentinel",
			config: Config{
				Mode: ModeSentinel, Addrs: []string{"sentinel:26379"}, MasterName: "primary",
			},
			assertType: func(t *testing.T, client redis.UniversalClient) {
				if _, ok := client.(*redis.Client); !ok {
					t.Fatalf("NewClient() type = %T, want failover *redis.Client", client)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := mustFactory(t, test.config).NewClient()
			t.Cleanup(func() { _ = client.Close() })
			test.assertType(t, client)
		})
	}
}

func completeConfig(mode Mode) Config {
	return Config{
		Mode:                mode,
		Addrs:               []string{"redis:6379"},
		MasterName:          "primary",
		DataCredentials:     Credentials{Username: "data-user", Password: "data-password"},
		SentinelCredentials: Credentials{Username: "sentinel-user", Password: "sentinel-password"},
		DB:                  2,
		TLSConfig: &tls.Config{
			ServerName: "redis.internal",
			MinVersion: tls.VersionTLS13,
			NextProtos: []string{"redis"},
		},
		MaxRetries:      7,
		MaxRedirects:    11,
		MinRetryBackoff: 12 * time.Millisecond,
		MaxRetryBackoff: 13 * time.Millisecond,
		DialTimeout:     14 * time.Second,
		ReadTimeout:     15 * time.Second,
		WriteTimeout:    16 * time.Second,
		PoolSize:        17,
		MinIdleConns:    18,
		MaxIdleConns:    19,
		MaxActiveConns:  20,
		PoolTimeout:     21 * time.Second,
		ConnMaxIdleTime: 22 * time.Minute,
		ConnMaxLifetime: 23 * time.Minute,
	}
}

func mustFactory(t *testing.T, cfg Config) *Factory {
	t.Helper()
	factory, err := NewFactory(cfg)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	return factory
}

func assertCommonOptionFields(t *testing.T, options any, cfg Config) {
	t.Helper()
	wants := map[string]any{
		"MaxRetries":      cfg.MaxRetries,
		"MinRetryBackoff": cfg.MinRetryBackoff,
		"MaxRetryBackoff": cfg.MaxRetryBackoff,
		"DialTimeout":     cfg.DialTimeout,
		"ReadTimeout":     cfg.ReadTimeout,
		"WriteTimeout":    cfg.WriteTimeout,
		"PoolSize":        cfg.PoolSize,
		"MinIdleConns":    cfg.MinIdleConns,
		"MaxIdleConns":    cfg.MaxIdleConns,
		"MaxActiveConns":  cfg.MaxActiveConns,
		"PoolTimeout":     cfg.PoolTimeout,
		"ConnMaxIdleTime": cfg.ConnMaxIdleTime,
		"ConnMaxLifetime": cfg.ConnMaxLifetime,
	}
	value := reflect.ValueOf(options).Elem()
	for field, want := range wants {
		got := value.FieldByName(field).Interface()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %v, want %v", field, got, want)
		}
	}
}

func assertStaticDataCredentials(t *testing.T, username, password string, want Credentials) {
	t.Helper()
	if username != want.Username {
		t.Fatal("mapped ACL username does not match the configured username")
	}
	if password != want.Password {
		t.Fatal("mapped ACL password does not match the configured password")
	}
}

func assertIndependentTLSClone(t *testing.T, first, second, source *tls.Config) {
	t.Helper()
	if first == nil || second == nil {
		t.Fatal("TLSConfig was not mapped")
	}
	if first == source || second == source || first == second {
		t.Fatal("TLSConfig pointers are shared between config or option snapshots")
	}
	first.ServerName = "mutated.invalid"
	first.NextProtos[0] = "mutated"
	if second.ServerName != source.ServerName || second.NextProtos[0] != source.NextProtos[0] {
		t.Fatal("TLSConfig mutable state is shared between client option snapshots")
	}
}
