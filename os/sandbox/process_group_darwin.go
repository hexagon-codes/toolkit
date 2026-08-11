//go:build darwin

package sandbox

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// inspectDarwinProcessGroup 用 PID 与启动时间冻结原进程组成员，避免 PGID 复用误伤新进程。
func inspectDarwinProcessGroup(processGroupID int) ([]posixProcessIdentity, error) {
	if processGroupID <= 0 {
		return nil, fmt.Errorf("macOS process group ID must be positive")
	}
	processes, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, fmt.Errorf("inspect macOS process table: %w", err)
	}
	members := make([]posixProcessIdentity, 0)
	for index := range processes {
		process := &processes[index]
		if process.Eproc.Pgid != int32(processGroupID) {
			continue
		}
		members = append(members, posixProcessIdentity{
			pid:       int(process.Proc.P_pid),
			startSec:  process.Proc.P_starttime.Sec,
			startUsec: int64(process.Proc.P_starttime.Usec),
		})
	}
	return members, nil
}
