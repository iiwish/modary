package identity_local

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"modary/core/config"
	"modary/core/database"
	"modary/core/identity"
	"modary/core/module"
)

//go:embed module.yaml
var manifestData []byte

//go:embed migrations/sqlite/*.sql
var migrationFiles embed.FS

func Module() module.Registration {
	return module.Registration{Manifest: module.MustParseManifest(manifestData), Install: install}
}

func install(ctx context.Context, host *module.Host) error {
	db, err := module.ServiceAs[*sql.DB](host, module.ServiceDatabase)
	if err != nil {
		return err
	}
	cfg, err := module.ServiceAs[config.Runtime](host, module.ServiceConfig)
	if err != nil {
		return err
	}
	sub, err := fs.Sub(migrationFiles, "migrations/sqlite")
	if err != nil {
		return err
	}
	if err := database.ApplyMigrations(ctx, db, "identity-local", sub); err != nil {
		return err
	}
	service := &Service{db: db}
	if err := service.seedUsers(ctx, cfg.DemoPassword); err != nil {
		return err
	}
	if err := host.Provide(module.ServiceIdentityResolver, identity.Resolver(service)); err != nil {
		return err
	}
	return host.Provide(module.ServiceIdentityStore, identity.Authenticator(service))
}

type Service struct{ db *sql.DB }

func (s *Service) Login(ctx context.Context, username, password string) (identity.Session, error) {
	var userID, displayName, workspaceID string
	var salt, expected []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id, display_name, workspace_id, password_salt, password_hash
		FROM modary_user WHERE username = ? AND active = 1`, strings.TrimSpace(username),
	).Scan(&userID, &displayName, &workspaceID, &salt, &expected)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identity.Session{}, fmt.Errorf("invalid username or password")
		}
		return identity.Session{}, err
	}
	actual := derivePassword(password, salt)
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		return identity.Session{}, fmt.Errorf("invalid username or password")
	}
	token, err := randomToken(32)
	if err != nil {
		return identity.Session{}, err
	}
	csrf, err := randomToken(24)
	if err != nil {
		return identity.Session{}, err
	}
	expires := time.Now().UTC().Add(12 * time.Hour)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO modary_session (session_id, token_hash, user_id, csrf_token, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"ses_"+token[:24], hashToken(token), userID, csrf, expires.Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return identity.Session{}, err
	}
	actor, err := s.ResolveByID(ctx, userID)
	if err != nil {
		return identity.Session{}, err
	}
	actor.DisplayName = displayName
	actor.WorkspaceID = workspaceID
	return identity.Session{Token: token, CSRFToken: csrf, Actor: actor, ExpiresAt: expires}, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM modary_session WHERE token_hash = ?`, hashToken(token))
	return err
}

func (s *Service) ResolveSession(ctx context.Context, token string) (identity.Actor, error) {
	session, err := s.Session(ctx, token)
	return session.Actor, err
}

func (s *Service) Session(ctx context.Context, token string) (identity.Session, error) {
	var userID string
	var csrf, expiresAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id, csrf_token, expires_at FROM modary_session
		WHERE token_hash = ? AND expires_at > ?`, hashToken(token), time.Now().UTC().Format(time.RFC3339Nano),
	).Scan(&userID, &csrf, &expiresAt)
	if err != nil {
		return identity.Session{}, err
	}
	actor, err := s.ResolveByID(ctx, userID)
	if err != nil {
		return identity.Session{}, err
	}
	expires, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return identity.Session{}, err
	}
	return identity.Session{Token: token, CSRFToken: csrf, Actor: actor, ExpiresAt: expires}, nil
}

func (s *Service) ResolveByID(ctx context.Context, userID string) (identity.Actor, error) {
	var actor identity.Actor
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id, display_name, workspace_id FROM modary_user
		WHERE user_id = ? AND active = 1`, userID,
	).Scan(&actor.ID, &actor.DisplayName, &actor.WorkspaceID)
	if err != nil {
		return identity.Actor{}, err
	}
	actor.Type = "user"
	roles, permissions, err := s.loadRoleData(ctx, actor.ID, actor.WorkspaceID)
	if err != nil {
		return identity.Actor{}, err
	}
	actor.Roles = roles
	actor.Permissions = permissions
	return actor, nil
}

func (s *Service) ResolveAgentToken(ctx context.Context, token string) (identity.Actor, error) {
	var actor identity.Actor
	var actionsJSON []byte
	var expiresAt string
	err := s.db.QueryRowContext(ctx, `
			SELECT grant.agent_id, grant.display_name, grant.workspace_id, grant.delegated_by,
			       grant.grant_id, grant.grant_version, grant.actions_json, grant.max_rows, grant.expires_at
			FROM modary_agent_grant grant
			JOIN modary_user delegator ON delegator.user_id = grant.delegated_by
			WHERE grant.token_hash = ? AND grant.active = 1 AND grant.expires_at > ?
			  AND delegator.active = 1 AND delegator.workspace_id = grant.workspace_id`,
		hashToken(token), time.Now().UTC().Format(time.RFC3339Nano),
	).Scan(&actor.ID, &actor.DisplayName, &actor.WorkspaceID, &actor.DelegatedBy, &actor.GrantID,
		&actor.GrantVersion, &actionsJSON, &actor.MaxRows, &expiresAt)
	if err != nil {
		return identity.Actor{}, err
	}
	actor.Type = "agent"
	if err := json.Unmarshal(actionsJSON, &actor.AllowedActions); err != nil {
		return identity.Actor{}, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return identity.Actor{}, err
	}
	actor.ExpiresAtUnix = parsed.Unix()
	roles, permissions, err := s.loadRoleData(ctx, actor.DelegatedBy, actor.WorkspaceID)
	if err != nil {
		return identity.Actor{}, err
	}
	actor.Roles = roles
	actor.Permissions = permissions
	return actor, nil
}

func (s *Service) loadRoleData(ctx context.Context, userID, workspaceID string) ([]string, []string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT rb.role_id, rp.permission
		FROM modary_role_binding rb
		JOIN modary_role_permission rp ON rp.role_id = rb.role_id
		WHERE rb.user_id = ? AND rb.workspace_id = ?
		ORDER BY rb.role_id, rp.permission`, userID, workspaceID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	defer rows.Close()
	roleSet := make(map[string]struct{})
	permissionSet := make(map[string]struct{})
	roles := make([]string, 0)
	permissions := make([]string, 0)
	for rows.Next() {
		var role, permission string
		if err := rows.Scan(&role, &permission); err != nil {
			return nil, nil, err
		}
		if _, exists := roleSet[role]; !exists {
			roleSet[role] = struct{}{}
			roles = append(roles, role)
		}
		if _, exists := permissionSet[permission]; !exists {
			permissionSet[permission] = struct{}{}
			permissions = append(permissions, permission)
		}
	}
	return roles, permissions, rows.Err()
}

func (s *Service) seedUsers(ctx context.Context, password string) error {
	type bootstrapUser struct {
		id, username, display string
	}
	users := []bootstrapUser{
		{"user_author", "author", "Rule Author"},
		{"user_publisher", "publisher", "Rule Publisher"},
		{"user_operator", "operator", "Run Operator"},
		{"user_auditor", "auditor", "Auditor"},
		{"user_admin", "admin", "Workspace Admin"},
	}
	rotateExisting := false
	var adminSalt, adminHash []byte
	err := s.db.QueryRowContext(ctx, `SELECT password_salt, password_hash FROM modary_user WHERE user_id = 'user_admin'`).Scan(&adminSalt, &adminHash)
	switch {
	case err == nil:
		rotateExisting = subtle.ConstantTimeCompare(derivePassword(password, adminSalt), adminHash) != 1
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}

	userExists := make([]bool, len(users))
	needsCredential := false
	for index, user := range users {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM modary_user WHERE user_id = ?`, user.id).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			userExists[index] = true
		}
		if count > 0 && !rotateExisting {
			continue
		}
		needsCredential = true
	}
	if !needsCredential {
		return nil
	}

	// F0 bootstrap identities intentionally share one initial credential. Derive
	// it once so password hardening does not dominate cold-start readiness.
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	hash := derivePassword(password, salt)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for index, user := range users {
		if userExists[index] && !rotateExisting {
			continue
		}
		if userExists[index] {
			if _, err := tx.ExecContext(ctx, `UPDATE modary_user SET password_salt = ?, password_hash = ? WHERE user_id = ?`, salt, hash, user.id); err != nil {
				return err
			}
		} else {
			if _, err := tx.ExecContext(ctx, `
					INSERT INTO modary_user
					(user_id, username, display_name, workspace_id, password_salt, password_hash, active, created_at)
					VALUES (?, ?, ?, 'ws_default', ?, ?, 1, ?)`,
				user.id, user.username, user.display, salt, hash, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				return err
			}
		}
	}
	if rotateExisting {
		if _, err := tx.ExecContext(ctx, `DELETE FROM modary_session`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func derivePassword(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, 2, 19*1024, 1, 32)
}

func randomToken(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
