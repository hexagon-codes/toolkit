package main

import "testing"

func TestRedisAddressRequiresExplicitConfiguration(t *testing.T) {
	t.Setenv("REDIS_ADDR", "")
	if address, ok := configuredRedisAddress(); ok || address != "" {
		t.Fatalf("configuredRedisAddress() = (%q, %v), want empty and false", address, ok)
	}
}
