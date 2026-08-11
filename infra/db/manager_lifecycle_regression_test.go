package db

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type lifecycleClient struct {
	name       string
	closeErr   error
	closeCount atomic.Int32
	onClose    func()
}

func (*lifecycleClient) Ping(context.Context) error { return nil }

func (c *lifecycleClient) Close() error {
	c.closeCount.Add(1)
	if c.onClose != nil {
		c.onClose()
	}
	return c.closeErr
}

func (c *lifecycleClient) Name() string { return c.name }

func TestManagerLifecycleCallbacksRunOutsideManagerLock(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Manager, *lifecycleClient) error
	}{
		{
			name: "replace",
			run: func(manager *Manager, client *lifecycleClient) error {
				manager.Register(client)
				manager.Register(&lifecycleClient{name: client.name})
				return nil
			},
		},
		{
			name: "unregister",
			run: func(manager *Manager, client *lifecycleClient) error {
				manager.Register(client)
				return manager.Unregister(client.name)
			},
		},
		{
			name: "close",
			run: func(manager *Manager, client *lifecycleClient) error {
				manager.Register(client)
				return manager.Close()
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := NewManager()
			client := &lifecycleClient{name: "primary"}
			client.onClose = func() { _ = manager.Get("primary") }

			done := make(chan error, 1)
			go func() { done <- test.run(manager, client) }()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("lifecycle operation error = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("lifecycle callback deadlocked while re-entering Manager")
			}
		})
	}
}

func TestManagerClosePreservesEveryClientError(t *testing.T) {
	firstErr := errors.New("first close failure")
	secondErr := errors.New("second close failure")
	manager := NewManager()
	manager.Register(&lifecycleClient{name: "first", closeErr: firstErr})
	manager.Register(&lifecycleClient{name: "second", closeErr: secondErr})

	err := manager.Close()
	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Fatalf("Close() error = %v, want both client error chains", err)
	}
}

func TestManagerRegisteringSameClientDoesNotCloseIt(t *testing.T) {
	manager := NewManager()
	client := &lifecycleClient{name: "primary"}
	manager.Register(client)
	manager.Register(client)

	if got := client.closeCount.Load(); got != 0 {
		t.Fatalf("same-client replacement close count = %d, want 0", got)
	}
	if got := manager.Get(client.name); got != client {
		t.Fatalf("registered client = %p, want %p", got, client)
	}
}

func TestGlobalManagerConcurrentGetAndSet(t *testing.T) {
	globalManagerMu.Lock()
	previous := globalManager
	globalManager = nil
	globalManagerMu.Unlock()
	t.Cleanup(func() { SetGlobalManager(previous) })

	const iterations = 1000
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		for range iterations {
			_ = GlobalManager()
		}
	}()
	go func() {
		defer wait.Done()
		for range iterations {
			SetGlobalManager(NewManager())
		}
	}()
	wait.Wait()
}
