package identity

import (
	"context"
	"errors"
	"time"

	"github.com/iiwish/modary/scope"
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

// Actor is the validated principal envelope passed to authorization and Action
// execution. Scope is identity context, not authorization policy.
type Actor struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	DisplayName string          `json:"display_name"`
	Scope       scope.Execution `json:"scope"`
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

// Authenticator manages password login and revocable server-side sessions. Its
// methods share Resolver's concurrency and context contract.
type Authenticator interface {
	Resolver
	// Login returns ErrAuthenticationFailed for rejected credentials.
	Login(context.Context, string, string) (Session, error)
	Logout(context.Context, string) error
	// Session returns ErrSessionInvalid for unknown, expired, or revoked tokens.
	Session(context.Context, string) (Session, error)
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
