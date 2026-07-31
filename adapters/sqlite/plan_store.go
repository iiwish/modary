package sqlite

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
	"github.com/iiwish/modary/internal/actionpersistence"
	"github.com/iiwish/modary/internal/databasecontrol"
)

type planStore struct {
	control databasecontrol.Control
}

type projectedPlan struct {
	planHash, actionID, actionVersion, contractHash sql.NullString
	actorID, actorType, channel                     sql.NullString
	scopeKind, scopeID, inputHash                   sql.NullString
	impactRows                                      sql.NullInt64
	snapshotHash, fingerprint                       sql.NullString
	createdAt, expiresAt                            sql.NullString
	expiresAtUnixNano                               sql.NullInt64
}

// Save persists one validated Action plan.
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
	resources, err := json.Marshal(plan.Impact.Resources)
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
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(plan_hash) DO NOTHING`,
		plan.Hash, plan.ActionID, plan.ActionVersion, plan.ContractHash, plan.ActorID, plan.ActorType,
		plan.Channel, plan.Scope.Kind, plan.Scope.ID, plan.InputHash, []byte(plan.Payload), plan.Impact.Rows,
		string(resources), plan.SnapshotHash, plan.DecisionFingerprint,
		plan.CreatedAt.Format(time.RFC3339Nano), plan.ExpiresAt.Format(time.RFC3339Nano), plan.ExpiresAt.UnixNano())
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

// Get loads one plan by its provenance hash.
func (store *planStore) Get(ctx context.Context, hash string) (action.Plan, error) {
	if ctx == nil {
		return action.Plan{}, fmt.Errorf("plan lookup context is required")
	}
	if store == nil || store.control == nil {
		return action.Plan{}, fmt.Errorf("plan store database is required")
	}
	if err := action.ValidatePlanHash(hash); err != nil {
		return action.Plan{}, err
	}
	var plan action.Plan
	var payload, resources []byte
	var projected projectedPlan
	executor, err := executorFor(ctx, store.control)
	if err != nil {
		return action.Plan{}, err
	}
	arguments := appendStoredProjectionArguments(nil, sql.Named("lookup_hash", hash))
	err = executor.QueryRowContext(ctx, `
		SELECT
		       CASE WHEN typeof(plan_hash) = 'text' AND length(CAST(plan_hash AS BLOB)) <= @hash_bytes THEN plan_hash END,
		       CASE WHEN typeof(action_id) = 'text' AND length(CAST(action_id AS BLOB)) <= @action_id_bytes THEN action_id END,
		       CASE WHEN typeof(action_version) = 'text' AND length(CAST(action_version AS BLOB)) <= @version_bytes THEN action_version END,
		       CASE WHEN typeof(contract_hash) = 'text' AND length(CAST(contract_hash AS BLOB)) <= @hash_bytes THEN contract_hash END,
		       CASE WHEN typeof(actor_id) = 'text' AND length(CAST(actor_id AS BLOB)) <= @actor_id_bytes THEN actor_id END,
		       CASE WHEN typeof(actor_type) = 'text' AND length(CAST(actor_type AS BLOB)) <= @actor_type_bytes THEN actor_type END,
		       CASE WHEN typeof(channel) = 'text' AND length(CAST(channel AS BLOB)) <= @channel_bytes THEN channel END,
		       CASE WHEN typeof(scope_kind) = 'text' AND length(CAST(scope_kind AS BLOB)) <= @scope_kind_bytes THEN scope_kind END,
		       CASE WHEN typeof(scope_id) = 'text' AND length(CAST(scope_id AS BLOB)) <= @scope_id_bytes THEN scope_id END,
		       CASE WHEN typeof(input_hash) = 'text' AND length(CAST(input_hash AS BLOB)) <= @hash_bytes THEN input_hash END,
		       CASE WHEN typeof(payload_json) = 'blob' AND length(CAST(payload_json AS BLOB)) <= @json_bytes THEN payload_json END,
		       CASE WHEN typeof(impact_rows) = 'integer' THEN impact_rows END,
		       CASE WHEN typeof(impact_resources_json) = 'text' AND length(CAST(impact_resources_json AS BLOB)) <= @json_bytes THEN impact_resources_json END,
		       CASE WHEN typeof(snapshot_hash) = 'text' AND length(CAST(snapshot_hash AS BLOB)) <= @hash_bytes THEN snapshot_hash END,
		       CASE WHEN typeof(decision_fingerprint) = 'text' AND length(CAST(decision_fingerprint AS BLOB)) <= @fingerprint_bytes THEN decision_fingerprint END,
		       CASE WHEN typeof(created_at) = 'text' AND length(CAST(created_at AS BLOB)) <= @timestamp_bytes THEN created_at END,
		       CASE WHEN typeof(expires_at) = 'text' AND length(CAST(expires_at AS BLOB)) <= @timestamp_bytes THEN expires_at END,
		       CASE WHEN typeof(expires_at_unix_nano) = 'integer' THEN expires_at_unix_nano END
		FROM modary_action_plan WHERE plan_hash = @lookup_hash`, arguments...).Scan(
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
	plan.Payload = append([]byte(nil), payload...)
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

func populateProjectedPlan(plan *action.Plan, value projectedPlan) error {
	var err error
	if plan.Hash, err = requireProjectedText(value.planHash, "Action plan hash"); err != nil {
		return err
	}
	if plan.ActionID, err = requireProjectedText(value.actionID, "Action plan action id"); err != nil {
		return err
	}
	if plan.ActionVersion, err = requireProjectedText(value.actionVersion, "Action plan action version"); err != nil {
		return err
	}
	if plan.ContractHash, err = requireProjectedText(value.contractHash, "Action plan contract hash"); err != nil {
		return err
	}
	if plan.ActorID, err = requireProjectedText(value.actorID, "Action plan actor id"); err != nil {
		return err
	}
	if plan.ActorType, err = requireProjectedText(value.actorType, "Action plan actor type"); err != nil {
		return err
	}
	channel, err := requireProjectedText(value.channel, "Action plan channel")
	if err != nil {
		return err
	}
	plan.Channel = action.Channel(channel)
	if plan.Scope.Kind, err = requireProjectedText(value.scopeKind, "Action plan scope kind"); err != nil {
		return err
	}
	if plan.Scope.ID, err = requireProjectedText(value.scopeID, "Action plan scope id"); err != nil {
		return err
	}
	if plan.InputHash, err = requireProjectedText(value.inputHash, "Action plan input hash"); err != nil {
		return err
	}
	impactRows, err := requireProjectedInteger(value.impactRows, "Action plan impact rows")
	if err != nil {
		return err
	}
	plan.Impact.Rows = int(impactRows)
	if int64(plan.Impact.Rows) != impactRows {
		return fmt.Errorf("stored Action plan impact rows exceed the platform integer range")
	}
	if plan.SnapshotHash, err = requireProjectedText(value.snapshotHash, "Action plan snapshot hash"); err != nil {
		return err
	}
	if plan.DecisionFingerprint, err = requireProjectedText(value.fingerprint, "Action plan decision fingerprint"); err != nil {
		return err
	}
	if _, err = requireProjectedText(value.createdAt, "Action plan creation time"); err != nil {
		return err
	}
	if _, err = requireProjectedText(value.expiresAt, "Action plan expiry time"); err != nil {
		return err
	}
	if _, err = requireProjectedInteger(value.expiresAtUnixNano, "Action plan expiry epoch"); err != nil {
		return err
	}
	return nil
}

// DeleteExpired removes plans at or before the supplied boundary.
func (store *planStore) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	if ctx == nil {
		return 0, fmt.Errorf("plan cleanup context is required")
	}
	if store == nil || store.control == nil {
		return 0, fmt.Errorf("plan store database is required")
	}
	if err := validateStoredTime("plan cleanup boundary", before.UTC()); err != nil {
		return 0, err
	}
	executor, err := executorFor(ctx, store.control)
	if err != nil {
		return 0, err
	}
	result, err := executor.ExecContext(ctx,
		`DELETE FROM modary_action_plan WHERE expires_at_unix_nano <= ?`, before.UTC().UnixNano())
	if err != nil {
		return 0, fmt.Errorf("delete expired Action plans: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("inspect expired Action plan cleanup: %w", err)
	}
	return deleted, nil
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
