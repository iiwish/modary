//go:build !linux && !darwin

package projecttool

import (
	"io/fs"
	"os"
)

func validateSecureBuildPlatform() error { return ErrBuildUnsupported }

func validateBuildStagingParentProtection(*os.File, fs.FileInfo) error { return ErrBuildUnsupported }

func validateBuildStagingProtection(*os.File, fs.FileInfo) error { return ErrBuildUnsupported }

func validBuildOutputMode(fs.FileMode) bool {
	return false
}
