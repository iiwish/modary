//go:build !darwin && !linux

package sqlite

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnsupportedPlatformRejectsFileBackedProfileBeforeFilesystemAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "modary.db")
	if _, err := Module(Options{Path: path}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("file-backed Module error = %v", err)
	}
	if _, err := os.Lstat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsupported file-backed profile touched filesystem: %v", err)
	}
	if _, err := Module(Options{Path: ":memory:"}); err != nil {
		t.Fatalf("portable in-memory profile rejected: %v", err)
	}
}
