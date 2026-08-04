// Package rbac provides an explicit, durable, default-deny RBAC Adapter.
//
// Stability: alpha. Consumers should pin an exact pre-v1 Modary version.
package rbac

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/moduleassembly"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/scope"
)

// ModuleID is the stable Module manifest and migration owner identifier.
const ModuleID = "rbac"

var policyIdentifierPattern = regexp.MustCompile(`^[a-z][a-z0-9._/-]{0,126}$`)

//go:embed migrations/postgres/*.sql
var migrationFiles embed.FS

var postgresMigrations = mustMigrationFS()

// Role grants a set of exact permissions. MaxRows zero is unbounded. When
// several roles grant the same permission, the least restrictive bound wins;
// any unbounded granting role makes the effective grant unbounded.
type Role struct {
	ID          string
	Permissions []string
	MaxRows     int
}

// Binding assigns a role to one exact actor and execution scope.
type Binding struct {
	ActorID   string
	ActorType string
	Scope     scope.Execution
	RoleID    string
}

// Options is an explicit provisioning and revocation patch. Empty Options
// installs schema and a default-deny Authorizer; omitted durable policy remains
// unchanged.
type Options struct {
	Roles           []Role
	Bindings        []Binding
	RevokedRoleIDs  []string
	RevokedBindings []Binding
}

// Module returns a pure Registration after validating and copying Options.
func Module(options Options) (module.Registration, error) {
	normalized, err := normalizeOptions(options)
	if err != nil {
		return module.Registration{}, err
	}
	return module.Registration{
		Definition: module.Definition{
			Manifest: module.Manifest{
				SchemaVersion: module.SchemaVersion,
				ID:            ModuleID,
				Version:       "0.1.0",
				Type:          module.ModuleTypeAdapter,
				Requires:      []module.Capability{module.CapabilityDatabase},
				Provides:      []module.Capability{module.CapabilityAuthorization},
			},
			Migrations: []module.MigrationSource{{Driver: "postgres", Files: postgresMigrations}},
		},
		Start: func(ctx context.Context, installation module.Scope) error {
			return start(ctx, installation, normalized)
		},
	}, nil
}

func start(ctx context.Context, installation module.Scope, options Options) error {
	if ctx == nil {
		return fmt.Errorf("RBAC start context is required")
	}
	control, err := moduleassembly.ResolveDatabaseControl(installation)
	if err != nil {
		return fmt.Errorf("resolve database control: %w", err)
	}
	service := &authorizer{control: control}
	if err := service.provision(ctx, options); err != nil {
		return fmt.Errorf("provision RBAC: %w", err)
	}
	return module.Provide(installation, module.Authorizer(), authz.Authorizer(service))
}

func normalizeOptions(options Options) (Options, error) {
	roles := append([]Role(nil), options.Roles...)
	bindings := append([]Binding(nil), options.Bindings...)
	revokedRoles := append([]string(nil), options.RevokedRoleIDs...)
	revokedBindings := append([]Binding(nil), options.RevokedBindings...)
	roleIDs := make(map[string]struct{}, len(roles))
	for index := range roles {
		role := &roles[index]
		if err := validatePolicyIdentifier("role id", role.ID); err != nil {
			return Options{}, fmt.Errorf("RBAC role %d: %w", index, err)
		}
		if role.MaxRows < 0 {
			return Options{}, fmt.Errorf("RBAC role %s max rows cannot be negative", role.ID)
		}
		if _, duplicate := roleIDs[role.ID]; duplicate {
			return Options{}, fmt.Errorf("RBAC role %q is declared more than once", role.ID)
		}
		roleIDs[role.ID] = struct{}{}
		permissions := append([]string(nil), role.Permissions...)
		sort.Strings(permissions)
		for permissionIndex, permission := range permissions {
			if err := validatePermission(permission); err != nil {
				return Options{}, fmt.Errorf("RBAC role %s permission %d: %w", role.ID, permissionIndex, err)
			}
			if permissionIndex > 0 && permission == permissions[permissionIndex-1] {
				return Options{}, fmt.Errorf("RBAC role %s permission %q is declared more than once", role.ID, permission)
			}
		}
		role.Permissions = permissions
	}
	seenBindings := make(map[string]struct{}, len(bindings))
	for index, binding := range bindings {
		key, err := validateBinding(binding)
		if err != nil {
			return Options{}, fmt.Errorf("RBAC binding %d: %w", index, err)
		}
		if _, duplicate := seenBindings[key]; duplicate {
			return Options{}, fmt.Errorf("RBAC binding %d is declared more than once", index)
		}
		seenBindings[key] = struct{}{}
	}
	seenRevokedRoles := make(map[string]struct{}, len(revokedRoles))
	for _, roleID := range revokedRoles {
		if err := validatePolicyIdentifier("revoked role id", roleID); err != nil {
			return Options{}, err
		}
		if _, duplicate := seenRevokedRoles[roleID]; duplicate {
			return Options{}, fmt.Errorf("RBAC revoked role %q is declared more than once", roleID)
		}
		if _, provisioned := roleIDs[roleID]; provisioned {
			return Options{}, fmt.Errorf("RBAC role %q cannot be provisioned and revoked together", roleID)
		}
		seenRevokedRoles[roleID] = struct{}{}
	}
	for index, binding := range bindings {
		if _, revoked := seenRevokedRoles[binding.RoleID]; revoked {
			return Options{}, fmt.Errorf("RBAC binding %d cannot target revoked role %q", index, binding.RoleID)
		}
	}
	seenRevokedBindings := make(map[string]struct{}, len(revokedBindings))
	for index, binding := range revokedBindings {
		key, err := validateBinding(binding)
		if err != nil {
			return Options{}, fmt.Errorf("RBAC revoked binding %d: %w", index, err)
		}
		if _, duplicate := seenRevokedBindings[key]; duplicate {
			return Options{}, fmt.Errorf("RBAC revoked binding %d is declared more than once", index)
		}
		if _, provisioned := seenBindings[key]; provisioned {
			return Options{}, fmt.Errorf("RBAC binding %d cannot be provisioned and revoked together", index)
		}
		seenRevokedBindings[key] = struct{}{}
	}
	options.Roles = roles
	options.Bindings = bindings
	options.RevokedRoleIDs = revokedRoles
	options.RevokedBindings = revokedBindings
	return options, nil
}

func validateBinding(binding Binding) (string, error) {
	if err := identity.ValidateActorID(binding.ActorID); err != nil {
		return "", err
	}
	if err := identity.ValidateActorType(binding.ActorType); err != nil {
		return "", err
	}
	if err := binding.Scope.Validate(); err != nil {
		return "", fmt.Errorf("scope: %w", err)
	}
	if err := validatePolicyIdentifier("role id", binding.RoleID); err != nil {
		return "", err
	}
	return binding.ActorID + "\x00" + binding.ActorType + "\x00" + binding.Scope.Key() + "\x00" + binding.RoleID, nil
}

func validatePolicyIdentifier(name, value string) error {
	if !policyIdentifierPattern.MatchString(value) {
		return fmt.Errorf("%s %q is invalid", name, value)
	}
	return nil
}

func validatePermission(value string) error {
	if !action.ValidIdentifier(value) {
		return fmt.Errorf("permission %q is invalid", value)
	}
	return nil
}

func mustMigrationFS() fs.FS {
	files, err := fs.Sub(migrationFiles, "migrations/postgres")
	if err != nil {
		panic(err)
	}
	return files
}
