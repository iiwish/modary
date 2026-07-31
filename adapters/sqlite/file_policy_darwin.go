//go:build darwin

package sqlite

import (
	"fmt"
	"os"

	"github.com/iiwish/modary/internal/filepolicy"
)

func validateFinalSQLitePathACL(file *os.File, path, kind string) error {
	acl, err := filepolicy.InspectExtendedACL(file)
	if err != nil {
		return fmt.Errorf("verify SQLite %s %q ACL: %w", kind, path, err)
	}
	if acl.Present {
		return fmt.Errorf("SQLite %s %q must not have an extended ACL", kind, path)
	}
	return nil
}

func validateAncestorSQLitePathACL(file *os.File, path, kind string) error {
	acl, err := filepolicy.InspectExtendedACL(file)
	if err != nil {
		return fmt.Errorf("verify SQLite %s %q ACL: %w", kind, path, err)
	}
	if acl.AllowsAccess {
		return fmt.Errorf("SQLite %s %q extended ACL must not contain permit entries", kind, path)
	}
	return nil
}
