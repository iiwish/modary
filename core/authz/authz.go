package authz

import (
	"context"

	"modary/core/identity"
)

type Phase string

const (
	PhaseIntent Phase = "intent"
	PhaseImpact Phase = "impact"
)

type Impact struct {
	Rows      int      `json:"rows,omitempty"`
	Resources []string `json:"resources,omitempty"`
}

type Request struct {
	Actor       identity.Actor
	ActionID    string
	Permission  string
	WorkspaceID string
	Phase       Phase
	Impact      Impact
}

type Constraints struct {
	MaxRows int `json:"max_rows,omitempty"`
}

type Decision struct {
	Allowed            bool        `json:"allowed"`
	Code               string      `json:"code,omitempty"`
	Reason             string      `json:"reason,omitempty"`
	RequiredPermission string      `json:"required_permission,omitempty"`
	Constraints        Constraints `json:"constraints,omitempty"`
	Fingerprint        string      `json:"fingerprint"`
}

type Authorizer interface {
	Authorize(context.Context, Request) (Decision, error)
}
