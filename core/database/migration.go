package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func ApplyMigrations(ctx context.Context, db *sql.DB, moduleID string, files fs.FS) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS modary_module_migration (
			migration_id TEXT PRIMARY KEY,
			module_id TEXT NOT NULL,
			checksum TEXT NOT NULL,
			applied_at TEXT NOT NULL
		)`); err != nil {
		return fmt.Errorf("create migration registry: %w", err)
	}
	entries, err := fs.Glob(files, "*.sql")
	if err != nil {
		return fmt.Errorf("list migrations for %s: %w", moduleID, err)
	}
	sort.Strings(entries)
	for _, name := range entries {
		data, err := fs.ReadFile(files, name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		migrationID := moduleID + "/" + filepath.Base(name)
		hash := sha256.Sum256(data)
		checksum := "sha256:" + hex.EncodeToString(hash[:])
		var existing string
		err = db.QueryRowContext(ctx, `SELECT checksum FROM modary_module_migration WHERE migration_id = ?`, migrationID).Scan(&existing)
		switch {
		case err == nil:
			if existing != checksum {
				return fmt.Errorf("applied migration %s checksum changed", migrationID)
			}
			continue
		case err != sql.ErrNoRows:
			return fmt.Errorf("check migration %s: %w", migrationID, err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", migrationID, err)
		}
		if _, err := tx.ExecContext(ctx, strings.TrimSpace(string(data))); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", migrationID, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO modary_module_migration (migration_id, module_id, checksum, applied_at) VALUES (?, ?, ?, ?)`, migrationID, moduleID, checksum, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", migrationID, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", migrationID, err)
		}
	}
	return nil
}
