// Package sqlite provides Modary's explicit, neutral SQLite durable profile.
//
// Stability: alpha. Consumers should pin an exact pre-v1 Modary version.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/iiwish/modary/internal/actionpersistence"
	"github.com/iiwish/modary/internal/databasecontrol"
	"github.com/iiwish/modary/internal/moduleassembly"
	"github.com/iiwish/modary/module"

	_ "modernc.org/sqlite"
)

// ModuleID is the stable Module manifest and migration owner identifier.
const ModuleID = "sqlite"

const (
	databaseDirectoryPermissions os.FileMode = 0o700
	databaseFilePermissions      os.FileMode = 0o600
)

var sqliteSidecarSuffixes = [...]string{"-wal", "-shm", "-journal"}

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// Module returns a side-effect-free registration. Filesystem and database work
// begins only when the Host invokes Start.
func Module(options Options) (module.Registration, error) {
	normalized, err := normalizeOptions(options)
	if err != nil {
		return module.Registration{}, err
	}
	migrations, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		return module.Registration{}, fmt.Errorf("prepare SQLite migrations: %w", err)
	}
	manifest := module.Manifest{
		SchemaVersion: module.SchemaVersion,
		ID:            ModuleID,
		Version:       "0.1.0",
		Type:          module.ModuleTypeAdapter,
		Provides:      []module.Capability{module.CapabilityDatabase},
	}
	return module.Registration{
		Definition: module.Definition{
			Manifest:   manifest,
			Migrations: []module.MigrationSource{{Driver: "sqlite", Files: migrations}},
		},
		Start: func(ctx context.Context, scope module.Scope) error {
			return start(ctx, scope, normalized)
		},
	}, nil
}

func start(ctx context.Context, scope module.Scope, options Options) error {
	if ctx == nil {
		return fmt.Errorf("SQLite start context is required")
	}
	var expectedOwnerUID uint64
	if options.Path != ":memory:" {
		var err error
		expectedOwnerUID, err = effectiveOwnerUID()
		if err != nil {
			return err
		}
		securePath, err := prepareSecureDatabase(options.Path, expectedOwnerUID)
		if err != nil {
			return err
		}
		options.Path = securePath
	}
	db, err := sql.Open("sqlite", dataSourceName(options))
	if err != nil {
		return fmt.Errorf("open SQLite database: %w", err)
	}
	resource := &databaseResource{db: db}
	if err := module.OnStop(scope, resource.close); err != nil {
		_ = db.Close()
		return err
	}
	db.SetMaxOpenConns(options.MaxOpenConnections)
	db.SetMaxIdleConns(options.MaxIdleConnections)
	db.SetConnMaxLifetime(options.ConnectionMaxLifetime)
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect SQLite database: %w", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode=WAL`); err != nil {
		return fmt.Errorf("configure SQLite journal mode: %w", err)
	}
	if options.Path != ":memory:" {
		if err := validateSecureDatabaseState(options.Path, expectedOwnerUID); err != nil {
			return err
		}
	}

	control, err := databasecontrol.New(&backend{db: db})
	if err != nil {
		return fmt.Errorf("create SQLite database control: %w", err)
	}
	transactions := &transactionManager{control: control}
	plans := actionpersistence.PlanStore(&planStore{control: control})
	idempotency := actionpersistence.IdempotencyStore(&idempotencyStore{control: control})
	if err := moduleassembly.ProvideDatabase(scope, control); err != nil {
		return err
	}
	return moduleassembly.ProvideActionPersistence(scope, plans, idempotency, transactions)
}

func dataSourceName(options Options) string {
	return fmt.Sprintf("%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(%d)&_txlock=immediate",
		options.Path, options.BusyTimeout.Milliseconds())
}

func prepareSecureDatabase(path string, expectedOwnerUID uint64) (string, error) {
	directory, err := prepareSecureDatabaseDirectory(filepath.Dir(path), expectedOwnerUID)
	if err != nil {
		return "", err
	}
	path = filepath.Join(directory, filepath.Base(path))
	if filepath.Clean(path) != path || !filepath.IsAbs(path) || strings.ContainsRune(path, '?') {
		return "", fmt.Errorf("resolve SQLite database path: canonical path is invalid")
	}
	databaseExists, err := inspectSecureFile(path, "database", false, expectedOwnerUID)
	if err != nil {
		return "", err
	}

	for _, suffix := range sqliteSidecarSuffixes {
		sidecarPath := path + suffix
		sidecarExists, err := inspectSecureFile(sidecarPath, "sidecar", false, expectedOwnerUID)
		if err != nil {
			return "", err
		}
		if !databaseExists && sidecarExists {
			return "", fmt.Errorf("SQLite sidecar %q exists without its database", sidecarPath)
		}
	}
	if databaseExists {
		return path, nil
	}

	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, databaseFilePermissions)
	if err != nil {
		return "", fmt.Errorf("securely create SQLite database: %w", err)
	}
	info, err := file.Stat()
	if err == nil {
		err = validateOpenedSQLitePath(file, path, "database", info, expectedOwnerUID, validateOwnerOnlyFileInfo, validateFinalSQLitePathACL)
	}
	closeErr := file.Close()
	if err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("inspect new SQLite database: %w", err)
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close new SQLite database: %w", closeErr)
	}
	return path, nil
}

func prepareSecureDatabaseDirectory(path string, expectedOwnerUID uint64) (string, error) {
	// Resolve configured ancestor symlinks once, then give SQLite only the
	// canonical path. The protected final directory prevents an untrusted user
	// from swapping the database or a sidecar between validation and open.
	cursor := filepath.Clean(path)
	missing := make([]string, 0, 2)
	for {
		info, err := os.Lstat(cursor)
		if err == nil {
			if info.Mode()&os.ModeSymlink == 0 && !info.IsDir() {
				return "", fmt.Errorf("SQLite database directory ancestor %q is not a directory", cursor)
			}
			break
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect SQLite database directory %q: %w", cursor, err)
		}
		parent := filepath.Dir(cursor)
		if parent == cursor {
			return "", fmt.Errorf("locate existing SQLite database directory ancestor for %q", path)
		}
		missing = append(missing, filepath.Base(cursor))
		cursor = parent
	}

	resolved, err := filepath.EvalSymlinks(cursor)
	if err != nil {
		return "", fmt.Errorf("resolve SQLite database directory ancestor %q: %w", cursor, err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("make SQLite database directory absolute: %w", err)
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect resolved SQLite database directory ancestor %q: %w", resolved, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("resolved SQLite database directory ancestor %q is not a real directory", resolved)
	}
	if len(missing) == 0 {
		if err := validateProtectedDirectory(resolved, info, expectedOwnerUID, true); err != nil {
			return "", err
		}
		if err := validateProtectedDirectoryAncestry(filepath.Dir(resolved), expectedOwnerUID); err != nil {
			return "", err
		}
		return resolved, nil
	}
	if err := validateProtectedDirectoryAncestry(resolved, expectedOwnerUID); err != nil {
		return "", err
	}
	for index := len(missing) - 1; index >= 0; index-- {
		resolved = filepath.Join(resolved, missing[index])
		if err := os.Mkdir(resolved, databaseDirectoryPermissions); err != nil {
			return "", fmt.Errorf("securely create SQLite database directory %q: %w", resolved, err)
		}
		info, err = os.Lstat(resolved)
		if err != nil {
			return "", fmt.Errorf("inspect new SQLite database directory %q: %w", resolved, err)
		}
		if err := validateProtectedDirectory(resolved, info, expectedOwnerUID, index == 0); err != nil {
			return "", err
		}
	}
	return resolved, nil
}

func validateSecureDatabaseState(path string, expectedOwnerUID uint64) error {
	directory := filepath.Dir(path)
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("inspect SQLite database directory %q: %w", directory, err)
	}
	if err := validateProtectedDirectory(directory, info, expectedOwnerUID, true); err != nil {
		return err
	}
	if err := validateProtectedDirectoryAncestry(filepath.Dir(directory), expectedOwnerUID); err != nil {
		return err
	}
	if _, err := inspectSecureFile(path, "database", true, expectedOwnerUID); err != nil {
		return err
	}
	for _, suffix := range sqliteSidecarSuffixes {
		if _, err := inspectSecureFile(path+suffix, "sidecar", false, expectedOwnerUID); err != nil {
			return err
		}
	}
	return nil
}

func inspectSecureFile(path, kind string, required bool, expectedOwnerUID uint64) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) && !required {
			return false, nil
		}
		return false, fmt.Errorf("inspect SQLite %s %q: %w", kind, path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("SQLite %s %q must not be a symbolic link", kind, path)
	}
	if err := validateOwnerOnlyFileInfo(path, kind, info, expectedOwnerUID); err != nil {
		return false, err
	}
	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("open SQLite %s %q for security validation: %w", kind, path, err)
	}
	validationErr := validateOpenedSQLitePath(file, path, kind, info, expectedOwnerUID, validateOwnerOnlyFileInfo, validateFinalSQLitePathACL)
	closeErr := file.Close()
	if validationErr != nil {
		return false, validationErr
	}
	if closeErr != nil {
		return false, fmt.Errorf("close SQLite %s %q after security validation: %w", kind, path, closeErr)
	}
	return true, nil
}

type sqlitePathInfoValidator func(string, string, os.FileInfo, uint64) error

func validateOpenedSQLitePath(
	file *os.File,
	path, kind string,
	expectedInfo os.FileInfo,
	expectedOwnerUID uint64,
	validateInfo sqlitePathInfoValidator,
	validateACL func(*os.File, string, string) error,
) error {
	if file == nil || expectedInfo == nil || validateInfo == nil || validateACL == nil {
		return fmt.Errorf("SQLite %s %q security validation state is unavailable", kind, path)
	}
	openedInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect opened SQLite %s %q: %w", kind, path, err)
	}
	if !os.SameFile(expectedInfo, openedInfo) {
		return fmt.Errorf("SQLite %s %q changed while it was opened for security validation", kind, path)
	}
	if err := validateInfo(path, kind, openedInfo, expectedOwnerUID); err != nil {
		return err
	}
	if err := validateACL(file, path, kind); err != nil {
		return err
	}
	currentInfo, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("reinspect SQLite %s %q after security validation: %w", kind, path, err)
	}
	if currentInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(openedInfo, currentInfo) {
		return fmt.Errorf("SQLite %s %q changed during security validation", kind, path)
	}
	return nil
}

func validateOwnerOnlyFileInfo(path, kind string, info os.FileInfo, expectedOwnerUID uint64) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("SQLite %s %q must be a regular file", kind, path)
	}
	if info.Mode().Perm() != databaseFilePermissions {
		return fmt.Errorf("SQLite %s %q must have owner-only permissions 0600; got %04o", kind, path, info.Mode().Perm())
	}
	return validateEffectiveOwner(path, kind, info, expectedOwnerUID)
}

func validateProtectedDirectoryInfo(path string, info os.FileInfo, expectedOwnerUID uint64) error {
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("SQLite database directory %q must be a real directory", path)
	}
	permissions := info.Mode().Perm()
	if permissions&databaseDirectoryPermissions != databaseDirectoryPermissions || permissions&0o022 != 0 {
		return fmt.Errorf("SQLite database directory %q must grant owner rwx and deny group/other write; got %04o", path, permissions)
	}
	return validateEffectiveOwner(path, "database directory", info, expectedOwnerUID)
}

func validateProtectedDirectory(path string, info os.FileInfo, expectedOwnerUID uint64, final bool) error {
	if err := validateProtectedDirectoryInfo(path, info, expectedOwnerUID); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open SQLite database directory %q for security validation: %w", path, err)
	}
	validateACL := validateAncestorSQLitePathACL
	kind := "database directory ancestor"
	if final {
		validateACL = validateFinalSQLitePathACL
		kind = "database directory"
	}
	validationErr := validateOpenedSQLitePath(
		file,
		path,
		kind,
		info,
		expectedOwnerUID,
		func(path, _ string, info os.FileInfo, expectedOwnerUID uint64) error {
			return validateProtectedDirectoryInfo(path, info, expectedOwnerUID)
		},
		validateACL,
	)
	closeErr := file.Close()
	if validationErr != nil {
		return validationErr
	}
	if closeErr != nil {
		return fmt.Errorf("close SQLite %s %q after security validation: %w", kind, path, closeErr)
	}
	return nil
}

func effectiveOwnerUID() (uint64, error) {
	uid := os.Geteuid()
	if uid < 0 {
		return 0, fmt.Errorf("determine effective UID for secure SQLite storage: ownership metadata is unavailable")
	}
	return uint64(uid), nil
}

func validateEffectiveOwner(path, kind string, info os.FileInfo, expectedOwnerUID uint64) error {
	ownerUID, err := fileOwnerUID(info)
	if err != nil {
		return fmt.Errorf("determine owner UID for SQLite %s %q: %w", kind, path, err)
	}
	if ownerUID != expectedOwnerUID {
		return fmt.Errorf("SQLite %s %q must be owned by effective UID %d; got UID %d", kind, path, expectedOwnerUID, ownerUID)
	}
	return nil
}

func fileOwnerUID(info os.FileInfo) (uint64, error) {
	if info == nil || info.Sys() == nil {
		return 0, fmt.Errorf("POSIX stat ownership metadata is unavailable")
	}
	metadata := reflect.ValueOf(info.Sys())
	if metadata.Kind() == reflect.Pointer {
		if metadata.IsNil() {
			return 0, fmt.Errorf("POSIX stat ownership metadata is unavailable")
		}
		metadata = metadata.Elem()
	}
	if metadata.Kind() != reflect.Struct {
		return 0, fmt.Errorf("POSIX stat ownership metadata has unsupported type %T", info.Sys())
	}
	uid := metadata.FieldByName("Uid")
	if !uid.IsValid() {
		return 0, fmt.Errorf("POSIX stat ownership metadata %T has no Uid field", info.Sys())
	}
	switch uid.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return uid.Uint(), nil
	default:
		return 0, fmt.Errorf("POSIX stat ownership metadata %T has non-unsigned Uid field", info.Sys())
	}
}

func validateProtectedDirectoryAncestry(path string, expectedOwnerUID uint64) error {
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect SQLite database directory ancestor %q: %w", current, err)
		}
		if err := validateProtectedDirectoryAncestor(current, info, expectedOwnerUID); err != nil {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
	}
}

func validateProtectedDirectoryAncestor(path string, info os.FileInfo, expectedOwnerUID uint64) error {
	if err := validateProtectedDirectoryAncestorInfo(path, info, expectedOwnerUID); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open SQLite database directory ancestor %q for security validation: %w", path, err)
	}
	validationErr := validateOpenedSQLitePath(
		file,
		path,
		"database directory ancestor",
		info,
		expectedOwnerUID,
		func(path, _ string, info os.FileInfo, expectedOwnerUID uint64) error {
			return validateProtectedDirectoryAncestorInfo(path, info, expectedOwnerUID)
		},
		validateAncestorSQLitePathACL,
	)
	closeErr := file.Close()
	if validationErr != nil {
		return validationErr
	}
	if closeErr != nil {
		return fmt.Errorf("close SQLite database directory ancestor %q after security validation: %w", path, closeErr)
	}
	return nil
}

func validateProtectedDirectoryAncestorInfo(path string, info os.FileInfo, expectedOwnerUID uint64) error {
	if info == nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("SQLite database directory ancestor %q must be a real directory", path)
	}
	ownerUID, err := fileOwnerUID(info)
	if err != nil {
		return fmt.Errorf("determine owner UID for SQLite database directory ancestor %q: %w", path, err)
	}
	if ownerUID != expectedOwnerUID && ownerUID != 0 {
		return fmt.Errorf(
			"SQLite database directory ancestor %q must be owned by effective UID %d or root; got UID %d",
			path,
			expectedOwnerUID,
			ownerUID,
		)
	}
	if info.Mode().Perm()&0o022 != 0 && (ownerUID != 0 || info.Mode()&os.ModeSticky == 0) {
		return fmt.Errorf(
			"writable SQLite database directory ancestor %q must be root-owned and sticky; got UID %d and mode %04o",
			path,
			ownerUID,
			info.Mode().Perm(),
		)
	}
	return nil
}

type databaseResource struct {
	db   *sql.DB
	once sync.Once
	err  error
}

func (resource *databaseResource) close(context.Context) error {
	resource.once.Do(func() { resource.err = resource.db.Close() })
	return resource.err
}
