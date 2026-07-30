package audit

import (
	"context"
	"time"
)

type Event struct {
	RequestID     string    `json:"request_id"`
	ActorID       string    `json:"actor_id,omitempty"`
	ActorType     string    `json:"actor_type,omitempty"`
	Channel       string    `json:"channel,omitempty"`
	ActionID      string    `json:"action_id"`
	WorkspaceID   string    `json:"workspace_id,omitempty"`
	InputHash     string    `json:"input_hash,omitempty"`
	PlanHash      string    `json:"plan_hash,omitempty"`
	Decision      string    `json:"decision"`
	ResultSummary string    `json:"result_summary,omitempty"`
	ErrorCode     string    `json:"error_code,omitempty"`
	Reason        string    `json:"reason,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
}

type Hook interface {
	Record(context.Context, Event) error
}

type NopHook struct{}

func (NopHook) Record(context.Context, Event) error { return nil }
