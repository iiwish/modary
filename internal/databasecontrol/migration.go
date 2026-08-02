package databasecontrol

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/iiwish/modary/database"
)

type migration struct {
	id, checksum, sql string
}

var migrationModuleIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

const (
	maxMigrationFiles          = 256
	maxMigrationFileBytes      = 1 << 20
	maxMigrationSetBytes       = 16 << 20
	maxMigrationNameBytes      = 255
	maxAppliedMigrationIDBytes = 63 + 1 + maxMigrationNameBytes
	maxMigrationChecksumBytes  = len("sha256:") + sha256.Size*2
)

// ApplyMigrations atomically applies one declared migration source through
// privileged database control. The Host invokes it after database control is
// available and before the target Module's handlers are constructed.
func (control *control) ApplyMigrations(ctx context.Context, moduleID string, files fs.FS) (err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			err = &dependencyError{operation: "apply database migrations", cause: database.ErrDependencyPanic}
		}
	}()
	err = control.applyMigrations(ctx, moduleID, files)
	returned = true
	return err
}

func (control *control) applyMigrations(ctx context.Context, moduleID string, files fs.FS) error {
	if ctx == nil {
		return fmt.Errorf("migration context is required")
	}
	if control == nil || isNil(control.backend) {
		return ErrControlUnavailable
	}
	current, err := readMigrations(moduleID, files)
	if err != nil {
		return err
	}
	for _, item := range current {
		if err := invokeDependencyError("validate migration SQL", func() error {
			return control.backend.ValidateMigration(item.sql)
		}); err != nil {
			return fmt.Errorf("validate migration %s: %w", item.id, err)
		}
	}
	return control.withinTransaction(ctx, func(txCtx context.Context) error {
		executor, err := control.Executor(txCtx)
		if err != nil {
			return fmt.Errorf("resolve migration executor: %w", err)
		}
		if locker, ok := control.backend.(migrationLocker); ok {
			if err := invokeDependencyError("acquire database migration lock", func() error {
				return locker.LockMigrations(txCtx, executor)
			}); err != nil {
				return err
			}
		}
		if _, err := invokeDependency("create migration registry", func() (sql.Result, error) {
			return executor.ExecContext(txCtx, `
			CREATE TABLE IF NOT EXISTS modary_module_migration (
				migration_id TEXT PRIMARY KEY CHECK (octet_length(migration_id) <= 319),
				module_id TEXT NOT NULL CHECK (octet_length(module_id) <= 63),
				checksum TEXT NOT NULL CHECK (octet_length(checksum) <= 71),
				applied_at TEXT NOT NULL CHECK (octet_length(applied_at) BETWEEN 20 AND 30)
			)`)
		}); err != nil {
			return fmt.Errorf("create migration registry: %w", err)
		}
		applied, err := readAppliedMigrations(txCtx, executor, moduleID)
		if err != nil {
			return err
		}
		if len(applied) > len(current) {
			return fmt.Errorf("module %s migration source removed %d applied migration(s)", moduleID, len(applied)-len(current))
		}
		for index, existing := range applied {
			declared := current[index]
			if existing.id != declared.id {
				return fmt.Errorf("applied migration %s is not the current migration prefix at position %d (found %s)", existing.id, index+1, declared.id)
			}
			if existing.checksum != declared.checksum {
				return fmt.Errorf("applied migration %s checksum changed", existing.id)
			}
		}
		for _, item := range current[len(applied):] {
			if _, err := invokeDependency("execute migration statement", func() (sql.Result, error) {
				return executor.ExecContext(txCtx, item.sql)
			}); err != nil {
				return fmt.Errorf("apply migration %s: %w", item.id, err)
			}
			if _, err := invokeDependency("record applied migration", func() (sql.Result, error) {
				return executor.ExecContext(txCtx, `INSERT INTO modary_module_migration (migration_id, module_id, checksum, applied_at) VALUES ($1, $2, $3, $4)`, item.id, moduleID, item.checksum, time.Now().UTC().Format(time.RFC3339Nano))
			}); err != nil {
				return fmt.Errorf("record migration %s: %w", item.id, err)
			}
		}
		return nil
	}, true)
}

func readMigrations(moduleID string, files fs.FS) ([]migration, error) {
	if !migrationModuleIDPattern.MatchString(moduleID) {
		return nil, fmt.Errorf("migration module id %q is invalid", moduleID)
	}
	if isNilFS(files) {
		return nil, fmt.Errorf("migration files for %s are required", moduleID)
	}
	directoryEntries, err := readMigrationDirectory(files)
	if err != nil {
		return nil, fmt.Errorf("list migrations for %s: %w", moduleID, err)
	}
	entries := make([]string, 0, len(directoryEntries))
	seenEntries := make(map[string]struct{}, len(directoryEntries))
	for _, entry := range directoryEntries {
		if entry == nil {
			return nil, fmt.Errorf("migration source for %s returned an invalid directory entry", moduleID)
		}
		name := entry.Name()
		if !utf8.ValidString(name) || len(name) > maxMigrationNameBytes || !fs.ValidPath(name) || path.Base(name) != name {
			return nil, fmt.Errorf("migration source for %s returned an invalid file name", moduleID)
		}
		if _, duplicate := seenEntries[name]; duplicate {
			return nil, fmt.Errorf("migration source for %s returned duplicate entry %s", moduleID, name)
		}
		seenEntries[name] = struct{}{}
		matched, matchErr := path.Match("*.sql", name)
		if matchErr != nil {
			return nil, fmt.Errorf("match migration file %s: %w", name, matchErr)
		}
		if matched {
			if entry.IsDir() {
				return nil, fmt.Errorf("migration %s/%s is not a regular file", moduleID, name)
			}
			entries = append(entries, name)
		}
	}
	sort.Strings(entries)
	if len(entries) == 0 {
		return nil, fmt.Errorf("module %s migration source has no .sql files", moduleID)
	}
	migrations := make([]migration, 0, len(entries))
	totalBytes := 0
	for _, name := range entries {
		data, err := readMigrationFile(files, name, maxMigrationSetBytes-totalBytes)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}
		totalBytes += len(data)
		if totalBytes > maxMigrationSetBytes {
			return nil, fmt.Errorf("migration source for %s exceeds %d bytes", moduleID, maxMigrationSetBytes)
		}
		sqlText := strings.TrimSpace(string(data))
		if sqlText == "" {
			return nil, fmt.Errorf("migration %s/%s is empty", moduleID, name)
		}
		hash := sha256.Sum256(data)
		migrations = append(migrations, migration{
			id:       moduleID + "/" + path.Base(name),
			checksum: "sha256:" + hex.EncodeToString(hash[:]),
			sql:      sqlText,
		})
	}
	return migrations, nil
}

func readMigrationDirectory(files fs.FS) (entries []fs.DirEntry, err error) {
	directory, err := invokeDependency("list migration files", func() (fs.File, error) {
		return files.Open(".")
	})
	if err != nil {
		return nil, err
	}
	if isNil(directory) {
		return nil, fmt.Errorf("open migration directory returned no file")
	}
	defer func() {
		closeErr := invokeDependencyError("close migration directory", directory.Close)
		if closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	reader, ok := directory.(fs.ReadDirFile)
	if !ok || isNil(reader) {
		return nil, fmt.Errorf("migration root is not a directory")
	}
	const batchSize = 32
	for {
		batch, readErr := invokeDependency("list migration files", func() ([]fs.DirEntry, error) {
			return reader.ReadDir(batchSize)
		})
		if len(batch) > batchSize {
			return nil, fmt.Errorf("migration source violated fs.ReadDirFile batch bounds")
		}
		entries = append(entries, batch...)
		if len(entries) > maxMigrationFiles {
			return nil, fmt.Errorf("migration source contains more than %d entries", maxMigrationFiles)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, readErr
		}
		if len(batch) == 0 {
			return nil, fmt.Errorf("migration source violated fs.ReadDirFile progress contract")
		}
	}
	return append([]fs.DirEntry(nil), entries...), nil
}

func readMigrationFile(files fs.FS, name string, remainingSetBytes int) (data []byte, err error) {
	file, err := invokeDependency("open migration file", func() (fs.File, error) {
		return files.Open(name)
	})
	if err != nil {
		return nil, err
	}
	if isNil(file) {
		return nil, fmt.Errorf("open migration file returned no file")
	}
	defer func() {
		closeErr := invokeDependencyError("close migration file", file.Close)
		if closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	readLimit := maxMigrationFileBytes
	if remainingSetBytes < readLimit {
		readLimit = remainingSetBytes
	}
	if readLimit < 0 {
		readLimit = 0
	}
	data, err = invokeDependency("read migration file", func() ([]byte, error) {
		return io.ReadAll(io.LimitReader(file, int64(readLimit)+1))
	})
	if err != nil {
		return nil, err
	}
	if len(data) > maxMigrationFileBytes {
		return nil, fmt.Errorf("migration file exceeds %d bytes", maxMigrationFileBytes)
	}
	return data, nil
}

func isNilFS(files fs.FS) bool {
	if files == nil {
		return true
	}
	value := reflect.ValueOf(files)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func readAppliedMigrations(ctx context.Context, executor database.Executor, moduleID string) ([]migration, error) {
	rows, err := invokeDependency("list applied migrations", func() (database.Rows, error) {
		return executor.QueryContext(ctx, `
				SELECT
					CASE WHEN octet_length(migration_id) <= $1 THEN migration_id END,
					CASE WHEN octet_length(checksum) <= $2 THEN checksum END
				FROM modary_module_migration
				WHERE module_id = $3
				ORDER BY migration_id
				LIMIT $4`, maxAppliedMigrationIDBytes, maxMigrationChecksumBytes, moduleID, maxMigrationFiles+1)
	})
	if err != nil {
		return nil, fmt.Errorf("list applied migrations for %s: %w", moduleID, err)
	}
	if rows == nil {
		return nil, ErrControlUnavailable
	}
	defer rows.Close()
	applied := make([]migration, 0)
	for rows.Next() {
		if len(applied) == maxMigrationFiles {
			return nil, fmt.Errorf("module %s has more than %d applied migrations", moduleID, maxMigrationFiles)
		}
		var storedID, storedChecksum sql.NullString
		if err := rows.Scan(&storedID, &storedChecksum); err != nil {
			return nil, fmt.Errorf("read applied migration for %s: %w", moduleID, err)
		}
		if !storedID.Valid || !storedChecksum.Valid {
			return nil, fmt.Errorf("applied migration history for %s contains oversized or non-text fields", moduleID)
		}
		item := migration{id: storedID.String, checksum: storedChecksum.String}
		applied = append(applied, item)
	}
	if err := invokeDependencyError("finish reading applied migrations", rows.Err); err != nil {
		return nil, fmt.Errorf("list applied migrations for %s: %w", moduleID, err)
	}
	return applied, nil
}
