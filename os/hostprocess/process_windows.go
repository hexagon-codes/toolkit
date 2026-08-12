package hostprocess

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const procThreadAttributeJobList = 0x0002000D

type jobAccountingInformation struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

func run(ctx context.Context, spec Spec, result Result) (_ Result, resultErr error) {
	job, jobErr := windows.CreateJobObject(nil, nil)
	if jobErr != nil {
		return result, fmt.Errorf("create host process job: %w", jobErr)
	}
	defer func() { resultErr = errors.Join(resultErr, windows.CloseHandle(job)) }()
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return result, fmt.Errorf("configure host process job: %w", err)
	}

	streams := []*os.File{spec.Stdin, spec.Stdout, spec.Stderr}
	handles := make([]windows.Handle, 0, len(streams))
	defer func() {
		for _, handle := range handles {
			resultErr = errors.Join(resultErr, windows.CloseHandle(handle))
		}
	}()
	for _, stream := range streams {
		var handle windows.Handle
		if err := windows.DuplicateHandle(windows.CurrentProcess(), windows.Handle(stream.Fd()), windows.CurrentProcess(), &handle, 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
			return result, fmt.Errorf("duplicate host process stream: %w", err)
		}
		handles = append(handles, handle)
	}
	attributes, attributesErr := windows.NewProcThreadAttributeList(2)
	if attributesErr != nil {
		return result, fmt.Errorf("create host process attributes: %w", attributesErr)
	}
	defer attributes.Delete()
	if err := attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return result, fmt.Errorf("set host process streams: %w", err)
	}
	// 创建时原子加入 Job，载荷不能在纳管之前运行或生成无归属后代。
	if err := attributes.Update(procThreadAttributeJobList, unsafe.Pointer(&job), unsafe.Sizeof(job)); err != nil {
		return result, fmt.Errorf("set host process job: %w", err)
	}
	startup := windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{})), Flags: windows.STARTF_USESTDHANDLES,
			StdInput: handles[0], StdOutput: handles[1], StdErr: handles[2],
		},
		ProcThreadAttributeList: attributes.List(),
	}
	executable, err := windows.UTF16PtrFromString(spec.Executable)
	if err != nil {
		return result, fmt.Errorf("invalid executable: %w", err)
	}
	command, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{spec.Executable}, spec.Args...)))
	if err != nil {
		return result, fmt.Errorf("invalid host process arguments: %w", err)
	}
	directory, err := windows.UTF16PtrFromString(spec.Dir)
	if err != nil {
		return result, fmt.Errorf("invalid host process directory: %w", err)
	}
	environment := utf16.Encode([]rune(strings.Join(spec.Env, "\x00") + "\x00\x00"))
	process := windows.ProcessInformation{}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := windows.CreateProcess(executable, command, nil, nil, true,
		windows.CREATE_UNICODE_ENVIRONMENT|windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_NO_WINDOW,
		&environment[0], directory, &startup.StartupInfo, &process); err != nil {
		return result, fmt.Errorf("start host process: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, windows.CloseHandle(process.Process)) }()
	defer func() { resultErr = errors.Join(resultErr, windows.CloseHandle(process.Thread)) }()
	result.Started, result.PID = true, int(process.ProcessId)

	var cancelErr error
	var cancelDeadline time.Time
	for {
		state, err := windows.WaitForSingleObject(process.Process, 100)
		if err != nil {
			return result, errors.Join(cancelErr, fmt.Errorf("wait for host process: %w", err))
		}
		if state == windows.WAIT_OBJECT_0 {
			var exitCode uint32
			if err := windows.GetExitCodeProcess(process.Process, &exitCode); err != nil {
				return result, errors.Join(cancelErr, fmt.Errorf("read host process exit code: %w", err))
			}
			result.ExitCode, result.Settled = int(exitCode), true
			if cancelErr != nil {
				return result, cancelErr
			}
			var accounting jobAccountingInformation
			if err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil); err != nil {
				return result, fmt.Errorf("inspect host process job: %w", err)
			}
			result.BackgroundProcessesRunning = accounting.ActiveProcesses > 0
			// 正常启动的后台应用保持运行；取消才终止整个本次 Job。
			limits.BasicLimitInformation.LimitFlags = 0
			if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
				return result, fmt.Errorf("release host process job: %w", err)
			}
			if exitCode != 0 {
				return result, fmt.Errorf("host process exited with code %d", exitCode)
			}
			return result, nil
		}
		if state != uint32(windows.WAIT_TIMEOUT) {
			return result, fmt.Errorf("unexpected host process wait state %d", state)
		}
		if cancelErr == nil && ctx.Err() != nil {
			cancelErr = errors.Join(ctx.Err(), windows.TerminateJobObject(job, 1))
			cancelDeadline = time.Now().Add(5 * time.Second)
		}
		if !cancelDeadline.IsZero() && time.Now().After(cancelDeadline) {
			return result, errors.Join(cancelErr, fmt.Errorf("host process termination could not be confirmed"))
		}
	}
}
