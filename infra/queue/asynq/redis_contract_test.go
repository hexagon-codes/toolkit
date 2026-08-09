package asynq

import (
	"crypto/tls"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/hexagon-codes/toolkit/infra/redisconn"
)

func TestRedisConnOptPreservesExplicitTopologyACLAndTLS(t *testing.T) {
	tlsConfig := &tls.Config{
		ServerName: "redis.internal",
		MinVersion: tls.VersionTLS13,
	}
	credentials := redisconn.Credentials{
		Username: "queue-worker",
		Password: "queue-secret",
	}

	tests := []struct {
		name   string
		config redisconn.Config
		check  func(t *testing.T, client redis.UniversalClient)
	}{
		{
			name: "single",
			config: redisconn.Config{
				Mode:            redisconn.ModeSingle,
				Addrs:           []string{"redis.internal:6379"},
				DataCredentials: credentials,
				DB:              5,
				TLSConfig:       tlsConfig,
			},
			check: func(t *testing.T, client redis.UniversalClient) {
				t.Helper()
				standalone, ok := client.(*redis.Client)
				if !ok {
					t.Fatalf("client type = %T, want *redis.Client", client)
				}
				options := standalone.Options()
				if options.Addr != "redis.internal:6379" || options.DB != 5 {
					t.Fatalf("standalone options = addr %q db %d, want redis.internal:6379 db 5", options.Addr, options.DB)
				}
				assertClientSecurityOptions(t, options.Username, options.Password, options.TLSConfig)
			},
		},
		{
			name: "single seed cluster remains cluster",
			config: redisconn.Config{
				Mode:            redisconn.ModeCluster,
				Addrs:           []string{"cluster-seed.internal:6379"},
				DataCredentials: credentials,
				TLSConfig:       tlsConfig,
			},
			check: func(t *testing.T, client redis.UniversalClient) {
				t.Helper()
				cluster, ok := client.(*redis.ClusterClient)
				if !ok {
					t.Fatalf("client type = %T, want *redis.ClusterClient", client)
				}
				options := cluster.Options()
				if len(options.Addrs) != 1 || options.Addrs[0] != "cluster-seed.internal:6379" {
					t.Fatalf("cluster addrs = %v, want one explicit seed", options.Addrs)
				}
				assertClientSecurityOptions(t, options.Username, options.Password, options.TLSConfig)
			},
		},
		{
			name: "sentinel",
			config: redisconn.Config{
				Mode:                redisconn.ModeSentinel,
				Addrs:               []string{"sentinel.internal:26379"},
				MasterName:          "primary",
				DataCredentials:     credentials,
				SentinelCredentials: redisconn.Credentials{Username: "sentinel", Password: "sentinel-secret"},
				DB:                  3,
				TLSConfig:           tlsConfig,
			},
			check: func(t *testing.T, client redis.UniversalClient) {
				t.Helper()
				failover, ok := client.(*redis.Client)
				if !ok {
					t.Fatalf("client type = %T, want *redis.Client", client)
				}
				options := failover.Options()
				if options.Addr != "FailoverClient" || options.DB != 3 {
					t.Fatalf("failover options = addr %q db %d, want FailoverClient db 3", options.Addr, options.DB)
				}
				assertClientSecurityOptions(t, options.Username, options.Password, options.TLSConfig)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory, err := redisconn.NewFactory(test.config)
			if err != nil {
				t.Fatalf("redisconn.NewFactory() error = %v", err)
			}
			client, ok := newRedisConnOpt(factory).MakeRedisClient().(redis.UniversalClient)
			if !ok {
				t.Fatalf("MakeRedisClient() type = %T, want redis.UniversalClient", client)
			}
			t.Cleanup(func() { _ = client.Close() })
			test.check(t, client)
		})
	}
}

func assertClientSecurityOptions(t *testing.T, username, password string, tlsConfig *tls.Config) {
	t.Helper()
	if username != "queue-worker" {
		t.Fatal("ACL username option was not propagated")
	}
	if password != "queue-secret" {
		t.Fatal("ACL password option was not propagated")
	}
	if tlsConfig == nil || tlsConfig.ServerName != "redis.internal" || tlsConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("TLS config = %+v, want configured TLS settings", tlsConfig)
	}
}
