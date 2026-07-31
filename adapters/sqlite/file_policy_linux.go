//go:build linux

package sqlite

import "os"

// Linux POSIX ACL access is constrained by the group-class mode mask. The
// exact 0600 file policy and non-writable final-directory policy therefore
// bound any named-user/group ACL entry to no effective file access or parent
// mutation access for distinct unprivileged users.
func validateFinalSQLitePathACL(*os.File, string, string) error { return nil }

func validateAncestorSQLitePathACL(*os.File, string, string) error { return nil }
