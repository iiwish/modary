//go:build darwin

package filepolicy

import (
	"encoding/binary"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestInspectExtendedACLClassifiesDenyAndPermitEntries(t *testing.T) {
	directory := t.TempDir()
	file, err := os.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	acl, err := InspectExtendedACL(file)
	if err != nil {
		t.Fatal(err)
	}
	if acl.Present {
		t.Fatalf("fresh temporary directory unexpectedly has ACL %#v", acl)
	}
	if output, err := exec.Command("/bin/chmod", "+a", "everyone deny delete", directory).CombinedOutput(); err != nil {
		t.Fatalf("install deny ACL: %v: %s", err, output)
	}
	t.Cleanup(func() {
		if output, err := exec.Command("/bin/chmod", "-N", directory).CombinedOutput(); err != nil {
			t.Errorf("remove test ACL: %v: %s", err, output)
		}
	})
	acl, err = InspectExtendedACL(file)
	if err != nil || acl != (ExtendedACL{Present: true}) {
		t.Fatalf("deny-only ACL = %#v, %v", acl, err)
	}
	if output, err := exec.Command("/bin/chmod", "+a", "everyone allow list,search", directory).CombinedOutput(); err != nil {
		t.Fatalf("install permit ACL: %v: %s", err, output)
	}
	acl, err = InspectExtendedACL(file)
	if err != nil || acl != (ExtendedACL{Present: true, AllowsAccess: true}) {
		t.Fatalf("ACL with permit entry = %#v, %v", acl, err)
	}
}

func TestParseExtendedSecurityResponse(t *testing.T) {
	validWithoutACL := extendedSecurityResponse(32, 0, 0, 0, 32)
	validEmptyACL := extendedSecurityResponse(32, unix.ATTR_CMN_EXTENDED_SECURITY, 0, 0, 32)
	validACL := extendedSecurityResponseWithACL(kauthACEDeny)
	validPermitACL := extendedSecurityResponseWithACL(kauthACEPermit)
	tests := []struct {
		name       string
		buffer     []byte
		wantACL    bool
		wantPermit bool
		wantErr    string
	}{
		{name: "truncated", buffer: make([]byte, 31), wantErr: "truncated"},
		{name: "declared header too short", buffer: extendedSecurityResponse(31, 0, 0, 0, 32), wantErr: "invalid length"},
		{name: "declared response too long", buffer: extendedSecurityResponse(33, 0, 0, 0, 32), wantErr: "invalid length"},
		{name: "attribute not returned", buffer: validWithoutACL},
		{name: "empty extended security", buffer: validEmptyACL},
		{name: "extended ACL", buffer: validACL, wantACL: true},
		{name: "permit ACL", buffer: validPermitACL, wantACL: true, wantPermit: true},
		{name: "negative reference", buffer: extendedSecurityResponse(36, unix.ATTR_CMN_EXTENDED_SECURITY, -25, 1, 36), wantErr: "reference is invalid"},
		{name: "reference inside header", buffer: extendedSecurityResponse(36, unix.ATTR_CMN_EXTENDED_SECURITY, 0, 1, 36), wantErr: "reference is invalid"},
		{name: "reference past response", buffer: extendedSecurityResponse(36, unix.ATTR_CMN_EXTENDED_SECURITY, 8, 5, 40), wantErr: "reference is invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			acl, err := parseExtendedSecurityResponse(test.buffer)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("parse error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil || acl.Present != test.wantACL || acl.AllowsAccess != test.wantPermit {
				t.Fatalf("parse result = (%#v, %v), want present=%v permit=%v", acl, err, test.wantACL, test.wantPermit)
			}
		})
	}
}

func TestParseKauthFileSecurityFailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{name: "truncated", data: make([]byte, kauthFileSecurityHeaderLength-1), wantErr: "truncated"},
		{name: "invalid magic", data: kauthFileSecurity(0), wantErr: "invalid magic"},
		{name: "too many entries", data: func() []byte {
			data := kauthFileSecurity(kauthMaximumACLEntries + 1)
			binary.LittleEndian.PutUint32(data[0:4], kauthFileSecurityMagic)
			return data
		}(), wantErr: "exceeds"},
		{name: "truncated entry", data: kauthFileSecurityWithKinds(kauthACEDeny)[:kauthFileSecurityHeaderLength], wantErr: "invalid length"},
		{name: "unknown entry kind", data: kauthFileSecurityWithKinds(3), wantErr: "unsupported kind"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseKauthFileSecurity(test.data)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("parse error = %v, want %q", err, test.wantErr)
			}
		})
	}

	noACL := kauthFileSecurity(kauthFileSecurityNoACL)
	binary.LittleEndian.PutUint32(noACL[0:4], kauthFileSecurityMagic)
	if acl, err := parseKauthFileSecurity(noACL); err != nil || acl != (ExtendedACL{}) {
		t.Fatalf("no ACL result = %#v, %v", acl, err)
	}

	emptyACL := kauthFileSecurity(0)
	binary.LittleEndian.PutUint32(emptyACL[0:4], kauthFileSecurityMagic)
	if acl, err := parseKauthFileSecurity(emptyACL); err != nil || acl != (ExtendedACL{Present: true}) {
		t.Fatalf("empty ACL result = %#v, %v", acl, err)
	}

	denyOnly := kauthFileSecurityWithKinds(kauthACEDeny, kauthACEDeny)
	if acl, err := parseKauthFileSecurity(denyOnly); err != nil || acl != (ExtendedACL{Present: true}) {
		t.Fatalf("deny-only ACL result = %#v, %v", acl, err)
	}

	mixed := kauthFileSecurityWithKinds(kauthACEDeny, kauthACEPermit)
	if acl, err := parseKauthFileSecurity(mixed); err != nil || acl != (ExtendedACL{Present: true, AllowsAccess: true}) {
		t.Fatalf("mixed ACL result = %#v, %v", acl, err)
	}
}

func FuzzParseExtendedSecurityResponse(f *testing.F) {
	f.Add([]byte(nil))
	f.Add(extendedSecurityResponse(32, 0, 0, 0, 32))
	f.Add(extendedSecurityResponseWithACL(kauthACEDeny))
	f.Add(extendedSecurityResponseWithACL(kauthACEPermit))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseExtendedSecurityResponse(data)
	})
}

func FuzzParseKauthFileSecurity(f *testing.F) {
	f.Add([]byte(nil))
	f.Add(kauthFileSecurityWithKinds(kauthACEDeny))
	f.Add(kauthFileSecurityWithKinds(kauthACEPermit))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseKauthFileSecurity(data)
	})
}

func extendedSecurityResponse(total uint32, returned uint32, offset int32, length uint32, size int) []byte {
	buffer := make([]byte, size)
	binary.LittleEndian.PutUint32(buffer[0:4], total)
	binary.LittleEndian.PutUint32(buffer[4:8], returned)
	binary.LittleEndian.PutUint32(buffer[24:28], uint32(offset))
	binary.LittleEndian.PutUint32(buffer[28:32], length)
	return buffer
}

func extendedSecurityResponseWithACL(kinds ...uint32) []byte {
	acl := kauthFileSecurityWithKinds(kinds...)
	total := extendedSecurityHeaderLength + len(acl)
	buffer := extendedSecurityResponse(uint32(total), unix.ATTR_CMN_EXTENDED_SECURITY, 8, uint32(len(acl)), total)
	copy(buffer[extendedSecurityHeaderLength:], acl)
	return buffer
}

func kauthFileSecurity(entryCount uint32) []byte {
	length := kauthFileSecurityHeaderLength
	if entryCount <= kauthMaximumACLEntries {
		length += int(entryCount) * kauthACEByteLength
	}
	data := make([]byte, length)
	binary.LittleEndian.PutUint32(data[36:40], entryCount)
	return data
}

func kauthFileSecurityWithKinds(kinds ...uint32) []byte {
	data := kauthFileSecurity(uint32(len(kinds)))
	binary.LittleEndian.PutUint32(data[0:4], kauthFileSecurityMagic)
	for index, kind := range kinds {
		start := kauthFileSecurityHeaderLength + index*kauthACEByteLength
		binary.LittleEndian.PutUint32(data[start+16:start+20], kind)
	}
	return data
}
