package appkit

import "github.com/iiwish/modary/identity"

// Identities returns the optional actor resolver installed by the consumer's
// Modules.
func (application *Application) Identities() (identity.Resolver, error) {
	if application == nil {
		return nil, ErrApplicationUnavailable
	}
	if !application.Ready() {
		return nil, ErrApplicationUnavailable
	}
	if application.identities == nil {
		return nil, ErrIdentitiesUnavailable
	}
	return application.identities, nil
}

// Passwords returns the optional password verifier installed by the consumer's
// Modules. OIDC-only applications can omit it.
func (application *Application) Passwords() (identity.PasswordAuthenticator, error) {
	if application == nil || !application.Ready() {
		return nil, ErrApplicationUnavailable
	}
	if application.passwords == nil {
		return nil, ErrPasswordsUnavailable
	}
	return application.passwords, nil
}

// BrowserAuthentication returns the selected redirect-based browser
// authenticator. Local-password-only applications may omit it.
func (application *Application) BrowserAuthentication() (identity.BrowserAuthenticator, error) {
	if application == nil || !application.Ready() {
		return nil, ErrApplicationUnavailable
	}
	if application.browserAuth == nil {
		return nil, ErrBrowserAuthenticationUnavailable
	}
	return application.browserAuth, nil
}

// Sessions returns the optional session manager installed by the consumer's
// Modules.
func (application *Application) Sessions() (identity.SessionManager, error) {
	if application == nil {
		return nil, ErrApplicationUnavailable
	}
	if !application.Ready() {
		return nil, ErrApplicationUnavailable
	}
	if application.sessions == nil {
		return nil, ErrSessionsUnavailable
	}
	return application.sessions, nil
}

// Tokens returns the optional bearer-token authenticator installed by the
// consumer's Modules.
func (application *Application) Tokens() (identity.TokenAuthenticator, error) {
	if application == nil {
		return nil, ErrApplicationUnavailable
	}
	if !application.Ready() {
		return nil, ErrApplicationUnavailable
	}
	if application.tokens == nil {
		return nil, ErrTokensUnavailable
	}
	return application.tokens, nil
}
