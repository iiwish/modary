package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/actionpersistence"
	"github.com/iiwish/modary/internal/databasecontrol"
	"github.com/iiwish/modary/internal/moduleassembly"
	"github.com/iiwish/modary/internal/runtimecontrol"
	"github.com/iiwish/modary/module"
)

type testServices struct {
	host         *module.Host
	db           *sql.DB
	access       database.Access
	control      databasecontrol.Control
	transactions runtimecontrol.TransactionManager
	plans        actionpersistence.PlanStore
	idempotency  actionpersistence.IdempotencyStore
}

type ownerlessFileInfo struct {
	os.FileInfo
}

func (ownerlessFileInfo) Sys() any { return nil }

type syntheticSQLiteDirectoryInfo struct {
	mode os.FileMode
	uid  uint64
}

func (info syntheticSQLiteDirectoryInfo) Name() string       { return "ancestor" }
func (info syntheticSQLiteDirectoryInfo) Size() int64        { return 0 }
func (info syntheticSQLiteDirectoryInfo) Mode() os.FileMode  { return info.mode }
func (info syntheticSQLiteDirectoryInfo) ModTime() time.Time { return time.Time{} }
func (info syntheticSQLiteDirectoryInfo) IsDir() bool        { return info.mode.IsDir() }
func (info syntheticSQLiteDirectoryInfo) Sys() any           { return struct{ Uid uint64 }{Uid: info.uid} }

func TestProtectedDirectoryAncestorPolicy(t *testing.T) {
	const effectiveUID = uint64(501)
	for _, test := range []struct {
		name    string
		mode    os.FileMode
		uid     uint64
		wantErr string
	}{
		{name: "effective owner private", mode: os.ModeDir | 0o700, uid: effectiveUID},
		{name: "effective owner traversable", mode: os.ModeDir | 0o755, uid: effectiveUID},
		{name: "root traversable", mode: os.ModeDir | 0o755, uid: 0},
		{name: "root sticky writable", mode: os.ModeDir | os.ModeSticky | 0o777, uid: 0},
		{name: "foreign owner", mode: os.ModeDir | 0o755, uid: 777, wantErr: "effective UID 501 or root"},
		{name: "effective owner sticky writable", mode: os.ModeDir | os.ModeSticky | 0o777, uid: effectiveUID, wantErr: "root-owned and sticky"},
		{name: "root writable without sticky", mode: os.ModeDir | 0o777, uid: 0, wantErr: "root-owned and sticky"},
		{name: "symlink", mode: os.ModeSymlink | 0o777, uid: effectiveUID, wantErr: "real directory"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateProtectedDirectoryAncestorInfo("/ancestor", syntheticSQLiteDirectoryInfo{mode: test.mode, uid: test.uid}, effectiveUID)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("ancestor policy rejected secure state: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ancestor policy error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestModuleIsExplicitAndSideEffectFree(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-created", "modary.db")
	registration, err := Module(Options{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	manifest := registration.Definition.Manifest
	if manifest.SchemaVersion != module.SchemaVersion || manifest.ID != ModuleID || manifest.Version != "0.1.0" || manifest.Type != "adapter" {
		t.Fatalf("manifest = %#v", manifest)
	}
	if !reflect.DeepEqual(manifest.Provides, []module.Capability{module.CapabilityDatabase}) || len(manifest.Requires) != 0 {
		t.Fatalf("capabilities = requires %#v, provides %#v", manifest.Requires, manifest.Provides)
	}
	if registration.Start == nil {
		t.Fatal("registration has no Start function")
	}
	if len(registration.Definition.Actions) != 0 || len(registration.Definition.Migrations) != 1 {
		t.Fatalf("definition = %#v", registration.Definition)
	}
	migration := registration.Definition.Migrations[0]
	if migration.Driver != "sqlite" {
		t.Fatalf("migration driver = %q", migration.Driver)
	}
	entries, err := fs.ReadDir(migration.Files, ".")
	if err != nil || len(entries) != 1 || entries[0].Name() != "0001_neutral.sql" {
		t.Fatalf("migration entries = %#v, %v", entries, err)
	}

	host := module.NewHost()
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	if _, err := host.Catalog(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Module or catalog inspection created %s: %v", filepath.Dir(path), err)
	}
}

func TestModuleRejectsInvalidOptionsBeforeSideEffects(t *testing.T) {
	validPath := filepath.Join(t.TempDir(), "not-created", "modary.db")
	tests := map[string]Options{
		"empty path":                {},
		"surrounding whitespace":    {Path: " modary.db"},
		"query injection":           {Path: "modary.db?_pragma=x"},
		"URI path":                  {Path: "file:modary.db"},
		"control character":         {Path: "modary\ndb"},
		"negative open connections": {Path: validPath, MaxOpenConnections: -1},
		"negative idle connections": {Path: validPath, MaxIdleConnections: -1},
		"idle exceeds open":         {Path: validPath, MaxOpenConnections: 1, MaxIdleConnections: 2},
		"negative lifetime":         {Path: validPath, ConnectionMaxLifetime: -time.Second},
		"negative busy timeout":     {Path: validPath, BusyTimeout: -time.Second},
		"sub-millisecond timeout":   {Path: validPath, BusyTimeout: time.Nanosecond},
		"excessive timeout":         {Path: validPath, BusyTimeout: time.Hour + time.Millisecond},
		"memory pool fanout":        {Path: ":memory:", MaxOpenConnections: 2},
		"memory connection recycle": {Path: ":memory:", ConnectionMaxLifetime: time.Second},
	}
	for name, options := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Module(options); err == nil {
				t.Fatal("Module accepted invalid options")
			}
		})
	}
	if _, err := os.Stat(filepath.Dir(validPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid options created %s: %v", filepath.Dir(validPath), err)
	}
}

func TestStartProvidesNeutralSchemaAndConnectionPragmas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "modary.db")
	services := startTestServices(t, Options{
		Path:               path,
		MaxOpenConnections: 4,
		MaxIdleConnections: 4,
		BusyTimeout:        2300 * time.Millisecond,
	})

	wantTables := []string{"modary_action_idempotency", "modary_action_plan", "modary_module_migration"}
	rows, err := services.db.Query(`
		SELECT name FROM sqlite_schema
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tables, wantTables) {
		t.Fatalf("schema tables = %#v, want %#v", tables, wantTables)
	}
	for _, table := range []string{"modary_action_plan", "modary_action_idempotency"} {
		var count int
		if err := services.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("empty installation populated %s with %d rows", table, count)
		}
	}
	var migrations int
	if err := services.db.QueryRow(`SELECT COUNT(*) FROM modary_module_migration WHERE module_id = ?`, ModuleID).Scan(&migrations); err != nil {
		t.Fatal(err)
	}
	if migrations != 1 {
		t.Fatalf("migration count = %d", migrations)
	}

	connections := make([]*sql.Conn, 0, 4)
	defer func() {
		for _, connection := range connections {
			_ = connection.Close()
		}
	}()
	for index := 0; index < 4; index++ {
		connection, err := services.db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, connection)
		var foreignKeys, busyTimeout int
		var journalMode string
		if err := connection.QueryRowContext(context.Background(), `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
			t.Fatal(err)
		}
		if err := connection.QueryRowContext(context.Background(), `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
			t.Fatal(err)
		}
		if err := connection.QueryRowContext(context.Background(), `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
			t.Fatal(err)
		}
		if foreignKeys != 1 || busyTimeout != 2300 || journalMode != "wal" {
			t.Fatalf("connection %d pragmas: foreign_keys=%d busy_timeout=%d journal_mode=%q", index, foreignKeys, busyTimeout, journalMode)
		}
	}
}

func TestInMemoryInstallationKeepsItsSchema(t *testing.T) {
	services := startTestServices(t, Options{Path: ":memory:"})
	for index := 0; index < 3; index++ {
		var count int
		if err := services.db.QueryRow(`SELECT COUNT(*) FROM modary_action_plan`).Scan(&count); err != nil {
			t.Fatalf("query %d: %v", index, err)
		}
	}
	if stats := services.db.Stats(); stats.MaxOpenConnections != 1 {
		t.Fatalf("in-memory maximum connections = %d", stats.MaxOpenConnections)
	}
}

func TestFileBackedDatabaseUsesOwnerOnlyPermissions(t *testing.T) {
	requirePOSIXPermissions(t)
	root := t.TempDir()
	firstDirectory := filepath.Join(root, "private")
	databaseDirectory := filepath.Join(firstDirectory, "data")
	path := filepath.Join(databaseDirectory, "modary.db")
	startTestServices(t, Options{Path: path})

	assertExactPermissions(t, firstDirectory, 0o700)
	assertExactPermissions(t, databaseDirectory, 0o700)
	assertExactPermissions(t, path, 0o600)
	for _, suffix := range []string{"-wal", "-shm"} {
		sidecar := path + suffix
		if _, err := os.Stat(sidecar); err != nil {
			t.Fatalf("stat SQLite sidecar %s: %v", sidecar, err)
		}
		assertExactPermissions(t, sidecar, 0o600)
	}
}

func TestOwnershipValidationRejectsExistingForeignArtifacts(t *testing.T) {
	requirePOSIXPermissions(t)
	directory := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "modary.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	sidecar := path + "-wal"
	if err := os.WriteFile(sidecar, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	foreignEffectiveUID := uint64(0)
	if os.Geteuid() == 0 {
		foreignEffectiveUID = 1
	}
	directoryInfo, err := os.Lstat(directory)
	if err != nil {
		t.Fatal(err)
	}
	ownerUID, err := fileOwnerUID(directoryInfo)
	if err != nil {
		t.Fatal(err)
	}
	if ownerUID != uint64(os.Geteuid()) {
		t.Fatalf("test directory owner UID = %d, effective UID = %d", ownerUID, os.Geteuid())
	}
	resolved, err := prepareSecureDatabaseDirectory(directory, foreignEffectiveUID)
	if err == nil || resolved != "" || !strings.Contains(err.Error(), "effective UID") {
		t.Fatalf("foreign directory ownership result = path %q, error %v", resolved, err)
	}
	for kind, artifact := range map[string]string{"database": path, "sidecar": sidecar} {
		exists, err := inspectSecureFile(artifact, kind, true, foreignEffectiveUID)
		if err == nil || exists || !strings.Contains(err.Error(), "effective UID") {
			t.Fatalf("foreign %s ownership result = exists %t, error %v", kind, exists, err)
		}
	}
	assertExactPermissions(t, directory, 0o700)
	assertExactPermissions(t, path, 0o600)
	assertExactPermissions(t, sidecar, 0o600)
}

func TestOwnershipValidationFailsClosedWithoutPOSIXOwnerMetadata(t *testing.T) {
	requirePOSIXPermissions(t)
	directory := t.TempDir()
	directoryInfo, err := os.Lstat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateProtectedDirectoryInfo(directory, ownerlessFileInfo{directoryInfo}, uint64(os.Geteuid())); err == nil || !strings.Contains(err.Error(), "determine owner UID") {
		t.Fatalf("ownerless directory error = %v", err)
	}
	path := filepath.Join(directory, "modary.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateOwnerOnlyFileInfo(path, "database", ownerlessFileInfo{fileInfo}, uint64(os.Geteuid())); err == nil || !strings.Contains(err.Error(), "determine owner UID") {
		t.Fatalf("ownerless database error = %v", err)
	}
}

func TestStartRejectsInsecureExistingDatabaseWithoutChangingIt(t *testing.T) {
	requirePOSIXPermissions(t)
	directory := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "modary.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}

	err := startExpectingFailure(t, Options{Path: path})
	if !strings.Contains(err.Error(), "start callback failed") {
		t.Fatalf("insecure database error = %v", err)
	}
	assertExactPermissions(t, path, 0o640)
}

func TestStartAcceptsReadOnlyTraversalOfExistingDirectory(t *testing.T) {
	requirePOSIXPermissions(t)
	directory := filepath.Join(t.TempDir(), "shared-readable")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "modary.db")
	startTestServices(t, Options{Path: path})
	assertExactPermissions(t, directory, 0o755)
	assertExactPermissions(t, path, 0o600)
}

func TestStartRejectsWritableExistingDirectoryWithoutChangingIt(t *testing.T) {
	requirePOSIXPermissions(t)
	directory := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o770); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "modary.db")

	err := startExpectingFailure(t, Options{Path: path})
	if !strings.Contains(err.Error(), "start callback failed") {
		t.Fatalf("insecure directory error = %v", err)
	}
	assertExactPermissions(t, directory, 0o770)
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("insecure directory start created database: %v", err)
	}
}

func TestStartRejectsDatabaseSymlinkWithoutFollowingIt(t *testing.T) {
	requirePOSIXPermissions(t)
	directory := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "target.db")
	if err := os.WriteFile(target, []byte("do not open"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "modary.db")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}

	err := startExpectingFailure(t, Options{Path: path})
	if !strings.Contains(err.Error(), "start callback failed") {
		t.Fatalf("database symlink error = %v", err)
	}
	contents, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(contents) != "do not open" {
		t.Fatalf("database symlink target was modified: %q", contents)
	}
}

func TestStartRejectsInsecureSQLiteSidecarBeforeOpeningDatabase(t *testing.T) {
	requirePOSIXPermissions(t)
	directory := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "modary.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	walPath := path + "-wal"
	if err := os.WriteFile(walPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(walPath, 0o644); err != nil {
		t.Fatal(err)
	}

	err := startExpectingFailure(t, Options{Path: path})
	if !strings.Contains(err.Error(), "start callback failed") {
		t.Fatalf("insecure sidecar error = %v", err)
	}
	assertExactPermissions(t, walPath, 0o644)
}

func TestEmptyInstallationRemainsEmptyAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "modary.db")
	first := startTestServices(t, Options{Path: path})
	if err := first.host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	second := startTestServices(t, Options{Path: path})
	for _, table := range []string{"modary_action_plan", "modary_action_idempotency"} {
		var count int
		if err := second.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("restart populated %s with %d rows", table, count)
		}
	}
	var migrations int
	if err := second.db.QueryRow(`SELECT COUNT(*) FROM modary_module_migration WHERE module_id = ?`, ModuleID).Scan(&migrations); err != nil {
		t.Fatal(err)
	}
	if migrations != 1 {
		t.Fatalf("migration count after restart = %d", migrations)
	}
}

func TestHostShutdownClosesDatabaseExactlyOnce(t *testing.T) {
	services := startTestServices(t, Options{Path: filepath.Join(t.TempDir(), "modary.db")})
	if err := services.host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := services.host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := services.db.PingContext(context.Background()); err == nil {
		t.Fatalf("database remains usable after shutdown: %v", err)
	}

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	resource := &databaseResource{db: db}
	var wait sync.WaitGroup
	errorsSeen := make(chan error, 16)
	for index := 0; index < cap(errorsSeen); index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errorsSeen <- resource.close(context.Background())
		}()
	}
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatalf("concurrent close: %v", err)
		}
	}
	if err := db.PingContext(context.Background()); err == nil {
		t.Fatalf("resource did not close database: %v", err)
	}
}

func TestStartFailureReleasesDatabaseResource(t *testing.T) {
	registration, err := Module(Options{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	registration.Definition.Migrations[0].Files = fstest.MapFS{
		"0001_invalid.sql": {Data: []byte("CREATE TABLE")},
	}

	var startedDB *sql.DB
	start := registration.Start
	registration.Start = func(ctx context.Context, scope module.Scope) error {
		if err := start(ctx, scope); err != nil {
			return err
		}
		control, err := moduleassembly.ResolveDatabaseControl(scope)
		if err != nil {
			return err
		}
		executor, err := control.Executor(ctx)
		if err != nil {
			return err
		}
		sqliteExecutor, ok := executor.(sqlExecutor)
		if !ok {
			return errors.New("database executor is not SQLite")
		}
		startedDB, ok = sqliteExecutor.runner.(*sql.DB)
		if !ok {
			return errors.New("SQLite executor does not use sql.DB")
		}
		if startedDB.Stats().OpenConnections == 0 {
			return errors.New("SQLite database has no open connection before migration")
		}
		return nil
	}

	host := module.NewHost()
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err == nil {
		t.Fatal("Start accepted an invalid SQLite migration")
	}
	if host.State() != module.StateFailed {
		t.Fatalf("host state after failed Start = %s", host.State())
	}
	if startedDB == nil {
		t.Fatal("SQLite database was not captured after its successful Start callback")
	}
	if stats := startedDB.Stats(); stats.OpenConnections != 0 {
		t.Fatalf("failed Start retained %d SQLite connection(s)", stats.OpenConnections)
	}
	if err := startedDB.PingContext(context.Background()); err == nil {
		t.Fatal("SQLite database remains usable after failed Start")
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTypedNilServicesAndTransactionContextFailClosed(t *testing.T) {
	var plans *planStore
	if err := plans.Save(context.Background(), validPlan()); err == nil {
		t.Fatal("typed-nil plan store accepted Save")
	}
	var idempotency *idempotencyStore
	if _, err := idempotency.Lookup(context.Background(), validReservation()); err == nil {
		t.Fatal("typed-nil idempotency store accepted Lookup")
	}
	var transactions *transactionManager
	if err := transactions.WithinTransaction(context.Background(), func(context.Context) error { return nil }); err == nil {
		t.Fatal("typed-nil transaction manager accepted an operation")
	}

}

func startTestServices(t *testing.T, options Options) testServices {
	t.Helper()
	registration, err := Module(options)
	if err != nil {
		t.Fatal(err)
	}
	services := testServices{}
	start := registration.Start
	registration.Start = func(ctx context.Context, scope module.Scope) error {
		if err := start(ctx, scope); err != nil {
			return err
		}
		control, err := moduleassembly.ResolveDatabaseControl(scope)
		if err != nil {
			return err
		}
		services.control = control
		services.access = control.Access()
		services.transactions = &transactionManager{control: control}
		services.plans = actionpersistence.PlanStore(&planStore{control: control})
		services.idempotency = actionpersistence.IdempotencyStore(&idempotencyStore{control: control})
		executor, err := control.Executor(ctx)
		if err != nil {
			return err
		}
		sqliteExecutor, ok := executor.(sqlExecutor)
		if !ok {
			return errors.New("database executor is not SQLite")
		}
		services.db, ok = sqliteExecutor.runner.(*sql.DB)
		if !ok {
			return errors.New("SQLite executor does not use sql.DB")
		}
		return nil
	}
	host := module.NewHost()
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	if err := host.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	services.host = host
	if services.db == nil || services.access == nil || services.control == nil ||
		services.transactions == nil || services.plans == nil || services.idempotency == nil {
		t.Fatal("SQLite test services were not captured during startup")
	}
	t.Cleanup(func() {
		if services.host.State() == module.StateRunning {
			if err := services.host.Shutdown(context.Background()); err != nil {
				t.Errorf("shutdown SQLite test host: %v", err)
			}
		}
	})
	return services
}

func startExpectingFailure(t *testing.T, options Options) error {
	t.Helper()
	registration, err := Module(options)
	if err != nil {
		t.Fatal(err)
	}
	host := module.NewHost()
	if err := host.Register(registration); err != nil {
		t.Fatal(err)
	}
	err = host.Start(context.Background())
	if err == nil {
		_ = host.Shutdown(context.Background())
		t.Fatal("SQLite Start succeeded, want failure")
	}
	if shutdownErr := host.Shutdown(context.Background()); shutdownErr != nil {
		t.Fatalf("shutdown failed SQLite host: %v", shutdownErr)
	}
	return err
}

func requirePOSIXPermissions(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows permissions require an ACL-aware secure file profile")
	}
}

func assertExactPermissions(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("permissions for %s = %04o, want %04o", path, got, want)
	}
}
