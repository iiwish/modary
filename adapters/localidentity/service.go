package localidentity

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
	"github.com/iiwish/modary/scope"
	"golang.org/x/crypto/argon2"
)

// Adapter errors mirror public identity classifications and add local failure
// modes for context and injected randomness.
var (
	ErrContextRequired      = errors.New("local Identity context is required")
	ErrActorNotFound        = identity.ErrActorNotFound
	ErrAuthenticationFailed = identity.ErrAuthenticationFailed
	ErrSessionInvalid       = identity.ErrSessionInvalid
	ErrRandomSourcePanic    = errors.New("local Identity random source panicked")
)

const (
	passwordMemoryKiB        = uint32(19 * 1024)
	passwordIterations       = uint32(2)
	passwordThreads          = uint8(1)
	passwordSaltBytes        = 16
	passwordKeyBytes         = uint32(32)
	passwordHashEncodedBytes = 97
	sqliteTimeFormat         = "2006-01-02T15:04:05.000000000Z07:00"

	maxStoredActorIDBytes       = identity.MaxActorIDRunes * utf8.UTFMax
	maxStoredActorTypeBytes     = identity.MaxActorTypeRunes * utf8.UTFMax
	maxStoredDisplayNameBytes   = identity.MaxDisplayNameRunes * utf8.UTFMax
	maxStoredScopeKindBytes     = 64 * utf8.UTFMax
	maxStoredScopeIDBytes       = 256 * utf8.UTFMax
	maxStoredCSRFTokenBytes     = 64
	maxStoredCanonicalTimeBytes = len("2006-01-02T15:04:05.000000000Z")
)

const boundedActorProjection = `
	CASE WHEN typeof(p.actor_id) = 'text' AND length(CAST(p.actor_id AS BLOB)) <= ? THEN p.actor_id END,
	CASE WHEN typeof(p.actor_type) = 'text' AND length(CAST(p.actor_type AS BLOB)) <= ? THEN p.actor_type END,
	CASE WHEN typeof(p.display_name) = 'text' AND length(CAST(p.display_name AS BLOB)) <= ? THEN p.display_name END,
	CASE WHEN typeof(p.scope_kind) = 'text' AND length(CAST(p.scope_kind AS BLOB)) <= ? THEN p.scope_kind END,
	CASE WHEN typeof(p.scope_id) = 'text' AND length(CAST(p.scope_id AS BLOB)) <= ? THEN p.scope_id END`

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
		return identity.Actor{}, fmt.Errorf("resolve local Identity actor: %w", err)
	}
	return actor, nil
}

// Login verifies credentials and creates a durable local session.
func (service *service) Login(ctx context.Context, username, password string) (identity.Session, error) {
	if ctx == nil {
		return identity.Session{}, ErrContextRequired
	}
	if validateText("username", username, 256) != nil || len(password) > 4096 || !utf8.ValidString(password) {
		return identity.Session{}, ErrAuthenticationFailed
	}
	var storedActorID, storedEncoded sql.NullString
	executor, err := service.executor(ctx)
	if err != nil {
		return identity.Session{}, err
	}
	err = executor.QueryRowContext(ctx, `
			SELECT
				CASE WHEN typeof(p.actor_id) = 'text' AND length(CAST(p.actor_id AS BLOB)) <= ? THEN p.actor_id END,
				CASE WHEN typeof(c.password_hash) = 'text' AND length(CAST(c.password_hash AS BLOB)) <= ? THEN c.password_hash END
			FROM modary_identity_password c
			JOIN modary_identity_principal p ON p.actor_id = c.actor_id
			WHERE c.username = ? AND p.active = 1`, maxStoredActorIDBytes, passwordHashEncodedBytes, username).Scan(
		&storedActorID, &storedEncoded)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return identity.Session{}, fmt.Errorf("authenticate local Identity password: %w", err)
	}
	actorID, encoded := storedActorID.String, storedEncoded.String
	valid, verifyErr := service.verifyCredential(ctx, encoded, password)
	if verifyErr != nil {
		return identity.Session{}, fmt.Errorf("wait for local Identity password verification: %w", verifyErr)
	}
	if err == nil {
		_, _, _, _, _, wellFormed := parsePasswordHash(encoded)
		if !storedActorID.Valid || !storedEncoded.Valid || identity.ValidateActorID(actorID) != nil || !wellFormed {
			return identity.Session{}, fmt.Errorf("authenticate local Identity password: stored credential is invalid")
		}
	}
	if err != nil || !valid {
		return identity.Session{}, ErrAuthenticationFailed
	}
	sessionToken, err := service.randomHex(32)
	if err != nil {
		return identity.Session{}, fmt.Errorf("create local Identity session token: %w", err)
	}
	csrfToken, err := service.randomHex(32)
	if err != nil {
		return identity.Session{}, fmt.Errorf("create local Identity CSRF token: %w", err)
	}
	sessionID, err := service.randomHex(16)
	if err != nil {
		return identity.Session{}, fmt.Errorf("create local Identity session id: %w", err)
	}
	var session identity.Session
	err = service.control.WithinTransaction(ctx, func(txCtx context.Context) error {
		tx, err := service.executor(txCtx)
		if err != nil {
			return err
		}
		var currentActorID, currentEncoded sql.NullString
		err = tx.QueryRowContext(txCtx, `
			SELECT
				CASE WHEN typeof(p.actor_id) = 'text' AND length(CAST(p.actor_id AS BLOB)) <= ? THEN p.actor_id END,
				CASE WHEN typeof(c.password_hash) = 'text' AND length(CAST(c.password_hash AS BLOB)) <= ? THEN c.password_hash END
			FROM modary_identity_password c
			JOIN modary_identity_principal p ON p.actor_id = c.actor_id
			WHERE c.username = ? AND p.active = 1`, maxStoredActorIDBytes, passwordHashEncodedBytes, username).Scan(
			&currentActorID, &currentEncoded)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAuthenticationFailed
		}
		if err != nil {
			return fmt.Errorf("revalidate local Identity password: %w", err)
		}
		if !currentActorID.Valid || !currentEncoded.Valid {
			return fmt.Errorf("revalidate local Identity password: stored credential is invalid")
		}
		if currentActorID.String != actorID || currentEncoded.String != encoded {
			return ErrAuthenticationFailed
		}
		actor, err := loadActor(txCtx, tx, actorID)
		if err != nil {
			return fmt.Errorf("revalidate local Identity actor: %w", err)
		}
		now := service.now()
		expiresAt := now.Add(service.sessionTTL)
		if _, err := tx.ExecContext(txCtx, `DELETE FROM modary_identity_session WHERE expires_at <= ?`, formatSQLiteTime(now)); err != nil {
			return fmt.Errorf("remove expired local Identity sessions: %w", err)
		}
		if _, err := tx.ExecContext(txCtx, `
		INSERT INTO modary_identity_session
		(session_id, token_hash, actor_id, csrf_token, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, "ses_"+sessionID, credentialHash(sessionToken), actor.ID,
			csrfToken, formatSQLiteTime(expiresAt), formatSQLiteTime(now)); err != nil {
			return fmt.Errorf("persist local Identity session: %w", err)
		}
		session = identity.Session{Token: sessionToken, CSRFToken: csrfToken, Actor: actor, ExpiresAt: expiresAt}
		return nil
	})
	if err != nil {
		return identity.Session{}, fmt.Errorf("persist local Identity login: %w", err)
	}
	return session, nil
}

// Logout deletes the session identified by token.
func (service *service) Logout(ctx context.Context, token string) error {
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
		if _, err := executor.ExecContext(txCtx, `DELETE FROM modary_identity_session WHERE token_hash = ?`, credentialHash(token)); err != nil {
			return fmt.Errorf("delete local Identity session: %w", err)
		}
		return nil
	})
}

// Session resolves one active, unexpired local session.
func (service *service) Session(ctx context.Context, token string) (identity.Session, error) {
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
	arguments := storedActorLimitArguments(maxStoredCSRFTokenBytes, maxStoredCanonicalTimeBytes, credentialHash(token))
	destinations := append(storedActor.scanTargets(), &csrfToken, &expiresText)
	err = executor.QueryRowContext(ctx, `SELECT `+boundedActorProjection+`,
				CASE WHEN typeof(s.csrf_token) = 'text' AND length(CAST(s.csrf_token AS BLOB)) <= ? THEN s.csrf_token END,
				CASE WHEN typeof(s.expires_at) = 'text' AND length(CAST(s.expires_at AS BLOB)) <= ? THEN s.expires_at END
			FROM modary_identity_session s
			JOIN modary_identity_principal p ON p.actor_id = s.actor_id
			WHERE s.token_hash = ? AND p.active = 1`, arguments...).Scan(destinations...)
	if errors.Is(err, sql.ErrNoRows) {
		return identity.Session{}, ErrSessionInvalid
	}
	if err != nil {
		return identity.Session{}, fmt.Errorf("resolve local Identity session: %w", err)
	}
	if !csrfToken.Valid || !expiresText.Valid {
		return identity.Session{}, fmt.Errorf("decode local Identity session: stored session fields are oversized or not text")
	}
	expiresAt, err := parseSQLiteTime(expiresText.String)
	if err != nil {
		return identity.Session{}, fmt.Errorf("decode local Identity session expiry: %w", err)
	}
	if !service.now().Before(expiresAt) {
		_ = service.control.WithinTransaction(ctx, func(txCtx context.Context) error {
			executor, executorErr := service.executor(txCtx)
			if executorErr != nil {
				return executorErr
			}
			_, executorErr = executor.ExecContext(txCtx, `DELETE FROM modary_identity_session WHERE token_hash = ?`, credentialHash(token))
			return executorErr
		})
		return identity.Session{}, ErrSessionInvalid
	}
	if !validLowerHex(csrfToken.String, maxStoredCSRFTokenBytes) {
		return identity.Session{}, fmt.Errorf("decode local Identity session: CSRF token is invalid")
	}
	actor, err := storedActor.decode()
	if err != nil {
		return identity.Session{}, fmt.Errorf("decode local Identity session actor: %w", err)
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
	err = executor.QueryRowContext(ctx, `SELECT `+boundedActorProjection+`
			FROM modary_identity_bearer b
			JOIN modary_identity_principal p ON p.actor_id = b.actor_id
			WHERE b.token_hash = ? AND b.active = 1 AND p.active = 1`,
		storedActorLimitArguments(credentialHash(token))...).Scan(storedActor.scanTargets()...)
	if errors.Is(err, sql.ErrNoRows) {
		return identity.Actor{}, ErrAuthenticationFailed
	}
	if err != nil {
		return identity.Actor{}, fmt.Errorf("authenticate local Identity bearer token: %w", err)
	}
	actor, err := storedActor.decode()
	if err != nil {
		return identity.Actor{}, fmt.Errorf("decode local Identity bearer actor: %w", err)
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
		now := formatSQLiteTime(service.now())
		for _, user := range options.Users {
			var existingType, existingScopeKind, existingScopeID sql.NullString
			principalErr := tx.QueryRowContext(txCtx, `
					SELECT
						CASE WHEN typeof(actor_type) = 'text' AND length(CAST(actor_type AS BLOB)) <= ? THEN actor_type END,
						CASE WHEN typeof(scope_kind) = 'text' AND length(CAST(scope_kind AS BLOB)) <= ? THEN scope_kind END,
						CASE WHEN typeof(scope_id) = 'text' AND length(CAST(scope_id AS BLOB)) <= ? THEN scope_id END
					FROM modary_identity_principal WHERE actor_id = ?`, maxStoredActorTypeBytes,
				maxStoredScopeKindBytes, maxStoredScopeIDBytes, user.ActorID).Scan(
				&existingType, &existingScopeKind, &existingScopeID)
			if principalErr != nil && !errors.Is(principalErr, sql.ErrNoRows) {
				return principalErr
			}
			if principalErr == nil && (!existingType.Valid || !existingScopeKind.Valid || !existingScopeID.Valid) {
				return fmt.Errorf("stored local Identity principal contains oversized or non-text security context")
			}
			securityContextChanged := principalErr == nil &&
				(existingType.String != user.ActorType || existingScopeKind.String != user.Scope.Kind || existingScopeID.String != user.Scope.ID)
			var existingHash sql.NullString
			existingErr := tx.QueryRowContext(txCtx, `
				SELECT CASE WHEN typeof(password_hash) = 'text' AND length(CAST(password_hash AS BLOB)) <= ? THEN password_hash END
				FROM modary_identity_password WHERE actor_id = ?`, passwordHashEncodedBytes, user.ActorID).Scan(&existingHash)
			passwordMatches, verifyErr := service.verifyCredential(txCtx, existingHash.String, user.Password)
			if verifyErr != nil {
				return verifyErr
			}
			if existingErr == nil {
				_, _, _, _, _, wellFormed := parsePasswordHash(existingHash.String)
				if !existingHash.Valid || !wellFormed {
					return fmt.Errorf("stored local Identity password credential is invalid")
				}
			}
			passwordChanged := !passwordMatches
			encoded := existingHash.String
			if passwordChanged {
				encoded, err = service.hashPassword(user.Password)
				if err != nil {
					return err
				}
			}
			if existingErr != nil && !errors.Is(existingErr, sql.ErrNoRows) {
				return existingErr
			}
			if _, err := tx.ExecContext(txCtx, `
				INSERT INTO modary_identity_principal
				(actor_id, actor_type, display_name, scope_kind, scope_id, active, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, 1, ?, ?)
				ON CONFLICT(actor_id) DO UPDATE SET actor_type = excluded.actor_type,
					display_name = excluded.display_name, scope_kind = excluded.scope_kind,
					scope_id = excluded.scope_id, active = 1, updated_at = excluded.updated_at`,
				user.ActorID, user.ActorType, user.DisplayName, user.Scope.Kind, user.Scope.ID, now, now); err != nil {
				return err
			}
			if _, err := tx.ExecContext(txCtx, `
				INSERT INTO modary_identity_password (username, actor_id, password_hash, updated_at)
				VALUES (?, ?, ?, ?)
				ON CONFLICT(actor_id) DO UPDATE SET username = excluded.username,
					password_hash = excluded.password_hash, updated_at = excluded.updated_at`,
				user.Username, user.ActorID, encoded, now); err != nil {
				return err
			}
			if (passwordChanged && existingErr == nil) || securityContextChanged {
				if _, err := tx.ExecContext(txCtx, `DELETE FROM modary_identity_session WHERE actor_id = ?`, user.ActorID); err != nil {
					return err
				}
			}
			if securityContextChanged {
				if _, err := tx.ExecContext(txCtx, `UPDATE modary_identity_bearer SET active = 0, updated_at = ? WHERE actor_id = ?`, now, user.ActorID); err != nil {
					return err
				}
			}
		}
		for _, token := range options.BearerTokens {
			if _, err := tx.ExecContext(txCtx, `
				INSERT INTO modary_identity_bearer
				(token_id, token_hash, actor_id, active, created_at, updated_at)
				VALUES (?, ?, ?, 1, ?, ?)
				ON CONFLICT(token_id) DO UPDATE SET token_hash = excluded.token_hash,
					actor_id = excluded.actor_id, active = 1, updated_at = excluded.updated_at`,
				token.TokenID, credentialHash(token.Token), token.ActorID, now, now); err != nil {
				return err
			}
		}
		for _, actorID := range options.RevokedActorIDs {
			if _, err := tx.ExecContext(txCtx, `UPDATE modary_identity_principal SET active = 0, updated_at = ? WHERE actor_id = ?`, now, actorID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(txCtx, `DELETE FROM modary_identity_session WHERE actor_id = ?`, actorID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(txCtx, `UPDATE modary_identity_bearer SET active = 0, updated_at = ? WHERE actor_id = ?`, now, actorID); err != nil {
				return err
			}
		}
		for _, tokenID := range options.RevokedTokenIDs {
			if _, err := tx.ExecContext(txCtx, `UPDATE modary_identity_bearer SET active = 0, updated_at = ? WHERE token_id = ?`, now, tokenID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (service *service) executor(ctx context.Context) (database.Executor, error) {
	if service == nil || service.control == nil {
		return nil, fmt.Errorf("local Identity database is unavailable")
	}
	return service.control.Executor(ctx)
}

func loadActor(ctx context.Context, executor database.Executor, actorID string) (identity.Actor, error) {
	var storedActor storedActorColumns
	err := executor.QueryRowContext(ctx, `SELECT `+boundedActorProjection+`
			FROM modary_identity_principal p WHERE p.actor_id = ? AND p.active = 1`,
		storedActorLimitArguments(actorID)...).Scan(storedActor.scanTargets()...)
	if err != nil {
		return identity.Actor{}, err
	}
	return storedActor.decode()
}

type storedActorColumns struct {
	id, actorType, displayName, scopeKind, scopeID sql.NullString
}

func (stored *storedActorColumns) scanTargets() []any {
	return []any{&stored.id, &stored.actorType, &stored.displayName, &stored.scopeKind, &stored.scopeID}
}

func (stored storedActorColumns) decode() (identity.Actor, error) {
	if !stored.id.Valid || !stored.actorType.Valid || !stored.displayName.Valid || !stored.scopeKind.Valid || !stored.scopeID.Valid {
		return identity.Actor{}, fmt.Errorf("stored actor contains oversized or non-text fields")
	}
	actor := identity.Actor{
		ID: stored.id.String, Type: stored.actorType.String, DisplayName: stored.displayName.String,
		Scope: scope.Execution{Kind: stored.scopeKind.String, ID: stored.scopeID.String},
	}
	if err := validateStoredActor(actor); err != nil {
		return identity.Actor{}, err
	}
	return actor, nil
}

func storedActorLimitArguments(trailing ...any) []any {
	arguments := []any{
		maxStoredActorIDBytes,
		maxStoredActorTypeBytes,
		maxStoredDisplayNameBytes,
		maxStoredScopeKindBytes,
		maxStoredScopeIDBytes,
	}
	return append(arguments, trailing...)
}

func validateStoredActor(actor identity.Actor) error {
	if err := actor.Scope.Validate(); err != nil {
		return fmt.Errorf("stored actor scope is invalid: %w", err)
	}
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

func formatSQLiteTime(value time.Time) string {
	return value.UTC().Format(sqliteTimeFormat)
}

func parseSQLiteTime(value string) (time.Time, error) {
	parsed, err := time.Parse(sqliteTimeFormat, value)
	if err != nil || value != formatSQLiteTime(parsed) {
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

var _ identity.Authenticator = (*service)(nil)
var _ identity.TokenAuthenticator = (*service)(nil)
