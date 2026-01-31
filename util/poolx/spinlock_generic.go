//go:build !amd64 && !arm64

package poolx

// procyieldImpl is the generic fallback for unsupported platforms.
// It uses a busy loop as a pause hint.
//
//go:noinline
func procyieldImpl(cycles int) {
	for i := 0; i < cycles; i++ {
		// This loop acts as a pause/yield hint to the CPU
		// The noinline directive prevents the compiler from optimizing this away
	}
}
