//go:build !darwin && !linux

package appcmd

import (
	"os"
)

const cliTokenFilePermissionsEnforced = true

func validateCLITokenFilePathSupport() error { return errCLITokenFilePathUnsupported }

func validateCLITokenFileMetadata(os.FileInfo) error {
	return errCLITokenFilePathUnsupported
}

func validateOpenedCLITokenFile(*os.File, os.FileInfo) error {
	return errCLITokenFilePathUnsupported
}
