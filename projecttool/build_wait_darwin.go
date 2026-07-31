//go:build darwin

package projecttool

import (
	"os/exec"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

const darwinWaitPID = 1

func waitBuildCommandExit(command *exec.Cmd) error {
	// Darwin's x/sys package exposes waitid constants but not its wrapper.
	// siginfo_t is currently 104 bytes and pointer-aligned; this deliberately
	// over-allocated uintptr array provides both the required size and alignment.
	var info [16]uintptr
	for {
		_, _, errno := unix.Syscall6(
			unix.SYS_WAITID,
			darwinWaitPID,
			uintptr(command.Process.Pid),
			uintptr(unsafe.Pointer(&info[0])),
			unix.WEXITED|unix.WNOWAIT,
			0,
			0,
		)
		runtime.KeepAlive(&info)
		if errno == unix.EINTR {
			continue
		}
		if errno != 0 {
			return errno
		}
		return nil
	}
}
