package identitystore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/databasecontrol"
	"golang.org/x/crypto/argon2"
)

// Adapter errors mirror public identity classifications and add local failure
// modes for context and injected randomness.
var (
	ErrContextRequired      = errors.New("PostgreSQL identity store context is required")
	ErrActorNotFound        = identity.ErrActorNotFound
	ErrAuthenticationFailed = identity.ErrAuthenticationFailed
	ErrSessionInvalid       = identity.ErrSessionInvalid
	ErrRandomSourcePanic    = errors.New("PostgreSQL identity store random source panicked")
)

const (
	passwordMemoryKiB        = uint32(19 * 1024)
	passwordIterations       = uint32(2)
	passwordThreads          = uint8(1)
	passwordSaltBytes        = 16
	passwordKeyBytes         = uint32(32)
	passwordHashEncodedBytes = 97
	databaseTimeFormat       = "2006-01-02T15:04:05.000000000Z07:00"

	maxStoredActorTypeBytes   = identity.MaxActorTypeRunes * utf8.UTFMax
	maxStoredDisplayNameBytes = identity.MaxDisplayNameRunes * utf8.UTFMax
	maxStoredCSRFTokenBytes   = 64
)

const actorProjection = `p.actor_id, p.actor_type, p.display_name`

type service struct {
	control          databasecontrol.Control
	sessionTTL       time.Duration
	clock            func() time.Time
	random           io.Reader
	randomMu         sync.Mutex
	passwordChecks   chan struct{}
	passwordVerifier func(string, string) bool
}

func newService(control databasecontrol.Control, options Options) *service {
	randomSource := io.Reader(rand.Reader)
	if options.Random != nil {
		randomSource = options.Random
	}
	passwordConcurrency := options.MaxConcurrentPasswordChecks
	if passwordConcurrency == 0 {
		passwordConcurrency = StandardPasswordCheckConcurrency
	}
	return &service{
		control: control, sessionTTL: options.SessionTTL, clock: time.Now, random: randomSource,
		passwordChecks:   make(chan struct{}, passwordConcurrency),
		passwordVerifier: verifyPassword,
	}
}

// ResolveByID loads one active local actor.
func (service *service) ResolveByID(ctx context.Context, actorID string) (identity.Actor, error) {
	if ctx == nil {
		return identity.Actor{}, ErrContextRequired
	}
	if err := identity.ValidateActorID(actorID); err != nil {
		return identity.Actor{}, ErrActorNotFound
	}
	executor, err := service.executor(ctx)
	if err != nil {
		return identity.Actor{}, err
	}
	actor, err := loadActor(ctx, executor, actorID)
	if errors.Is(err, sql.ErrNoRows) {
		return identity.Actor{}, ErrActorNotFound
	}
	if err != nil {
		return identity.Actor{}, fmt.Errorf("resolve PostgreSQL identity store actor: %w", err)
	}
	return actor, nil
}

// AuthenticatePassword verifies one local password without creating a browser
// session. Browser transports explicitly compose this with SessionManager.
func (service *service) AuthenticatePassword(ctx context.Context, username, password string) (identity.Authentication, error) {
	if ctx == nil {
		return identity.Authentication{}, ErrContextRequired
	}
	if validateText("username", username, 256) != nil || len(password) > 4096 || !utf8.ValidString(password) {
		return identity.Authentication{}, ErrAuthenticationFailed
	}
	var storedActorID, storedEncoded, storedVersion sql.NullString
	executor, err := service.executor(ctx)
	if err != nil {
		return identity.Authentication{}, err
	}
	err = executor.QueryRowContext(ctx, `
			SELECT
					p.actor_id, c.password_hash, c.updated_at
			FROM modary_identity_password c
			JOIN modary_identity_principal p ON p.actor_id = c.actor_id
				WHERE c.username = $1 AND p.active = TRUE`, username).Scan(
		&storedActorID, &storedEncoded, &storedVersion)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return identity.Authentication{}, fmt.Errorf("authenticate PostgreSQL identity store password: %w", err)
	}
	actorID, encoded := storedActorID.String, storedEncoded.String
	valid, verifyErr := service.verifyCredential(ctx, encoded, password)
	if verifyErr != nil {
		return identity.Authentication{}, fmt.Errorf("wait for PostgreSQL identity store password verification: %w", verifyErr)
	}
	if err == nil {
		_, _, _, _, _, wellFormed := parsePasswordHash(encoded)
		if !storedActorID.Valid || !storedEncoded.Valid || !storedVersion.Valid || identity.ValidateActorID(actorID) != nil || !wellFormed {
			return identity.Authentication{}, fmt.Errorf("authenticate PostgreSQL identity store password: stored credential is invalid")
		}
		if _, parseErr := parseDatabaseTime(storedVersion.String); parseErr != nil {
			return identity.Authentication{}, fmt.Errorf("authenticate PostgreSQL identity store password: stored credential version is invalid")
		}
	}
	if err != nil || !valid {
		return identity.Authentication{}, ErrAuthenticationFailed
	}
	var actor identity.Actor
	err = service.control.WithinTransaction(ctx, func(txCtx context.Context) error {
		tx, err := service.executor(txCtx)
		if err != nil {
			return err
		}
		var currentActorID, currentEncoded, currentVersion sql.NullString
		err = tx.QueryRowContext(txCtx, `
			SELECT
					p.actor_id, c.password_hash, c.updated_at
			FROM modary_identity_password c
			JOIN modary_identity_principal p ON p.actor_id = c.actor_id
				WHERE c.username = $1 AND p.active = TRUE`, username).Scan(
			&currentActorID, &currentEncoded, &currentVersion)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAuthenticationFailed
		}
		if err != nil {
			return fmt.Errorf("revalidate PostgreSQL identity store password: %w", err)
		}
		if !currentActorID.Valid || !currentEncoded.Valid || !currentVersion.Valid {
			return fmt.Errorf("revalidate PostgreSQL identity store password: stored credential is invalid")
		}
		if currentActorID.String != actorID || currentEncoded.String != encoded || currentVersion.String != storedVersion.String {
			return ErrAuthenticationFailed
		}
		actor, err = loadActor(txCtx, tx, actorID)
		if err != nil {
			return fmt.Errorf("revalidate PostgreSQL identity store actor: %w", err)
		}
		return nil
	})
	if err != nil {
		return identity.Authentication{}, fmt.Errorf("revalidate PostgreSQL identity store password: %w", err)
	}
	return identity.Authentication{
		Actor: actor, Method: identity.AuthenticationMethodPassword, CredentialVersion: storedVersion.String,
	}, nil
}

// CreateSession creates a durable session for a current local principal.
func (service *service) CreateSession(ctx context.Context, authentication identity.Authentication) (identity.Session, error) {
	if ctx == nil {
		return identity.Session{}, ErrContextRequired
	}
	if err := identity.ValidateAuthentication(authentication); err != nil {
		return identity.Session{}, ErrActorNotFound
	}
	actor := authentication.Actor
	sessionToken, err := service.randomHex(32)
	if err != nil {
		return identity.Session{}, fmt.Errorf("create PostgreSQL identity store session token: %w", err)
	}
	csrfToken, err := service.randomHex(32)
	if err != nil {
		return identity.Session{}, fmt.Errorf("create PostgreSQL identity store CSRF token: %w", err)
	}
	sessionID, err := service.randomHex(16)
	if err != nil {
		return identity.Session{}, fmt.Errorf("create PostgreSQL identity store session id: %w", err)
	}
	var session identity.Session
	err = service.control.WithinTransaction(ctx, func(txCtx context.Context) error {
		tx, err := service.executor(txCtx)
		if err != nil {
			return err
		}
		current, err := loadActor(txCtx, tx, actor.ID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && current.Type != actor.Type) {
			return ErrActorNotFound
		}
		if err != nil {
			return fmt.Errorf("resolve PostgreSQL identity store session actor: %w", err)
		}
		if authentication.Method == identity.AuthenticationMethodPassword {
			var currentVersion sql.NullString
			err := tx.QueryRowContext(txCtx, `
				SELECT c.updated_at
				FROM modary_identity_password c
				JOIN modary_identity_principal p ON p.actor_id = c.actor_id
				WHERE c.actor_id = $1 AND p.active = TRUE`, current.ID).Scan(&currentVersion)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && (!currentVersion.Valid || currentVersion.String != authentication.CredentialVersion)) {
				return ErrAuthenticationFailed
			}
			if err != nil {
				return fmt.Errorf("revalidate PostgreSQL identity store session credential: %w", err)
			}
		}
		now := service.now()
		expiresAt := now.Add(service.sessionTTL)
		if _, err := tx.ExecContext(txCtx, `DELETE FROM modary_identity_session WHERE expires_at <= $1`, formatDatabaseTime(now)); err != nil {
			return fmt.Errorf("remove expired PostgreSQL identity store sessions: %w", err)
		}
		if _, err := tx.ExecContext(txCtx, `
		INSERT INTO modary_identity_session
		(session_id, token_hash, actor_id, csrf_token, expires_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $6)`, "ses_"+sessionID, credentialHash(sessionToken), current.ID,
			csrfToken, formatDatabaseTime(expiresAt), formatDatabaseTime(now)); err != nil {
			return fmt.Errorf("persist PostgreSQL identity store session: %w", err)
		}
		session = identity.Session{Token: sessionToken, CSRFToken: csrfToken, Actor: current, ExpiresAt: expiresAt}
		return nil
	})
	if err != nil {
		return identity.Session{}, fmt.Errorf("persist PostgreSQL identity store session: %w", err)
	}
	return session, nil
}

// RevokeSession deletes the session identified by token.
func (service *service) RevokeSession(ctx context.Context, token string) error {
	if ctx == nil {
		return ErrContextRequired
	}
	if token == "" {
		return nil
	}
	if !validCredentialToken(token) {
		return ErrSessionInvalid
	}
	return service.control.WithinTransaction(ctx, func(txCtx context.Context) error {
		executor, err := service.executor(txCtx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(txCtx, `DELETE FROM modary_identity_session WHERE token_hash = $1`, credentialHash(token)); err != nil {
			return fmt.Errorf("delete PostgreSQL identity store session: %w", err)
		}
		return nil
	})
}

// ResolveSession resolves one active, unexpired local session.
func (service *service) ResolveSession(ctx context.Context, token string) (identity.Session, error) {
	if ctx == nil {
		return identity.Session{}, ErrContextRequired
	}
	if !validCredentialToken(token) {
		return identity.Session{}, ErrSessionInvalid
	}
	var storedActor storedActorColumns
	var csrfToken, expiresText sql.NullString
	executor, err := service.executor(ctx)
	if err != nil {
		return identity.Session{}, err
	}
	destinations := append(storedActor.scanTargets(), &csrfToken, &expiresText)
	err = executor.QueryRowContext(ctx, `SELECT `+actorProjection+`, s.csrf_token, s.expires_at
			FROM modary_identity_session s
			JOIN modary_identity_principal p ON p.actor_id = s.actor_id
				WHERE s.token_hash = $1 AND p.active = TRUE`, credentialHash(token)).Scan(destinations...)
	if errors.Is(err, sql.ErrNoRows) {
		return identity.Session{}, ErrSessionInvalid
	}
	if err != nil {
		return identity.Session{}, fmt.Errorf("resolve PostgreSQL identity store session: %w", err)
	}
	if !csrfToken.Valid || !expiresText.Valid {
		return identity.Session{}, fmt.Errorf("decode PostgreSQL identity store session: stored session fields are oversized or not text")
	}
	expiresAt, err := parseDatabaseTime(expiresText.String)
	if err != nil {
		return identity.Session{}, fmt.Errorf("decode PostgreSQL identity store session expiry: %w", err)
	}
	if !service.now().Before(expiresAt) {
		_ = service.control.WithinTransaction(ctx, func(txCtx context.Context) error {
			executor, executorErr := service.executor(txCtx)
			if executorErr != nil {
				return executorErr
			}
			_, executorErr = executor.ExecContext(txCtx, `DELETE FROM modary_identity_session WHERE token_hash = $1`, credentialHash(token))
			return executorErr
		})
		return identity.Session{}, ErrSessionInvalid
	}
	if !validLowerHex(csrfToken.String, maxStoredCSRFTokenBytes) {
		return identity.Session{}, fmt.Errorf("decode PostgreSQL identity store session: CSRF token is invalid")
	}
	actor, err := storedActor.decode()
	if err != nil {
		return identity.Session{}, fmt.Errorf("decode PostgreSQL identity store session actor: %w", err)
	}
	return identity.Session{Token: token, CSRFToken: csrfToken.String, Actor: actor, ExpiresAt: expiresAt}, nil
}

// AuthenticateToken resolves one active local bearer credential.
func (service *service) AuthenticateToken(ctx context.Context, token string) (identity.Actor, error) {
	if ctx == nil {
		return identity.Actor{}, ErrContextRequired
	}
	if !validCredentialToken(token) {
		return identity.Actor{}, ErrAuthenticationFailed
	}
	var storedActor storedActorColumns
	executor, err := service.executor(ctx)
	if err != nil {
		return identity.Actor{}, err
	}
	err = executor.QueryRowContext(ctx, `SELECT `+actorProjection+`
			FROM modary_identity_bearer b
			JOIN modary_identity_principal p ON p.actor_id = b.actor_id
				WHERE b.token_hash = $1 AND b.active = TRUE AND p.active = TRUE`,
		credentialHash(token)).Scan(storedActor.scanTargets()...)
	if errors.Is(err, sql.ErrNoRows) {
		return identity.Actor{}, ErrAuthenticationFailed
	}
	if err != nil {
		return identity.Actor{}, fmt.Errorf("authenticate PostgreSQL identity store bearer token: %w", err)
	}
	actor, err := storedActor.decode()
	if err != nil {
		return identity.Actor{}, fmt.Errorf("decode PostgreSQL identity store bearer actor: %w", err)
	}
	return actor, nil
}

func validCredentialToken(token string) bool {
	return token != "" && len(token) <= 4096 && utf8.ValidString(token) &&
		!strings.ContainsFunc(token, func(character rune) bool {
			return unicode.IsSpace(character) || unicode.IsControl(character)
		})
}

func (service *service) provision(ctx context.Context, options Options) error {
	return service.control.WithinTransaction(ctx, func(txCtx context.Context) error {
		tx, err := service.executor(txCtx)
		if err != nil {
			return err
		}
		now := formatDatabaseTime(service.now())
		for _, principal := range options.Principals {
			var existingType sql.NullString
			principalErr := tx.QueryRowContext(txCtx, `
					SELECT
							actor_type
						FROM modary_identity_principal WHERE actor_id = $1`, principal.ActorID).Scan(&existingType)
			if principalErr != nil && !errors.Is(principalErr, sql.ErrNoRows) {
				return principalErr
			}
			if principalErr == nil && !existingType.Valid {
				return fmt.Errorf("stored PostgreSQL identity store principal contains oversized or non-text security context")
			}
			securityContextChanged := principalErr == nil && existingType.String != principal.ActorType
			if _, err := tx.ExecContext(txCtx, `
				INSERT INTO modary_identity_principal
				(actor_id, actor_type, display_name, active, created_at, updated_at)
					VALUES ($1, $2, $3, TRUE, $4, $5)
				ON CONFLICT(actor_id) DO UPDATE SET actor_type = excluded.actor_type,
					display_name = excluded.display_name, active = TRUE, updated_at = excluded.updated_at`,
				principal.ActorID, principal.ActorType, principal.DisplayName, now, now); err != nil {
				return err
			}
			if securityContextChanged {
				if _, err := tx.ExecContext(txCtx, `DELETE FROM modary_identity_session WHERE actor_id = $1`, principal.ActorID); err != nil {
					return err
				}
				if _, err := tx.ExecContext(txCtx, `UPDATE modary_identity_bearer SET active = FALSE, updated_at = $1 WHERE actor_id = $2`, now, principal.ActorID); err != nil {
					return err
				}
			}
		}
		for _, credential := range options.PasswordCredentials {
			if err := requireActivePrincipal(txCtx, tx, credential.ActorID, "password credential"); err != nil {
				return err
			}
			var existingHash sql.NullString
			existingErr := tx.QueryRowContext(txCtx, `
					SELECT password_hash
					FROM modary_identity_password WHERE actor_id = $1`, credential.ActorID).Scan(&existingHash)
			if existingErr != nil && !errors.Is(existingErr, sql.ErrNoRows) {
				return existingErr
			}
			passwordMatches, verifyErr := service.verifyCredential(txCtx, existingHash.String, credential.Password)
			if verifyErr != nil {
				return verifyErr
			}
			if existingErr == nil {
				_, _, _, _, _, wellFormed := parsePasswordHash(existingHash.String)
				if !existingHash.Valid || !wellFormed {
					return fmt.Errorf("stored PostgreSQL identity store password credential is invalid")
				}
			}
			passwordChanged := !passwordMatches
			encoded := existingHash.String
			if passwordChanged {
				encoded, err = service.hashPassword(credential.Password)
				if err != nil {
					return err
				}
			}
			if _, err := tx.ExecContext(txCtx, `
				INSERT INTO modary_identity_password (username, actor_id, password_hash, updated_at)
					VALUES ($1, $2, $3, $4)
				ON CONFLICT(actor_id) DO UPDATE SET username = excluded.username,
					password_hash = excluded.password_hash,
					updated_at = CASE
						WHEN modary_identity_password.password_hash IS DISTINCT FROM excluded.password_hash
						THEN excluded.updated_at
						ELSE modary_identity_password.updated_at
					END`,
				credential.Username, credential.ActorID, encoded, now); err != nil {
				return err
			}
			if passwordChanged && existingErr == nil {
				if _, err := tx.ExecContext(txCtx, `DELETE FROM modary_identity_session WHERE actor_id = $1`, credential.ActorID); err != nil {
					return err
				}
			}
		}
		for _, token := range options.BearerTokens {
			if err := requireActivePrincipal(txCtx, tx, token.ActorID, "bearer credential"); err != nil {
				return err
			}
			if _, err := tx.ExecContext(txCtx, `
				INSERT INTO modary_identity_bearer
				(token_id, token_hash, actor_id, active, created_at, updated_at)
					VALUES ($1, $2, $3, TRUE, $4, $5)
				ON CONFLICT(token_id) DO UPDATE SET token_hash = excluded.token_hash,
						actor_id = excluded.actor_id, active = TRUE, updated_at = excluded.updated_at`,
				token.TokenID, credentialHash(token.Token), token.ActorID, now, now); err != nil {
				return err
			}
		}
		for _, actorID := range options.RevokedActorIDs {
			if _, err := tx.ExecContext(txCtx, `UPDATE modary_identity_principal SET active = FALSE, updated_at = $1 WHERE actor_id = $2`, now, actorID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(txCtx, `DELETE FROM modary_identity_session WHERE actor_id = $1`, actorID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(txCtx, `UPDATE modary_identity_bearer SET active = FALSE, updated_at = $1 WHERE actor_id = $2`, now, actorID); err != nil {
				return err
			}
		}
		for _, tokenID := range options.RevokedTokenIDs {
			if _, err := tx.ExecContext(txCtx, `UPDATE modary_identity_bearer SET active = FALSE, updated_at = $1 WHERE token_id = $2`, now, tokenID); err != nil {
				return err
			}
		}
		return nil
	})
}

func requireActivePrincipal(ctx context.Context, executor database.Executor, actorID, credentialKind string) error {
	var active bool
	err := executor.QueryRowContext(ctx, `SELECT active FROM modary_identity_principal WHERE actor_id = $1`, actorID).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("PostgreSQL identity store %s targets an unknown principal", credentialKind)
	}
	if err != nil {
		return fmt.Errorf("verify PostgreSQL identity store %s principal: %w", credentialKind, err)
	}
	if !active {
		return fmt.Errorf("PostgreSQL identity store %s targets an inactive principal", credentialKind)
	}
	return nil
}

func (service *service) executor(ctx context.Context) (database.Executor, error) {
	if service == nil || service.control == nil {
		return nil, fmt.Errorf("PostgreSQL identity store database is unavailable")
	}
	return service.control.Executor(ctx)
}

func loadActor(ctx context.Context, executor database.Executor, actorID string) (identity.Actor, error) {
	var storedActor storedActorColumns
	err := executor.QueryRowContext(ctx, `SELECT `+actorProjection+`
				FROM modary_identity_principal p WHERE p.actor_id = $1 AND p.active = TRUE`,
		actorID).Scan(storedActor.scanTargets()...)
	if err != nil {
		return identity.Actor{}, err
	}
	return storedActor.decode()
}

type storedActorColumns struct{ id, actorType, displayName sql.NullString }

func (stored *storedActorColumns) scanTargets() []any {
	return []any{&stored.id, &stored.actorType, &stored.displayName}
}

func (stored storedActorColumns) decode() (identity.Actor, error) {
	if !stored.id.Valid || !stored.actorType.Valid || !stored.displayName.Valid {
		return identity.Actor{}, fmt.Errorf("stored actor contains oversized or non-text fields")
	}
	actor := identity.Actor{ID: stored.id.String, Type: stored.actorType.String, DisplayName: stored.displayName.String}
	if err := validateStoredActor(actor); err != nil {
		return identity.Actor{}, err
	}
	return actor, nil
}

func validateStoredActor(actor identity.Actor) error {
	if err := identity.ValidateActorID(actor.ID); err != nil {
		return err
	}
	if err := identity.ValidateActorType(actor.Type); err != nil {
		return err
	}
	if err := identity.ValidateDisplayName(actor.DisplayName); err != nil {
		return err
	}
	return nil
}

func formatDatabaseTime(value time.Time) string {
	return value.UTC().Format(databaseTimeFormat)
}

func parseDatabaseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(databaseTimeFormat, value)
	if err != nil || value != formatDatabaseTime(parsed) {
		return time.Time{}, fmt.Errorf("timestamp is not canonical UTC")
	}
	return parsed, nil
}

func (service *service) now() time.Time {
	return service.clock().UTC()
}

func (service *service) randomHex(size int) (string, error) {
	data := make([]byte, size)
	if err := service.readRandom(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func (service *service) hashPassword(password string) (string, error) {
	salt := make([]byte, passwordSaltBytes)
	if err := service.readRandom(salt); err != nil {
		return "", fmt.Errorf("read password salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, passwordIterations, passwordMemoryKiB, passwordThreads, passwordKeyBytes)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", passwordMemoryKiB,
		passwordIterations, passwordThreads, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func (service *service) readRandom(target []byte) (err error) {
	service.randomMu.Lock()
	defer service.randomMu.Unlock()
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			err = ErrRandomSourcePanic
		}
	}()
	_, err = io.ReadFull(service.random, target)
	returned = true
	return err
}

func (service *service) verifyCredential(ctx context.Context, encoded, password string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	select {
	case service.passwordChecks <- struct{}{}:
		defer func() { <-service.passwordChecks }()
		if err := ctx.Err(); err != nil {
			return false, err
		}
	case <-ctx.Done():
		return false, ctx.Err()
	}
	return service.passwordVerifier(encoded, password), nil
}

func verifyPassword(encoded, password string) bool {
	memory, iterations, threads, salt, expected, ok := parsePasswordHash(encoded)
	if !ok {
		memory, iterations, threads = passwordMemoryKiB, passwordIterations, passwordThreads
		salt = []byte("modary-dummy-salt")
		expected = make([]byte, passwordKeyBytes)
	}
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(expected)))
	return ok && subtle.ConstantTimeCompare(actual, expected) == 1
}

func parsePasswordHash(encoded string) (uint32, uint32, uint8, []byte, []byte, bool) {
	if len(encoded) != passwordHashEncodedBytes {
		return 0, 0, 0, nil, nil, false
	}
	parts := strings.SplitN(encoded, "$", 6)
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return 0, 0, 0, nil, nil, false
	}
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil ||
		parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", memory, iterations, threads) ||
		memory != passwordMemoryKiB || iterations != passwordIterations || threads != passwordThreads {
		return 0, 0, 0, nil, nil, false
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) != passwordSaltBytes {
		return 0, 0, 0, nil, nil, false
	}
	hash, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(hash) != int(passwordKeyBytes) {
		return 0, 0, 0, nil, nil, false
	}
	return memory, iterations, threads, salt, hash, true
}

func credentialHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(hash[:])
}

func validLowerHex(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for index := range len(value) {
		character := value[index]
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

var _ identity.PasswordAuthenticator = (*service)(nil)
var _ identity.SessionManager = (*service)(nil)
var _ identity.TokenAuthenticator = (*service)(nil)
