//go:build darwin

package sqlite

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwinSQLiteFinalPathsRejectExtendedACLs(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "data")
	if err := os.Mkdir(directory, databaseDirectoryPermissions); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "modary.db")
	if err := os.WriteFile(path, nil, databaseFilePermissions); err != nil {
		t.Fatal(err)
	}
	sidecar := path + "-wal"
	if err := os.WriteFile(sidecar, nil, databaseFilePermissions); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name  string
		path  string
		entry string
		kind  string
	}{
		{name: "directory deny ACL", path: directory, entry: "everyone deny delete", kind: "database directory"},
		{name: "database permit ACL", path: path, entry: "everyone allow read", kind: "database"},
		{name: "sidecar permit ACL", path: sidecar, entry: "everyone allow read", kind: "sidecar"},
	} {
		t.Run(test.name, func(t *testing.T) {
			installDarwinACL(t, test.path, test.entry)
			info, err := os.Lstat(test.path)
			if err != nil {
				t.Fatal(err)
			}
			if info.IsDir() {
				err = validateProtectedDirectory(test.path, info, uint64(os.Geteuid()), true)
			} else {
				_, err = inspectSecureFile(test.path, test.kind, true, uint64(os.Geteuid()))
			}
			if err == nil || !strings.Contains(err.Error(), "must not have an extended ACL") {
				t.Fatalf("final path ACL validation error = %v", err)
			}
		})
	}
}

func TestDarwinSQLiteAncestorAllowsDenyOnlyACLAndRejectsPermitACL(t *testing.T) {
	ancestor := filepath.Join(t.TempDir(), "ancestor")
	if err := os.Mkdir(ancestor, databaseDirectoryPermissions); err != nil {
		t.Fatal(err)
	}
	final := filepath.Join(ancestor, "data")
	if err := os.Mkdir(final, databaseDirectoryPermissions); err != nil {
		t.Fatal(err)
	}
	resolvedAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		t.Fatal(err)
	}
	resolvedFinal := filepath.Join(resolvedAncestor, "data")

	installDarwinACL(t, ancestor, "everyone deny delete")
	if err := validateProtectedDirectoryAncestry(resolvedAncestor, uint64(os.Geteuid())); err != nil {
		t.Fatalf("deny-only ancestor ACL rejected: %v", err)
	}
	if info, err := os.Lstat(resolvedFinal); err != nil {
		t.Fatal(err)
	} else if err := validateProtectedDirectory(resolvedFinal, info, uint64(os.Geteuid()), true); err != nil {
		t.Fatalf("ACL-free final directory rejected: %v", err)
	}

	installDarwinACL(t, ancestor, "everyone allow list,search")
	if err := validateProtectedDirectoryAncestry(resolvedAncestor, uint64(os.Geteuid())); err == nil || !strings.Contains(err.Error(), "must not contain permit entries") {
		t.Fatalf("permit ancestor ACL validation error = %v", err)
	}
}

func TestDarwinSQLitePrepareAndPostWALValidationApplyACLPolicy(t *testing.T) {
	ancestor := filepath.Join(t.TempDir(), "ancestor")
	if err := os.Mkdir(ancestor, databaseDirectoryPermissions); err != nil {
		t.Fatal(err)
	}
	installDarwinACL(t, ancestor, "everyone deny delete")
	configuredPath := filepath.Join(ancestor, "data", "modary.db")
	preparedPath, err := prepareSecureDatabase(configuredPath, uint64(os.Geteuid()))
	if err != nil {
		t.Fatalf("prepare below deny-only ancestor: %v", err)
	}
	// t.TempDir may be reached through /var -> /private/var; the secure path
	// must still be absolute and canonical regardless of that platform detail.
	resolved, resolveErr := filepath.EvalSymlinks(filepath.Dir(configuredPath))
	if resolveErr != nil || filepath.Dir(preparedPath) != resolved {
		t.Fatalf("prepared path = %q, resolved directory = %q, %v", preparedPath, resolved, resolveErr)
	}

	installDarwinACL(t, preparedPath, "everyone allow read")
	if err := validateSecureDatabaseState(preparedPath, uint64(os.Geteuid())); err == nil || !strings.Contains(err.Error(), "must not have an extended ACL") {
		t.Fatalf("post-WAL state accepted database ACL: %v", err)
	}
}

func installDarwinACL(t *testing.T, path, entry string) {
	t.Helper()
	if output, err := exec.Command("/bin/chmod", "+a", entry, path).CombinedOutput(); err != nil {
		t.Fatalf("install ACL on %s: %v: %s", path, err, output)
	}
	t.Cleanup(func() {
		if output, err := exec.Command("/bin/chmod", "-N", path).CombinedOutput(); err != nil {
			t.Errorf("remove ACL from %s: %v: %s", path, err, output)
		}
	})
}
