// Package localidentity provides an explicit, SQLite-backed local identity
// Adapter. It creates no principals or credentials unless the consumer supplies
// them in Options.
//
// Stability: alpha. Consumers should pin an exact pre-v1 Modary version.
package localidentity

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/moduleassembly"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/scope"
)

// Adapter identity, session, and Argon2 concurrency limits.
const (
	ModuleID                         = "local-identity"
	DefaultSessionTTL                = 12 * time.Hour
	MaximumSessionTTL                = 30 * 24 * time.Hour
	StandardPasswordCheckConcurrency = 2
	MaximumConcurrentPasswordChecks  = 32
)

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)

//go:embed migrations/sqlite/*.sql
var migrationFiles embed.FS

var sqliteMigrations = mustMigrationFS()

// User is one explicitly provisioned password principal.
type User struct {
	ActorID     string
	ActorType   string
	DisplayName string
	Scope       scope.Execution
	Username    string
	Password    string
}

// BearerToken is one explicitly provisioned bearer credential. TokenID is a
// non-secret stable identifier used for rotation and revocation. Token must be
// generated from at least 256 bits of cryptographically secure randomness;
// GenerateBearerToken provides the recommended representation.
type BearerToken struct {
	TokenID string
	ActorID string
	Token   string
}

// Options is an explicit provisioning and revocation patch. Empty provisioning
// creates only schema; omitted durable principals and credentials are retained.
// A password change invalidates sessions. An actor type or scope change also
// invalidates sessions and deactivates bearer credentials; a BearerTokens entry
// in the same patch explicitly reactivates or replaces that credential. Password
// verification concurrency is bounded in-process; network and account rate
// limiting remain deployment responsibilities.
type Options struct {
	Users           []User
	BearerTokens    []BearerToken
	RevokedActorIDs []string
	RevokedTokenIDs []string
	SessionTTL      time.Duration
	// MaxConcurrentPasswordChecks bounds Argon2 memory use. Zero selects the
	// conservative limit; values above MaximumConcurrentPasswordChecks fail.
	MaxConcurrentPasswordChecks int
	// Random overrides crypto/rand for password salts and session material. A
	// production override must be a concurrency-safe CSPRNG. Reads are serialized;
	// deterministic readers are suitable only for tests.
	Random io.Reader
}

// Module returns a pure Registration. Options are validated and defensively
// copied before any database, migration, random, or hashing work occurs. The
// returned consumer-owned Registration captures that copy, including plaintext
// provisioning credentials; callers should retain it only as long as their
// composition source requires. A started Application does not retain the
// startup callback after provisioning completes.
func Module(options Options) (module.Registration, error) {
	normalized, err := normalizeOptions(options)
	if err != nil {
		return module.Registration{}, err
	}
	manifest := module.Manifest{
		SchemaVersion: module.SchemaVersion,
		ID:            ModuleID,
		Version:       "0.1.0",
		Type:          module.ModuleTypeAdapter,
		Requires:      []module.Capability{module.CapabilityDatabase},
		Provides:      []module.Capability{module.CapabilityIdentity},
	}
	return module.Registration{
		Definition: module.Definition{
			Manifest:   manifest,
			Migrations: []module.MigrationSource{{Driver: "sqlite", Files: sqliteMigrations}},
		},
		Start: func(ctx context.Context, installation module.Scope) error {
			return start(ctx, installation, normalized)
		},
	}, nil
}

func start(ctx context.Context, installation module.Scope, options Options) error {
	if ctx == nil {
		return fmt.Errorf("local Identity start context is required")
	}
	control, err := moduleassembly.ResolveDatabaseControl(installation)
	if err != nil {
		return fmt.Errorf("resolve database control: %w", err)
	}
	service := newService(control, options)
	if err := service.provision(ctx, options); err != nil {
		return fmt.Errorf("provision local Identity: %w", err)
	}
	if err := module.Provide(installation, module.IdentityResolver(), identity.Resolver(service)); err != nil {
		return err
	}
	if err := module.Provide(installation, module.SessionAuthenticator(), identity.Authenticator(service)); err != nil {
		return err
	}
	return module.Provide(installation, module.TokenAuthenticator(), identity.TokenAuthenticator(service))
}

func normalizeOptions(options Options) (Options, error) {
	if options.SessionTTL < 0 {
		return Options{}, fmt.Errorf("local Identity session TTL cannot be negative")
	}
	if options.SessionTTL == 0 {
		options.SessionTTL = DefaultSessionTTL
	}
	if options.SessionTTL < time.Minute || options.SessionTTL > MaximumSessionTTL {
		return Options{}, fmt.Errorf("local Identity session TTL must be between one minute and %s", MaximumSessionTTL)
	}
	if options.MaxConcurrentPasswordChecks < 0 || options.MaxConcurrentPasswordChecks > MaximumConcurrentPasswordChecks {
		return Options{}, fmt.Errorf("local Identity maximum concurrent password checks must be zero or between 1 and %d", MaximumConcurrentPasswordChecks)
	}
	if options.MaxConcurrentPasswordChecks == 0 {
		options.MaxConcurrentPasswordChecks = StandardPasswordCheckConcurrency
	}
	if typedNil(options.Random) {
		return Options{}, fmt.Errorf("local Identity random source cannot be typed nil")
	}

	users := append([]User(nil), options.Users...)
	tokens := append([]BearerToken(nil), options.BearerTokens...)
	revokedActors := append([]string(nil), options.RevokedActorIDs...)
	revokedTokens := append([]string(nil), options.RevokedTokenIDs...)
	seenActors := make(map[string]struct{}, len(users))
	seenUsernames := make(map[string]struct{}, len(users))
	for index, user := range users {
		if err := identity.ValidateActorID(user.ActorID); err != nil {
			return Options{}, fmt.Errorf("local Identity user %d: %w", index, err)
		}
		if err := identity.ValidateActorType(user.ActorType); err != nil {
			return Options{}, fmt.Errorf("local Identity user %d: %w", index, err)
		}
		if err := identity.ValidateDisplayName(user.DisplayName); err != nil {
			return Options{}, fmt.Errorf("local Identity user %d: %w", index, err)
		}
		if err := user.Scope.Validate(); err != nil {
			return Options{}, fmt.Errorf("local Identity user %d scope: %w", index, err)
		}
		if err := validateText("username", user.Username, 256); err != nil {
			return Options{}, fmt.Errorf("local Identity user %d: %w", index, err)
		}
		if utf8.RuneCountInString(user.Password) < 12 || len(user.Password) > 4096 || !utf8.ValidString(user.Password) {
			return Options{}, fmt.Errorf("local Identity user %d password must contain at least 12 characters and at most 4096 bytes of valid UTF-8", index)
		}
		if _, duplicate := seenActors[user.ActorID]; duplicate {
			return Options{}, fmt.Errorf("local Identity actor id %q is declared more than once", user.ActorID)
		}
		seenActors[user.ActorID] = struct{}{}
		if _, duplicate := seenUsernames[user.Username]; duplicate {
			return Options{}, fmt.Errorf("local Identity username %q is declared more than once", user.Username)
		}
		seenUsernames[user.Username] = struct{}{}
	}
	seenTokenIDs := make(map[string]struct{}, len(tokens))
	seenTokenHashes := make(map[string]struct{}, len(tokens))
	for index, token := range tokens {
		if err := validateIdentifier("token id", token.TokenID); err != nil {
			return Options{}, fmt.Errorf("local Identity bearer token %d: %w", index, err)
		}
		if err := identity.ValidateActorID(token.ActorID); err != nil {
			return Options{}, fmt.Errorf("local Identity bearer token %d: %w", index, err)
		}
		if len(token.Token) < 32 || !validCredentialToken(token.Token) {
			return Options{}, fmt.Errorf("local Identity bearer token %d secret must contain 32 to 4096 bytes of valid UTF-8 without whitespace or control characters", index)
		}
		if _, duplicate := seenTokenIDs[token.TokenID]; duplicate {
			return Options{}, fmt.Errorf("local Identity token id %q is declared more than once", token.TokenID)
		}
		seenTokenIDs[token.TokenID] = struct{}{}
		hash := credentialHash(token.Token)
		if _, duplicate := seenTokenHashes[hash]; duplicate {
			return Options{}, fmt.Errorf("local Identity bearer credentials must be unique")
		}
		seenTokenHashes[hash] = struct{}{}
	}
	revokedActorSet, err := validateUniqueActorIDs(revokedActors)
	if err != nil {
		return Options{}, err
	}
	if err := validateUniqueIdentifiers("revoked token id", revokedTokens); err != nil {
		return Options{}, err
	}
	for _, id := range revokedActors {
		if _, provisioned := seenActors[id]; provisioned {
			return Options{}, fmt.Errorf("local Identity actor %q cannot be provisioned and revoked together", id)
		}
	}
	for _, token := range tokens {
		if _, revoked := revokedActorSet[token.ActorID]; revoked {
			return Options{}, fmt.Errorf("local Identity bearer token %q cannot target revoked actor %q", token.TokenID, token.ActorID)
		}
	}
	for _, id := range revokedTokens {
		if _, provisioned := seenTokenIDs[id]; provisioned {
			return Options{}, fmt.Errorf("local Identity token %q cannot be provisioned and revoked together", id)
		}
	}
	options.Users = users
	options.BearerTokens = tokens
	options.RevokedActorIDs = revokedActors
	options.RevokedTokenIDs = revokedTokens
	return options, nil
}

func validateUniqueActorIDs(values []string) (map[string]struct{}, error) {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := identity.ValidateActorID(value); err != nil {
			return nil, fmt.Errorf("local Identity revoked actor id: %w", err)
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, fmt.Errorf("local Identity revoked actor id %q is declared more than once", value)
		}
		seen[value] = struct{}{}
	}
	return seen, nil
}

func validateUniqueIdentifiers(name string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := validateIdentifier(name, value); err != nil {
			return fmt.Errorf("local Identity: %w", err)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("local Identity %s %q is declared more than once", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateIdentifier(name, value string) error {
	if !identifierPattern.MatchString(value) {
		return fmt.Errorf("%s %q is invalid", name, value)
	}
	return nil
}

func validateText(name, value string, limit int) error {
	if !utf8.ValidString(value) || value == "" || strings.TrimSpace(value) != value || utf8.RuneCountInString(value) > limit || strings.ContainsFunc(value, unicode.IsControl) {
		return fmt.Errorf("%s must contain 1 to %d non-control characters without surrounding whitespace", name, limit)
	}
	return nil
}

func typedNil(value any) bool {
	if value == nil {
		return false
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func mustMigrationFS() fs.FS {
	files, err := fs.Sub(migrationFiles, "migrations/sqlite")
	if err != nil {
		panic(err)
	}
	return files
}
