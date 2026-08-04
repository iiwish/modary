package governedpostgres

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

type idempotencyStore struct{ control databasecontrol.Control }

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
	resourcesValue := record.Impact.Resources
	if resourcesValue == nil {
		resourcesValue = []string{}
	}
	resources, err := json.Marshal(resourcesValue)
	if err != nil {
		return nil, fmt.Errorf("encode idempotency impact resources: %w", err)
	}
	references, _ := json.Marshal([]audit.Reference{})
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
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::jsonb, $14, 'running', NULL, '', $15::jsonb, $16, $17)
		ON CONFLICT(scope_kind, scope_id, actor_id, actor_type, action_id, idempotency_key) DO NOTHING`,
		record.Scope.Kind, record.Scope.ID, record.ActorID, record.ActorType,
		record.ActionID, record.ActionVersion, record.ContractHash, record.Channel,
		record.Key, record.InputHash, record.PlanHash, record.Impact.Rows,
		string(resources), record.DecisionFingerprint, string(references), now, now)
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
		SET status = 'completed', result_data_json = $1, result_summary = $2,
		    result_references_json = $3::jsonb, updated_at = $4
		WHERE scope_kind = $5 AND scope_id = $6 AND actor_id = $7 AND actor_type = $8
		  AND action_id = $9 AND action_version = $10 AND contract_hash = $11 AND channel = $12
		  AND idempotency_key = $13 AND input_hash = $14 AND plan_hash = $15
		  AND impact_rows = $16 AND impact_resources_json = $17::jsonb AND decision_fingerprint = $18
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
	resourcesValue := record.Impact.Resources
	if resourcesValue == nil {
		resourcesValue = []string{}
	}
	resources, err := json.Marshal(resourcesValue)
	if err != nil {
		return fmt.Errorf("encode idempotency impact resources: %w", err)
	}
	executor, err := executorFor(ctx, store.control)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `
		DELETE FROM modary_action_idempotency
		WHERE scope_kind = $1 AND scope_id = $2 AND actor_id = $3 AND actor_type = $4
		  AND action_id = $5 AND action_version = $6 AND contract_hash = $7 AND channel = $8
		  AND idempotency_key = $9 AND input_hash = $10 AND plan_hash = $11
		  AND impact_rows = $12 AND impact_resources_json = $13::jsonb AND decision_fingerprint = $14
		  AND status = 'running'`,
		record.Scope.Kind, record.Scope.ID, record.ActorID, record.ActorType,
		record.ActionID, record.ActionVersion, record.ContractHash, record.Channel,
		record.Key, record.InputHash, record.PlanHash, record.Impact.Rows,
		string(resources), record.DecisionFingerprint)
	if err != nil {
		return fmt.Errorf("abort idempotency reservation: %w", err)
	}
	return nil
}

func (store *idempotencyStore) load(ctx context.Context, key actionpersistence.IdempotencyRecord) (actionpersistence.IdempotencyRecord, error) {
	executor, err := executorFor(ctx, store.control)
	if err != nil {
		return actionpersistence.IdempotencyRecord{}, err
	}
	var record actionpersistence.IdempotencyRecord
	var resources, resultData, references []byte
	var projected projectedIdempotencyRecord
	err = executor.QueryRowContext(ctx, `
		SELECT
		       CASE WHEN octet_length(scope_kind) <= $7 THEN scope_kind END,
		       CASE WHEN octet_length(scope_id) <= $8 THEN scope_id END,
		       CASE WHEN octet_length(actor_id) <= $9 THEN actor_id END,
		       CASE WHEN octet_length(actor_type) <= $10 THEN actor_type END,
		       CASE WHEN octet_length(action_id) <= $11 THEN action_id END,
		       CASE WHEN octet_length(action_version) <= $12 THEN action_version END,
		       CASE WHEN octet_length(contract_hash) <= $13 THEN contract_hash END,
		       CASE WHEN octet_length(channel) <= $14 THEN channel END,
		       CASE WHEN octet_length(idempotency_key) <= $15 THEN idempotency_key END,
		       CASE WHEN octet_length(input_hash) <= $13 THEN input_hash END,
		       CASE WHEN octet_length(plan_hash) <= $13 THEN plan_hash END,
		       impact_rows,
		       CASE WHEN octet_length(impact_resources_json::text) <= $16 THEN impact_resources_json END,
		       CASE WHEN octet_length(decision_fingerprint) <= $17 THEN decision_fingerprint END,
		       CASE WHEN octet_length(status) <= $18 THEN status END,
		       CASE WHEN result_data_json IS NULL THEN -1 ELSE octet_length(result_data_json) END,
		       CASE WHEN result_data_json IS NULL OR octet_length(result_data_json) <= $16 THEN result_data_json END,
		       CASE WHEN octet_length(result_summary) <= $19 THEN result_summary END,
		       CASE WHEN octet_length(result_references_json::text) <= $16 THEN result_references_json END,
		       CASE WHEN octet_length(created_at) <= $20 THEN created_at END,
		       CASE WHEN octet_length(updated_at) <= $20 THEN updated_at END
		FROM modary_action_idempotency
		WHERE scope_kind = $1 AND scope_id = $2 AND actor_id = $3 AND actor_type = $4
		  AND action_id = $5 AND idempotency_key = $6`,
		key.Scope.Kind, key.Scope.ID, key.ActorID, key.ActorType, key.ActionID, key.Key,
		maxStoredScopeKindBytes, maxStoredScopeIDBytes, maxStoredActorIDBytes,
		maxStoredActorTypeBytes, maxStoredActionIDBytes, maxStoredVersionBytes,
		maxStoredHashBytes, maxStoredChannelBytes, maxStoredIdempotencyKeyBytes,
		action.MaxJSONDocumentBytes, maxStoredFingerprintBytes, maxStoredStatusBytes,
		maxStoredSummaryBytes, maxStoredTimestampBytes).Scan(
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
	record.Result.Data = append(json.RawMessage(nil), resultData...)
	record, err = normalizeStoredRecord(record)
	if err != nil {
		return actionpersistence.IdempotencyRecord{}, fmt.Errorf("stored idempotency record is invalid: %w", err)
	}
	return record, nil
}

func encodeIdempotencyCollections(record actionpersistence.IdempotencyRecord) ([]byte, []byte, error) {
	resourcesValue := record.Impact.Resources
	if resourcesValue == nil {
		resourcesValue = []string{}
	}
	resources, err := json.Marshal(resourcesValue)
	if err != nil {
		return nil, nil, fmt.Errorf("encode idempotency impact resources: %w", err)
	}
	referencesValue := record.Result.References
	if referencesValue == nil {
		referencesValue = []audit.Reference{}
	}
	references, err := json.Marshal(referencesValue)
	if err != nil {
		return nil, nil, fmt.Errorf("encode idempotency result references: %w", err)
	}
	return resources, references, nil
}
