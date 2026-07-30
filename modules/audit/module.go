package audit_module

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"time"

	"modary/core/action"
	coreaudit "modary/core/audit"
	"modary/core/database"
	"modary/core/module"
)

//go:embed module.yaml
var manifestData []byte

//go:embed migrations/sqlite/*.sql
var migrationFiles embed.FS

func Module() module.Registration {
	return module.Registration{Manifest: module.MustParseManifest(manifestData), Install: install}
}

func install(ctx context.Context, host *module.Host) error {
	db, err := module.ServiceAs[*sql.DB](host, module.ServiceDatabase)
	if err != nil {
		return err
	}
	registry, err := module.ServiceAs[*action.Registry](host, module.ServiceActionRegistry)
	if err != nil {
		return err
	}
	sub, err := fs.Sub(migrationFiles, "migrations/sqlite")
	if err != nil {
		return err
	}
	if err := database.ApplyMigrations(ctx, db, "audit", sub); err != nil {
		return err
	}
	store := &Store{db: db}
	if err := host.Provide(module.ServiceAuditHook, coreaudit.Hook(store)); err != nil {
		return err
	}
	if err := host.Provide(module.ServiceAuditStore, store); err != nil {
		return err
	}
	return registry.Register("audit", auditQueryDescriptor(), &queryHandler{store: store})
}

type Store struct{ db *sql.DB }

func (s *Store) Record(ctx context.Context, event coreaudit.Event) error {
	data := make([]byte, 8)
	if _, err := rand.Read(data); err != nil {
		return err
	}
	auditID := fmt.Sprintf("aud_%d_%s", time.Now().UTC().UnixNano(), hex.EncodeToString(data))
	_, err := database.ExecutorFor(ctx, s.db).ExecContext(ctx, `
		INSERT INTO modary_audit_log
		(audit_id, request_id, actor_id, actor_type, channel, action_id, workspace_id,
		 input_hash, plan_hash, decision, result_summary, error_code, reason, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		auditID, event.RequestID, event.ActorID, event.ActorType, event.Channel, event.ActionID,
		event.WorkspaceID, event.InputHash, event.PlanHash, event.Decision, event.ResultSummary,
		event.ErrorCode, event.Reason, event.StartedAt.Format(time.RFC3339Nano), event.FinishedAt.Format(time.RFC3339Nano))
	return err
}

type Query struct {
	ActionID string `json:"action_id"`
	ActorID  string `json:"actor_id"`
	Decision string `json:"decision"`
	Limit    int    `json:"limit"`
}

func (s *Store) Query(ctx context.Context, workspaceID string, query Query) ([]coreaudit.Event, error) {
	limit := query.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := database.ExecutorFor(ctx, s.db).QueryContext(ctx, `
		SELECT request_id, actor_id, actor_type, channel, action_id, workspace_id,
		       input_hash, plan_hash, decision, result_summary, error_code, reason, started_at, finished_at
		FROM modary_audit_log
		WHERE workspace_id = ?
		  AND (? = '' OR action_id = ?)
		  AND (? = '' OR actor_id = ?)
		  AND (? = '' OR decision = ?)
		ORDER BY finished_at DESC
		LIMIT ?`, workspaceID, query.ActionID, query.ActionID, query.ActorID, query.ActorID, query.Decision, query.Decision, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]coreaudit.Event, 0)
	for rows.Next() {
		var event coreaudit.Event
		var started, finished string
		if err := rows.Scan(&event.RequestID, &event.ActorID, &event.ActorType, &event.Channel, &event.ActionID,
			&event.WorkspaceID, &event.InputHash, &event.PlanHash, &event.Decision, &event.ResultSummary,
			&event.ErrorCode, &event.Reason, &started, &finished); err != nil {
			return nil, err
		}
		event.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
		event.FinishedAt, _ = time.Parse(time.RFC3339Nano, finished)
		events = append(events, event)
	}
	return events, rows.Err()
}

type queryHandler struct{ store *Store }

func (h *queryHandler) Plan(_ context.Context, request action.Request) (action.PlanData, error) {
	var query Query
	if err := json.Unmarshal(request.Input, &query); err != nil {
		return action.PlanData{}, action.NewError(action.CodeValidationFailed, "invalid audit query")
	}
	payload, _ := json.Marshal(query)
	return action.PlanData{Payload: payload, Summary: json.RawMessage(`{"operation":"query audit events"}`)}, nil
}

func (h *queryHandler) Execute(ctx context.Context, plan action.Plan) (action.Result, error) {
	var query Query
	if err := json.Unmarshal(plan.Payload, &query); err != nil {
		return action.Result{}, err
	}
	events, err := h.store.Query(ctx, plan.WorkspaceID, query)
	if err != nil {
		return action.Result{}, err
	}
	data, _ := json.Marshal(map[string]any{"events": events})
	return action.Result{Data: data, Summary: fmt.Sprintf("returned %d audit events", len(events))}, nil
}

func auditQueryDescriptor() action.Descriptor {
	return action.Descriptor{
		ID:          "audit.query",
		Title:       "Query audit events",
		Description: "Returns governed action audit events for the current workspace.",
		InputSchema: action.ObjectSchema(
			`"action_id":{"type":"string"},"actor_id":{"type":"string"},"decision":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":200}`,
		),
		OutputSchema: action.ObjectSchema(`"events":{"type":"array"}`, "events"),
		Permission:   "audit.read",
		Preview:      action.PreviewNone,
		AuditLevel:   action.AuditMetadata,
		Channels:     []string{"http", "cli"},
	}
}
