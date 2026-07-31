//go:build darwin

package appcmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestDarwinCLITokenFilePolicy(t *testing.T) {
	name := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(name, []byte(strings.Repeat("t", int(minimumCLITokenBytes))), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []os.FileMode{0o400, 0o600} {
		if err := os.Chmod(name, mode); err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := validateCLITokenFileMetadata(info); err != nil {
			_ = file.Close()
			t.Fatalf("validate mode %04o metadata: %v", mode, err)
		}
		if err := validateOpenedCLITokenFile(file, info); err != nil {
			_ = file.Close()
			t.Fatalf("validate opened mode %04o file: %v", mode, err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.Chmod(name, 0o640); err != nil {
		t.Fatal(err)
	}
	insecure, err := os.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCLITokenFileMetadata(insecure); err == nil || !strings.Contains(err.Error(), "0400 or 0600") {
		t.Fatalf("insecure mode validation error = %v", err)
	}
	if directory, err := os.Lstat(t.TempDir()); err != nil {
		t.Fatal(err)
	} else if err := validateCLITokenFileMetadata(directory); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("directory validation error = %v", err)
	}
}

func TestDarwinCLITokenFilePolicyRejectsForeignOrMissingOwnership(t *testing.T) {
	name := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(name, []byte(strings.Repeat("t", int(minimumCLITokenBytes))), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}
	metadata := *info.Sys().(*syscall.Stat_t)
	metadata.Uid = uint32(uint64(os.Geteuid()) + 1)
	foreign := tokenFileInfoWithMetadata{FileInfo: info, metadata: &metadata}
	if err := validateCLITokenFileMetadata(foreign); err == nil || !strings.Contains(err.Error(), "effective user") {
		t.Fatalf("foreign owner validation error = %v", err)
	}
	missing := tokenFileInfoWithMetadata{FileInfo: info}
	if err := validateCLITokenFileMetadata(missing); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("missing owner validation error = %v", err)
	}
}

func TestDarwinCLITokenFilePolicyRejectsExtendedACLFromOpenedDescriptor(t *testing.T) {
	name := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(name, []byte(strings.Repeat("t", int(minimumCLITokenBytes))), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if output, err := exec.Command("/bin/chmod", "+a", "everyone allow read", name).CombinedOutput(); err != nil {
		t.Fatalf("install test ACL: %v: %s", err, output)
	}
	t.Cleanup(func() {
		if output, err := exec.Command("/bin/chmod", "-N", name).CombinedOutput(); err != nil {
			t.Errorf("remove test ACL: %v: %s", err, output)
		}
	})
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateOpenedCLITokenFile(file, info); err == nil || !strings.Contains(err.Error(), "must not have an extended ACL") {
		t.Fatalf("extended ACL validation error = %v", err)
	}
	if _, err := readCLITokenFile(context.Background(), name, maximumCLITokenBytes+2); err == nil || !strings.Contains(err.Error(), "must not have an extended ACL") {
		t.Fatalf("read extended-ACL token file error = %v", err)
	}
}

type tokenFileInfoWithMetadata struct {
	os.FileInfo
	metadata any
}

func (info tokenFileInfoWithMetadata) Sys() any { return info.metadata }
