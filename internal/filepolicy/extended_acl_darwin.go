//go:build darwin

// Package filepolicy implements operating-system file security checks shared by
// framework surfaces that retain an opened file descriptor.
package filepolicy

import (
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	extendedSecurityHeaderLength  = 32
	kauthFileSecurityHeaderLength = 44
	kauthACEByteLength            = 24
	kauthMaximumACLEntries        = 128
	kauthFileSecurityMagic        = 0x012cc16d
	kauthFileSecurityNoACL        = ^uint32(0)
	kauthACEKindMask              = 0x0f
	kauthACEPermit                = 1
	kauthACEDeny                  = 2
)

// ExtendedACL describes the access-control entries attached to a retained
// Darwin file descriptor. Present distinguishes an empty ACL from no ACL.
// AllowsAccess is true when at least one permit ACE is present.
type ExtendedACL struct {
	Present      bool
	AllowsAccess bool
}

// HasExtendedACL reports whether file has a Darwin extended ACL. It queries the
// retained descriptor rather than resolving a path, and fails on malformed
// kernel responses instead of treating them as ACL-free.
func HasExtendedACL(file *os.File) (bool, error) {
	acl, err := InspectExtendedACL(file)
	return acl.Present, err
}

// InspectExtendedACL parses the Darwin kauth ACL returned for file. Deny-only
// ACLs are reported separately from permit ACLs so callers can accept
// strengthening ACLs on ancestors without accepting an alternate access path.
// Unknown ACE kinds and malformed kernel responses fail closed.
func InspectExtendedACL(file *os.File) (ExtendedACL, error) {
	if file == nil {
		return ExtendedACL{}, fmt.Errorf("file security handle is unavailable")
	}
	attributes := unix.Attrlist{
		Bitmapcount: unix.ATTR_BIT_MAP_COUNT,
		Commonattr:  unix.ATTR_CMN_RETURNED_ATTRS | unix.ATTR_CMN_EXTENDED_SECURITY,
	}
	buffer := make([]byte, 4096)
	_, _, errno := unix.Syscall6(
		unix.SYS_FGETATTRLIST,
		file.Fd(),
		uintptr(unsafe.Pointer(&attributes)),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
		0,
		0,
	)
	runtime.KeepAlive(file)
	runtime.KeepAlive(buffer)
	if errno != 0 {
		return ExtendedACL{}, errno
	}
	return parseExtendedSecurityResponse(buffer)
}

func parseExtendedSecurityResponse(buffer []byte) (ExtendedACL, error) {
	if len(buffer) < extendedSecurityHeaderLength {
		return ExtendedACL{}, fmt.Errorf("extended-security attribute response is truncated")
	}
	total := int64(binary.LittleEndian.Uint32(buffer[0:4]))
	if total < extendedSecurityHeaderLength || total > int64(len(buffer)) {
		return ExtendedACL{}, fmt.Errorf("extended-security attribute response has invalid length %d", total)
	}
	returnedCommon := binary.LittleEndian.Uint32(buffer[4:8])
	if returnedCommon&unix.ATTR_CMN_EXTENDED_SECURITY == 0 {
		return ExtendedACL{}, nil
	}
	dataOffset := int64(int32(binary.LittleEndian.Uint32(buffer[24:28])))
	dataLength := int64(binary.LittleEndian.Uint32(buffer[28:32]))
	if dataLength == 0 {
		return ExtendedACL{}, nil
	}
	start := int64(24) + dataOffset
	end := start + dataLength
	if start < extendedSecurityHeaderLength || end < start || end > total {
		return ExtendedACL{}, fmt.Errorf("extended-security attribute reference is invalid")
	}
	return parseKauthFileSecurity(buffer[start:end])
}

func parseKauthFileSecurity(data []byte) (ExtendedACL, error) {
	if len(data) < kauthFileSecurityHeaderLength {
		return ExtendedACL{}, fmt.Errorf("extended-security ACL payload is truncated")
	}
	if magic := binary.LittleEndian.Uint32(data[0:4]); magic != kauthFileSecurityMagic {
		return ExtendedACL{}, fmt.Errorf("extended-security ACL payload has invalid magic %#x", magic)
	}
	entryCount := binary.LittleEndian.Uint32(data[36:40])
	if entryCount == kauthFileSecurityNoACL {
		if len(data) != kauthFileSecurityHeaderLength {
			return ExtendedACL{}, fmt.Errorf("extended-security no-ACL payload has invalid length %d", len(data))
		}
		return ExtendedACL{}, nil
	}
	if entryCount > kauthMaximumACLEntries {
		return ExtendedACL{}, fmt.Errorf("extended-security ACL entry count %d exceeds %d", entryCount, kauthMaximumACLEntries)
	}
	wantLength := kauthFileSecurityHeaderLength + int(entryCount)*kauthACEByteLength
	if len(data) != wantLength {
		return ExtendedACL{}, fmt.Errorf("extended-security ACL payload has invalid length %d for %d entries", len(data), entryCount)
	}

	acl := ExtendedACL{Present: true}
	for index := uint32(0); index < entryCount; index++ {
		start := kauthFileSecurityHeaderLength + int(index)*kauthACEByteLength
		flags := binary.LittleEndian.Uint32(data[start+16 : start+20])
		switch flags & kauthACEKindMask {
		case kauthACEPermit:
			acl.AllowsAccess = true
		case kauthACEDeny:
		default:
			return ExtendedACL{}, fmt.Errorf("extended-security ACL entry %d has unsupported kind %d", index, flags&kauthACEKindMask)
		}
	}
	return acl, nil
}
