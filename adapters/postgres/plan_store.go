package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"time"
	"unicode/utf8"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/actionpersistence"
	"github.com/iiwish/modary/internal/databasecontrol"
)

type planStore struct{ control databasecontrol.Control }

func (store *planStore) Save(ctx context.Context, supplied action.Plan) error {
	if ctx == nil {
		return fmt.Errorf("plan save context is required")
	}
	if store == nil || store.control == nil {
		return fmt.Errorf("plan store database is required")
	}
	plan, err := normalizePlan(supplied)
	if err != nil {
		return err
	}
	resourcesValue := plan.Impact.Resources
	if resourcesValue == nil {
		resourcesValue = []string{}
	}
	resources, err := json.Marshal(resourcesValue)
	if err != nil {
		return fmt.Errorf("encode plan impact resources: %w", err)
	}
	executor, err := executorFor(ctx, store.control)
	if err != nil {
		return err
	}
	result, err := executor.ExecContext(ctx, `
		INSERT INTO modary_action_plan (
			plan_hash, action_id, action_version, contract_hash, actor_id, actor_type,
			channel, scope_kind, scope_id, input_hash, payload_json, impact_rows,
			impact_resources_json, snapshot_hash, decision_fingerprint, created_at,
			expires_at, expires_at_unix_nano
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::jsonb, $14, $15, $16, $17, $18)
		ON CONFLICT(plan_hash) DO NOTHING`,
		plan.Hash, plan.ActionID, plan.ActionVersion, plan.ContractHash, plan.ActorID,
		plan.ActorType, plan.Channel, plan.Scope.Kind, plan.Scope.ID, plan.InputHash,
		[]byte(plan.Payload), plan.Impact.Rows, string(resources), plan.SnapshotHash,
		plan.DecisionFingerprint, plan.CreatedAt.Format(time.RFC3339Nano),
		plan.ExpiresAt.Format(time.RFC3339Nano), plan.ExpiresAt.UnixNano())
	if err != nil {
		return fmt.Errorf("save Action plan: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect saved Action plan: %w", err)
	}
	if rows == 1 {
		return nil
	}
	existing, err := store.Get(ctx, plan.Hash)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(existing, plan) {
		return fmt.Errorf("plan hash %s is already bound to different provenance", plan.Hash)
	}
	return nil
}

func (store *planStore) Get(ctx context.Context, hash string) (action.Plan, error) {
	if ctx == nil {
		return action.Plan{}, fmt.Errorf("plan load context is required")
	}
	if store == nil || store.control == nil {
		return action.Plan{}, fmt.Errorf("plan store database is required")
	}
	if err := action.ValidatePlanHash(hash); err != nil {
		return action.Plan{}, err
	}
	executor, err := executorFor(ctx, store.control)
	if err != nil {
		return action.Plan{}, err
	}
	var plan action.Plan
	var payload, resources []byte
	var projected projectedPlan
	err = executor.QueryRowContext(ctx, `
		SELECT
		       CASE WHEN octet_length(plan_hash) <= $2 THEN plan_hash END,
		       CASE WHEN octet_length(action_id) <= $3 THEN action_id END,
		       CASE WHEN octet_length(action_version) <= $4 THEN action_version END,
		       CASE WHEN octet_length(contract_hash) <= $2 THEN contract_hash END,
		       CASE WHEN octet_length(actor_id) <= $5 THEN actor_id END,
		       CASE WHEN octet_length(actor_type) <= $6 THEN actor_type END,
		       CASE WHEN octet_length(channel) <= $7 THEN channel END,
		       CASE WHEN octet_length(scope_kind) <= $8 THEN scope_kind END,
		       CASE WHEN octet_length(scope_id) <= $9 THEN scope_id END,
		       CASE WHEN octet_length(input_hash) <= $2 THEN input_hash END,
		       CASE WHEN octet_length(payload_json) <= $10 THEN payload_json END,
		       impact_rows,
		       CASE WHEN octet_length(impact_resources_json::text) <= $10 THEN impact_resources_json END,
		       CASE WHEN octet_length(snapshot_hash) <= $2 THEN snapshot_hash END,
		       CASE WHEN octet_length(decision_fingerprint) <= $11 THEN decision_fingerprint END,
		       CASE WHEN octet_length(created_at) <= $12 THEN created_at END,
		       CASE WHEN octet_length(expires_at) <= $12 THEN expires_at END,
		       expires_at_unix_nano
		FROM modary_action_plan WHERE plan_hash = $1`, hash, maxStoredHashBytes,
		maxStoredActionIDBytes, maxStoredVersionBytes, maxStoredActorIDBytes,
		maxStoredActorTypeBytes, maxStoredChannelBytes, maxStoredScopeKindBytes,
		maxStoredScopeIDBytes, action.MaxJSONDocumentBytes, maxStoredFingerprintBytes,
		maxStoredTimestampBytes).Scan(
		&projected.planHash, &projected.actionID, &projected.actionVersion, &projected.contractHash,
		&projected.actorID, &projected.actorType, &projected.channel, &projected.scopeKind,
		&projected.scopeID, &projected.inputHash, &payload, &projected.impactRows, &resources,
		&projected.snapshotHash, &projected.fingerprint, &projected.createdAt,
		&projected.expiresAt, &projected.expiresAtUnixNano)
	if errors.Is(err, sql.ErrNoRows) {
		return action.Plan{}, fmt.Errorf("plan %s: %w", hash, actionpersistence.ErrPlanNotFound)
	}
	if err != nil {
		return action.Plan{}, fmt.Errorf("load Action plan: %w", err)
	}
	if payload == nil {
		return action.Plan{}, fmt.Errorf("stored Action plan payload exceeds the JSON resource limit")
	}
	if resources == nil {
		return action.Plan{}, fmt.Errorf("stored Action plan impact resources exceed the JSON resource limit")
	}
	if err := populateProjectedPlan(&plan, projected); err != nil {
		return action.Plan{}, err
	}
	plan.Payload = append(json.RawMessage(nil), payload...)
	if err := decodeStrictJSON(resources, &plan.Impact.Resources); err != nil {
		return action.Plan{}, fmt.Errorf("decode stored plan impact resources: %w", err)
	}
	plan.CreatedAt, err = parseStoredTime("plan creation time", projected.createdAt.String)
	if err != nil {
		return action.Plan{}, err
	}
	plan.ExpiresAt, err = parseStoredTime("plan expiry time", projected.expiresAt.String)
	if err != nil {
		return action.Plan{}, err
	}
	if plan.ExpiresAt.UnixNano() != projected.expiresAtUnixNano.Int64 {
		return action.Plan{}, fmt.Errorf("stored plan expiry representations disagree")
	}
	plan, err = normalizePlan(plan)
	if err != nil {
		return action.Plan{}, fmt.Errorf("stored Action plan is invalid: %w", err)
	}
	if plan.Hash != hash {
		return action.Plan{}, fmt.Errorf("stored Action plan key does not match requested hash")
	}
	return plan, nil
}

func (store *planStore) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	if ctx == nil {
		return 0, fmt.Errorf("plan cleanup context is required")
	}
	if store == nil || store.control == nil {
		return 0, fmt.Errorf("plan store database is required")
	}
	before = before.UTC()
	if err := validateStoredTime("plan cleanup boundary", before); err != nil {
		return 0, err
	}
	executor, err := executorFor(ctx, store.control)
	if err != nil {
		return 0, err
	}
	result, err := executor.ExecContext(ctx, `DELETE FROM modary_action_plan WHERE expires_at_unix_nano <= $1`, before.UnixNano())
	if err != nil {
		return 0, fmt.Errorf("delete expired Action plans: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("inspect expired Action plan cleanup: %w", err)
	}
	return deleted, nil
}

func executorFor(ctx context.Context, control databasecontrol.Control) (database.Executor, error) {
	executor, err := control.Executor(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve PostgreSQL executor: %w", err)
	}
	return executor, nil
}

func decodeStrictJSON(data []byte, target any) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("JSON value must be valid UTF-8")
	}
	if err := action.ValidateJSONDocument(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("JSON contains a trailing value")
		}
		return err
	}
	return nil
}
