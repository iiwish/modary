//go:build darwin

package projecttool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwinBuildStagingRejectsInheritedExtendedACL(t *testing.T) {
	parent := t.TempDir()
	entry := "everyone allow list,search,add_file,add_subdirectory,file_inherit,directory_inherit"
	if output, err := exec.Command("/bin/chmod", "+a", entry, parent).CombinedOutput(); err != nil {
		t.Fatalf("install test ACL: %v: %s", err, output)
	}
	defer func() {
		if output, err := exec.Command("/bin/chmod", "-RN", parent).CombinedOutput(); err != nil {
			t.Errorf("remove test ACL: %v: %s", err, output)
		}
	}()
	parentFile, err := os.Open(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer parentFile.Close()
	parentInfo, err := parentFile.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateBuildStagingParentProtection(parentFile, parentInfo); err == nil || !strings.Contains(err.Error(), "must not have an extended ACL") {
		t.Fatalf("temporary parent ACL validation error = %v", err)
	}
	directory := filepath.Join(parent, "staging")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateBuildStagingProtection(file, info); err == nil || !strings.Contains(err.Error(), "must not have an extended ACL") {
		t.Fatalf("inherited ACL validation error = %v", err)
	}
}
