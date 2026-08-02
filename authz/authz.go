package authz

import (
	"context"

	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/scope"
)

// Decision field limits bound every policy-controlled value that can enter a
// public error, governed plan, or audit record.
const (
	MaxDecisionCodeRunes   = 64
	MaxDecisionReasonRunes = 512
	MaxFingerprintRunes    = 256
)

// Phase identifies whether policy is evaluating requested intent or a concrete
// planned impact.
type Phase string

// Authorization phases describe an operation before and after its concrete
// mutation footprint is known. Ordinary business handlers usually evaluate
// only PhaseIntent.
const (
	PhaseIntent Phase = "intent"
	PhaseImpact Phase = "impact"
)

// Impact is the bounded mutation footprint presented to policy.
type Impact struct {
	Rows      int      `json:"rows,omitempty"`
	Resources []string `json:"resources,omitempty"`
}

// Request contains the complete policy input for one authorization decision.
type Request struct {
	Actor       identity.Actor
	OperationID string
	Permission  string
	Scope       scope.Execution
	Phase       Phase
	Impact      Impact
}

// Constraints contains limits that an allowed decision still enforces.
type Constraints struct {
	MaxRows int `json:"max_rows,omitempty"`
}

// Decision records allow/deny state, public diagnostics, constraints, and a
// policy fingerprint used to detect changes between Preview and Execute. A
// denied custom Code is public only when the Action descriptor declares it with
// action.ErrorKindDenied; Reason is presented directly and must satisfy the
// documented bounded text contract.
type Decision struct {
	Allowed            bool        `json:"allowed"`
	Code               string      `json:"code,omitempty"`
	Reason             string      `json:"reason,omitempty"`
	RequiredPermission string      `json:"required_permission,omitempty"`
	Constraints        Constraints `json:"constraints,omitempty"`
	Fingerprint        string      `json:"fingerprint"`
}

// Authorizer evaluates current policy for ordinary or governed operations.
// Policy denial is expressed as a successful denied Decision. A returned error
// is an operational dependency failure and Runtime classifies it as
// CodeInternal. The same Authorizer may be called concurrently; implementations
// must be safe for concurrent use, honor context cancellation and deadlines,
// and treat each Request as immutable for the duration of the call.
type Authorizer interface {
	Authorize(context.Context, Request) (Decision, error)
}
