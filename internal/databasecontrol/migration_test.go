package databasecontrol

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/iiwish/modary/database"

	_ "modernc.org/sqlite"
)

type typedNilMigrationFS struct{}

func (*typedNilMigrationFS) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }

type failingMigrationFS struct{ err error }

func (files failingMigrationFS) Open(string) (fs.File, error) { return nil, files.err }

type partialReadDirFS struct {
	fs.FS
	batch int
}

func (files partialReadDirFS) Open(name string) (fs.File, error) {
	file, err := files.FS.Open(name)
	if err != nil {
		return nil, err
	}
	if directory, ok := file.(fs.ReadDirFile); ok {
		return partialReadDirFile{ReadDirFile: directory, batch: files.batch}, nil
	}
	return file, nil
}

type partialReadDirFile struct {
	fs.ReadDirFile
	batch int
}

func (file partialReadDirFile) ReadDir(count int) ([]fs.DirEntry, error) {
	if count > file.batch {
		count = file.batch
	}
	return file.ReadDirFile.ReadDir(count)
}

type migrationReadStats struct {
	bytes  int
	opens  int
	closes int
}

type countingMigrationFS struct {
	fs.FS
	target string
	stats  *migrationReadStats
}

func (files countingMigrationFS) Open(name string) (fs.File, error) {
	file, err := files.FS.Open(name)
	if err != nil {
		return nil, err
	}
	if name == "." || (files.target != "" && name != files.target) {
		return file, nil
	}
	files.stats.opens++
	return &countingMigrationFile{File: file, stats: files.stats}, nil
}

type countingMigrationFile struct {
	fs.File
	stats *migrationReadStats
}

func (file *countingMigrationFile) Read(buffer []byte) (int, error) {
	count, err := file.File.Read(buffer)
	file.stats.bytes += count
	return count, err
}

func (file *countingMigrationFile) Close() error {
	file.stats.closes++
	return file.File.Close()
}

type readDirOverrideFS struct {
	fs.FS
	readDir func(int) ([]fs.DirEntry, error)
}

func (files readDirOverrideFS) Open(name string) (fs.File, error) {
	file, err := files.FS.Open(name)
	if err != nil {
		return nil, err
	}
	if name != "." {
		return file, nil
	}
	directory, ok := file.(fs.ReadDirFile)
	if !ok {
		_ = file.Close()
		return nil, errors.New("test migration root is not a directory")
	}
	return &readDirOverrideFile{ReadDirFile: directory, readDir: files.readDir}, nil
}

type readDirOverrideFile struct {
	fs.ReadDirFile
	readDir func(int) ([]fs.DirEntry, error)
}

func (file *readDirOverrideFile) ReadDir(count int) ([]fs.DirEntry, error) {
	return file.readDir(count)
}

type migrationBoundaryBackend struct {
	validations    int
	transactions   int
	adminResolvers int
	executions     int
	queries        int
}

func (*migrationBoundaryBackend) Driver() string { return "bounded" }

func (backend *migrationBoundaryBackend) ValidateMigration(string) error {
	backend.validations++
	return nil
}

func (backend *migrationBoundaryBackend) ReadExecutor(context.Context) (database.Executor, error) {
	return migrationBoundaryExecutor{backend: backend}, nil
}

func (backend *migrationBoundaryBackend) WriteExecutor(context.Context) (database.Executor, error) {
	return migrationBoundaryExecutor{backend: backend}, nil
}

func (backend *migrationBoundaryBackend) AdminExecutor(context.Context) (database.Executor, error) {
	backend.adminResolvers++
	return migrationBoundaryExecutor{backend: backend}, nil
}

func (backend *migrationBoundaryBackend) WithinTransaction(context.Context, func(context.Context) error) error {
	backend.transactions++
	return errors.New("unexpected migration transaction")
}

type migrationBoundaryExecutor struct{ backend *migrationBoundaryBackend }

func (executor migrationBoundaryExecutor) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	executor.backend.executions++
	return nil, errors.New("unexpected migration execution")
}

func (executor migrationBoundaryExecutor) QueryContext(context.Context, string, ...any) (database.Rows, error) {
	executor.backend.queries++
	return nil, errors.New("unexpected migration query")
}

func (executor migrationBoundaryExecutor) QueryRowContext(context.Context, string, ...any) database.Row {
	executor.backend.queries++
	return nil
}

func TestApplyMigrationsRollsBackModuleWhenLaterMigrationFails(t *testing.T) {
	db := openMigrationTestDB(t)
	control := newTestControl(t, db)
	migrations := fstest.MapFS{
		"0001_first.sql":  {Data: []byte(`CREATE TABLE first_table (id TEXT PRIMARY KEY);`)},
		"0002_broken.sql": {Data: []byte(`CREATE TABLE broken_table (`)},
	}

	if err := control.ApplyMigrations(context.Background(), "atomic-test", migrations); err == nil {
		t.Fatal("ApplyMigrations() succeeded with an invalid second migration")
	}
	assertSQLiteCount(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'first_table'`, 0)
	assertSQLiteCount(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'modary_module_migration'`, 0)
}

func TestApplyMigrationsCommitsOrderedModuleAndRestarts(t *testing.T) {
	db := openMigrationTestDB(t)
	control := newTestControl(t, db)
	migrations := fstest.MapFS{
		"0001_parent.sql": {Data: []byte(`CREATE TABLE parent (id TEXT PRIMARY KEY);`)},
		"0002_child.sql":  {Data: []byte(`CREATE TABLE child (id TEXT PRIMARY KEY, parent_id TEXT NOT NULL REFERENCES parent(id));`)},
	}

	if err := control.ApplyMigrations(context.Background(), "ordered-test", migrations); err != nil {
		t.Fatalf("first ApplyMigrations() error = %v", err)
	}
	if err := control.ApplyMigrations(context.Background(), "ordered-test", migrations); err != nil {
		t.Fatalf("restart ApplyMigrations() error = %v", err)
	}
	assertSQLiteCount(t, db, `SELECT COUNT(*) FROM modary_module_migration WHERE module_id = 'ordered-test'`, 2)
	assertSQLiteCount(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN ('parent', 'child')`, 2)
}

func TestReadMigrationsAcceptsLegalPartialDirectoryBatches(t *testing.T) {
	files := make(fstest.MapFS, 40)
	for index := 0; index < 40; index++ {
		name := fmt.Sprintf("%04d_table.sql", index)
		files[name] = &fstest.MapFile{Data: []byte(fmt.Sprintf("CREATE TABLE table_%04d (id INTEGER);", index))}
	}
	migrations, err := readMigrations("partial-batches", partialReadDirFS{FS: files, batch: 3})
	if err != nil {
		t.Fatalf("readMigrations() error = %v", err)
	}
	if len(migrations) != len(files) {
		t.Fatalf("readMigrations() count = %d, want %d", len(migrations), len(files))
	}
}

func TestReadMigrationFileStopsAtOneBytePastPerFileLimitAndClosesOnce(t *testing.T) {
	stats := &migrationReadStats{}
	files := countingMigrationFS{
		FS: fstest.MapFS{
			"0001_large.sql": {Data: []byte(strings.Repeat("x", maxMigrationFileBytes+4096))},
		},
		target: "0001_large.sql",
		stats:  stats,
	}

	_, err := readMigrations("bounded-read", files)
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("migration file exceeds %d bytes", maxMigrationFileBytes)) {
		t.Fatalf("readMigrations() error = %v, want per-file bound", err)
	}
	if stats.bytes != maxMigrationFileBytes+1 {
		t.Fatalf("migration bytes read = %d, want %d", stats.bytes, maxMigrationFileBytes+1)
	}
	if stats.opens != 1 || stats.closes != 1 {
		t.Fatalf("migration file lifecycle = %d open(s), %d close(s), want 1 and 1", stats.opens, stats.closes)
	}
}

func TestReadMigrationDirectoryEnforcesStreamingContractAndEntryLimit(t *testing.T) {
	t.Run("empty batch without terminal error", func(t *testing.T) {
		files := readDirOverrideFS{
			FS: fstest.MapFS{"0001.sql": {Data: []byte("SELECT 1;")}},
			readDir: func(count int) ([]fs.DirEntry, error) {
				if count != 32 {
					t.Fatalf("ReadDir() count = %d, want 32", count)
				}
				return nil, nil
			},
		}
		_, err := readMigrationDirectory(files)
		if err == nil || !strings.Contains(err.Error(), "progress contract") {
			t.Fatalf("readMigrationDirectory() error = %v, want progress-contract failure", err)
		}
	})

	t.Run("batch larger than requested", func(t *testing.T) {
		base := makeMigrationMap(maxMigrationFiles+1, []byte("SELECT 1;"))
		entries, err := fs.ReadDir(base, ".")
		if err != nil {
			t.Fatal(err)
		}
		files := readDirOverrideFS{
			FS: base,
			readDir: func(count int) ([]fs.DirEntry, error) {
				return entries[:count+1], nil
			},
		}
		_, err = readMigrationDirectory(files)
		if err == nil || !strings.Contains(err.Error(), "batch bounds") {
			t.Fatalf("readMigrationDirectory() error = %v, want batch-bound failure", err)
		}
	})

	t.Run("exact entry limit", func(t *testing.T) {
		base := makeMigrationMap(maxMigrationFiles, []byte("SELECT 1;"))
		entries, err := fs.ReadDir(base, ".")
		if err != nil {
			t.Fatal(err)
		}
		offset := 0
		files := readDirOverrideFS{
			FS: base,
			readDir: func(count int) ([]fs.DirEntry, error) {
				end := offset + count
				if end > len(entries) {
					end = len(entries)
				}
				batch := append([]fs.DirEntry(nil), entries[offset:end]...)
				offset = end
				if offset == len(entries) {
					return batch, io.EOF
				}
				return batch, nil
			},
		}
		got, err := readMigrationDirectory(files)
		if err != nil {
			t.Fatalf("readMigrationDirectory() error = %v", err)
		}
		if len(got) != maxMigrationFiles {
			t.Fatalf("readMigrationDirectory() count = %d, want %d", len(got), maxMigrationFiles)
		}
	})

	t.Run("entry 257", func(t *testing.T) {
		base := makeMigrationMap(maxMigrationFiles+1, []byte("SELECT 1;"))
		entries, err := fs.ReadDir(base, ".")
		if err != nil {
			t.Fatal(err)
		}
		offset := 0
		calls := 0
		files := readDirOverrideFS{
			FS: base,
			readDir: func(count int) ([]fs.DirEntry, error) {
				calls++
				end := offset + count
				if end > len(entries) {
					end = len(entries)
				}
				batch := append([]fs.DirEntry(nil), entries[offset:end]...)
				offset = end
				if offset == len(entries) {
					return batch, io.EOF
				}
				return batch, nil
			},
		}
		_, err = readMigrationDirectory(files)
		if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("more than %d entries", maxMigrationFiles)) {
			t.Fatalf("readMigrationDirectory() error = %v, want entry-bound failure", err)
		}
		if calls != 9 || offset != maxMigrationFiles+1 {
			t.Fatalf("ReadDir() consumed %d entries in %d calls, want 257 in 9 calls", offset, calls)
		}
	})
}

func TestApplyMigrationsRejectsAggregateBytesBeforeDatabaseEffects(t *testing.T) {
	prefix := "CREATE TABLE aggregate_bound (id INTEGER);"
	data := []byte(prefix + strings.Repeat(" ", maxMigrationFileBytes-len(prefix)))
	stats := &migrationReadStats{}
	files := countingMigrationFS{
		FS:    makeMigrationMap(maxMigrationSetBytes/maxMigrationFileBytes+1, data),
		stats: stats,
	}
	backend := &migrationBoundaryBackend{}
	control, err := New(backend)
	if err != nil {
		t.Fatal(err)
	}

	err = control.ApplyMigrations(context.Background(), "aggregate-bound", files)
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("exceeds %d bytes", maxMigrationSetBytes)) {
		t.Fatalf("ApplyMigrations() error = %v, want aggregate byte bound", err)
	}
	if stats.bytes != maxMigrationSetBytes+1 {
		t.Fatalf("aggregate migration bytes read = %d, want %d", stats.bytes, maxMigrationSetBytes+1)
	}
	wantFilesOpened := maxMigrationSetBytes/maxMigrationFileBytes + 1
	if stats.opens != wantFilesOpened || stats.closes != wantFilesOpened {
		t.Fatalf("migration lifecycle = %d open(s), %d close(s), want %d and %d", stats.opens, stats.closes, wantFilesOpened, wantFilesOpened)
	}
	if backend.validations != 0 || backend.transactions != 0 || backend.adminResolvers != 0 || backend.executions != 0 || backend.queries != 0 {
		t.Fatalf(
			"database effects before aggregate rejection: validation=%d transaction=%d admin=%d exec=%d query=%d",
			backend.validations,
			backend.transactions,
			backend.adminResolvers,
			backend.executions,
			backend.queries,
		)
	}
}

func TestReadAppliedMigrationsRejectsEntry257(t *testing.T) {
	db := openMigrationTestDB(t)
	if _, err := db.Exec(`
		CREATE TABLE modary_module_migration (
			migration_id TEXT PRIMARY KEY,
			module_id TEXT NOT NULL,
			checksum TEXT NOT NULL,
			applied_at TEXT NOT NULL
		)`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	statement, err := tx.Prepare(`INSERT INTO modary_module_migration (migration_id, module_id, checksum, applied_at) VALUES (?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	for index := 0; index < maxMigrationFiles; index++ {
		if _, err := statement.Exec(fmt.Sprintf("bounded/%04d.sql", index), "bounded", "sha256:test", "2026-07-31T00:00:00Z"); err != nil {
			_ = statement.Close()
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := statement.Close(); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	applied, err := readAppliedMigrations(context.Background(), testExecutor{runner: db}, "bounded")
	if err != nil {
		t.Fatalf("readAppliedMigrations() at exact limit error = %v", err)
	}
	if len(applied) != maxMigrationFiles {
		t.Fatalf("readAppliedMigrations() count = %d, want %d", len(applied), maxMigrationFiles)
	}
	if _, err := db.Exec(
		`INSERT INTO modary_module_migration (migration_id, module_id, checksum, applied_at) VALUES (?, ?, ?, ?)`,
		fmt.Sprintf("bounded/%04d.sql", maxMigrationFiles),
		"bounded",
		"sha256:test",
		"2026-07-31T00:00:00Z",
	); err != nil {
		t.Fatal(err)
	}

	_, err = readAppliedMigrations(context.Background(), testExecutor{runner: db}, "bounded")
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("more than %d applied migrations", maxMigrationFiles)) {
		t.Fatalf("readAppliedMigrations() error = %v, want applied-history bound", err)
	}
}

func TestReadAppliedMigrationsBoundsStoredTextBeforeScan(t *testing.T) {
	tests := []struct {
		name      string
		migration any
		checksum  any
	}{
		{
			name:      "oversized migration id",
			migration: strings.Repeat("m", maxAppliedMigrationIDBytes+1),
			checksum:  "sha256:test",
		},
		{
			name:      "oversized checksum",
			migration: "bounded/0001.sql",
			checksum:  strings.Repeat("c", maxMigrationChecksumBytes+1),
		},
		{
			name:      "non-text migration id",
			migration: []byte("bounded/0001.sql"),
			checksum:  "sha256:test",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := openMigrationTestDB(t)
			if _, err := db.Exec(`
				CREATE TABLE modary_module_migration (
					migration_id TEXT PRIMARY KEY,
					module_id TEXT NOT NULL,
					checksum TEXT NOT NULL,
					applied_at TEXT NOT NULL
				)`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(
				`INSERT INTO modary_module_migration (migration_id, module_id, checksum, applied_at) VALUES (?, ?, ?, ?)`,
				test.migration,
				"bounded",
				test.checksum,
				"2026-07-31T00:00:00Z",
			); err != nil {
				t.Fatal(err)
			}
			_, err := readAppliedMigrations(context.Background(), testExecutor{runner: db}, "bounded")
			if err == nil || !strings.Contains(err.Error(), "oversized or non-text") {
				t.Fatalf("readAppliedMigrations() error = %v", err)
			}
		})
	}
}

func makeMigrationMap(count int, data []byte) fstest.MapFS {
	files := make(fstest.MapFS, count)
	for index := 0; index < count; index++ {
		files[fmt.Sprintf("%04d.sql", index)] = &fstest.MapFile{Data: data}
	}
	return files
}

func TestApplyMigrationsRejectsRemovedOrInsertedAppliedHistory(t *testing.T) {
	db := openMigrationTestDB(t)
	control := newTestControl(t, db)
	initial := fstest.MapFS{
		"0001_parent.sql": {Data: []byte(`CREATE TABLE parent (id TEXT PRIMARY KEY);`)},
		"0002_child.sql":  {Data: []byte(`CREATE TABLE child (id TEXT PRIMARY KEY);`)},
	}
	if err := control.ApplyMigrations(context.Background(), "forward-only", initial); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		files fstest.MapFS
	}{
		{
			name: "remove applied migration",
			files: fstest.MapFS{
				"0002_child.sql": initial["0002_child.sql"],
			},
		},
		{
			name: "insert before applied history",
			files: fstest.MapFS{
				"0000_earlier.sql": {Data: []byte(`SELECT 1;`)},
				"0001_parent.sql":  initial["0001_parent.sql"],
				"0002_child.sql":   initial["0002_child.sql"],
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := control.ApplyMigrations(context.Background(), "forward-only", test.files); err == nil {
				t.Fatal("ApplyMigrations() accepted non-prefix migration history")
			}
		})
	}
	assertSQLiteCount(t, db, `SELECT COUNT(*) FROM modary_module_migration WHERE module_id = 'forward-only'`, 2)
}

func TestApplyMigrationsValidatesBeforeDatabaseSideEffects(t *testing.T) {
	db := openMigrationTestDB(t)
	controller := newTestControl(t, db)
	if err := controller.ApplyMigrations(nil, "valid", fstest.MapFS{}); err == nil {
		t.Fatal("ApplyMigrations() accepted a nil context")
	}
	var nilControl *control
	if err := nilControl.ApplyMigrations(context.Background(), "valid", fstest.MapFS{}); err == nil {
		t.Fatal("ApplyMigrations() accepted nil control")
	}

	var typedNil *typedNilMigrationFS
	tooMany := make(fstest.MapFS, maxMigrationFiles+1)
	for index := 0; index <= maxMigrationFiles; index++ {
		tooMany[fmt.Sprintf("%04d.sql", index)] = &fstest.MapFile{Data: []byte(`CREATE TABLE bounded (id INTEGER);`)}
	}
	tests := []struct {
		name     string
		moduleID string
		files    fs.FS
	}{
		{name: "invalid module", moduleID: "", files: fstest.MapFS{}},
		{name: "nil files", moduleID: "valid"},
		{name: "typed nil files", moduleID: "valid", files: typedNil},
		{name: "empty migration set", moduleID: "valid", files: fstest.MapFS{}},
		{name: "empty migration", moduleID: "valid", files: fstest.MapFS{"0001_empty.sql": {Data: []byte(" \n")}}},
		{name: "oversized migration", moduleID: "valid", files: fstest.MapFS{"0001_large.sql": {Data: []byte(strings.Repeat("x", maxMigrationFileBytes+1))}}},
		{name: "too many entries", moduleID: "valid", files: tooMany},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := openMigrationTestDB(t)
			control := newTestControl(t, db)
			if err := control.ApplyMigrations(context.Background(), test.moduleID, test.files); err == nil {
				t.Fatal("ApplyMigrations() accepted invalid input")
			}
			assertSQLiteCount(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'modary_module_migration'`, 0)
		})
	}
}

func TestApplyMigrationsContainsHostileFilesystemErrors(t *testing.T) {
	db := openMigrationTestDB(t)
	control := newTestControl(t, db)
	hostile := &hostileDatabaseError{secret: "migration-secret"}
	err := control.ApplyMigrations(context.Background(), "hostile-source", failingMigrationFS{err: hostile})
	if err == nil {
		t.Fatal("ApplyMigrations() ignored filesystem failure")
	}
	if got := err.Error(); !strings.Contains(got, "list migration files failed") || strings.Contains(got, hostile.secret) {
		t.Fatalf("migration error = %q", got)
	}
	if !errors.Is(err, hostile) {
		t.Fatal("migration filesystem cause was not preserved")
	}
	assertSQLiteCount(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'modary_module_migration'`, 0)
}

func openMigrationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", t.TempDir()+"/migration.db?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func assertSQLiteCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("query count error = %v", err)
	}
	if got != want {
		t.Fatalf("query count = %d, want %d", got, want)
	}
}
