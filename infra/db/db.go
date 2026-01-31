// Package db provides database client management utilities.
//
// It defines common interfaces for database clients and provides
// a Manager for unified health checks and graceful shutdown.
package db

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Common errors.
var (
	ErrNotInitialized = errors.New("db: client not initialized")
	ErrAlreadyClosed  = errors.New("db: client already closed")
	ErrHealthCheck    = errors.New("db: health check failed")
	ErrInvalidConfig  = errors.New("db: invalid configuration")
)

// Client defines the common interface for database clients.
// All database implementations should implement this interface.
type Client interface {
	// Ping performs a health check on the database connection.
	// It should return an error if the connection is unhealthy.
	Ping(ctx context.Context) error

	// Close closes the database connection gracefully.
	// It should be safe to call multiple times.
	Close() error

	// Name returns the client name for logging and identification.
	Name() string
}

// Logger defines the logging interface for database operations.
type Logger interface {
	Debug(msg string, keysAndValues ...any)
	Info(msg string, keysAndValues ...any)
	Warn(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
}

// noopLogger is a no-operation logger.
type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}

// DefaultLogger returns a no-op logger.
func DefaultLogger() Logger {
	return noopLogger{}
}

// HealthResult represents the health check result for a single client.
type HealthResult struct {
	Name      string        `json:"name"`
	Healthy   bool          `json:"healthy"`
	Latency   time.Duration `json:"latency"`
	Error     error         `json:"-"`
	ErrorMsg  string        `json:"error,omitempty"`
	CheckedAt time.Time     `json:"checked_at"`
}

// Manager manages multiple database clients for unified health checks and graceful shutdown.
type Manager struct {
	mu      sync.RWMutex
	clients map[string]Client
	logger  Logger
}

// ManagerOption configures a Manager.
type ManagerOption func(*Manager)

// WithLogger sets the logger for the Manager.
func WithManagerLogger(l Logger) ManagerOption {
	return func(m *Manager) {
		if l != nil {
			m.logger = l
		}
	}
}

// NewManager creates a new database client manager.
func NewManager(opts ...ManagerOption) *Manager {
	m := &Manager{
		clients: make(map[string]Client),
		logger:  DefaultLogger(),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Register registers a database client with the manager.
// If a client with the same name exists, it will be replaced.
func (m *Manager) Register(c Client) {
	if c == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	name := c.Name()
	if old, exists := m.clients[name]; exists {
		m.logger.Warn("replacing existing client", "name", name)
		_ = old.Close()
	}
	m.clients[name] = c
	m.logger.Info("registered client", "name", name)
}

// Unregister removes and closes a database client from the manager.
func (m *Manager) Unregister(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, exists := m.clients[name]
	if !exists {
		return nil
	}

	delete(m.clients, name)
	m.logger.Info("unregistered client", "name", name)
	return c.Close()
}

// Get returns a client by name, or nil if not found.
func (m *Manager) Get(name string) Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clients[name]
}

// MustGet returns a client by name, or panics if not found.
func (m *Manager) MustGet(name string) Client {
	c := m.Get(name)
	if c == nil {
		panic("db: client not found: " + name)
	}
	return c
}

// Clients returns all registered clients.
func (m *Manager) Clients() []Client {
	m.mu.RLock()
	defer m.mu.RUnlock()

	clients := make([]Client, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	return clients
}

// Names returns all registered client names.
func (m *Manager) Names() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}
	return names
}

// Len returns the number of registered clients.
func (m *Manager) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients)
}

// HealthCheck performs health checks on all registered clients concurrently.
func (m *Manager) HealthCheck(ctx context.Context) []HealthResult {
	m.mu.RLock()
	clients := make([]Client, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	m.mu.RUnlock()

	if len(clients) == 0 {
		return nil
	}

	results := make([]HealthResult, len(clients))
	var wg sync.WaitGroup

	for i, c := range clients {
		wg.Add(1)
		go func(idx int, client Client) {
			defer wg.Done()

			start := time.Now()
			err := client.Ping(ctx)
			latency := time.Since(start)

			result := HealthResult{
				Name:      client.Name(),
				Healthy:   err == nil,
				Latency:   latency,
				Error:     err,
				CheckedAt: start,
			}
			if err != nil {
				result.ErrorMsg = err.Error()
			}
			results[idx] = result
		}(i, c)
	}

	wg.Wait()
	return results
}

// HealthCheckMap performs health checks and returns results as a map.
func (m *Manager) HealthCheckMap(ctx context.Context) map[string]error {
	results := m.HealthCheck(ctx)
	m2 := make(map[string]error, len(results))
	for _, r := range results {
		m2[r.Name] = r.Error
	}
	return m2
}

// IsHealthy returns true if all clients are healthy.
func (m *Manager) IsHealthy(ctx context.Context) bool {
	for _, r := range m.HealthCheck(ctx) {
		if !r.Healthy {
			return false
		}
	}
	return true
}

// Close closes all registered clients gracefully.
// It attempts to close all clients even if some fail.
// Returns the last error encountered, if any.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var lastErr error
	for name, c := range m.clients {
		if err := c.Close(); err != nil {
			m.logger.Error("failed to close client", "name", name, "error", err)
			lastErr = err
		} else {
			m.logger.Info("closed client", "name", name)
		}
	}
	m.clients = make(map[string]Client)
	return lastErr
}

// Global manager instance.
var (
	globalManager     *Manager
	globalManagerOnce sync.Once
)

// GlobalManager returns the global database client manager singleton.
func GlobalManager() *Manager {
	globalManagerOnce.Do(func() {
		globalManager = NewManager()
	})
	return globalManager
}

// SetGlobalManager sets the global manager instance.
// This should be called before any calls to GlobalManager() if custom configuration is needed.
func SetGlobalManager(m *Manager) {
	globalManager = m
}
