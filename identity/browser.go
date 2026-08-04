package identity

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrBrowserFlowInvalid reports an unknown, expired, replayed, or malformed
	// redirect authentication flow without exposing provider details.
	ErrBrowserFlowInvalid = errors.New("browser authentication flow is invalid or expired")
)

// BrowserFlow is one bounded redirect authentication start result. State is
// also bound to the initiating browser by the official HTTP contribution.
type BrowserFlow struct {
	AuthorizationURL string
	State            string
	ExpiresAt        time.Time
}

// BrowserCallback contains the exact OAuth/OIDC callback values accepted by a
// BrowserAuthenticator. HTTP parsing, duplicate rejection, and flow-cookie
// binding remain transport responsibilities.
type BrowserCallback struct {
	State string
	Code  string
}

// BrowserAuthenticator performs a redirect-based upstream authentication
// ceremony and returns a principal Authentication, never roles or product
// scope. Implementations must consume callback state at most once.
type BrowserAuthenticator interface {
	Begin(context.Context) (BrowserFlow, error)
	Complete(context.Context, BrowserCallback) (Authentication, error)
}
