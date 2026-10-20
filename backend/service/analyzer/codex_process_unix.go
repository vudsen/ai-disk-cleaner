//go:build !windows

package analyzer

import (
	"fmt"
	"os/exec"
	"syscall"
)

func configureCodexProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func attachCodexProcessTree(command *exec.Cmd) (func() error, func() error, error) {
	if command.Process == nil {
		return nil, nil, fmt.Errorf("Codex process was not started")
	}
	groupID := command.Process.Pid
	return func() error {
		err := syscall.Kill(-groupID, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return nil
		}
		return err
	}, func() error { return nil }, nil
}
