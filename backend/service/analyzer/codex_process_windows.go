//go:build windows

package analyzer

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func configureCodexProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

func attachCodexProcessTree(command *exec.Cmd) (func() error, func() error, error) {
	if command.Process == nil {
		return nil, nil, fmt.Errorf("Codex process was not started")
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return nil, nil, err
	}
	processHandle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(command.Process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return nil, nil, err
	}
	assignErr := windows.AssignProcessToJobObject(job, processHandle)
	_ = windows.CloseHandle(processHandle)
	if assignErr != nil {
		_ = windows.CloseHandle(job)
		return nil, nil, assignErr
	}
	return func() error {
			return windows.TerminateJobObject(job, 1)
		}, func() error {
			return windows.CloseHandle(job)
		}, nil
}
