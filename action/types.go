package action

import (
	"context"
	"encoding/json"
	"time"

	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/scope"
)

// Channel identifies an execution surface. The standard framework transports
// use the constants below; consumers may define additional non-empty channels.
type Channel string

const (
	// ChannelCLI identifies the command-line execution surface.
	ChannelCLI Channel = "cli"
	// ChannelHTTP identifies the HTTP execution surface.
	ChannelHTTP Channel = "http"
	// ChannelMCP identifies the Model Context Protocol execution surface.
	ChannelMCP Channel = "mcp"
)

// PreviewPolicy controls whether callers may or must provide a Preview plan for execution.
type PreviewPolicy string

const (
	// PreviewNone disables caller-visible Preview and rejects supplied plan hashes.
	PreviewNone PreviewPolicy = "none"
	// PreviewOptional permits execution with either a prior Preview plan or internal planning.
	PreviewOptional PreviewPolicy = "optional"
	// PreviewRequired requires execution to bind to a prior Preview plan.
	PreviewRequired PreviewPolicy = "required"
)

// AuditLevel controls how much normalized Action detail an audit event retains.
type AuditLevel string

const (
	// AuditMetadata records bounded decision metadata without impact or result details.
	AuditMetadata AuditLevel = "metadata"
	// AuditDetailed records bounded decision metadata, impact, summary, and references.
	AuditDetailed AuditLevel = "detailed"
)

// Descriptor is the complete static governance, schema, and public error
// contract for an Action. Errors declares only consumer-owned codes; framework
// codes and their kinds are defined by BuiltinErrorKind.
type Descriptor struct {
	ID                  string          `json:"id"`
	Version             string          `json:"version"`
	Title               string          `json:"title"`
	Description         string          `json:"description,omitempty"`
	InputSchema         json.RawMessage `json:"input_schema"`
	PreviewSchema       json.RawMessage `json:"preview_schema,omitempty"`
	OutputSchema        json.RawMessage `json:"output_schema"`
	Permission          string          `json:"permission"`
	Preview             PreviewPolicy   `json:"preview"`
	AuditLevel          AuditLevel      `json:"audit_level"`
	Channels            []Channel       `json:"channels,omitempty"`
	Errors              []ErrorSpec     `json:"errors,omitempty"`
	RequiresIdempotency bool            `json:"requires_idempotency"`
}

// Request is the channel-independent execution envelope submitted to a Runtime.
type Request struct {
	RequestID      string          `json:"request_id"`
	Actor          identity.Actor  `json:"actor"`
	Channel        Channel         `json:"channel"`
	ActionID       string          `json:"action_id"`
	Scope          scope.Execution `json:"scope"`
	Input          json.RawMessage `json:"input"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	PlanHash       string          `json:"plan_hash,omitempty"`
}

// PlanData is the Handler-produced execution payload, preview summary, impact,
// and optional optimistic-concurrency snapshot used to create a Plan.
// SnapshotHash, when present, is a lowercase SHA-256 digest.
type PlanData struct {
	Payload      json.RawMessage `json:"payload"`
	Summary      json.RawMessage `json:"summary"`
	Impact       authz.Impact    `json:"impact"`
	SnapshotHash string          `json:"snapshot_hash,omitempty"`
}

// Plan binds execution to an Action contract, caller identity, scope, input,
// authorized impact, and expiration. SnapshotHash, when present, is a
// lowercase SHA-256 digest. DecisionFingerprint is an opaque policy token
// bounded by authz.MaxFingerprintRunes.
type Plan struct {
	Hash                string          `json:"plan_hash"`
	ActionID            string          `json:"action_id"`
	ActionVersion       string          `json:"action_version"`
	ContractHash        string          `json:"contract_hash"`
	ActorID             string          `json:"actor_id"`
	ActorType           string          `json:"actor_type"`
	Channel             Channel         `json:"channel"`
	Scope               scope.Execution `json:"scope"`
	InputHash           string          `json:"input_hash"`
	Payload             json.RawMessage `json:"payload"`
	Impact              authz.Impact    `json:"impact"`
	SnapshotHash        string          `json:"snapshot_hash,omitempty"`
	DecisionFingerprint string          `json:"decision_fingerprint"`
	CreatedAt           time.Time       `json:"created_at"`
	ExpiresAt           time.Time       `json:"expires_at"`
}

// Preview is the caller-visible summary and hash of an authorized execution Plan.
type Preview struct {
	PlanHash  string          `json:"plan_hash"`
	Summary   json.RawMessage `json:"summary"`
	Impact    authz.Impact    `json:"impact"`
	ExpiresAt time.Time       `json:"expires_at"`
}

// Result is the validated Action output together with bounded audit-facing metadata.
type Result struct {
	Data       json.RawMessage   `json:"data"`
	Summary    string            `json:"summary,omitempty"`
	References []audit.Reference `json:"references,omitempty"`
}

// Handler supplies Action-specific planning and mutation behavior. The same
// Handler instance may receive concurrent Plan and Execute calls and must be
// safe for concurrent use. Implementations must honor context cancellation and
// deadlines, return promptly after cancellation, and treat Request and Plan
// values as immutable for the duration of a call.
//
// Runtime calls Execute only after validation and authorization and within its
// transaction boundary. A Handler may return one *Error, directly or through a
// bounded trusted standard-library error chain, using an allowed framework
// business code or a code declared by Descriptor.Errors. A declared denied code
// is reserved for Authorizer decisions. Invalid envelopes, ordinary errors, and
// panics are classified as CodeInternal.
type Handler interface {
	Plan(context.Context, Request) (PlanData, error)
	Execute(context.Context, Plan) (Result, error)
}
