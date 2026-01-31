//go:build amd64 || arm64

package poolx

// procyieldAsm is implemented in assembly (spinlock_amd64.s or spinlock_arm64.s)
// It uses PAUSE (x86) or YIELD (ARM) instruction for efficient spin-waiting.
func procyieldAsm(cycles int)

// procyieldImpl uses the assembly implementation on supported platforms
func procyieldImpl(cycles int) {
	procyieldAsm(cycles)
}
