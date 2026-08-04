package sqlaudit

import (
	"context"
	"fmt"

	"github.com/iiwish/modary/audit"
)

func (store *hook) List(ctx context.Context, options audit.ListOptions) (audit.Page, error) {
	if ctx == nil {
		return audit.Page{}, ErrContextRequired
	}
	if store == nil || store.control == nil {
		return audit.Page{}, fmt.Errorf("SQL Audit Reader is unavailable")
	}
	normalized, err := audit.NormalizeListOptions(options)
	if err != nil {
		return audit.Page{}, err
	}
	executor, err := store.control.Executor(ctx)
	if err != nil {
		return audit.Page{}, fmt.Errorf("SQL Audit executor is unavailable: %w", err)
	}
	query, arguments := auditInspectionQuery(normalized)
	rows, err := executor.QueryContext(ctx, query, arguments...)
	if err != nil {
		return audit.Page{}, fmt.Errorf("list audit metadata: %w", err)
	}
	defer rows.Close()
	items := make([]audit.Summary, 0, normalized.Limit+1)
	for rows.Next() {
		var item audit.Summary
		var started, finished string
		if err := rows.Scan(
			&item.ID, &item.RequestID, &item.ActorID, &item.ActorType, &item.Channel,
			&item.ActionID, &item.Decision, &item.ErrorCode, &item.Scope.Kind, &item.Scope.ID,
			&started, &finished,
		); err != nil {
			return audit.Page{}, fmt.Errorf("read audit metadata: %w", err)
		}
		item.StartedAt, err = parseTimestamp(started)
		if err != nil {
			return audit.Page{}, fmt.Errorf("decode audit start time: %w", err)
		}
		item.FinishedAt, err = parseTimestamp(finished)
		if err != nil {
			return audit.Page{}, fmt.Errorf("decode audit finish time: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return audit.Page{}, fmt.Errorf("finish audit metadata: %w", err)
	}
	page := audit.Page{Events: items}
	if len(items) > normalized.Limit {
		page.Events = items[:normalized.Limit]
		page.NextBeforeID = page.Events[len(page.Events)-1].ID
	}
	return page, nil
}

func auditInspectionQuery(options audit.ListOptions) (string, []any) {
	query := `SELECT event_id, request_id, actor_id, actor_type, channel, action_id,
		decision, error_code, scope_kind, scope_id, started_at, finished_at
		FROM modary_audit_event
		WHERE scope_kind = $1 AND scope_id = $2`
	arguments := []any{options.Scope.Kind, options.Scope.ID}
	if options.BeforeID != 0 {
		query += ` AND event_id < $3`
		arguments = append(arguments, options.BeforeID)
	}
	arguments = append(arguments, options.Limit+1)
	query += fmt.Sprintf(` ORDER BY event_id DESC LIMIT $%d`, len(arguments))
	return query, arguments
}

var _ audit.Reader = (*hook)(nil)
