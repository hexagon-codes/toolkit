package main

import "testing"

func TestDatabaseAddressesRequireExplicitConfiguration(t *testing.T) {
	t.Setenv("MYSQL_DSN", "")
	t.Setenv("REDIS_ADDR", "")
	if dsn, ok := configuredMySQLDSN(); ok || dsn != "" {
		t.Fatalf("configuredMySQLDSN() = (%q, %v), want empty and false", dsn, ok)
	}
	if address, ok := configuredRedisAddress(); ok || address != "" {
		t.Fatalf("configuredRedisAddress() = (%q, %v), want empty and false", address, ok)
	}
}
