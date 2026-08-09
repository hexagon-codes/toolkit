package httpx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
)

func TestNewHostPoolRejectsInvalidCapacity(t *testing.T) {
	t.Parallel()

	for _, maxHosts := range []int{0, -1} {
		t.Run(fmt.Sprintf("max_%d", maxHosts), func(t *testing.T) {
			t.Parallel()

			hostPool, err := NewHostPool(HostPoolConfig{
				Pool:     DefaultPoolConfig(),
				MaxHosts: maxHosts,
			})
			if hostPool != nil {
				hostPool.Close()
				t.Fatalf("NewHostPool() pool = %#v, want nil", hostPool)
			}
			if !errors.Is(err, ErrInvalidPoolConfig) {
				t.Fatalf("NewHostPool() error = %v, want %v", err, ErrInvalidPoolConfig)
			}
		})
	}
}

func TestHostPoolRejectsNewHostsAtCapacity(t *testing.T) {
	t.Parallel()

	hostPool, createErr := NewHostPool(HostPoolConfig{
		Pool:     DefaultPoolConfig(),
		MaxHosts: 2,
	})
	if createErr != nil {
		t.Fatal(createErr)
	}
	defer hostPool.Close()

	first, firstErr := hostPool.GetPool("API.EXAMPLE.COM:443")
	if firstErr != nil {
		t.Fatal(firstErr)
	}
	canonical, canonicalErr := hostPool.GetPool("api.example.com:443")
	if canonicalErr != nil {
		t.Fatal(canonicalErr)
	}
	if canonical != first {
		t.Fatal("equivalent host names created different pools")
	}
	if _, secondErr := hostPool.GetPool("second.example.com:443"); secondErr != nil {
		t.Fatal(secondErr)
	}
	if pool, capacityErr := hostPool.GetPool("third.example.com:443"); pool != nil || !errors.Is(capacityErr, ErrHostPoolCapacity) {
		t.Fatalf("third GetPool() = (%v, %v), want nil and capacity error", pool, capacityErr)
	}
	if got := len(hostPool.GetAllStats()); got != 2 {
		t.Fatalf("host pool count = %d, want 2", got)
	}
	if removeErr := hostPool.RemoveHost("api.example.com:443"); removeErr != nil {
		t.Fatalf("RemoveHost() error = %v", removeErr)
	}
	if _, replacementErr := hostPool.GetPool("third.example.com:443"); replacementErr != nil {
		t.Fatalf("GetPool() after removal error = %v", replacementErr)
	}
	request, requestErr := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.example.com", http.NoBody)
	if requestErr != nil {
		t.Fatal(requestErr)
	}
	response, doErr := first.Do(request)
	if response != nil {
		_ = response.Body.Close()
	}
	if !errors.Is(doErr, ErrPoolClosed) {
		t.Fatalf("removed pool Do() error = %v, want pool closed", doErr)
	}
}

func TestHostPoolCapacityIncludesConfiguredHosts(t *testing.T) {
	t.Parallel()

	hostPool, err := NewHostPool(HostPoolConfig{
		Pool:     DefaultPoolConfig(),
		MaxHosts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer hostPool.Close()

	if err := hostPool.SetHostConfig("configured.example.com", DefaultPoolConfig()); err != nil {
		t.Fatal(err)
	}
	if err := hostPool.SetHostConfig("other.example.com", DefaultPoolConfig()); !errors.Is(err, ErrHostPoolCapacity) {
		t.Fatalf("second SetHostConfig() error = %v, want capacity error", err)
	}
	if _, err := hostPool.GetPool("configured.example.com"); err != nil {
		t.Fatalf("GetPool() for reserved host error = %v", err)
	}
	if pool, err := hostPool.GetPool("other.example.com"); pool != nil || !errors.Is(err, ErrHostPoolCapacity) {
		t.Fatalf("unconfigured GetPool() = (%v, %v), want nil and capacity error", pool, err)
	}
}

func TestHostPoolConcurrentCreationNeverExceedsCapacity(t *testing.T) {
	t.Parallel()

	const maxHosts = 8
	hostPool, err := NewHostPool(HostPoolConfig{
		Pool:     DefaultPoolConfig(),
		MaxHosts: maxHosts,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer hostPool.Close()

	var wg sync.WaitGroup
	for index := range 128 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool, getErr := hostPool.GetPool(fmt.Sprintf("host-%d.example.com", index))
			if getErr == nil {
				if pool == nil {
					t.Errorf("GetPool() returned nil without an error")
				}
				return
			}
			if !errors.Is(getErr, ErrHostPoolCapacity) {
				t.Errorf("GetPool() error = %v, want capacity error", getErr)
			}
		}()
	}
	wg.Wait()
	if got := len(hostPool.GetAllStats()); got != maxHosts {
		t.Fatalf("host pool count = %d, want %d", got, maxHosts)
	}
}

func TestHostPoolRejectsOutOfRangePort(t *testing.T) {
	hostPool := NewDefaultHostPool()
	defer hostPool.Close()

	for _, host := range []string{"example.com:", "example.com:0", "example.com:65536"} {
		t.Run(host, func(t *testing.T) {
			pool, err := hostPool.GetPool(host)
			if pool != nil {
				pool.Close()
				t.Fatalf("GetPool(%q) pool = %#v, want nil", host, pool)
			}
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("GetPool(%q) error = %v, want ErrInvalidRequest", host, err)
			}
		})
	}
}
