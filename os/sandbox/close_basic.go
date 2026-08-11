//go:build !darwin && !linux && !windows

package sandbox

func (*basicSandbox) sandboxCloseRetryable() {}

// Close 重试收敛仍由基础 POSIX 后端持有的根进程 Wait 所有权。
func (s *basicSandbox) Close() error {
	return s.executions.Close(posixTerminationWaitLimit)
}
