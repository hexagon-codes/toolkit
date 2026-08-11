package main

import "testing"

func TestMySQLDSNRequiresExplicitConfiguration(t *testing.T) {
	t.Setenv("MYSQL_DSN", "")
	if dsn, ok := configuredMySQLDSN(); ok || dsn != "" {
		t.Fatalf("configuredMySQLDSN() = (%q, %v), want empty and false", dsn, ok)
	}
}
