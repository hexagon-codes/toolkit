package poolx

import (
	"runtime"
	"sync/atomic"
	"unsafe"
)

// ============================================================================
// Sharded Counter - Reduces contention on high-frequency counters
// ============================================================================

const (
	// numShards is the number of counter shards.
	// Should be a power of 2 for efficient modulo operation.
	// 32 shards works well for most multi-core systems.
	numShards = 32
	shardMask = numShards - 1
)

// counterShard is a single shard with cache line padding to prevent false sharing
type counterShard struct {
	_     CacheLinePad
	value atomic.Int64
	_     CacheLinePad
}

// ShardedCounter is a high-performance counter that distributes updates
// across multiple shards to reduce contention in high-concurrency scenarios.
type ShardedCounter struct {
	shards [numShards]counterShard
}

// NewShardedCounter creates a new sharded counter
func NewShardedCounter() *ShardedCounter {
	return &ShardedCounter{}
}

// getShard returns the shard for the current goroutine.
// Uses a simple hash based on the goroutine ID (approximated by stack address).
func (c *ShardedCounter) getShard() *counterShard {
	// Use runtime.fastrand for better distribution
	// This is cheaper than getting actual goroutine ID
	var x [1]byte
	ptr := uintptr(unsafe_pointer(&x[0]))
	idx := (ptr >> 12) & shardMask // Use bits from stack address
	return &c.shards[idx]
}

// Add atomically adds delta to the counter
func (c *ShardedCounter) Add(delta int64) int64 {
	return c.getShard().value.Add(delta)
}

// Inc increments the counter by 1
func (c *ShardedCounter) Inc() int64 {
	return c.Add(1)
}

// Dec decrements the counter by 1
func (c *ShardedCounter) Dec() int64 {
	return c.Add(-1)
}

// Load returns the total value across all shards.
// Note: This is not atomic with respect to concurrent updates.
func (c *ShardedCounter) Load() int64 {
	var total int64
	for i := range c.shards {
		total += c.shards[i].value.Load()
	}
	return total
}

// Store sets the counter to a specific value.
// This is achieved by storing the value in shard 0 and zeroing others.
func (c *ShardedCounter) Store(val int64) {
	c.shards[0].value.Store(val)
	for i := 1; i < numShards; i++ {
		c.shards[i].value.Store(0)
	}
}

// Reset resets the counter to zero
func (c *ShardedCounter) Reset() {
	for i := range c.shards {
		c.shards[i].value.Store(0)
	}
}

// ============================================================================
// Sharded Int32 Counter (for smaller counters)
// ============================================================================

// counterShard32 is a single shard for int32 counters
type counterShard32 struct {
	_     CacheLinePad
	value atomic.Int32
	_     CacheLinePad
}

// ShardedCounter32 is a high-performance int32 counter
type ShardedCounter32 struct {
	shards [numShards]counterShard32
}

// NewShardedCounter32 creates a new sharded int32 counter
func NewShardedCounter32() *ShardedCounter32 {
	return &ShardedCounter32{}
}

// getShard returns the shard for the current goroutine
func (c *ShardedCounter32) getShard() *counterShard32 {
	var x [1]byte
	ptr := uintptr(unsafe_pointer(&x[0]))
	idx := (ptr >> 12) & shardMask
	return &c.shards[idx]
}

// Add atomically adds delta to the counter
func (c *ShardedCounter32) Add(delta int32) int32 {
	return c.getShard().value.Add(delta)
}

// Inc increments the counter by 1
func (c *ShardedCounter32) Inc() int32 {
	return c.Add(1)
}

// Dec decrements the counter by 1
func (c *ShardedCounter32) Dec() int32 {
	return c.Add(-1)
}

// Load returns the total value across all shards
func (c *ShardedCounter32) Load() int32 {
	var total int32
	for i := range c.shards {
		total += c.shards[i].value.Load()
	}
	return total
}

// Store sets the counter to a specific value
func (c *ShardedCounter32) Store(val int32) {
	c.shards[0].value.Store(val)
	for i := 1; i < numShards; i++ {
		c.shards[i].value.Store(0)
	}
}

// Reset resets the counter to zero
func (c *ShardedCounter32) Reset() {
	for i := range c.shards {
		c.shards[i].value.Store(0)
	}
}

// ============================================================================
// Fast Counter - Uses GOMAXPROCS-based sharding
// ============================================================================

// FastCounter uses GOMAXPROCS-based sharding for optimal performance.
// It's simpler but effective for most use cases.
type FastCounter struct {
	_      CacheLinePad
	shards []counterShard
	mask   uint64
	_      CacheLinePad
}

// NewFastCounter creates a new fast counter with automatic shard count
func NewFastCounter() *FastCounter {
	// Use at least 8 shards, up to GOMAXPROCS * 2
	n := runtime.GOMAXPROCS(0) * 2
	if n < 8 {
		n = 8
	}
	// Round up to power of 2
	size := 1
	for size < n {
		size <<= 1
	}

	return &FastCounter{
		shards: make([]counterShard, size),
		mask:   uint64(size - 1),
	}
}

// Add atomically adds delta to the counter
func (c *FastCounter) Add(delta int64) {
	var x [1]byte
	ptr := uint64(uintptr(unsafe_pointer(&x[0])))
	idx := (ptr >> 12) & c.mask
	c.shards[idx].value.Add(delta)
}

// Load returns the total value
func (c *FastCounter) Load() int64 {
	var total int64
	for i := range c.shards {
		total += c.shards[i].value.Load()
	}
	return total
}

// Reset resets the counter
func (c *FastCounter) Reset() {
	for i := range c.shards {
		c.shards[i].value.Store(0)
	}
}

// ============================================================================
// unsafe_pointer helper
// ============================================================================

//go:nosplit
//go:nocheckptr
func unsafe_pointer(p *byte) unsafe.Pointer {
	return unsafe.Pointer(p)
}
