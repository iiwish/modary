//go:build !darwin && !linux

package appcmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOtherPlatformCLITokenFilePathsFailClosed(t *testing.T) {
	name := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(name, []byte(strings.Repeat("t", int(minimumCLITokenBytes))), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCLITokenFileMetadata(info); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("path metadata validation error = %v", err)
	}
	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := validateOpenedCLITokenFile(file, info); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("opened path validation error = %v", err)
	}
	if _, err := readCLITokenFile(context.Background(), name, maximumCLITokenBytes+2); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("read token path error = %v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing-token")
	if _, err := readCLITokenFile(context.Background(), missing, maximumCLITokenBytes+2); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("missing token path error = %v", err)
	}
}

func TestOtherPlatformCLITokenStdinRemainsAvailable(t *testing.T) {
	token := strings.Repeat("t", int(minimumCLITokenBytes))
	data, err := loadCLIToken(context.Background(), "-", io.NopCloser(strings.NewReader(token)))
	if err != nil {
		t.Fatalf("load stdin token: %v", err)
	}
	if string(data) != token {
		t.Fatalf("stdin token = %q", data)
	}
}
