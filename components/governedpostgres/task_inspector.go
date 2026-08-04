package governedpostgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/iiwish/modary/task"
)

func (service *taskService) List(ctx context.Context, options task.ListOptions) (task.Page, error) {
	if ctx == nil {
		return task.Page{}, fmt.Errorf("task inspection context is required")
	}
	if service == nil || service.db == nil {
		return task.Page{}, task.ErrUnavailable
	}
	service.mu.Lock()
	closed := service.closed
	service.mu.Unlock()
	if closed {
		return task.Page{}, task.ErrUnavailable
	}
	normalized, err := task.NormalizeListOptions(options)
	if err != nil {
		return task.Page{}, err
	}
	riverFilter, err := riverState(normalized.State)
	if err != nil {
		return task.Page{}, err
	}
	query, arguments := taskInspectionQuery(service.queueSchema, normalized, riverFilter)
	rows, err := service.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return task.Page{}, fmt.Errorf("list durable task metadata: %w", err)
	}
	defer rows.Close()
	items := make([]task.Summary, 0, normalized.Limit+1)
	for rows.Next() {
		var item task.Summary
		var finalized sql.NullTime
		var riverJobState string
		if err := rows.Scan(
			&item.ID, &item.Kind, &item.Queue, &riverJobState, &item.Attempt, &item.MaxAttempts,
			&item.ScheduledAt, &item.CreatedAt, &finalized,
		); err != nil {
			return task.Page{}, fmt.Errorf("read durable task metadata: %w", err)
		}
		item.State, err = taskStateFromRiver(riverJobState)
		if err != nil {
			return task.Page{}, err
		}
		item.ScheduledAt = item.ScheduledAt.UTC()
		item.CreatedAt = item.CreatedAt.UTC()
		if finalized.Valid {
			value := finalized.Time.UTC()
			item.FinalizedAt = &value
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return task.Page{}, fmt.Errorf("finish durable task metadata: %w", err)
	}
	page := task.Page{Tasks: items}
	if len(items) > normalized.Limit {
		page.Tasks = items[:normalized.Limit]
		page.NextBeforeID = page.Tasks[len(page.Tasks)-1].ID
	}
	return page, nil
}

func taskInspectionQuery(schema string, options task.ListOptions, riverFilter string) (string, []any) {
	conditions := make([]string, 0, 3)
	arguments := make([]any, 0, 4)
	bind := func(value any) string {
		arguments = append(arguments, value)
		return fmt.Sprintf("$%d", len(arguments))
	}
	if options.BeforeID != 0 {
		conditions = append(conditions, "id < "+bind(options.BeforeID))
	}
	if options.Queue != "" {
		conditions = append(conditions, "queue = "+bind(options.Queue))
	}
	if riverFilter != "" {
		enumType := quoteIdentifier(schema) + `.river_job_state`
		conditions = append(conditions, "state = "+bind(riverFilter)+"::"+enumType)
	}
	query := `SELECT id, kind, queue, state, attempt, max_attempts,
		scheduled_at, created_at, finalized_at
		FROM ` + quoteIdentifier(schema) + `.river_job`
	if len(conditions) != 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY id DESC LIMIT " + bind(options.Limit+1)
	return query, arguments
}

func riverState(state task.State) (string, error) {
	switch state {
	case "":
		return "", nil
	case task.StateQueued:
		return "available", nil
	case task.StatePending:
		return "pending", nil
	case task.StateScheduled:
		return "scheduled", nil
	case task.StateRunning:
		return "running", nil
	case task.StateRetrying:
		return "retryable", nil
	case task.StateSucceeded:
		return "completed", nil
	case task.StateFailed:
		return "discarded", nil
	case task.StateCancelled:
		return "cancelled", nil
	default:
		return "", fmt.Errorf("task state %q is unsupported by governed PostgreSQL", state)
	}
}

func taskStateFromRiver(state string) (task.State, error) {
	switch state {
	case "available":
		return task.StateQueued, nil
	case "pending":
		return task.StatePending, nil
	case "scheduled":
		return task.StateScheduled, nil
	case "running":
		return task.StateRunning, nil
	case "retryable":
		return task.StateRetrying, nil
	case "completed":
		return task.StateSucceeded, nil
	case "discarded":
		return task.StateFailed, nil
	case "cancelled":
		return task.StateCancelled, nil
	default:
		return "", fmt.Errorf("map River task state %q into public contract", state)
	}
}

var _ task.Inspector = (*taskService)(nil)
