//go:build windows

package sandbox

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

// Win32 API bindings — pure Go syscall, zero CGo.
//
// Phase 8 D29: Token + Job Object + ACL + Network + Desktop isolation.
// References:
//   - Codex codex-windows-sandbox (~2000 lines Rust)
//   - https://learn.microsoft.com/en-us/windows/win32/api/

var (
	modAdvapi32 = syscall.NewLazyDLL("advapi32.dll")
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")

	// 令牌管理。
	procCreateRestrictedToken = modAdvapi32.NewProc("CreateRestrictedToken")
	procSetTokenInformation   = modAdvapi32.NewProc("SetTokenInformation")

	// Job Object 管理。
	procCreateJobObjectW         = modKernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = modKernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = modKernel32.NewProc("AssignProcessToJobObject")
	procTerminateJobObject       = modKernel32.NewProc("TerminateJobObject")
)

const securityMandatoryLowRID = 0x1000

// 下列常量对应 JOBOBJECT_EXTENDED_LIMIT_INFORMATION 的限制位。
const (
	jobObjectLimitProcessMemory = 0x00000100
	jobObjectLimitJobMemory     = 0x00000200
	jobObjectLimitActiveProcess = 0x00000008
	jobObjectLimitKillOnClose   = 0x00002000
)

// Placeholder: actual Win32 struct definitions will be filled on Windows
type (
	jobObjectExtendedLimitInformation struct {
		BasicLimitInformation struct {
			PerProcessUserTimeLimit int64
			PerJobUserTimeLimit     int64
			LimitFlags              uint32
			MinimumWorkingSetSize   uintptr
			MaximumWorkingSetSize   uintptr
			ActiveProcessLimit      uint32
			Affinity                uintptr
			PriorityClass           uint32
			SchedulingClass         uint32
		}
		IoInfo struct {
			ReadOperationCount  uint64
			WriteOperationCount uint64
			OtherOperationCount uint64
			ReadTransferCount   uint64
			WriteTransferCount  uint64
			OtherTransferCount  uint64
		}
		ProcessMemoryLimit    uintptr
		JobMemoryLimit        uintptr
		PeakProcessMemoryUsed uintptr
		PeakJobMemoryUsed     uintptr
	}
)

// createJobObject 创建始终具备关闭终止语义，并按需启用资源配额的 Job Object。
func createJobObject(memoryLimitBytes int64, maxProcesses int) (syscall.Handle, error) {
	info, err := windowsJobLimitInformation(memoryLimitBytes, maxProcesses)
	if err != nil {
		return 0, err
	}
	h, _, err := procCreateJobObjectW.Call(0, 0)
	if h == 0 {
		return 0, err
	}
	handle := syscall.Handle(h)

	r, _, err := procSetInformationJobObject.Call(
		uintptr(handle),
		9,                              // JobObjectExtendedLimitInformation 作业对象扩展限额信息类。
		uintptr(unsafe.Pointer(&info)), // #nosec G103 -- 结构体布局与 JOBOBJECT_EXTENDED_LIMIT_INFORMATION ABI 一致。
		unsafe.Sizeof(info),
	)
	if r == 0 {
		return 0, errors.Join(err, syscall.CloseHandle(handle))
	}
	return handle, nil
}

func windowsJobLimitInformation(memoryLimitBytes int64, maxProcesses int) (jobObjectExtendedLimitInformation, error) {
	if memoryLimitBytes < 0 {
		return jobObjectExtendedLimitInformation{}, fmt.Errorf("job object memory limit must not be negative")
	}
	if maxProcesses < 0 {
		return jobObjectExtendedLimitInformation{}, fmt.Errorf("job object process limit must not be negative")
	}
	if uint64(maxProcesses) > uint64(^uint32(0)) {
		return jobObjectExtendedLimitInformation{}, fmt.Errorf("job object process limit exceeds uint32")
	}

	info := jobObjectExtendedLimitInformation{}
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnClose
	if memoryLimitBytes > 0 {
		memoryBytes := uint64(memoryLimitBytes)
		if memoryBytes > uint64(^uintptr(0)) {
			return jobObjectExtendedLimitInformation{}, fmt.Errorf("job object memory limit exceeds uintptr")
		}
		info.BasicLimitInformation.LimitFlags |= jobObjectLimitProcessMemory | jobObjectLimitJobMemory
		info.ProcessMemoryLimit = uintptr(memoryBytes)
		info.JobMemoryLimit = uintptr(memoryBytes)
	}
	if maxProcesses > 0 {
		info.BasicLimitInformation.LimitFlags |= jobObjectLimitActiveProcess
		info.BasicLimitInformation.ActiveProcessLimit = uint32(maxProcesses) // #nosec G115 -- 上方已校验 uint32 边界。
	}
	return info, nil
}
