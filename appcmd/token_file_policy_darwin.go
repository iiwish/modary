//go:build darwin

package appcmd

import (
	"fmt"
	"os"
	"syscall"

	"github.com/iiwish/modary/internal/filepolicy"
)

const cliTokenFilePermissionsEnforced = true

func validateCLITokenFilePathSupport() error { return nil }

func validateCLITokenFileMetadata(info os.FileInfo) error {
	if err := validateCLITokenFileMode(info); err != nil {
		return err
	}
	metadata, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("token file ownership is unavailable")
	}
	if uint64(metadata.Uid) != uint64(os.Geteuid()) {
		return fmt.Errorf("token file must be owned by the effective user")
	}
	return nil
}

func validateOpenedCLITokenFile(file *os.File, expected os.FileInfo) error {
	if file == nil || expected == nil {
		return fmt.Errorf("opened token file security handle is unavailable")
	}
	current, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect opened token file security state: %w", err)
	}
	if !sameCLIFileState(expected, current) {
		return fmt.Errorf("opened token file changed during security validation")
	}
	if err := validateCLITokenFileMetadata(current); err != nil {
		return err
	}
	hasACL, err := filepolicy.HasExtendedACL(file)
	if err != nil {
		return fmt.Errorf("verify opened token file ACL: %w", err)
	}
	if hasACL {
		return fmt.Errorf("token file must not have an extended ACL")
	}
	return nil
}
