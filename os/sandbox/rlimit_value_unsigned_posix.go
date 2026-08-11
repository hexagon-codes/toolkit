//go:build !windows && !freebsd && !dragonfly

package sandbox

import "golang.org/x/sys/unix"

// posixRlimitValues 把平台原生无符号 rlimit 值转换为统一比较语义。
func posixRlimitValues(limit unix.Rlimit) (current, maximum uint64, currentInfinite, maximumInfinite bool) {
	return limit.Cur, limit.Max, limit.Cur == unix.RLIM_INFINITY, limit.Max == unix.RLIM_INFINITY
}

func setPosixRlimitCurrent(limit *unix.Rlimit, value uint64) error {
	limit.Cur = value
	return nil
}
