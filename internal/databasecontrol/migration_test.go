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
)

type typedNilMigrationFS struct{}

func (*typedNilMigrationFS) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }

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
	bytes, opens, closes int
}

type countingMigrationFS struct {
	fs.FS
	stats *migrationReadStats
}

func (files countingMigrationFS) Open(name string) (fs.File, error) {
	file, err := files.FS.Open(name)
	if err != nil {
		return nil, err
	}
	if name == "." {
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
	if err != nil || name != "." {
		return file, err
	}
	directory, ok := file.(fs.ReadDirFile)
	if !ok {
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

func TestReadMigrationsAcceptsPartialBatchesAndEnforcesByteBounds(t *testing.T) {
	files := make(fstest.MapFS, 40)
	for index := range 40 {
		files[fmt.Sprintf("%04d_table.sql", index)] = &fstest.MapFile{Data: []byte("SELECT 1;")}
	}
	items, err := readMigrations("partial-batches", partialReadDirFS{FS: files, batch: 3})
	if err != nil || len(items) != len(files) {
		t.Fatalf("readMigrations() count = %d, error = %v", len(items), err)
	}

	stats := &migrationReadStats{}
	_, err = readMigrations("bounded", countingMigrationFS{FS: fstest.MapFS{
		"0001_large.sql": {Data: []byte(strings.Repeat("x", maxMigrationFileBytes+4096))},
	}, stats: stats})
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("exceeds %d bytes", maxMigrationFileBytes)) {
		t.Fatalf("oversized migration error = %v", err)
	}
	if stats.bytes != maxMigrationFileBytes+1 || stats.opens != 1 || stats.closes != 1 {
		t.Fatalf("bounded read = %#v", stats)
	}
}

func TestReadMigrationDirectoryFailsClosedOnInvalidStreaming(t *testing.T) {
	base := fstest.MapFS{"0001.sql": {Data: []byte("SELECT 1;")}}
	_, err := readMigrationDirectory(readDirOverrideFS{FS: base, readDir: func(int) ([]fs.DirEntry, error) {
		return nil, nil
	}})
	if err == nil || !strings.Contains(err.Error(), "progress contract") {
		t.Fatalf("empty streaming batch error = %v", err)
	}

	entries, err := fs.ReadDir(makeMigrationMap(maxMigrationFiles+1), ".")
	if err != nil {
		t.Fatal(err)
	}
	offset := 0
	_, err = readMigrationDirectory(readDirOverrideFS{FS: makeMigrationMap(maxMigrationFiles + 1), readDir: func(count int) ([]fs.DirEntry, error) {
		end := min(offset+count, len(entries))
		batch := append([]fs.DirEntry(nil), entries[offset:end]...)
		offset = end
		if offset == len(entries) {
			return batch, io.EOF
		}
		return batch, nil
	}})
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("more than %d entries", maxMigrationFiles)) {
		t.Fatalf("entry bound error = %v", err)
	}
}

func TestApplyMigrationsUsesValidatedAtomicBackendBoundary(t *testing.T) {
	backend := &migrationUnitBackend{executor: &migrationUnitExecutor{}}
	control, err := New(backend)
	if err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{
		"0001_first.sql":  {Data: []byte("CREATE TABLE first_table (id TEXT);")},
		"0002_second.sql": {Data: []byte("CREATE TABLE second_table (id TEXT);")},
	}
	if err := control.ApplyMigrations(context.Background(), "ordered", files); err != nil {
		t.Fatal(err)
	}
	if backend.validations != 2 || backend.transactions != 1 || backend.executor.records != 2 {
		t.Fatalf("migration boundary = validations %d, transactions %d, records %d", backend.validations, backend.transactions, backend.executor.records)
	}
}

func TestApplyMigrationsRejectsInvalidSourceBeforeDatabaseEffects(t *testing.T) {
	backend := &migrationUnitBackend{executor: &migrationUnitExecutor{}}
	control, err := New(backend)
	if err != nil {
		t.Fatal(err)
	}
	var typedNil *typedNilMigrationFS
	for _, test := range []struct {
		moduleID string
		files    fs.FS
	}{
		{"", fstest.MapFS{"0001.sql": {Data: []byte("SELECT 1;")}}},
		{"valid", nil},
		{"valid", typedNil},
		{"valid", fstest.MapFS{}},
		{"valid", fstest.MapFS{"0001.sql": {Data: []byte(" ")}}},
	} {
		if err := control.ApplyMigrations(context.Background(), test.moduleID, test.files); err == nil {
			t.Fatalf("ApplyMigrations(%q) accepted invalid source", test.moduleID)
		}
	}
	if backend.validations != 0 || backend.transactions != 0 || backend.executor.executes != 0 {
		t.Fatalf("invalid sources reached database: %#v", backend)
	}
}

func TestReadAppliedMigrationsEnforcesHistoryAndTextBounds(t *testing.T) {
	rows := make([][2]sql.NullString, maxMigrationFiles+1)
	for index := range rows {
		rows[index] = [2]sql.NullString{
			{String: fmt.Sprintf("module/%04d.sql", index), Valid: true},
			{String: "sha256:test", Valid: true},
		}
	}
	executor := &migrationUnitExecutor{rows: rows}
	if _, err := readAppliedMigrations(context.Background(), executor, "module"); err == nil || !strings.Contains(err.Error(), "more than") {
		t.Fatalf("history bound error = %v", err)
	}
	executor.rows = [][2]sql.NullString{{{Valid: false}, {String: "sha256:test", Valid: true}}}
	if _, err := readAppliedMigrations(context.Background(), executor, "module"); err == nil || !strings.Contains(err.Error(), "oversized or non-text") {
		t.Fatalf("text bound error = %v", err)
	}
}

func makeMigrationMap(count int) fstest.MapFS {
	files := make(fstest.MapFS, count)
	for index := range count {
		files[fmt.Sprintf("%04d.sql", index)] = &fstest.MapFile{Data: []byte("SELECT 1;")}
	}
	return files
}

type migrationUnitBackend struct {
	executor     *migrationUnitExecutor
	validations  int
	transactions int
}

func (*migrationUnitBackend) Driver() string { return "test" }
func (backend *migrationUnitBackend) ValidateMigration(string) error {
	backend.validations++
	return nil
}
func (backend *migrationUnitBackend) ReadExecutor(context.Context) (database.Executor, error) {
	return backend.executor, nil
}
func (backend *migrationUnitBackend) WriteExecutor(context.Context) (database.Executor, error) {
	return backend.executor, nil
}
func (backend *migrationUnitBackend) AdminExecutor(context.Context) (database.Executor, error) {
	return backend.executor, nil
}
func (backend *migrationUnitBackend) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	backend.transactions++
	return operation(ctx)
}

type migrationUnitExecutor struct {
	rows     [][2]sql.NullString
	executes int
	records  int
}

func (executor *migrationUnitExecutor) ExecContext(_ context.Context, query string, _ ...any) (sql.Result, error) {
	executor.executes++
	if strings.HasPrefix(strings.TrimSpace(query), "INSERT INTO modary_module_migration") {
		executor.records++
	}
	return unitResult(1), nil
}
func (executor *migrationUnitExecutor) QueryContext(context.Context, string, ...any) (database.Rows, error) {
	return &migrationUnitRows{rows: append([][2]sql.NullString(nil), executor.rows...)}, nil
}
func (*migrationUnitExecutor) QueryRowContext(context.Context, string, ...any) database.Row {
	return unitRow{}
}

type migrationUnitRows struct {
	rows  [][2]sql.NullString
	index int
}

func (rows *migrationUnitRows) Next() bool { return rows.index < len(rows.rows) }
func (rows *migrationUnitRows) Scan(destinations ...any) error {
	if len(destinations) != 2 || rows.index >= len(rows.rows) {
		return errors.New("unexpected migration row scan")
	}
	left, leftOK := destinations[0].(*sql.NullString)
	right, rightOK := destinations[1].(*sql.NullString)
	if !leftOK || !rightOK {
		return errors.New("unexpected migration row targets")
	}
	*left, *right = rows.rows[rows.index][0], rows.rows[rows.index][1]
	rows.index++
	return nil
}
func (*migrationUnitRows) Err() error { return nil }
func (*migrationUnitRows) Columns() ([]string, error) {
	return []string{"migration_id", "checksum"}, nil
}
func (*migrationUnitRows) Close() error { return nil }
