package authz_basic

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"sort"
	"time"

	"modary/core/action"
	"modary/core/authz"
	"modary/core/config"
	"modary/core/database"
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
	if err := database.ApplyMigrations(ctx, db, "authz-basic", sub); err != nil {
		return err
	}
	if err := seedPolicy(ctx, db, cfg.AgentToken); err != nil {
		return err
	}
	return host.Provide(module.ServiceAuthorizer, authz.Authorizer(Authorizer{}))
}

type Authorizer struct{}

func (Authorizer) Authorize(_ context.Context, request authz.Request) (authz.Decision, error) {
	fingerprint := actorFingerprint(request)
	decision := authz.Decision{
		RequiredPermission: request.Permission,
		Fingerprint:        fingerprint,
		Constraints:        authz.Constraints{MaxRows: request.Actor.MaxRows},
	}
	if request.Actor.WorkspaceID == "" || request.Actor.WorkspaceID != request.WorkspaceID {
		decision.Reason = "actor is outside the requested workspace"
		return decision, nil
	}
	if request.Actor.Type == "agent" {
		if request.Actor.ExpiresAtUnix <= time.Now().UTC().Unix() {
			decision.Reason = "agent grant expired"
			return decision, nil
		}
		if !slices.Contains(request.Actor.AllowedActions, request.ActionID) {
			decision.Reason = "action is not in the agent grant allowlist"
			return decision, nil
		}
	}
	if !slices.Contains(request.Actor.Permissions, request.Permission) {
		decision.Reason = "required permission is not granted"
		return decision, nil
	}
	if request.Phase == authz.PhaseImpact && request.Actor.Type == "agent" && request.Actor.MaxRows > 0 && request.Impact.Rows > request.Actor.MaxRows {
		decision.Code = action.CodeLimitExceeded
		decision.Reason = fmt.Sprintf("planned impact of %d rows exceeds agent limit of %d", request.Impact.Rows, request.Actor.MaxRows)
		return decision, nil
	}
	decision.Allowed = true
	return decision, nil
}

func actorFingerprint(request authz.Request) string {
	roles := append([]string(nil), request.Actor.Roles...)
	permissions := append([]string(nil), request.Actor.Permissions...)
	actions := append([]string(nil), request.Actor.AllowedActions...)
	sort.Strings(roles)
	sort.Strings(permissions)
	sort.Strings(actions)
	data, _ := json.Marshal(struct {
		ID           string
		WorkspaceID  string
		Roles        []string
		Permissions  []string
		GrantID      string
		GrantVersion string
		Actions      []string
		MaxRows      int
	}{request.Actor.ID, request.Actor.WorkspaceID, roles, permissions, request.Actor.GrantID, request.Actor.GrantVersion, actions, request.Actor.MaxRows})
	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:])
}

func seedPolicy(ctx context.Context, db *sql.DB, agentToken string) error {
	rolePermissions := map[string][]string{
		"rulary_author": {
			"rulary.ruleset.create", "rulary.ruleset.edit", "rulary.ruleset.preview",
		},
		"rulary_publisher": {
			"rulary.ruleset.create", "rulary.ruleset.edit", "rulary.ruleset.preview", "rulary.ruleset.publish",
		},
		"rulary_operator": {
			"rulary.ruleset.preview", "rulary.run.execute",
		},
		"rulary_auditor": {"audit.read"},
		"workspace_admin": {
			"rulary.ruleset.create", "rulary.ruleset.edit", "rulary.ruleset.preview", "rulary.ruleset.publish",
			"rulary.run.execute", "audit.read",
		},
	}
	for role, permissions := range rolePermissions {
		for _, permission := range permissions {
			if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO modary_role_permission (role_id, permission) VALUES (?, ?)`, role, permission); err != nil {
				return err
			}
		}
	}
	bindings := []struct{ user, role string }{
		{"user_author", "rulary_author"},
		{"user_publisher", "rulary_publisher"},
		{"user_operator", "rulary_operator"},
		{"user_auditor", "rulary_auditor"},
		{"user_admin", "workspace_admin"},
	}
	for _, binding := range bindings {
		if _, err := db.ExecContext(ctx, `
			INSERT OR IGNORE INTO modary_role_binding (user_id, workspace_id, role_id)
			VALUES (?, 'ws_default', ?)`, binding.user, binding.role); err != nil {
			return err
		}
	}
	actions, _ := json.Marshal([]string{
		"rulary.ruleset.validate",
		"rulary.ruleset.preview",
		"rulary.run.execute",
	})
	expires := time.Date(2036, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	hash := sha256.Sum256([]byte(agentToken))
	_, err := db.ExecContext(ctx, `
		INSERT INTO modary_agent_grant
		(grant_id, agent_id, display_name, token_hash, delegated_by, workspace_id, actions_json, max_rows, expires_at, grant_version, active, created_at)
		VALUES ('grant_rulary_operator', 'agent_rulary_operator', 'Rulary Operator Agent', ?, 'user_operator',
		        'ws_default', ?, 50, ?, '1', 1, ?)
		ON CONFLICT(grant_id) DO UPDATE SET token_hash = excluded.token_hash, actions_json = excluded.actions_json,
		    max_rows = excluded.max_rows, expires_at = excluded.expires_at`,
		hex.EncodeToString(hash[:]), actions, expires, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
