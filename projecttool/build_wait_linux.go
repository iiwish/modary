//go:build linux

package projecttool

import (
	"os/exec"

	"golang.org/x/sys/unix"
)

func waitBuildCommandExit(command *exec.Cmd) error {
	var info unix.Siginfo
	for {
		err := unix.Waitid(unix.P_PID, command.Process.Pid, &info, unix.WEXITED|unix.WNOWAIT, nil)
		if err == unix.EINTR {
			continue
		}
		return err
	}
}
