//go:build freebsd || dragonfly

package sandbox

import (
	"fmt"
	"math"

	"golang.org/x/sys/unix"
)

// posixRlimitValues 把 FreeBSD/DragonFly 的有符号 rlimit 值转换为统一比较语义。
func posixRlimitValues(limit unix.Rlimit) (current, maximum uint64, currentInfinite, maximumInfinite bool) {
	currentInfinite = limit.Cur == unix.RLIM_INFINITY
	maximumInfinite = limit.Max == unix.RLIM_INFINITY
	if !currentInfinite && limit.Cur >= 0 {
		current = uint64(limit.Cur)
	}
	if !maximumInfinite && limit.Max >= 0 {
		maximum = uint64(limit.Max)
	}
	return current, maximum, currentInfinite, maximumInfinite
}

func setPosixRlimitCurrent(limit *unix.Rlimit, value uint64) error {
	if value > math.MaxInt64 {
		return fmt.Errorf("POSIX resource limit exceeds platform range")
	}
	limit.Cur = int64(value)
	return nil
}
