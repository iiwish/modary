package identity

import (
	"context"
	"time"
)

type Actor struct {
	ID             string   `json:"id"`
	Type           string   `json:"type"`
	DisplayName    string   `json:"display_name"`
	WorkspaceID    string   `json:"workspace_id"`
	Roles          []string `json:"roles,omitempty"`
	Permissions    []string `json:"permissions,omitempty"`
	DelegatedBy    string   `json:"delegated_by,omitempty"`
	GrantID        string   `json:"grant_id,omitempty"`
	GrantVersion   string   `json:"grant_version,omitempty"`
	AllowedActions []string `json:"allowed_actions,omitempty"`
	MaxRows        int      `json:"max_rows,omitempty"`
	ExpiresAtUnix  int64    `json:"expires_at_unix,omitempty"`
}

type Resolver interface {
	ResolveSession(context.Context, string) (Actor, error)
	ResolveAgentToken(context.Context, string) (Actor, error)
	ResolveByID(context.Context, string) (Actor, error)
}

type Session struct {
	Token     string    `json:"-"`
	CSRFToken string    `json:"csrf_token"`
	Actor     Actor     `json:"actor"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Authenticator interface {
	Resolver
	Login(context.Context, string, string) (Session, error)
	Logout(context.Context, string) error
	Session(context.Context, string) (Session, error)
}
