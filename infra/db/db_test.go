package db

import (
	"context"
	"errors"
	"testing"
)

// mockClient implements Client interface for testing.
type mockClient struct {
	name      string
	pingErr   error
	closeErr  error
	pingCount int
}

func (m *mockClient) Ping(ctx context.Context) error {
	m.pingCount++
	return m.pingErr
}

func (m *mockClient) Close() error {
	return m.closeErr
}

func (m *mockClient) Name() string {
	return m.name
}

func TestManager_Register(t *testing.T) {
	m := NewManager()

	c1 := &mockClient{name: "client1"}
	c2 := &mockClient{name: "client2"}

	m.Register(c1)
	m.Register(c2)

	clients := m.Clients()
	if len(clients) != 2 {
		t.Errorf("expected 2 clients, got %d", len(clients))
	}
}

func TestManager_RegisterNil(t *testing.T) {
	m := NewManager()
	m.Register(nil)

	if len(m.Clients()) != 0 {
		t.Error("expected 0 clients after registering nil")
	}
}

func TestManager_Unregister(t *testing.T) {
	m := NewManager()

	c1 := &mockClient{name: "client1"}
	c2 := &mockClient{name: "client2"}

	m.Register(c1)
	m.Register(c2)
	m.Unregister("client1")

	clients := m.Clients()
	if len(clients) != 1 {
		t.Errorf("expected 1 client, got %d", len(clients))
	}
	if clients[0].Name() != "client2" {
		t.Errorf("expected client2, got %s", clients[0].Name())
	}
}

func TestManager_HealthCheck(t *testing.T) {
	m := NewManager()

	c1 := &mockClient{name: "healthy", pingErr: nil}
	c2 := &mockClient{name: "unhealthy", pingErr: errors.New("connection failed")}

	m.Register(c1)
	m.Register(c2)

	results := m.HealthCheck(context.Background())
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	healthyFound := false
	unhealthyFound := false
	for _, r := range results {
		if r.Name == "healthy" {
			healthyFound = true
			if !r.Healthy || r.Error != nil {
				t.Error("healthy client should be healthy")
			}
		}
		if r.Name == "unhealthy" {
			unhealthyFound = true
			if r.Healthy || r.Error == nil {
				t.Error("unhealthy client should be unhealthy")
			}
		}
	}

	if !healthyFound || !unhealthyFound {
		t.Error("expected both clients in results")
	}
}

func TestManager_HealthCheckMap(t *testing.T) {
	m := NewManager()

	c1 := &mockClient{name: "healthy", pingErr: nil}
	c2 := &mockClient{name: "unhealthy", pingErr: errors.New("failed")}

	m.Register(c1)
	m.Register(c2)

	results := m.HealthCheckMap(context.Background())
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	if results["healthy"] != nil {
		t.Error("healthy should have nil error")
	}
	if results["unhealthy"] == nil {
		t.Error("unhealthy should have error")
	}
}

func TestManager_IsHealthy(t *testing.T) {
	m := NewManager()

	c1 := &mockClient{name: "healthy1", pingErr: nil}
	c2 := &mockClient{name: "healthy2", pingErr: nil}

	m.Register(c1)
	m.Register(c2)

	if !m.IsHealthy(context.Background()) {
		t.Error("all clients are healthy, should return true")
	}

	c3 := &mockClient{name: "unhealthy", pingErr: errors.New("failed")}
	m.Register(c3)

	if m.IsHealthy(context.Background()) {
		t.Error("one client is unhealthy, should return false")
	}
}

func TestManager_Close(t *testing.T) {
	m := NewManager()

	c1 := &mockClient{name: "client1"}
	c2 := &mockClient{name: "client2", closeErr: errors.New("close error")}

	m.Register(c1)
	m.Register(c2)

	err := m.Close()
	if err == nil {
		t.Error("expected error from close")
	}

	if len(m.Clients()) != 0 {
		t.Error("clients should be cleared after close")
	}
}

func TestManager_Get(t *testing.T) {
	m := NewManager()

	c1 := &mockClient{name: "client1"}
	c2 := &mockClient{name: "client2"}

	m.Register(c1)
	m.Register(c2)

	got := m.Get("client1")
	if got == nil {
		t.Error("expected to get client1")
	}
	if got.Name() != "client1" {
		t.Errorf("expected client1, got %s", got.Name())
	}

	got = m.Get("nonexistent")
	if got != nil {
		t.Error("expected nil for nonexistent client")
	}
}

func TestGlobalManager(t *testing.T) {
	m1 := GlobalManager()
	m2 := GlobalManager()

	if m1 != m2 {
		t.Error("GlobalManager should return singleton")
	}
}
