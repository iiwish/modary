//go:build linux

package projecttool

import (
	"errors"
	"syscall"
)

func ignorableBuildGroupSignalError(err error) bool {
	return errors.Is(err, syscall.ESRCH)
}
