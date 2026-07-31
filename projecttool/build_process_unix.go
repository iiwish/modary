//go:build linux || darwin

package projecttool

import (
	"os"
	"os/exec"
	"syscall"
)

func configureBuildCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return os.ErrProcessDone
		}
		if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil {
			if ignorableBuildGroupSignalError(err) {
				return os.ErrProcessDone
			}
			return err
		}
		return nil
	}
}

func cleanupBuildCommand(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return nil
	}
	if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil && !ignorableBuildGroupSignalError(err) {
		return err
	}
	return nil
}
