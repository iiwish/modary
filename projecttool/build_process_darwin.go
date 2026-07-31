//go:build darwin

package projecttool

import (
	"errors"
	"syscall"
)

func ignorableBuildGroupSignalError(err error) bool {
	// Darwin reports EPERM when a process group contains only the observed
	// zombie leader. Any live descendant created by this same-user toolchain is
	// signalable, in which case kill(2) succeeds even though the zombie remains.
	return errors.Is(err, syscall.ESRCH) || errors.Is(err, syscall.EPERM)
}
