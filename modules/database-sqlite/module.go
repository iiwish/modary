package database_sqlite

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"modary/core/action"
	"modary/core/config"
	"modary/core/database"
	"modary/core/module"

	_ "modernc.org/sqlite"
)

//go:embed module.yaml
var manifestData []byte

//go:embed migrations/sqlite/*.sql
var migrationFiles embed.FS

func Module() module.Registration {
	return module.Registration{Manifest: module.MustParseManifest(manifestData), Install: install}
}

func install(ctx context.Context, host *module.Host) error {
	cfg, err := module.ServiceAs[config.Runtime](host, module.ServiceConfig)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.DatabasePath), 0o750); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	db, err := sql.Open("sqlite", sqliteDSN(cfg.DatabasePath))
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)
	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode=WAL;`); err != nil {
		_ = db.Close()
		return fmt.Errorf("configure sqlite: %w", err)
	}
	sub, err := fs.Sub(migrationFiles, "migrations/sqlite")
	if err != nil {
		_ = db.Close()
		return err
	}
	if err := database.ApplyMigrations(ctx, db, "database-sqlite", sub); err != nil {
		_ = db.Close()
		return err
	}
	if err := host.Provide(module.ServiceDatabase, db); err != nil {
		return err
	}
	if err := host.Provide(module.ServiceTransactions, transactionManager{db: db}); err != nil {
		return err
	}
	if err := host.Provide(module.ServicePlanStore, &planStore{db: db}); err != nil {
		return err
	}
	return host.Provide(module.ServiceIdempotencyStore, &idempotencyStore{db: db})
}

func sqliteDSN(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
}

type transactionManager struct{ db *sql.DB }

func (m transactionManager) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	txCtx := database.WithTransaction(ctx, tx)
	if err := fn(txCtx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

type planStore struct{ db *sql.DB }

func (s *planStore) Save(ctx context.Context, plan action.Plan) error {
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	_, err = database.ExecutorFor(ctx, s.db).ExecContext(ctx, `
		INSERT INTO modary_action_plan (plan_hash, action_id, actor_id, workspace_id, plan_json, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(plan_hash) DO UPDATE SET plan_json = excluded.plan_json, expires_at = excluded.expires_at`,
		plan.Hash, plan.ActionID, plan.ActorID, plan.WorkspaceID, data, plan.ExpiresAt.Format(time.RFC3339Nano), plan.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *planStore) Get(ctx context.Context, hash string) (action.Plan, error) {
	var data []byte
	if err := database.ExecutorFor(ctx, s.db).QueryRowContext(ctx, `SELECT plan_json FROM modary_action_plan WHERE plan_hash = ?`, hash).Scan(&data); err != nil {
		return action.Plan{}, err
	}
	var plan action.Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return action.Plan{}, err
	}
	return plan, nil
}

type idempotencyStore struct{ db *sql.DB }

func (s *idempotencyStore) Reserve(ctx context.Context, record action.IdempotencyRecord) (*action.IdempotencyRecord, error) {
	executor := database.ExecutorFor(ctx, s.db)
	existing, err := loadIdempotency(ctx, executor, record)
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	_, err = executor.ExecContext(ctx, `
		INSERT INTO modary_action_idempotency
		(workspace_id, actor_id, action_id, idempotency_key, input_hash, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'running', ?, ?)`,
		record.WorkspaceID, record.ActorID, record.ActionID, record.Key, record.InputHash,
		time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func (s *idempotencyStore) Complete(ctx context.Context, record action.IdempotencyRecord) error {
	data, err := json.Marshal(record.Result)
	if err != nil {
		return err
	}
	result, err := database.ExecutorFor(ctx, s.db).ExecContext(ctx, `
		UPDATE modary_action_idempotency
		SET status = 'completed', result_json = ?, updated_at = ?
		WHERE workspace_id = ? AND actor_id = ? AND action_id = ? AND idempotency_key = ? AND input_hash = ?`,
		data, time.Now().UTC().Format(time.RFC3339Nano), record.WorkspaceID, record.ActorID, record.ActionID, record.Key, record.InputHash)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("idempotency record was not reserved")
	}
	return nil
}

func loadIdempotency(ctx context.Context, executor database.Executor, key action.IdempotencyRecord) (action.IdempotencyRecord, error) {
	var record action.IdempotencyRecord
	var resultJSON []byte
	err := executor.QueryRowContext(ctx, `
		SELECT workspace_id, actor_id, action_id, idempotency_key, input_hash, status, COALESCE(result_json, '{}')
		FROM modary_action_idempotency
		WHERE workspace_id = ? AND actor_id = ? AND action_id = ? AND idempotency_key = ?`,
		key.WorkspaceID, key.ActorID, key.ActionID, key.Key,
	).Scan(&record.WorkspaceID, &record.ActorID, &record.ActionID, &record.Key, &record.InputHash, &record.Status, &resultJSON)
	if err != nil {
		return action.IdempotencyRecord{}, err
	}
	if record.Status == "completed" {
		if err := json.Unmarshal(resultJSON, &record.Result); err != nil {
			return action.IdempotencyRecord{}, err
		}
	}
	return record, nil
}
