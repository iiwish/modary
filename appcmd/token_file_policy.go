package appcmd

import (
	"errors"
	"fmt"
	"os"
)

var errCLITokenFilePathUnsupported = errors.New("CLI token file paths are unsupported on this operating system; use standard input")

func validateCLITokenFileMode(info os.FileInfo) error {
	if info == nil || !info.Mode().IsRegular() {
		return fmt.Errorf("token file must be a regular file")
	}
	if permissions := info.Mode().Perm(); permissions != 0o400 && permissions != 0o600 {
		return fmt.Errorf("token file permissions must be 0400 or 0600")
	}
	return nil
}
