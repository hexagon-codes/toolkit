package redisconn

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"math"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfigUsesProductionDefaults(t *testing.T) {
	inputAddrs := []string{"redis-1:6379", "redis-2:6379"}
	cfg := DefaultConfig(ModeCluster, inputAddrs...)

	if cfg.Mode != ModeCluster {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, ModeCluster)
	}
	if len(cfg.Addrs) != 2 || cfg.Addrs[0] != "redis-1:6379" || cfg.Addrs[1] != "redis-2:6379" {
		t.Fatalf("Addrs = %v, want copied input addresses", cfg.Addrs)
	}
	inputAddrs[0] = "mutated:6379"
	if cfg.Addrs[0] != "redis-1:6379" {
		t.Fatal("DefaultConfig retained the caller's Addrs backing array")
	}

	wantPoolSize := 5 * runtime.GOMAXPROCS(0)
	checks := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{name: "MinRetryBackoff", got: cfg.MinRetryBackoff, want: 8 * time.Millisecond},
		{name: "MaxRetryBackoff", got: cfg.MaxRetryBackoff, want: 512 * time.Millisecond},
		{name: "DialTimeout", got: cfg.DialTimeout, want: 5 * time.Second},
		{name: "ReadTimeout", got: cfg.ReadTimeout, want: 3 * time.Second},
		{name: "WriteTimeout", got: cfg.WriteTimeout, want: 3 * time.Second},
		{name: "PoolTimeout", got: cfg.PoolTimeout, want: 4 * time.Second},
		{name: "ConnMaxIdleTime", got: cfg.ConnMaxIdleTime, want: 30 * time.Minute},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if check.got != check.want {
				t.Fatalf("got %s, want %s", check.got, check.want)
			}
		})
	}
	if cfg.MaxRetries != -1 {
		t.Fatalf("cluster MaxRetries = %d, want -1 to preserve go-redis per-node retry default", cfg.MaxRetries)
	}
	if cfg.MaxRedirects != 3 {
		t.Fatalf("MaxRedirects = %d, want 3", cfg.MaxRedirects)
	}
	if cfg.PoolSize != wantPoolSize {
		t.Fatalf("PoolSize = %d, want %d", cfg.PoolSize, wantPoolSize)
	}
}

func TestDefaultConfigUsesTopologySpecificDefaults(t *testing.T) {
	tests := []struct {
		mode          Mode
		wantRetries   int
		wantRedirects int
		wantPoolSize  int
	}{
		{mode: ModeSingle, wantRetries: 3, wantRedirects: 3, wantPoolSize: 10 * runtime.GOMAXPROCS(0)},
		{mode: ModeCluster, wantRetries: -1, wantRedirects: 3, wantPoolSize: 5 * runtime.GOMAXPROCS(0)},
		{mode: ModeSentinel, wantRetries: 3, wantRedirects: 3, wantPoolSize: 10 * runtime.GOMAXPROCS(0)},
	}
	for _, test := range tests {
		t.Run(string(test.mode), func(t *testing.T) {
			cfg := DefaultConfig(test.mode, "redis:6379")
			if cfg.MaxRetries != test.wantRetries {
				t.Fatalf("MaxRetries = %d, want %d", cfg.MaxRetries, test.wantRetries)
			}
			if cfg.MaxRedirects != test.wantRedirects {
				t.Fatalf("MaxRedirects = %d, want %d", cfg.MaxRedirects, test.wantRedirects)
			}
			if cfg.PoolSize != test.wantPoolSize {
				t.Fatalf("PoolSize = %d, want %d", cfg.PoolSize, test.wantPoolSize)
			}
		})
	}
}

func TestConfigNormalizePreservesExplicitValuesAndDeepCopies(t *testing.T) {
	addrs := []string{"redis.internal:6379"}
	tlsConfig := &tls.Config{
		ServerName: "redis.internal",
		MinVersion: tls.VersionTLS13,
		NextProtos: []string{"redis"},
	}
	cfg := Config{
		Mode:                ModeSingle,
		Addrs:               addrs,
		TLSConfig:           tlsConfig,
		MaxRetries:          7,
		MaxRedirects:        8,
		MinRetryBackoff:     10 * time.Millisecond,
		MaxRetryBackoff:     time.Second,
		DialTimeout:         9 * time.Second,
		ReadTimeout:         8 * time.Second,
		WriteTimeout:        7 * time.Second,
		PoolTimeout:         6 * time.Second,
		ConnMaxIdleTime:     5 * time.Minute,
		ConnMaxLifetime:     time.Hour,
		PoolSize:            42,
		MinIdleConns:        3,
		MaxIdleConns:        9,
		MaxActiveConns:      50,
		DataCredentials:     Credentials{Username: "app", Password: "secret"},
		CredentialsProvider: nil,
	}

	normalized := cfg.Normalize()

	if normalized.MaxRetries != 7 || normalized.MaxRedirects != 8 || normalized.PoolSize != 42 || normalized.ConnMaxLifetime != time.Hour {
		t.Fatalf(
			"Normalize overwrote explicit numeric values: retries=%d redirects=%d pool=%d lifetime=%s",
			normalized.MaxRetries,
			normalized.MaxRedirects,
			normalized.PoolSize,
			normalized.ConnMaxLifetime,
		)
	}
	if normalized.TLSConfig == tlsConfig {
		t.Fatal("Normalize did not clone TLSConfig")
	}
	if normalized.TLSConfig.ServerName != "redis.internal" || normalized.TLSConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("cloned TLSConfig lost values: %+v", normalized.TLSConfig)
	}

	addrs[0] = "mutated:6379"
	tlsConfig.ServerName = "mutated.invalid"
	tlsConfig.NextProtos[0] = "mutated"
	if normalized.Addrs[0] != "redis.internal:6379" {
		t.Fatal("Normalize retained the caller's Addrs backing array")
	}
	if normalized.TLSConfig.ServerName != "redis.internal" {
		t.Fatal("Normalize retained the caller's TLSConfig pointer")
	}
	if got := normalized.TLSConfig.NextProtos[0]; got != "redis" {
		t.Fatalf("Normalize retained the caller's TLSConfig.NextProtos backing array: got %q", got)
	}
}

//nolint:staticcheck // Exercise deep-copy compatibility for deprecated NameToCertificate.
func TestConfigNormalizeDeepCopiesMutableTLSFields(t *testing.T) {
	certificate := tls.Certificate{
		Certificate:                  [][]byte{{1, 2, 3}},
		SupportedSignatureAlgorithms: []tls.SignatureScheme{tls.PSSWithSHA256},
		OCSPStaple:                   []byte{4, 5},
		SignedCertificateTimestamps:  [][]byte{{6, 7}},
	}
	tlsConfig := &tls.Config{
		Certificates:                   []tls.Certificate{certificate},
		NameToCertificate:              map[string]*tls.Certificate{"service": &certificate, "nil": nil},
		RootCAs:                        x509.NewCertPool(),
		ClientCAs:                      x509.NewCertPool(),
		CipherSuites:                   []uint16{tls.TLS_AES_128_GCM_SHA256},
		CurvePreferences:               []tls.CurveID{tls.X25519},
		EncryptedClientHelloConfigList: []byte{8, 9},
		EncryptedClientHelloKeys: []tls.EncryptedClientHelloKey{{
			Config:     []byte{10, 11},
			PrivateKey: []byte{12, 13},
		}},
	}

	cloned := Config{TLSConfig: tlsConfig}.Normalize().TLSConfig
	if cloned.RootCAs == tlsConfig.RootCAs || cloned.ClientCAs == tlsConfig.ClientCAs {
		t.Fatal("Normalize retained a caller-owned certificate pool")
	}
	if cloned.NameToCertificate["service"] == tlsConfig.NameToCertificate["service"] {
		t.Fatal("Normalize retained a caller-owned NameToCertificate value")
	}
	if value, exists := cloned.NameToCertificate["nil"]; !exists || value != nil {
		t.Fatal("Normalize did not preserve a nil NameToCertificate entry")
	}

	tlsConfig.Certificates[0].Certificate[0][0] = 99
	tlsConfig.Certificates[0].SupportedSignatureAlgorithms[0] = tls.PKCS1WithSHA256
	tlsConfig.Certificates[0].OCSPStaple[0] = 99
	tlsConfig.Certificates[0].SignedCertificateTimestamps[0][0] = 99
	tlsConfig.NameToCertificate["service"].Certificate[0][0] = 98
	tlsConfig.CipherSuites[0] = tls.TLS_AES_256_GCM_SHA384
	tlsConfig.CurvePreferences[0] = tls.CurveP256
	tlsConfig.EncryptedClientHelloConfigList[0] = 99
	tlsConfig.EncryptedClientHelloKeys[0].Config[0] = 99
	tlsConfig.EncryptedClientHelloKeys[0].PrivateKey[0] = 99

	if cloned.Certificates[0].Certificate[0][0] != 1 ||
		cloned.Certificates[0].SupportedSignatureAlgorithms[0] != tls.PSSWithSHA256 ||
		cloned.Certificates[0].OCSPStaple[0] != 4 ||
		cloned.Certificates[0].SignedCertificateTimestamps[0][0] != 6 ||
		cloned.NameToCertificate["service"].Certificate[0][0] != 1 ||
		cloned.CipherSuites[0] != tls.TLS_AES_128_GCM_SHA256 ||
		cloned.CurvePreferences[0] != tls.X25519 ||
		cloned.EncryptedClientHelloConfigList[0] != 8 ||
		cloned.EncryptedClientHelloKeys[0].Config[0] != 10 ||
		cloned.EncryptedClientHelloKeys[0].PrivateKey[0] != 12 {
		t.Fatal("Normalize retained mutable TLS backing storage")
	}
}

func TestConfigValidate(t *testing.T) {
	provider := CredentialsProvider(func(context.Context) (Credentials, error) {
		return Credentials{Username: "dynamic", Password: "secret"}, nil
	})

	tests := []struct {
		name      string
		config    Config
		wantError error
	}{
		{
			name:      "unsupported mode",
			config:    Config{Mode: Mode("proxy"), Addrs: []string{"redis:6379"}},
			wantError: ErrInvalidConfig,
		},
		{
			name:      "single needs one address",
			config:    Config{Mode: ModeSingle},
			wantError: ErrInvalidConfig,
		},
		{
			name:      "single rejects multiple addresses",
			config:    Config{Mode: ModeSingle, Addrs: []string{"redis-1:6379", "redis-2:6379"}},
			wantError: ErrInvalidConfig,
		},
		{
			name:      "cluster needs an address",
			config:    Config{Mode: ModeCluster},
			wantError: ErrInvalidConfig,
		},
		{
			name:      "sentinel needs an address",
			config:    Config{Mode: ModeSentinel, MasterName: "primary"},
			wantError: ErrInvalidConfig,
		},
		{
			name:      "sentinel needs master name",
			config:    Config{Mode: ModeSentinel, Addrs: []string{"sentinel:26379"}},
			wantError: ErrInvalidConfig,
		},
		{
			name:      "blank address",
			config:    Config{Mode: ModeCluster, Addrs: []string{"redis:6379", "  "}},
			wantError: ErrInvalidConfig,
		},
		{
			name:      "negative database",
			config:    Config{Mode: ModeSingle, Addrs: []string{"redis:6379"}, DB: -1},
			wantError: ErrInvalidConfig,
		},
		{
			name:      "cluster database must be zero",
			config:    Config{Mode: ModeCluster, Addrs: []string{"redis:6379"}, DB: 1},
			wantError: ErrInvalidConfig,
		},
		{
			name: "data password only is rejected",
			config: Config{
				Mode:            ModeSingle,
				Addrs:           []string{"redis:6379"},
				DataCredentials: Credentials{Password: "do-not-print-data-password"},
			},
			wantError: ErrInvalidCredentials,
		},
		{
			name: "data username only is rejected",
			config: Config{
				Mode:            ModeSingle,
				Addrs:           []string{"redis:6379"},
				DataCredentials: Credentials{Username: "do-not-print-data-user"},
			},
			wantError: ErrInvalidCredentials,
		},
		{
			name: "provider conflicts with static data credentials",
			config: Config{
				Mode:                ModeSingle,
				Addrs:               []string{"redis:6379"},
				DataCredentials:     Credentials{Username: "app", Password: "secret"},
				CredentialsProvider: provider,
			},
			wantError: ErrInvalidCredentials,
		},
		{
			name: "sentinel password only is rejected",
			config: Config{
				Mode:                ModeSentinel,
				Addrs:               []string{"sentinel:26379"},
				MasterName:          "primary",
				SentinelCredentials: Credentials{Password: "do-not-print-sentinel-password"},
			},
			wantError: ErrInvalidCredentials,
		},
		{
			name: "sentinel username only is rejected",
			config: Config{
				Mode:                ModeSentinel,
				Addrs:               []string{"sentinel:26379"},
				MasterName:          "primary",
				SentinelCredentials: Credentials{Username: "do-not-print-sentinel-user"},
			},
			wantError: ErrInvalidCredentials,
		},
		{
			name: "single rejects sentinel credentials",
			config: Config{
				Mode:                ModeSingle,
				Addrs:               []string{"redis:6379"},
				SentinelCredentials: Credentials{Username: "sentinel", Password: "secret"},
			},
			wantError: ErrInvalidConfig,
		},
		{
			name: "cluster rejects sentinel credentials",
			config: Config{
				Mode:                ModeCluster,
				Addrs:               []string{"redis:6379"},
				SentinelCredentials: Credentials{Username: "sentinel", Password: "secret"},
			},
			wantError: ErrInvalidConfig,
		},
		{
			name:   "single without authentication",
			config: Config{Mode: ModeSingle, Addrs: []string{"redis:6379"}},
		},
		{
			name: "single with static ACL credentials",
			config: Config{
				Mode:            ModeSingle,
				Addrs:           []string{"redis:6379"},
				DataCredentials: Credentials{Username: "app", Password: "secret"},
			},
		},
		{
			name: "cluster with provider",
			config: Config{
				Mode:                ModeCluster,
				Addrs:               []string{"redis-1:6379", "redis-2:6379"},
				CredentialsProvider: provider,
			},
		},
		{
			name: "sentinel with independent ACL credentials",
			config: Config{
				Mode:                ModeSentinel,
				Addrs:               []string{"sentinel:26379"},
				MasterName:          "primary",
				DataCredentials:     Credentials{Username: "data", Password: "data-secret"},
				SentinelCredentials: Credentials{Username: "sentinel", Password: "sentinel-secret"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.config.Validate()
			if test.wantError == nil {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Validate() error = %v, want errors.Is(%v)", err, test.wantError)
			}
			for _, secret := range []string{
				"do-not-print-data-password",
				"do-not-print-data-user",
				"do-not-print-sentinel-password",
				"do-not-print-sentinel-user",
			} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("Validate() leaked a credential value in error: %q", err)
				}
			}
		})
	}
}

func TestConfigValidateRejectsUnsafeNumericValues(t *testing.T) {
	type testCase struct {
		name   string
		mutate func(*Config)
	}
	tests := []testCase{
		{name: "max retries below disable sentinel", mutate: func(config *Config) { config.MaxRetries = -2 }},
		{name: "max redirects below disable sentinel", mutate: func(config *Config) { config.MaxRedirects = -2 }},
		{name: "minimum retry backoff below disable sentinel", mutate: func(config *Config) { config.MinRetryBackoff = -2 }},
		{name: "maximum retry backoff below disable sentinel", mutate: func(config *Config) { config.MaxRetryBackoff = -2 }},
		{name: "negative dial timeout", mutate: func(config *Config) { config.DialTimeout = -1 }},
		{name: "read timeout below supported sentinels", mutate: func(config *Config) { config.ReadTimeout = -3 }},
		{name: "write timeout below supported sentinels", mutate: func(config *Config) { config.WriteTimeout = -3 }},
		{name: "negative pool size", mutate: func(config *Config) { config.PoolSize = -1 }},
		{name: "negative minimum idle connections", mutate: func(config *Config) { config.MinIdleConns = -1 }},
		{name: "negative maximum idle connections", mutate: func(config *Config) { config.MaxIdleConns = -1 }},
		{name: "negative maximum active connections", mutate: func(config *Config) { config.MaxActiveConns = -1 }},
		{name: "negative pool timeout", mutate: func(config *Config) { config.PoolTimeout = -1 }},
	}
	if strconv.IntSize > 32 {
		overflow := int(int64(math.MaxInt32) + 1)
		tests = append(tests,
			testCase{name: "pool size exceeds go-redis int32 limit", mutate: func(config *Config) { config.PoolSize = overflow }},
			testCase{name: "minimum idle connections exceed go-redis int32 limit", mutate: func(config *Config) { config.MinIdleConns = overflow }},
			testCase{name: "maximum idle connections exceed go-redis int32 limit", mutate: func(config *Config) { config.MaxIdleConns = overflow }},
			testCase{name: "maximum active connections exceed go-redis int32 limit", mutate: func(config *Config) { config.MaxActiveConns = overflow }},
		)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := Config{Mode: ModeSingle, Addrs: []string{"redis:6379"}}
			test.mutate(&config)

			if err := config.Validate(); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("Validate() error = %v, want errors.Is(ErrInvalidConfig)", err)
			}
		})
	}
}

func TestConfigValidateAcceptsGoRedisDisableSentinels(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "single",
			config: Config{
				Mode:            ModeSingle,
				Addrs:           []string{"redis:6379"},
				MaxRetries:      -1,
				MaxRedirects:    -1,
				MinRetryBackoff: -1,
				MaxRetryBackoff: -1,
				ReadTimeout:     -1,
				WriteTimeout:    -2,
				ConnMaxIdleTime: -1,
				ConnMaxLifetime: -time.Second,
			},
		},
		{
			name: "cluster",
			config: Config{
				Mode:            ModeCluster,
				Addrs:           []string{"redis:6379"},
				MaxRetries:      -1,
				MaxRedirects:    -1,
				MinRetryBackoff: -1,
				MaxRetryBackoff: -1,
				ReadTimeout:     -2,
				WriteTimeout:    -1,
				ConnMaxIdleTime: -1,
				ConnMaxLifetime: -time.Second,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.config.Validate(); err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}
