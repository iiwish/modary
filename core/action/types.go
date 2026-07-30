package action

import (
	"context"
	"encoding/json"
	"time"

	"modary/core/authz"
	"modary/core/identity"
)

type PreviewPolicy string

const (
	PreviewNone     PreviewPolicy = "none"
	PreviewOptional PreviewPolicy = "optional"
	PreviewRequired PreviewPolicy = "required"
)

type AuditLevel string

const (
	AuditMetadata AuditLevel = "metadata"
	AuditDetailed AuditLevel = "detailed"
)

type Descriptor struct {
	ID                  string          `json:"id"`
	Title               string          `json:"title"`
	Description         string          `json:"description,omitempty"`
	InputSchema         json.RawMessage `json:"input_schema"`
	OutputSchema        json.RawMessage `json:"output_schema"`
	Permission          string          `json:"permission"`
	Preview             PreviewPolicy   `json:"preview"`
	AuditLevel          AuditLevel      `json:"audit_level"`
	Channels            []string        `json:"channels,omitempty"`
	RequiresIdempotency bool            `json:"requires_idempotency"`
}

type Request struct {
	RequestID      string          `json:"request_id"`
	Actor          identity.Actor  `json:"actor"`
	Channel        string          `json:"channel"`
	ActionID       string          `json:"action_id"`
	WorkspaceID    string          `json:"workspace_id"`
	Input          json.RawMessage `json:"input"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	PlanHash       string          `json:"plan_hash,omitempty"`
}

type PlanData struct {
	Payload      json.RawMessage `json:"payload"`
	Summary      json.RawMessage `json:"summary"`
	Impact       authz.Impact    `json:"impact"`
	SnapshotHash string          `json:"snapshot_hash,omitempty"`
}

type Plan struct {
	Hash                string          `json:"plan_hash"`
	ActionID            string          `json:"action_id"`
	ActorID             string          `json:"actor_id"`
	WorkspaceID         string          `json:"workspace_id"`
	Input               json.RawMessage `json:"input"`
	InputHash           string          `json:"input_hash"`
	Payload             json.RawMessage `json:"payload"`
	Summary             json.RawMessage `json:"summary"`
	Impact              authz.Impact    `json:"impact"`
	SnapshotHash        string          `json:"snapshot_hash,omitempty"`
	DecisionFingerprint string          `json:"decision_fingerprint"`
	CreatedAt           time.Time       `json:"created_at"`
	ExpiresAt           time.Time       `json:"expires_at"`
}

type Preview struct {
	PlanHash  string          `json:"plan_hash"`
	Summary   json.RawMessage `json:"summary"`
	Impact    authz.Impact    `json:"impact"`
	ExpiresAt time.Time       `json:"expires_at"`
}

type Result struct {
	Data    json.RawMessage `json:"data"`
	Summary string          `json:"summary,omitempty"`
}

type Handler interface {
	Plan(context.Context, Request) (PlanData, error)
	Execute(context.Context, Plan) (Result, error)
}

type PlanStore interface {
	Save(context.Context, Plan) error
	Get(context.Context, string) (Plan, error)
}

type IdempotencyRecord struct {
	WorkspaceID string
	ActorID     string
	ActionID    string
	Key         string
	InputHash   string
	Status      string
	Result      Result
}

type IdempotencyStore interface {
	Reserve(context.Context, IdempotencyRecord) (existing *IdempotencyRecord, err error)
	Complete(context.Context, IdempotencyRecord) error
}

type TransactionManager interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}
