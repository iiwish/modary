//go:build !darwin && !linux

package sqlite

import "os"

// normalizeOptions rejects file-backed storage before these hooks can run.
func validateFinalSQLitePathACL(*os.File, string, string) error { return nil }

func validateAncestorSQLitePathACL(*os.File, string, string) error { return nil }
