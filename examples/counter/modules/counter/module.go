// Package counter implements the consumer-owned Counter feature used to prove
// Modary's alpha external-consumer contract.
package counter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"time"

	"example.com/modary-counter-consumer/modules/clockcontract"
	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/scope"
	"github.com/iiwish/modary/task"
)

const (
	// ModuleID is the consumer-owned migration and lifecycle identity.
	ModuleID = "counter"
	// ActionID is the governed Counter mutation.
	ActionID = "counter.increment"
	// Permission is the exact RBAC grant required by ActionID.
	Permission = ActionID
	// IncrementedTaskKind identifies the durable event emitted with a committed
	// Counter increment.
	IncrementedTaskKind = "counter.incremented"
	// ErrorVersionConflict is the consumer-owned public code for an optimistic
	// version that does not match current Counter state.
	ErrorVersionConflict = "COUNTER.VERSION_CONFLICT"
)

//go:embed migrations/postgres/*.sql
var migrationFiles embed.FS

type incrementInput struct {
	Amount          int64 `json:"amount"`
	ExpectedVersion int64 `json:"expected_version"`
}

type incrementPlan struct {
	Amount         int64 `json:"amount"`
	CurrentValue   int64 `json:"current_value"`
	CurrentVersion int64 `json:"current_version"`
	NextValue      int64 `json:"next_value"`
	NextVersion    int64 `json:"next_version"`
}

type incrementPreview struct {
	CurrentValue   int64 `json:"current_value"`
	CurrentVersion int64 `json:"current_version"`
	NextValue      int64 `json:"next_value"`
	NextVersion    int64 `json:"next_version"`
}

// State is the durable Counter value and optimistic version for one execution
// scope. A zero State means no row has been created yet.
type State struct {
	Value   int64 `json:"value"`
	Version int64 `json:"version"`
}

// Module returns a pure feature Registration. The Host applies the declared
// migrations before constructing the Action handler.
func Module() (module.Registration, error) {
	migrations, err := fs.Sub(migrationFiles, "migrations/postgres")
	if err != nil {
		return module.Registration{}, fmt.Errorf("prepare Counter migrations: %w", err)
	}
	return module.Registration{
		Definition: module.Definition{
			Manifest: module.Manifest{
				SchemaVersion: module.SchemaVersion,
				ID:            ModuleID,
				Version:       "0.1.0",
				Type:          module.ModuleTypeFeature,
				Requires: []module.Capability{
					module.CapabilityDatabase,
					module.CapabilityTasks,
					clockcontract.Capability,
				},
			},
			Migrations: []module.MigrationSource{{Driver: "postgres", Files: migrations}},
			Actions: []module.ActionBinding{{
				Descriptor: descriptor(),
				NewHandler: func(_ context.Context, services module.Resolver) (action.Handler, error) {
					db, err := module.Resolve(services, module.ActionDatabase())
					if err != nil {
						return nil, fmt.Errorf("resolve Counter database: %w", err)
					}
					clock, err := module.Resolve(services, clockcontract.Key)
					if err != nil {
						return nil, fmt.Errorf("resolve Counter clock: %w", err)
					}
					tasks, err := module.Resolve(services, module.Tasks())
					if err != nil {
						return nil, fmt.Errorf("resolve Counter tasks: %w", err)
					}
					return &incrementHandler{db: db, clock: clock, tasks: tasks}, nil
				},
			}},
		},
	}, nil
}

func descriptor() action.Descriptor {
	integer := func(description string, options ...action.SchemaOption) action.Schema {
		return action.Integer(append([]action.SchemaOption{action.Description(description)}, options...)...)
	}
	return action.Descriptor{
		ID:          ActionID,
		Version:     "1.0.0",
		Title:       "Increment counter",
		Description: "Atomically increments the Counter in one execution scope.",
		InputSchema: action.Object(map[string]action.Field{
			"amount": action.RequiredField(integer(
				"Positive increment amount.",
				action.Minimum(1),
				action.Maximum(1_000_000),
			)),
			"expected_version": action.RequiredField(integer(
				"Current optimistic version; zero creates the Counter.",
				action.Minimum(0),
			)),
		}).JSON(),
		PreviewSchema: action.Object(map[string]action.Field{
			"current_value":   action.RequiredField(integer("Value observed by Preview.")),
			"current_version": action.RequiredField(integer("Version observed by Preview.", action.Minimum(0))),
			"next_value":      action.RequiredField(integer("Value after execution.")),
			"next_version":    action.RequiredField(integer("Version after execution.", action.Minimum(1))),
		}).JSON(),
		OutputSchema: action.Object(map[string]action.Field{
			"value":   action.RequiredField(integer("Committed Counter value.")),
			"version": action.RequiredField(integer("Committed optimistic version.", action.Minimum(1))),
		}).JSON(),
		Permission:          Permission,
		Preview:             action.PreviewRequired,
		AuditLevel:          action.AuditDetailed,
		Channels:            []action.Channel{action.ChannelCLI, action.ChannelHTTP, action.ChannelMCP, "test"},
		Errors:              []action.ErrorSpec{{Code: ErrorVersionConflict, Kind: action.ErrorKindConflict}},
		RequiresIdempotency: true,
	}
}

type incrementHandler struct {
	db    database.Access
	clock clockcontract.Clock
	tasks task.Service
}

func (handler *incrementHandler) Plan(ctx context.Context, request action.Request) (action.PlanData, error) {
	var input incrementInput
	if err := decodeStrictJSON(request.Input, &input); err != nil {
		return action.PlanData{}, action.NewError(action.CodeValidationFailed, "Counter input is invalid")
	}
	current, err := readState(ctx, handler.db, request.Scope)
	if err != nil {
		return action.PlanData{}, fmt.Errorf("read Counter state: %w", err)
	}
	if input.ExpectedVersion != current.Version {
		return action.PlanData{}, action.NewError(
			ErrorVersionConflict,
			fmt.Sprintf("Counter version is %d, expected %d", current.Version, input.ExpectedVersion),
		)
	}
	if input.Amount <= 0 || current.Value > math.MaxInt64-input.Amount {
		return action.PlanData{}, action.NewError(action.CodeLimitExceeded, "Counter increment exceeds its supported range")
	}
	planned := incrementPlan{
		Amount:         input.Amount,
		CurrentValue:   current.Value,
		CurrentVersion: current.Version,
		NextValue:      current.Value + input.Amount,
		NextVersion:    current.Version + 1,
	}
	payload, err := json.Marshal(planned)
	if err != nil {
		return action.PlanData{}, fmt.Errorf("encode Counter plan: %w", err)
	}
	summary, err := json.Marshal(incrementPreview{
		CurrentValue:   planned.CurrentValue,
		CurrentVersion: planned.CurrentVersion,
		NextValue:      planned.NextValue,
		NextVersion:    planned.NextVersion,
	})
	if err != nil {
		return action.PlanData{}, fmt.Errorf("encode Counter preview: %w", err)
	}
	return action.PlanData{
		Payload:      payload,
		Summary:      summary,
		Impact:       authz.Impact{Rows: 1, Resources: []string{resourceID(request.Scope)}},
		SnapshotHash: snapshotHash(request.Scope, current),
	}, nil
}

func (handler *incrementHandler) Execute(ctx context.Context, plan action.Plan) (action.Result, error) {
	var payload incrementPlan
	if err := decodeStrictJSON(plan.Payload, &payload); err != nil {
		return action.Result{}, fmt.Errorf("decode Counter plan: %w", err)
	}
	if payload.Amount <= 0 ||
		payload.CurrentVersion < 0 ||
		payload.NextVersion != payload.CurrentVersion+1 ||
		payload.CurrentValue > math.MaxInt64-payload.Amount ||
		payload.NextValue != payload.CurrentValue+payload.Amount {
		return action.Result{}, action.NewError(action.CodePlanStale, "Counter plan payload is inconsistent")
	}
	current, err := readState(ctx, handler.db, plan.Scope)
	if err != nil {
		return action.Result{}, fmt.Errorf("recheck Counter state: %w", err)
	}
	if current.Value != payload.CurrentValue ||
		current.Version != payload.CurrentVersion ||
		snapshotHash(plan.Scope, current) != plan.SnapshotHash {
		return action.Result{}, action.NewError(action.CodePlanStale, "Counter changed after Preview")
	}
	result, err := handler.db.ExecContext(ctx, `
		INSERT INTO consumer_counter
			(scope_kind, scope_id, value, version, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT(scope_kind, scope_id) DO UPDATE SET
			value = excluded.value,
			version = excluded.version,
			updated_at = excluded.updated_at
		WHERE consumer_counter.value = $6 AND consumer_counter.version = $7`,
		plan.Scope.Kind,
		plan.Scope.ID,
		payload.NextValue,
		payload.NextVersion,
		handler.clock.Now().UTC().Format(time.RFC3339Nano),
		payload.CurrentValue,
		payload.CurrentVersion,
	)
	if err != nil {
		return action.Result{}, fmt.Errorf("commit Counter increment: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return action.Result{}, fmt.Errorf("inspect Counter increment: %w", err)
	}
	if affected != 1 {
		return action.Result{}, action.NewError(action.CodePlanStale, "Counter changed while the increment was committed")
	}
	output, err := json.Marshal(State{Value: payload.NextValue, Version: payload.NextVersion})
	if err != nil {
		return action.Result{}, fmt.Errorf("encode Counter result: %w", err)
	}
	taskPayload, err := json.Marshal(struct {
		Scope   scope.Execution `json:"scope"`
		Value   int64           `json:"value"`
		Version int64           `json:"version"`
	}{Scope: plan.Scope, Value: payload.NextValue, Version: payload.NextVersion})
	if err != nil {
		return action.Result{}, fmt.Errorf("encode Counter task: %w", err)
	}
	taskIdentity := sha256.Sum256(taskPayload)
	if _, err := handler.tasks.Enqueue(ctx, task.Request{
		Kind:      IncrementedTaskKind,
		Payload:   taskPayload,
		UniqueKey: "sha256:" + hex.EncodeToString(taskIdentity[:]),
	}); err != nil {
		return action.Result{}, fmt.Errorf("enqueue Counter task: %w", err)
	}
	return action.Result{
		Data:       output,
		Summary:    fmt.Sprintf("Counter advanced to %d at version %d", payload.NextValue, payload.NextVersion),
		References: []audit.Reference{{Kind: "counter", ID: plan.Scope.ID}},
	}, nil
}

func decodeStrictJSON(data json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func readState(ctx context.Context, db database.Access, execution scope.Execution) (State, error) {
	var state State
	err := db.QueryRowContext(ctx, `
		SELECT value, version
		FROM consumer_counter
		WHERE scope_kind = $1 AND scope_id = $2`,
		execution.Kind,
		execution.ID,
	).Scan(&state.Value, &state.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	if state.Version < 1 {
		return State{}, fmt.Errorf("stored Counter version %d is invalid", state.Version)
	}
	return state, nil
}

func resourceID(execution scope.Execution) string {
	return "counter:" + execution.String()
}

func snapshotHash(execution scope.Execution, state State) string {
	material, err := json.Marshal(struct {
		Scope   scope.Execution `json:"scope"`
		Value   int64           `json:"value"`
		Version int64           `json:"version"`
	}{Scope: execution, Value: state.Value, Version: state.Version})
	if err != nil {
		panic(fmt.Sprintf("encode Counter snapshot: %v", err))
	}
	hash := sha256.Sum256(material)
	return "sha256:" + hex.EncodeToString(hash[:])
}

var _ action.Handler = (*incrementHandler)(nil)
