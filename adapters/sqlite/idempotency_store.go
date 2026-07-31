package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/internal/actionpersistence"
	"github.com/iiwish/modary/internal/databasecontrol"
)

type idempotencyStore struct {
	control databasecontrol.Control
}

type projectedIdempotencyRecord struct {
	scopeKind, scopeID, actorID, actorType sql.NullString
	actionID, actionVersion, contractHash  sql.NullString
	channel, key, inputHash, planHash      sql.NullString
	impactRows                             sql.NullInt64
	fingerprint, status                    sql.NullString
	resultLength                           sql.NullInt64
	resultSummary, createdAt, updatedAt    sql.NullString
}

// Lookup returns the record bound to one idempotency identity.
func (store *idempotencyStore) Lookup(ctx context.Context, key actionpersistence.IdempotencyRecord) (*actionpersistence.IdempotencyRecord, error) {
	if ctx == nil {
		return nil, fmt.Errorf("idempotency lookup context is required")
	}
	if store == nil || store.control == nil {
		return nil, fmt.Errorf("idempotency store database is required")
	}
	if err := validateLookupRecord(key); err != nil {
		return nil, err
	}
	record, err := store.load(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// Reserve creates a running record or returns the existing binding.
func (store *idempotencyStore) Reserve(ctx context.Context, supplied actionpersistence.IdempotencyRecord) (*actionpersistence.IdempotencyRecord, error) {
	if ctx == nil {
		return nil, fmt.Errorf("idempotency reservation context is required")
	}
	if store == nil || store.control == nil {
		return nil, fmt.Errorf("idempotency store database is required")
	}
	record, err := normalizeReservation(supplied)
	if err != nil {
		return nil, err
	}
	resources, err := json.Marshal(record.Impact.Resources)
	if err != nil {
		return nil, fmt.Errorf("encode idempotency impact resources: %w", err)
	}
	references, err := json.Marshal([]audit.Reference(nil))
	if err != nil {
		return nil, fmt.Errorf("encode empty idempotency references: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	executor, err := executorFor(ctx, store.control)
	if err != nil {
		return nil, err
	}
	result, err := executor.ExecContext(ctx, `
		INSERT INTO modary_action_idempotency (
			scope_kind, scope_id, actor_id, actor_type, action_id, action_version,
			contract_hash, channel, idempotency_key, input_hash, plan_hash, impact_rows,
			impact_resources_json, decision_fingerprint, status, result_data_json,
			result_summary, result_references_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'running', NULL, '', ?, ?, ?)
		ON CONFLICT(scope_kind, scope_id, actor_id, actor_type, action_id, idempotency_key) DO NOTHING`,
		record.Scope.Kind, record.Scope.ID, record.ActorID, record.ActorType, record.ActionID,
		record.ActionVersion, record.ContractHash, record.Channel, record.Key, record.InputHash,
		record.PlanHash, record.Impact.Rows, string(resources), record.DecisionFingerprint,
		string(references), now, now)
	if err != nil {
		return nil, fmt.Errorf("reserve idempotency key: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("inspect idempotency reservation: %w", err)
	}
	if rows == 1 {
		return nil, nil
	}
	existing, err := store.load(ctx, record)
	if err != nil {
		return nil, fmt.Errorf("load concurrent idempotency reservation: %w", err)
	}
	return &existing, nil
}

// Complete atomically attaches the successful Action result.
func (store *idempotencyStore) Complete(ctx context.Context, supplied actionpersistence.IdempotencyRecord) error {
	if ctx == nil {
		return fmt.Errorf("idempotency completion context is required")
	}
	if store == nil || store.control == nil {
		return fmt.Errorf("idempotency store database is required")
	}
	record, err := normalizeCompletion(supplied)
	if err != nil {
		return err
	}
	resources, references, err := encodeIdempotencyCollections(record)
	if err != nil {
		return err
	}
	executor, err := executorFor(ctx, store.control)
	if err != nil {
		return err
	}
	result, err := executor.ExecContext(ctx, `
		UPDATE modary_action_idempotency
		SET status = 'completed', result_data_json = ?, result_summary = ?,
		    result_references_json = ?, updated_at = ?
		WHERE scope_kind = ? AND scope_id = ? AND actor_id = ? AND actor_type = ?
		  AND action_id = ? AND action_version = ? AND contract_hash = ? AND channel = ?
		  AND idempotency_key = ? AND input_hash = ? AND plan_hash = ?
		  AND impact_rows = ? AND impact_resources_json = ? AND decision_fingerprint = ?
		  AND status = 'running'`,
		[]byte(record.Result.Data), record.Result.Summary, string(references), time.Now().UTC().Format(time.RFC3339Nano),
		record.Scope.Kind, record.Scope.ID, record.ActorID, record.ActorType, record.ActionID,
		record.ActionVersion, record.ContractHash, record.Channel, record.Key, record.InputHash,
		record.PlanHash, record.Impact.Rows, string(resources), record.DecisionFingerprint)
	if err != nil {
		return fmt.Errorf("complete idempotency record: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect idempotency completion: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("idempotency record has no matching running reservation")
	}
	return nil
}

// Abort removes a matching running reservation.
func (store *idempotencyStore) Abort(ctx context.Context, supplied actionpersistence.IdempotencyRecord) error {
	if ctx == nil {
		return fmt.Errorf("idempotency abort context is required")
	}
	if store == nil || store.control == nil {
		return fmt.Errorf("idempotency store database is required")
	}
	record, err := normalizeReservation(supplied)
	if err != nil {
		return err
	}
	resources, err := json.Marshal(record.Impact.Resources)
	if err != nil {
		return fmt.Errorf("encode idempotency impact resources: %w", err)
	}
	executor, err := executorFor(ctx, store.control)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `
		DELETE FROM modary_action_idempotency
		WHERE scope_kind = ? AND scope_id = ? AND actor_id = ? AND actor_type = ?
		  AND action_id = ? AND action_version = ? AND contract_hash = ? AND channel = ?
		  AND idempotency_key = ? AND input_hash = ? AND plan_hash = ?
		  AND impact_rows = ? AND impact_resources_json = ? AND decision_fingerprint = ?
		  AND status = 'running'`,
		record.Scope.Kind, record.Scope.ID, record.ActorID, record.ActorType, record.ActionID,
		record.ActionVersion, record.ContractHash, record.Channel, record.Key, record.InputHash,
		record.PlanHash, record.Impact.Rows, string(resources), record.DecisionFingerprint)
	if err != nil {
		return fmt.Errorf("abort idempotency reservation: %w", err)
	}
	return nil
}

func (store *idempotencyStore) load(ctx context.Context, key actionpersistence.IdempotencyRecord) (actionpersistence.IdempotencyRecord, error) {
	var record actionpersistence.IdempotencyRecord
	var resources, resultData, references []byte
	var projected projectedIdempotencyRecord
	executor, err := executorFor(ctx, store.control)
	if err != nil {
		return actionpersistence.IdempotencyRecord{}, err
	}
	arguments := appendStoredProjectionArguments(nil,
		sql.Named("lookup_scope_kind", key.Scope.Kind), sql.Named("lookup_scope_id", key.Scope.ID),
		sql.Named("lookup_actor_id", key.ActorID), sql.Named("lookup_actor_type", key.ActorType),
		sql.Named("lookup_action_id", key.ActionID), sql.Named("lookup_key", key.Key),
	)
	err = executor.QueryRowContext(ctx, `
		SELECT
		       CASE WHEN typeof(scope_kind) = 'text' AND length(CAST(scope_kind AS BLOB)) <= @scope_kind_bytes THEN scope_kind END,
		       CASE WHEN typeof(scope_id) = 'text' AND length(CAST(scope_id AS BLOB)) <= @scope_id_bytes THEN scope_id END,
		       CASE WHEN typeof(actor_id) = 'text' AND length(CAST(actor_id AS BLOB)) <= @actor_id_bytes THEN actor_id END,
		       CASE WHEN typeof(actor_type) = 'text' AND length(CAST(actor_type AS BLOB)) <= @actor_type_bytes THEN actor_type END,
		       CASE WHEN typeof(action_id) = 'text' AND length(CAST(action_id AS BLOB)) <= @action_id_bytes THEN action_id END,
		       CASE WHEN typeof(action_version) = 'text' AND length(CAST(action_version AS BLOB)) <= @version_bytes THEN action_version END,
		       CASE WHEN typeof(contract_hash) = 'text' AND length(CAST(contract_hash AS BLOB)) <= @hash_bytes THEN contract_hash END,
		       CASE WHEN typeof(channel) = 'text' AND length(CAST(channel AS BLOB)) <= @channel_bytes THEN channel END,
		       CASE WHEN typeof(idempotency_key) = 'text' AND length(CAST(idempotency_key AS BLOB)) <= @idempotency_key_bytes THEN idempotency_key END,
		       CASE WHEN typeof(input_hash) = 'text' AND length(CAST(input_hash AS BLOB)) <= @hash_bytes THEN input_hash END,
		       CASE WHEN typeof(plan_hash) = 'text' AND length(CAST(plan_hash AS BLOB)) <= @hash_bytes THEN plan_hash END,
		       CASE WHEN typeof(impact_rows) = 'integer' THEN impact_rows END,
		       CASE WHEN typeof(impact_resources_json) = 'text' AND length(CAST(impact_resources_json AS BLOB)) <= @json_bytes THEN impact_resources_json END,
		       CASE WHEN typeof(decision_fingerprint) = 'text' AND length(CAST(decision_fingerprint AS BLOB)) <= @fingerprint_bytes THEN decision_fingerprint END,
		       CASE WHEN typeof(status) = 'text' AND length(CAST(status AS BLOB)) <= @status_bytes THEN status END,
		       CASE
		           WHEN result_data_json IS NULL THEN -1
		           WHEN typeof(result_data_json) <> 'blob' THEN -2
		           ELSE length(CAST(result_data_json AS BLOB))
		       END,
		       CASE WHEN result_data_json IS NULL OR (typeof(result_data_json) = 'blob' AND length(CAST(result_data_json AS BLOB)) <= @json_bytes) THEN result_data_json END,
		       CASE WHEN typeof(result_summary) = 'text' AND length(CAST(result_summary AS BLOB)) <= @summary_bytes THEN result_summary END,
		       CASE WHEN typeof(result_references_json) = 'text' AND length(CAST(result_references_json AS BLOB)) <= @json_bytes THEN result_references_json END,
		       CASE WHEN typeof(created_at) = 'text' AND length(CAST(created_at AS BLOB)) <= @timestamp_bytes THEN created_at END,
		       CASE WHEN typeof(updated_at) = 'text' AND length(CAST(updated_at AS BLOB)) <= @timestamp_bytes THEN updated_at END
		FROM modary_action_idempotency
		WHERE scope_kind = @lookup_scope_kind AND scope_id = @lookup_scope_id
		  AND actor_id = @lookup_actor_id AND actor_type = @lookup_actor_type
		  AND action_id = @lookup_action_id AND idempotency_key = @lookup_key`, arguments...).Scan(
		&projected.scopeKind, &projected.scopeID, &projected.actorID, &projected.actorType,
		&projected.actionID, &projected.actionVersion, &projected.contractHash, &projected.channel,
		&projected.key, &projected.inputHash, &projected.planHash, &projected.impactRows, &resources,
		&projected.fingerprint, &projected.status, &projected.resultLength, &resultData,
		&projected.resultSummary, &references, &projected.createdAt, &projected.updatedAt)
	if err != nil {
		return actionpersistence.IdempotencyRecord{}, err
	}
	if !projected.resultLength.Valid || projected.resultLength.Int64 < -1 {
		return actionpersistence.IdempotencyRecord{}, fmt.Errorf("stored idempotency result has the wrong type")
	}
	if projected.resultLength.Int64 > action.MaxJSONDocumentBytes {
		return actionpersistence.IdempotencyRecord{}, fmt.Errorf("stored idempotency result exceeds the JSON resource limit")
	}
	if resources == nil {
		return actionpersistence.IdempotencyRecord{}, fmt.Errorf("stored idempotency impact resources exceed the JSON resource limit")
	}
	if references == nil {
		return actionpersistence.IdempotencyRecord{}, fmt.Errorf("stored idempotency references exceed the JSON resource limit")
	}
	if projected.resultLength.Int64 >= 0 && resultData == nil {
		return actionpersistence.IdempotencyRecord{}, fmt.Errorf("stored idempotency result is unavailable within its JSON resource limit")
	}
	if err := populateProjectedIdempotencyRecord(&record, projected); err != nil {
		return actionpersistence.IdempotencyRecord{}, err
	}
	if _, err := parseStoredTime("idempotency creation time", projected.createdAt.String); err != nil {
		return actionpersistence.IdempotencyRecord{}, err
	}
	if _, err := parseStoredTime("idempotency update time", projected.updatedAt.String); err != nil {
		return actionpersistence.IdempotencyRecord{}, err
	}
	if err := decodeStrictJSON(resources, &record.Impact.Resources); err != nil {
		return actionpersistence.IdempotencyRecord{}, fmt.Errorf("decode stored idempotency impact resources: %w", err)
	}
	if err := decodeStrictJSON(references, &record.Result.References); err != nil {
		return actionpersistence.IdempotencyRecord{}, fmt.Errorf("decode stored idempotency references: %w", err)
	}
	record.Result.Data = append([]byte(nil), resultData...)
	record, err = normalizeStoredRecord(record)
	if err != nil {
		return actionpersistence.IdempotencyRecord{}, fmt.Errorf("stored idempotency record is invalid: %w", err)
	}
	return record, nil
}

func populateProjectedIdempotencyRecord(record *actionpersistence.IdempotencyRecord, value projectedIdempotencyRecord) error {
	var err error
	if record.Scope.Kind, err = requireProjectedText(value.scopeKind, "idempotency scope kind"); err != nil {
		return err
	}
	if record.Scope.ID, err = requireProjectedText(value.scopeID, "idempotency scope id"); err != nil {
		return err
	}
	if record.ActorID, err = requireProjectedText(value.actorID, "idempotency actor id"); err != nil {
		return err
	}
	if record.ActorType, err = requireProjectedText(value.actorType, "idempotency actor type"); err != nil {
		return err
	}
	if record.ActionID, err = requireProjectedText(value.actionID, "idempotency action id"); err != nil {
		return err
	}
	if record.ActionVersion, err = requireProjectedText(value.actionVersion, "idempotency action version"); err != nil {
		return err
	}
	if record.ContractHash, err = requireProjectedText(value.contractHash, "idempotency contract hash"); err != nil {
		return err
	}
	channel, err := requireProjectedText(value.channel, "idempotency channel")
	if err != nil {
		return err
	}
	record.Channel = action.Channel(channel)
	if record.Key, err = requireProjectedText(value.key, "idempotency key"); err != nil {
		return err
	}
	if record.InputHash, err = requireProjectedText(value.inputHash, "idempotency input hash"); err != nil {
		return err
	}
	if record.PlanHash, err = requireProjectedText(value.planHash, "idempotency plan hash"); err != nil {
		return err
	}
	impactRows, err := requireProjectedInteger(value.impactRows, "idempotency impact rows")
	if err != nil {
		return err
	}
	record.Impact.Rows = int(impactRows)
	if int64(record.Impact.Rows) != impactRows {
		return fmt.Errorf("stored idempotency impact rows exceed the platform integer range")
	}
	if record.DecisionFingerprint, err = requireProjectedText(value.fingerprint, "idempotency decision fingerprint"); err != nil {
		return err
	}
	status, err := requireProjectedText(value.status, "idempotency status")
	if err != nil {
		return err
	}
	record.Status = actionpersistence.IdempotencyStatus(status)
	if record.Result.Summary, err = requireProjectedText(value.resultSummary, "idempotency result summary"); err != nil {
		return err
	}
	if _, err = requireProjectedText(value.createdAt, "idempotency creation time"); err != nil {
		return err
	}
	if _, err = requireProjectedText(value.updatedAt, "idempotency update time"); err != nil {
		return err
	}
	return nil
}

func encodeIdempotencyCollections(record actionpersistence.IdempotencyRecord) ([]byte, []byte, error) {
	resources, err := json.Marshal(record.Impact.Resources)
	if err != nil {
		return nil, nil, fmt.Errorf("encode idempotency impact resources: %w", err)
	}
	references, err := json.Marshal(record.Result.References)
	if err != nil {
		return nil, nil, fmt.Errorf("encode idempotency result references: %w", err)
	}
	return resources, references, nil
}
