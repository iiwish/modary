// Package project is the Counter consumer's single composition root. Both the
// application and project tool import Definition from this package.
package project

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"example.com/modary-counter-consumer/internal/ui"
	"example.com/modary-counter-consumer/modules/counter"
	"example.com/modary-counter-consumer/modules/systemclock"
	"github.com/iiwish/modary/appcmd"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/components/governedpostgres"
	"github.com/iiwish/modary/components/postgres/identitystore"
	"github.com/iiwish/modary/components/postgres/rbac"
	"github.com/iiwish/modary/components/postgres/sqlaudit"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/processkit"
	"github.com/iiwish/modary/scope"
	"github.com/iiwish/modary/transport/httpapi"
)

const (
	// DefaultDatabaseURL is for local development only. Deployments supply
	// MODARY_DATABASE_URL through their secret-management boundary.
	DefaultDatabaseURL       = "postgres://postgres:postgres@127.0.0.1:5432/modary_counter?sslmode=disable"
	DefaultApplicationSchema = "counter_app"
	DefaultQueueSchema       = "counter_queue"

	PrimaryActorID       = "counter-operator"
	PrimaryUsername      = "operator"
	PrimaryPassword      = "counter-passphrase-2026"
	PrimaryBearerToken   = "counter-primary-bearer-token-000000000001"
	SecondaryActorID     = "counter-reviewer"
	SecondaryUsername    = "reviewer"
	SecondaryPassword    = "reviewer-passphrase-2026"
	SecondaryBearerToken = "counter-secondary-bearer-token-0000000002"
)

var (
	// PrimaryScope and SecondaryScope prove that business state, authorization,
	// plans, idempotency, and audit all retain the complete execution scope.
	PrimaryScope   = scope.Must("account", "north")
	SecondaryScope = scope.Must("account", "south")
)

// Config contains the process inputs needed to assemble this consumer.
type Config struct {
	DatabaseURL       string
	ApplicationSchema string
	QueueSchema       string
}

// DefaultConfig returns the explicit local-development configuration.
func DefaultConfig() Config {
	url := os.Getenv("MODARY_DATABASE_URL")
	if url == "" {
		url = DefaultDatabaseURL
	}
	return Config{
		DatabaseURL: url, ApplicationSchema: DefaultApplicationSchema, QueueSchema: DefaultQueueSchema,
	}
}

// ApplicationMetadata returns the pure command identity shared by appcmd and
// every assembled Definition.
func ApplicationMetadata() appkit.Metadata {
	return appkit.Metadata{
		ID:      "counter-console",
		Name:    "Counter Console",
		Version: "0.1.0",
	}
}

// Definition is the fallible composition provider supplied to both appcmd and
// projecttool.
func Definition() (appkit.Definition, error) {
	return NewDefinition(DefaultConfig())
}

// NewDefinition assembles all official adapters and the consumer feature using
// explicit typed options. It performs no filesystem, database, migration,
// handler-construction, password-hashing, or random operation.
func NewDefinition(config Config) (appkit.Definition, error) {
	postgresModule, err := governedpostgres.Module(governedpostgres.Options{
		URL: config.DatabaseURL, ApplicationSchema: config.ApplicationSchema, QueueSchema: config.QueueSchema,
	})
	if err != nil {
		return appkit.Definition{}, fmt.Errorf("configure PostgreSQL: %w", err)
	}
	identityModule, err := identitystore.Module(identitystore.Options{
		Principals: []identitystore.Principal{
			{
				ActorID: PrimaryActorID, ActorType: "user", DisplayName: "Counter Operator",
			},
			{
				ActorID: SecondaryActorID, ActorType: "user", DisplayName: "Counter Reviewer",
			},
		},
		PasswordCredentials: []identitystore.PasswordCredential{
			{ActorID: PrimaryActorID, Username: PrimaryUsername, Password: PrimaryPassword},
			{ActorID: SecondaryActorID, Username: SecondaryUsername, Password: SecondaryPassword},
		},
		BearerTokens: []identitystore.BearerToken{
			{TokenID: "primary-cli", ActorID: PrimaryActorID, Token: PrimaryBearerToken},
			{TokenID: "secondary-cli", ActorID: SecondaryActorID, Token: SecondaryBearerToken},
		},
	})
	if err != nil {
		return appkit.Definition{}, fmt.Errorf("configure local Identity: %w", err)
	}
	rbacModule, err := rbac.Module(rbac.Options{
		Roles: []rbac.Role{{
			ID:          "counter-editor",
			Permissions: []string{counter.Permission},
			MaxRows:     1,
		}},
		Bindings: []rbac.Binding{
			{
				ActorID: PrimaryActorID, ActorType: "user",
				Scope: PrimaryScope, RoleID: "counter-editor",
			},
			{
				ActorID: SecondaryActorID, ActorType: "user",
				Scope: SecondaryScope, RoleID: "counter-editor",
			},
		},
	})
	if err != nil {
		return appkit.Definition{}, fmt.Errorf("configure RBAC: %w", err)
	}
	counterModule, err := counter.Module()
	if err != nil {
		return appkit.Definition{}, fmt.Errorf("configure Counter: %w", err)
	}
	return appkit.Definition{
		Metadata: ApplicationMetadata(),
		Modules: []module.Registration{
			postgresModule,
			identityModule,
			rbacModule,
			sqlaudit.Module(sqlaudit.Options{}),
			systemclock.Module(),
			counterModule,
		},
	}, nil
}

// NewHTTPHandler constructs every route explicitly from public framework
// handlers and consumer-owned assets.
func NewHTTPHandler(ctx context.Context, application *appkit.Application, process *processkit.Manager) (http.Handler, error) {
	if ctx == nil {
		return nil, fmt.Errorf("construct HTTP handler: context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if process == nil {
		return nil, fmt.Errorf("construct HTTP handler: process manager is required")
	}
	resolveScope := func(_ *http.Request, actor identity.Actor) (scope.Execution, error) {
		switch actor.ID {
		case PrimaryActorID:
			return PrimaryScope, nil
		case SecondaryActorID:
			return SecondaryScope, nil
		default:
			return scope.Execution{}, fmt.Errorf("actor has no configured execution scope")
		}
	}
	api, err := httpapi.NewAPI(application, httpapi.APIOptions{
		AllowInsecureCookie: true, EnablePasswordLogin: true, ResolveScope: resolveScope,
	})
	if err != nil {
		return nil, fmt.Errorf("construct Action API: %w", err)
	}
	mcp, err := httpapi.NewMCP(application, httpapi.MCPOptions{ResolveScope: resolveScope})
	if err != nil {
		return nil, fmt.Errorf("construct MCP handler: %w", err)
	}
	spa, err := httpapi.NewSPA(ui.Assets(), httpapi.SPAOptions{})
	if err != nil {
		return nil, fmt.Errorf("construct static UI handler: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/livez", process.LivenessHandler())
	mux.Handle("/readyz", process.ReadinessHandler())
	mux.Handle("/api/", api)
	mux.Handle("/mcp", mcp)
	mux.Handle("/", spa)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return mux, nil
}

// CommandOptions installs the consumer-owned HTTP composition into appcmd.
func CommandOptions() (appcmd.Options, error) {
	var application *appkit.Application
	process, err := processkit.New(processkit.Options{Checks: []processkit.Check{{
		Name: "database",
		Run: func(ctx context.Context) error {
			if application == nil || !application.Ready() {
				return appkit.ErrApplicationUnavailable
			}
			store, err := application.Database()
			if err != nil {
				return err
			}
			var value int
			return store.QueryRowContext(ctx, "SELECT 1").Scan(&value)
		},
	}}})
	if err != nil {
		return appcmd.Options{}, fmt.Errorf("configure process manager: %w", err)
	}
	return appcmd.Options{
		Metadata: ApplicationMetadata(),
		Process:  process,
		Handler: func(ctx context.Context, started *appkit.Application) (http.Handler, error) {
			application = started
			return NewHTTPHandler(ctx, started, process)
		},
	}, nil
}
