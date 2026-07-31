//go:build darwin

package projecttool

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"

	"github.com/iiwish/modary/internal/filepolicy"
)

func validateSecureBuildPlatform() error { return nil }

func validateBuildStagingParentProtection(file *os.File, info fs.FileInfo) error {
	if file == nil || info == nil || !info.IsDir() {
		return fmt.Errorf("operating system temporary directory security handle is unavailable")
	}
	metadata, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("operating system temporary directory ownership is unavailable")
	}
	owner := uint64(metadata.Uid)
	if effectiveUser := uint64(os.Geteuid()); owner != effectiveUser && owner != 0 {
		return fmt.Errorf("operating system temporary directory must be owned by the effective user or root")
	}
	if info.Mode().Perm()&0o022 != 0 && (owner != 0 || info.Mode()&fs.ModeSticky == 0) {
		return fmt.Errorf("writable operating system temporary directory must be root-owned and sticky")
	}
	hasACL, err := filepolicy.HasExtendedACL(file)
	if err != nil {
		return fmt.Errorf("verify operating system temporary directory ACL: %w", err)
	}
	if hasACL {
		return fmt.Errorf("operating system temporary directory must not have an extended ACL")
	}
	return nil
}

func validateBuildStagingProtection(file *os.File, info fs.FileInfo) error {
	if file == nil || info == nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return fmt.Errorf("build staging directory must have owner-only mode 0700")
	}
	metadata, ok := info.Sys().(*syscall.Stat_t)
	if !ok || uint64(metadata.Uid) != uint64(os.Geteuid()) {
		return fmt.Errorf("build staging directory must be owned by the effective user")
	}
	hasACL, err := filepolicy.HasExtendedACL(file)
	if err != nil {
		return fmt.Errorf("verify build staging directory ACL: %w", err)
	}
	if hasACL {
		return fmt.Errorf("build staging directory must not have an extended ACL")
	}
	return nil
}

func validBuildOutputMode(mode fs.FileMode) bool {
	return mode.Perm() == 0o755
}
