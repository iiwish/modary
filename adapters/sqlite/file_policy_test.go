package sqlite

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOpenedSQLitePathValidationPinsIdentity(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("secure file-backed profile is unavailable on this platform")
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "modary.db")
	if err := os.WriteFile(path, nil, databaseFilePermissions); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	err = validateOpenedSQLitePath(
		file,
		path,
		"database",
		info,
		uint64(os.Geteuid()),
		validateOwnerOnlyFileInfo,
		func(_ *os.File, _, _ string) error {
			if err := os.Rename(path, path+".original"); err != nil {
				return err
			}
			return os.WriteFile(path, nil, databaseFilePermissions)
		},
	)
	if err == nil || !strings.Contains(err.Error(), "changed during security validation") {
		t.Fatalf("path replacement validation error = %v", err)
	}
}
