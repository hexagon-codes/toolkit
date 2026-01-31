package poolx

import (
	"runtime"
	"sync/atomic"
)

// ============================================================================
// Cache Line Padding
// ============================================================================

// CacheLinePad is used to prevent false sharing between atomic variables
// Most modern CPUs have 64-byte cache lines
type CacheLinePad struct {
	_ [64]byte
}

// ============================================================================
// Spinlock Implementation
// ============================================================================

const (
	spinLocked   = 1
	spinUnlocked = 0

	// Adaptive spinning parameters
	maxSpinCount     = 16   // Maximum spins before yielding
	maxYieldCount    = 8    // Maximum yields before blocking
	spinBackoffBase  = 1    // Base backoff iterations
	spinBackoffLimit = 1024 // Maximum backoff iterations
)

// Spinlock is a low-overhead lock for low-contention scenarios.
// It uses adaptive spinning with exponential backoff.
type Spinlock struct {
	_     CacheLinePad // Prevent false sharing with preceding data
	state atomic.Uint32
	_     CacheLinePad // Prevent false sharing with following data
}

// TryLock attempts to acquire the lock without blocking.
// Returns true if the lock was acquired, false otherwise.
func (s *Spinlock) TryLock() bool {
	return s.state.CompareAndSwap(spinUnlocked, spinLocked)
}

// Lock acquires the lock, spinning with adaptive backoff.
func (s *Spinlock) Lock() {
	// Fast path: try to acquire immediately
	if s.TryLock() {
		return
	}

	// Slow path: adaptive spinning
	s.lockSlow()
}

// lockSlow handles the slow path with adaptive spinning and backoff
func (s *Spinlock) lockSlow() {
	backoff := spinBackoffBase
	spinCount := 0
	yieldCount := 0

	for {
		// Try to acquire the lock
		if s.TryLock() {
			return
		}

		// Adaptive spinning phase
		if spinCount < maxSpinCount {
			// Spin with exponential backoff
			for i := 0; i < backoff; i++ {
				procyield(4) // CPU pause instruction hint
			}
			spinCount++
			if backoff < spinBackoffLimit {
				backoff <<= 1 // Double the backoff
			}
			continue
		}

		// Yield phase - give up CPU to other goroutines
		if yieldCount < maxYieldCount {
			runtime.Gosched()
			yieldCount++
			continue
		}

		// Reset and try again with fresh backoff
		spinCount = 0
		yieldCount = 0
		backoff = spinBackoffBase
		runtime.Gosched()
	}
}

// Unlock releases the lock.
func (s *Spinlock) Unlock() {
	s.state.Store(spinUnlocked)
}

// IsLocked returns true if the lock is currently held.
func (s *Spinlock) IsLocked() bool {
	return s.state.Load() == spinLocked
}

// procyield is a CPU hint for spin-wait loops.
// On amd64/arm64, it uses the PAUSE/YIELD instruction via assembly.
// On other platforms, it falls back to a busy loop.
func procyield(cycles int) {
	procyieldImpl(cycles)
}

// ============================================================================
// Padded Atomic Types (prevent false sharing)
// ============================================================================

// PaddedAtomicInt32 is an atomic int32 with cache line padding
type PaddedAtomicInt32 struct {
	_     CacheLinePad
	value atomic.Int32
	_     CacheLinePad
}

// Load atomically loads the value
func (p *PaddedAtomicInt32) Load() int32 {
	return p.value.Load()
}

// Store atomically stores the value
func (p *PaddedAtomicInt32) Store(val int32) {
	p.value.Store(val)
}

// Add atomically adds delta and returns the new value
func (p *PaddedAtomicInt32) Add(delta int32) int32 {
	return p.value.Add(delta)
}

// CompareAndSwap atomically compares and swaps
func (p *PaddedAtomicInt32) CompareAndSwap(old, new int32) bool {
	return p.value.CompareAndSwap(old, new)
}

// PaddedAtomicInt64 is an atomic int64 with cache line padding
type PaddedAtomicInt64 struct {
	_     CacheLinePad
	value atomic.Int64
	_     CacheLinePad
}

// Load atomically loads the value
func (p *PaddedAtomicInt64) Load() int64 {
	return p.value.Load()
}

// Store atomically stores the value
func (p *PaddedAtomicInt64) Store(val int64) {
	p.value.Store(val)
}

// Add atomically adds delta and returns the new value
func (p *PaddedAtomicInt64) Add(delta int64) int64 {
	return p.value.Add(delta)
}

// CompareAndSwap atomically compares and swaps
func (p *PaddedAtomicInt64) CompareAndSwap(old, new int64) bool {
	return p.value.CompareAndSwap(old, new)
}

// PaddedAtomicUint64 is an atomic uint64 with cache line padding
type PaddedAtomicUint64 struct {
	_     CacheLinePad
	value atomic.Uint64
	_     CacheLinePad
}

// Load atomically loads the value
func (p *PaddedAtomicUint64) Load() uint64 {
	return p.value.Load()
}

// Store atomically stores the value
func (p *PaddedAtomicUint64) Store(val uint64) {
	p.value.Store(val)
}

// Add atomically adds delta and returns the new value
func (p *PaddedAtomicUint64) Add(delta uint64) uint64 {
	return p.value.Add(delta)
}

// CompareAndSwap atomically compares and swaps
func (p *PaddedAtomicUint64) CompareAndSwap(old, new uint64) bool {
	return p.value.CompareAndSwap(old, new)
}
