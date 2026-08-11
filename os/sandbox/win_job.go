//go:build windows

package sandbox

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

// 下列常量对应 JOBOBJECT_BASIC_UI_RESTRICTIONS 的界面限制位。
const (
	jobObjectUILimitHandles          = 0x00000001
	jobObjectUILimitReadClipboard    = 0x00000002
	jobObjectUILimitWriteClipboard   = 0x00000004
	jobObjectUILimitSystemParameters = 0x00000008
	jobObjectUILimitDisplaySettings  = 0x00000010
	jobObjectUILimitGlobalAtoms      = 0x00000020
	jobObjectUILimitDesktop          = 0x00000040
	jobObjectUILimitExitWindows      = 0x00000080

	jobObjectBasicUIRestrictionsClass = 4
)

type jobObjectBasicUIRestrictions struct {
	UIRestrictionsClass uint32
}

// createSandboxJobObject 创建具备终止兜底、可选资源配额和界面限制的 Job Object。
func createSandboxJobObject(memoryBytes int64, maxProcesses int) (syscall.Handle, error) {
	job, err := createJobObject(memoryBytes, maxProcesses)
	if err != nil {
		return 0, err
	}

	if err := setJobUIRestrictions(job); err != nil {
		return 0, errors.Join(err, syscall.CloseHandle(job))
	}

	return job, nil
}

// setJobUIRestrictions blocks clipboard access, global hooks, atom table,
// desktop creation, display settings changes, and inter-process handle access.
func setJobUIRestrictions(job syscall.Handle) error {
	restrictions := jobObjectBasicUIRestrictions{
		UIRestrictionsClass: jobObjectUILimitDesktop |
			jobObjectUILimitDisplaySettings |
			jobObjectUILimitExitWindows |
			jobObjectUILimitGlobalAtoms |
			jobObjectUILimitHandles |
			jobObjectUILimitReadClipboard |
			jobObjectUILimitSystemParameters |
			jobObjectUILimitWriteClipboard,
	}

	r, _, err := procSetInformationJobObject.Call(
		uintptr(job),
		uintptr(jobObjectBasicUIRestrictionsClass),
		uintptr(unsafe.Pointer(&restrictions)), // #nosec G103 -- 结构体布局与 JOBOBJECT_BASIC_UI_RESTRICTIONS ABI 一致。
		unsafe.Sizeof(restrictions),
	)
	if r == 0 {
		return fmt.Errorf("SetInformationJobObject (UI restrictions): %w", err)
	}
	return nil
}

// assignProcessToJob assigns a process handle to the Job Object.
func assignProcessToJob(job, process syscall.Handle) error {
	r, _, err := procAssignProcessToJobObject.Call(uintptr(job), uintptr(process))
	if r == 0 {
		return fmt.Errorf("AssignProcessToJobObject: %w", err)
	}
	return nil
}

// terminateJob terminates all processes in the Job Object.
func terminateJob(job syscall.Handle, exitCode uint32) error {
	r, _, err := procTerminateJobObject.Call(uintptr(job), uintptr(exitCode))
	if r == 0 {
		return fmt.Errorf("TerminateJobObject: %w", err)
	}
	return nil
}
