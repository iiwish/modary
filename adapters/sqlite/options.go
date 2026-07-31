package sqlite

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	defaultMaxOpenConnections = 4
	defaultMaxIdleConnections = 2
	defaultConnectionLifetime = time.Hour
	defaultBusyTimeout        = 5 * time.Second
)

// Options explicitly configures the SQLite durable profile. Path is required;
// ":memory:" is supported with one non-recycling connection. Relative file
// paths are anchored when Module is called. Missing database directories are
// created with 0700 permissions. An existing database directory must grant its
// owner read, write, and search access and must not be writable by group or
// other users; 0755 and 0750 are accepted. Every ancestor must be owned by the
// effective UID or root; a group/other-writable ancestor is accepted only when
// it is root-owned and sticky. The final database directory, database, and
// SQLite sidecar files must be owned by the process's effective UID, and files
// must have exactly 0600 permissions. Existing paths are validated and never
// chmodded or chowned. On Darwin, the final directory, database, and sidecars
// must have no extended ACL; ancestor ACLs may contain deny entries but no
// permit entries. On Linux, the exact file mode and directory write policy rely
// on the POSIX ACL mask being reflected in group mode bits. The file-backed
// secure profile is available only on Linux and Darwin; ":memory:" remains
// portable. Zero pool, lifetime, and timeout values select conservative
// file-backed defaults. No value is read from process environment or global
// configuration. These checks protect against distinct unprivileged users on
// supported local filesystems; root, the same UID, mount replacement, and
// filesystems that do not faithfully enforce stat and ACL metadata remain
// outside this adapter's isolation boundary.
type Options struct {
	Path                  string
	MaxOpenConnections    int
	MaxIdleConnections    int
	ConnectionMaxLifetime time.Duration
	BusyTimeout           time.Duration
}

func normalizeOptions(options Options) (Options, error) {
	if !utf8.ValidString(options.Path) || options.Path == "" || strings.TrimSpace(options.Path) != options.Path {
		return Options{}, fmt.Errorf("sqlite path must be a non-empty valid UTF-8 value without surrounding whitespace")
	}
	if strings.ContainsAny(options.Path, "?\x00") || strings.ContainsFunc(options.Path, unicode.IsControl) {
		return Options{}, fmt.Errorf("sqlite path contains unsupported characters")
	}
	if strings.HasPrefix(strings.ToLower(options.Path), "file:") {
		return Options{}, fmt.Errorf("sqlite URI paths are not supported")
	}
	if options.Path != ":memory:" {
		if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
			return Options{}, fmt.Errorf("the secure file-backed SQLite profile is unavailable on %s; use :memory: or a platform-aware adapter", runtime.GOOS)
		}
		absolutePath, err := filepath.Abs(options.Path)
		if err != nil {
			return Options{}, fmt.Errorf("resolve SQLite path: %w", err)
		}
		options.Path = filepath.Clean(absolutePath)
	}
	if options.MaxOpenConnections < 0 {
		return Options{}, fmt.Errorf("sqlite maximum open connections cannot be negative")
	}
	if options.MaxIdleConnections < 0 {
		return Options{}, fmt.Errorf("sqlite maximum idle connections cannot be negative")
	}
	if options.ConnectionMaxLifetime < 0 {
		return Options{}, fmt.Errorf("sqlite connection lifetime cannot be negative")
	}
	if options.BusyTimeout < 0 {
		return Options{}, fmt.Errorf("sqlite busy timeout cannot be negative")
	}
	if options.BusyTimeout != 0 && options.BusyTimeout < time.Millisecond {
		return Options{}, fmt.Errorf("sqlite busy timeout must be at least one millisecond")
	}
	if options.BusyTimeout > time.Hour {
		return Options{}, fmt.Errorf("sqlite busy timeout cannot exceed one hour")
	}

	if options.Path == ":memory:" {
		if options.MaxOpenConnections > 1 || options.MaxIdleConnections > 1 {
			return Options{}, fmt.Errorf("an in-memory SQLite database supports at most one pooled connection")
		}
		if options.ConnectionMaxLifetime > 0 {
			return Options{}, fmt.Errorf("an in-memory SQLite database cannot recycle its only pooled connection")
		}
		options.MaxOpenConnections = 1
		options.MaxIdleConnections = 1
	} else {
		if options.MaxOpenConnections == 0 {
			options.MaxOpenConnections = defaultMaxOpenConnections
		}
		if options.MaxIdleConnections == 0 {
			options.MaxIdleConnections = min(defaultMaxIdleConnections, options.MaxOpenConnections)
		}
	}
	if options.MaxIdleConnections > options.MaxOpenConnections {
		return Options{}, fmt.Errorf("sqlite maximum idle connections cannot exceed maximum open connections")
	}
	if options.Path != ":memory:" && options.ConnectionMaxLifetime == 0 {
		options.ConnectionMaxLifetime = defaultConnectionLifetime
	}
	if options.BusyTimeout == 0 {
		options.BusyTimeout = defaultBusyTimeout
	}
	return options, nil
}
