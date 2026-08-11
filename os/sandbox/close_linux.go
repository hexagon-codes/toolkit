//go:build linux

package sandbox

func (*linuxSandbox) sandboxCloseRetryable() {}

// Close 重试收敛仍由 Linux 后端持有的根进程 Wait 所有权。
func (s *linuxSandbox) Close() error {
	return s.executions.Close(posixTerminationWaitLimit)
}
