package identity

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrActorNotFound reports that an otherwise valid actor identifier has no
	// active identity. Adapters may wrap this value with operational context.
	ErrActorNotFound = errors.New("identity actor was not found")
	// ErrAuthenticationFailed reports an invalid or expired login or bearer
	// credential. It deliberately does not distinguish unknown principals from
	// incorrect secrets.
	ErrAuthenticationFailed = errors.New("identity authentication failed")
	// ErrSessionInvalid reports an unknown, expired, or revoked session token.
	ErrSessionInvalid = errors.New("identity session is invalid or expired")
)

// Authentication method identifiers used by official components.
const (
	AuthenticationMethodPassword = "password"
	AuthenticationMethodOIDC     = "oidc"
)

// Actor is the validated principal envelope passed to authorization and Action
// execution. Product scope and authorization grants deliberately live outside
// identity so one principal can participate in zero, one, or many scopes.
type Actor struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
}

// Resolver loads current active actor state by stable identifier. The same
// Resolver may be called concurrently; implementations must be safe for
// concurrent use, honor context cancellation and deadlines, and return promptly
// after cancellation.
type Resolver interface {
	// ResolveByID returns ErrActorNotFound when the identifier has no active
	// identity. Other errors are operational failures and must not be presented
	// to callers as an authentication decision.
	ResolveByID(context.Context, string) (Actor, error)
}

// Session contains one authenticated browser session and its CSRF binding.
type Session struct {
	Token     string    `json:"-"`
	CSRFToken string    `json:"csrf_token"`
	Actor     Actor     `json:"actor"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Authentication is the validated result of one upstream authentication
// ceremony. CredentialVersion is an opaque, non-secret freshness token used by
// a cooperating session store to reject stale password verification after
// credential rotation. Transports must treat it as sensitive process data.
type Authentication struct {
	Actor             Actor  `json:"actor"`
	Method            string `json:"method"`
	CredentialVersion string `json:"-"`
}

// PasswordAuthenticator verifies a password credential and returns the current
// principal. It does not create a browser session or own HTTP login ceremony.
type PasswordAuthenticator interface {
	// AuthenticatePassword returns ErrAuthenticationFailed for rejected
	// credentials without distinguishing unknown accounts from wrong secrets.
	AuthenticatePassword(context.Context, string, string) (Authentication, error)
}

// SessionManager creates, resolves, and revokes server-side application
// sessions. It is independent of the upstream authentication ceremony so local
// passwords and OIDC can share the same protected-session contract.
type SessionManager interface {
	CreateSession(context.Context, Authentication) (Session, error)
	RevokeSession(context.Context, string) error
	// ResolveSession returns ErrSessionInvalid for unknown, expired, or revoked
	// tokens.
	ResolveSession(context.Context, string) (Session, error)
}

// TokenAuthenticator resolves a bearer credential to an authenticated Actor.
// Authorization policy and delegated grants remain owned by an Authorizer. The
// same TokenAuthenticator may be called concurrently; implementations must be
// safe for concurrent use, honor context cancellation and deadlines, and return
// promptly after cancellation.
type TokenAuthenticator interface {
	// AuthenticateToken returns ErrAuthenticationFailed for an unknown,
	// expired, or revoked bearer credential.
	AuthenticateToken(context.Context, string) (Actor, error)
}
