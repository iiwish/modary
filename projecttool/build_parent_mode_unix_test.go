//go:build linux || darwin

package projecttool

import (
	"io/fs"
	"os"
	"syscall"
	"testing"
)

func TestBuildStagingParentOwnershipAndWritableModePolicy(t *testing.T) {
	directory := t.TempDir()
	file, err := os.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	base, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	effectiveUser := uint32(os.Geteuid())
	foreignUser := effectiveUser + 1
	if foreignUser == 0 {
		foreignUser = 1
	}
	for _, test := range []struct {
		name    string
		owner   uint32
		mode    fs.FileMode
		wantErr bool
	}{
		{name: "effective user private", owner: effectiveUser, mode: 0o700},
		{name: "effective user read-only shared", owner: effectiveUser, mode: 0o755},
		{name: "effective user writable", owner: effectiveUser, mode: 0o777, wantErr: true},
		{name: "effective user sticky writable", owner: effectiveUser, mode: 0o777 | fs.ModeSticky, wantErr: effectiveUser != 0},
		{name: "root sticky writable", owner: 0, mode: 0o777 | fs.ModeSticky},
		{name: "root non-sticky writable", owner: 0, mode: 0o777, wantErr: true},
		{name: "foreign owner", owner: foreignUser, mode: 0o755, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			info := buildParentPolicyInfo{FileInfo: base, mode: fs.ModeDir | test.mode, owner: test.owner}
			err := validateBuildStagingParentProtection(file, info)
			if (err != nil) != test.wantErr {
				t.Fatalf("parent policy error = %v, want error %t", err, test.wantErr)
			}
		})
	}
}

type buildParentPolicyInfo struct {
	fs.FileInfo
	mode  fs.FileMode
	owner uint32
}

func (info buildParentPolicyInfo) Mode() fs.FileMode { return info.mode }
func (info buildParentPolicyInfo) IsDir() bool       { return true }
func (info buildParentPolicyInfo) Sys() any          { return &syscall.Stat_t{Uid: info.owner} }
