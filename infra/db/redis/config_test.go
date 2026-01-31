package redis

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	addr := "localhost:6379"
	cfg := DefaultConfig(addr)

	if cfg.Mode != ModeSingle {
		t.Errorf("expected mode %s, got %s", ModeSingle, cfg.Mode)
	}

	if cfg.Addr != addr {
		t.Errorf("expected addr %s, got %s", addr, cfg.Addr)
	}

	if cfg.DB != 0 {
		t.Errorf("expected DB 0, got %d", cfg.DB)
	}

	// 验证默认值
	if cfg.PoolSize != 10 {
		t.Errorf("expected PoolSize 10, got %d", cfg.PoolSize)
	}

	if cfg.MinIdleConns != 2 {
		t.Errorf("expected MinIdleConns 2, got %d", cfg.MinIdleConns)
	}

	if cfg.MaxRetries != 3 {
		t.Errorf("expected MaxRetries 3, got %d", cfg.MaxRetries)
	}

	if cfg.PoolTimeout != 4*time.Second {
		t.Errorf("expected PoolTimeout 4s, got %v", cfg.PoolTimeout)
	}

	if cfg.DialTimeout != 5*time.Second {
		t.Errorf("expected DialTimeout 5s, got %v", cfg.DialTimeout)
	}

	if cfg.ReadTimeout != 3*time.Second {
		t.Errorf("expected ReadTimeout 3s, got %v", cfg.ReadTimeout)
	}

	if cfg.WriteTimeout != 3*time.Second {
		t.Errorf("expected WriteTimeout 3s, got %v", cfg.WriteTimeout)
	}

	if cfg.IdleTimeout != 5*time.Minute {
		t.Errorf("expected IdleTimeout 5m, got %v", cfg.IdleTimeout)
	}

	if cfg.IdleCheckFrequency != time.Minute {
		t.Errorf("expected IdleCheckFrequency 1m, got %v", cfg.IdleCheckFrequency)
	}
}

func TestDefaultClusterConfig(t *testing.T) {
	addrs := []string{"localhost:7000", "localhost:7001", "localhost:7002"}
	cfg := DefaultClusterConfig(addrs)

	if cfg.Mode != ModeCluster {
		t.Errorf("expected mode %s, got %s", ModeCluster, cfg.Mode)
	}

	if len(cfg.Addrs) != len(addrs) {
		t.Errorf("expected %d addrs, got %d", len(addrs), len(cfg.Addrs))
	}

	for i, addr := range addrs {
		if cfg.Addrs[i] != addr {
			t.Errorf("expected addr[%d] %s, got %s", i, addr, cfg.Addrs[i])
		}
	}

	// 验证默认值
	if cfg.PoolSize != 10 {
		t.Errorf("expected PoolSize 10, got %d", cfg.PoolSize)
	}

	if cfg.MinIdleConns != 2 {
		t.Errorf("expected MinIdleConns 2, got %d", cfg.MinIdleConns)
	}
}

func TestConfigToClientOptions(t *testing.T) {
	cfg := &Config{
		Mode:               ModeSingle,
		Addr:               "localhost:6379",
		Password:           "password123",
		DB:                 5,
		PoolSize:           20,
		MinIdleConns:       5,
		MaxRetries:         5,
		PoolTimeout:        10 * time.Second,
		DialTimeout:        8 * time.Second,
		ReadTimeout:        6 * time.Second,
		WriteTimeout:       6 * time.Second,
		IdleTimeout:        10 * time.Minute,
		IdleCheckFrequency: 2 * time.Minute,
	}

	opts := cfg.ToClientOptions()

	if opts.Addr != cfg.Addr {
		t.Errorf("expected Addr %s, got %s", cfg.Addr, opts.Addr)
	}

	if opts.Password != cfg.Password {
		t.Errorf("expected Password %s, got %s", cfg.Password, opts.Password)
	}

	if opts.DB != cfg.DB {
		t.Errorf("expected DB %d, got %d", cfg.DB, opts.DB)
	}

	if opts.PoolSize != cfg.PoolSize {
		t.Errorf("expected PoolSize %d, got %d", cfg.PoolSize, opts.PoolSize)
	}

	if opts.MinIdleConns != cfg.MinIdleConns {
		t.Errorf("expected MinIdleConns %d, got %d", cfg.MinIdleConns, opts.MinIdleConns)
	}

	if opts.MaxRetries != cfg.MaxRetries {
		t.Errorf("expected MaxRetries %d, got %d", cfg.MaxRetries, opts.MaxRetries)
	}

	if opts.ConnMaxIdleTime != cfg.IdleTimeout {
		t.Errorf("expected ConnMaxIdleTime %v, got %v", cfg.IdleTimeout, opts.ConnMaxIdleTime)
	}

	if opts.ConnMaxLifetime != 0 {
		t.Errorf("expected ConnMaxLifetime 0, got %v", opts.ConnMaxLifetime)
	}
}

func TestConfigToClusterOptions(t *testing.T) {
	cfg := &Config{
		Mode:               ModeCluster,
		Addrs:              []string{"localhost:7000", "localhost:7001"},
		Password:           "cluster-password",
		PoolSize:           30,
		MinIdleConns:       10,
		MaxRetries:         5,
		PoolTimeout:        10 * time.Second,
		DialTimeout:        8 * time.Second,
		ReadTimeout:        6 * time.Second,
		WriteTimeout:       6 * time.Second,
		IdleTimeout:        10 * time.Minute,
		IdleCheckFrequency: 2 * time.Minute,
	}

	opts := cfg.ToClusterOptions()

	if len(opts.Addrs) != len(cfg.Addrs) {
		t.Errorf("expected %d addrs, got %d", len(cfg.Addrs), len(opts.Addrs))
	}

	for i, addr := range cfg.Addrs {
		if opts.Addrs[i] != addr {
			t.Errorf("expected addr[%d] %s, got %s", i, addr, opts.Addrs[i])
		}
	}

	if opts.Password != cfg.Password {
		t.Errorf("expected Password %s, got %s", cfg.Password, opts.Password)
	}

	if opts.PoolSize != cfg.PoolSize {
		t.Errorf("expected PoolSize %d, got %d", cfg.PoolSize, opts.PoolSize)
	}

	if opts.ConnMaxLifetime != 0 {
		t.Errorf("expected ConnMaxLifetime 0, got %v", opts.ConnMaxLifetime)
	}
}

func TestModeConstants(t *testing.T) {
	tests := []struct {
		mode     Mode
		expected string
	}{
		{ModeSingle, "single"},
		{ModeCluster, "cluster"},
		{ModeSentinel, "sentinel"},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			if string(tt.mode) != tt.expected {
				t.Errorf("expected mode %s, got %s", tt.expected, string(tt.mode))
			}
		})
	}
}

func TestStdLogger(t *testing.T) {
	logger := &StdLogger{}

	// 这些方法不应该 panic
	logger.Printf("test message: %s", "value")
	logger.Error("test error", nil)
}

func TestConfigWithLogger(t *testing.T) {
	logger := &StdLogger{}
	cfg := DefaultConfig("localhost:6379")
	cfg.Logger = logger

	if cfg.Logger == nil {
		t.Error("expected logger to be set")
	}
}
