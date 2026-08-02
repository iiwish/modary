package rbac

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/databasecontrol"
)

// ErrContextRequired reports a nil authorization context.
var ErrContextRequired = errors.New("RBAC context is required")

const (
	maxStoredPolicyTokenBytes = 127
	maxEffectivePolicyRows    = 4096
)

type authorizer struct{ control databasecontrol.Control }

// Authorize returns the effective durable RBAC grant and constraints.
func (authorizer *authorizer) Authorize(ctx context.Context, request authz.Request) (authz.Decision, error) {
	if ctx == nil {
		return authz.Decision{}, ErrContextRequired
	}
	if authorizer == nil || authorizer.control == nil {
		return authz.Decision{}, fmt.Errorf("RBAC Authorizer is unavailable")
	}
	if !action.ValidIdentifier(request.ActionID) ||
		(request.Phase != authz.PhaseIntent && request.Phase != authz.PhaseImpact) ||
		request.Impact.Rows < 0 ||
		(request.Phase == authz.PhaseIntent && (request.Impact.Rows != 0 || len(request.Impact.Resources) != 0)) ||
		request.Scope.Validate() != nil || request.Actor.Scope != request.Scope ||
		identity.ValidateActor(request.Actor) != nil ||
		validatePermission(request.Permission) != nil {
		return authz.Decision{
			Reason:             "authorization request is invalid",
			RequiredPermission: request.Permission,
			Fingerprint:        emptyFingerprint(request),
		}, nil
	}
	policy, err := authorizer.loadPolicy(ctx, request)
	if err != nil {
		return authz.Decision{}, fmt.Errorf("resolve RBAC policy: %w", err)
	}
	decision := authz.Decision{
		RequiredPermission: request.Permission,
		Fingerprint:        policy.fingerprint,
	}
	grantingLimits := make([]int, 0)
	for _, role := range policy.roles {
		if role.grants(request.Permission) {
			grantingLimits = append(grantingLimits, role.maxRows)
		}
	}
	if len(grantingLimits) == 0 {
		decision.Reason = "required permission is not granted"
		return decision, nil
	}
	decision.Allowed = true
	decision.Constraints.MaxRows = effectiveRowLimit(grantingLimits)
	return decision, nil
}

type storedRole struct {
	id          string
	maxRows     int
	permissions []string
}

func (role storedRole) grants(permission string) bool {
	index := sort.SearchStrings(role.permissions, permission)
	return index < len(role.permissions) && role.permissions[index] == permission
}

type effectivePolicy struct {
	roles       []storedRole
	fingerprint string
}

func (authorizer *authorizer) loadPolicy(ctx context.Context, request authz.Request) (effectivePolicy, error) {
	executor, err := authorizer.control.Executor(ctx)
	if err != nil {
		return effectivePolicy{}, fmt.Errorf("RBAC policy executor is unavailable: %w", err)
	}
	rows, err := executor.QueryContext(ctx, `
		SELECT
				r.role_id,
				r.max_rows,
				p.permission IS NULL,
				p.permission
		FROM modary_rbac_binding b
		JOIN modary_rbac_role r ON r.role_id = b.role_id
		LEFT JOIN modary_rbac_role_permission p ON p.role_id = r.role_id
			WHERE b.actor_id = $1 AND b.actor_type = $2 AND b.scope_kind = $3 AND b.scope_id = $4
			  AND b.active = TRUE AND r.active = TRUE
			ORDER BY 1, 4
			LIMIT $5`, request.Actor.ID, request.Actor.Type, request.Scope.Kind, request.Scope.ID, maxEffectivePolicyRows+1)
	if err != nil {
		return effectivePolicy{}, err
	}
	defer rows.Close()
	roles := make([]storedRole, 0)
	rowCount := 0
	for rows.Next() {
		if rowCount == maxEffectivePolicyRows {
			return effectivePolicy{}, fmt.Errorf("stored RBAC policy exceeds %d effective rows", maxEffectivePolicyRows)
		}
		rowCount++
		var roleID sql.NullString
		var maxRows sql.NullInt64
		var permissionMissing bool
		var permission sql.NullString
		if err := rows.Scan(&roleID, &maxRows, &permissionMissing, &permission); err != nil {
			return effectivePolicy{}, err
		}
		boundedMaxRows := int(maxRows.Int64)
		if !roleID.Valid || !maxRows.Valid ||
			maxRows.Int64 < 0 || int64(boundedMaxRows) != maxRows.Int64 ||
			validatePolicyIdentifier("stored role id", roleID.String) != nil {
			return effectivePolicy{}, fmt.Errorf("stored RBAC role is invalid")
		}
		if len(roles) == 0 || roles[len(roles)-1].id != roleID.String {
			roles = append(roles, storedRole{id: roleID.String, maxRows: boundedMaxRows})
		} else if roles[len(roles)-1].maxRows != boundedMaxRows {
			return effectivePolicy{}, fmt.Errorf("stored RBAC role %s has inconsistent constraints", roleID.String)
		}
		if !permissionMissing {
			if !permission.Valid {
				return effectivePolicy{}, fmt.Errorf("stored RBAC permission is oversized or not text")
			}
			if err := validatePermission(permission.String); err != nil {
				return effectivePolicy{}, err
			}
			current := &roles[len(roles)-1]
			if len(current.permissions) > 0 && current.permissions[len(current.permissions)-1] == permission.String {
				return effectivePolicy{}, fmt.Errorf("stored RBAC role %s contains duplicate permission %s", roleID.String, permission.String)
			}
			current.permissions = append(current.permissions, permission.String)
		} else if permission.Valid {
			return effectivePolicy{}, fmt.Errorf("stored RBAC permission projection is invalid")
		}
	}
	if err := rows.Err(); err != nil {
		return effectivePolicy{}, err
	}
	fingerprint, err := policyFingerprint(request, roles)
	if err != nil {
		return effectivePolicy{}, err
	}
	return effectivePolicy{roles: roles, fingerprint: fingerprint}, nil
}

func effectiveRowLimit(limits []int) int {
	maximum := 0
	for _, limit := range limits {
		if limit == 0 {
			return 0
		}
		if limit > maximum {
			maximum = limit
		}
	}
	return maximum
}

func policyFingerprint(request authz.Request, roles []storedRole) (string, error) {
	type fingerprintRole struct {
		ID          string   `json:"id"`
		MaxRows     int      `json:"max_rows"`
		Permissions []string `json:"permissions"`
	}
	material := struct {
		ActorID   string            `json:"actor_id"`
		ActorType string            `json:"actor_type"`
		ScopeKind string            `json:"scope_kind"`
		ScopeID   string            `json:"scope_id"`
		Roles     []fingerprintRole `json:"roles"`
	}{ActorID: request.Actor.ID, ActorType: request.Actor.Type, ScopeKind: request.Scope.Kind, ScopeID: request.Scope.ID,
		Roles: make([]fingerprintRole, 0, len(roles))}
	for _, role := range roles {
		material.Roles = append(material.Roles, fingerprintRole{
			ID: role.id, MaxRows: role.maxRows, Permissions: append([]string(nil), role.permissions...),
		})
	}
	encoded, err := json.Marshal(material)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(hash[:]), nil
}

func emptyFingerprint(request authz.Request) string {
	fingerprint, _ := policyFingerprint(request, nil)
	return fingerprint
}

func (authorizer *authorizer) provision(ctx context.Context, options Options) error {
	return authorizer.control.WithinTransaction(ctx, func(txCtx context.Context) error {
		executor, err := authorizer.control.Executor(txCtx)
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for _, role := range options.Roles {
			if _, err := executor.ExecContext(txCtx, `
				INSERT INTO modary_rbac_role (role_id, max_rows, active, created_at, updated_at)
					VALUES ($1, $2, TRUE, $3, $4)
				ON CONFLICT(role_id) DO UPDATE SET max_rows = excluded.max_rows,
						active = TRUE, updated_at = excluded.updated_at`, role.ID, role.MaxRows, now, now); err != nil {
				return err
			}
			if _, err := executor.ExecContext(txCtx, `DELETE FROM modary_rbac_role_permission WHERE role_id = $1`, role.ID); err != nil {
				return err
			}
			for _, permission := range role.Permissions {
				if _, err := executor.ExecContext(txCtx, `INSERT INTO modary_rbac_role_permission (role_id, permission) VALUES ($1, $2)`, role.ID, permission); err != nil {
					return err
				}
			}
		}
		for _, binding := range options.Bindings {
			if _, err := executor.ExecContext(txCtx, `
				INSERT INTO modary_rbac_binding
			(actor_id, actor_type, scope_kind, scope_id, role_id, active, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, TRUE, $6, $7)
			ON CONFLICT(actor_id, actor_type, scope_kind, scope_id, role_id)
				DO UPDATE SET active = TRUE, updated_at = excluded.updated_at`, binding.ActorID,
				binding.ActorType, binding.Scope.Kind, binding.Scope.ID, binding.RoleID, now, now); err != nil {
				return err
			}
		}
		for _, roleID := range options.RevokedRoleIDs {
			if _, err := executor.ExecContext(txCtx, `UPDATE modary_rbac_role SET active = FALSE, updated_at = $1 WHERE role_id = $2`, now, roleID); err != nil {
				return err
			}
			if _, err := executor.ExecContext(txCtx, `UPDATE modary_rbac_binding SET active = FALSE, updated_at = $1 WHERE role_id = $2`, now, roleID); err != nil {
				return err
			}
		}
		for _, binding := range options.RevokedBindings {
			if _, err := executor.ExecContext(txCtx, `
				UPDATE modary_rbac_binding SET active = FALSE, updated_at = $1
				WHERE actor_id = $2 AND actor_type = $3 AND scope_kind = $4 AND scope_id = $5 AND role_id = $6`,
				now, binding.ActorID, binding.ActorType, binding.Scope.Kind, binding.Scope.ID, binding.RoleID); err != nil {
				return err
			}
		}
		return nil
	})
}

var _ authz.Authorizer = (*authorizer)(nil)
