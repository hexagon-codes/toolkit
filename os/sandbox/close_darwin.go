//go:build darwin

package sandbox

func (*darwinSandbox) sandboxCloseRetryable() {}

// Close 重试收敛仍由 macOS 后端持有的根进程 Wait 所有权。
func (s *darwinSandbox) Close() error {
	return s.executions.Close(posixTerminationWaitLimit)
}
