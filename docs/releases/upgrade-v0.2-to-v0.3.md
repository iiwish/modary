# Upgrade Guide: v0.2 Alpha 1 To v0.3 Alpha 1

v0.3 deliberately breaks the v0.2 identity boundary. A principal is stable
identity, not a product workspace or tenant. Authorization receives execution
scope independently and evaluates the exact actor/type/scope binding.

## Actor And Principal Changes

Remove `Scope` from every `identity.Actor` and
`identitystore.Principal` literal. Keep product scope in consumer composition,
routing, request context, or another explicit policy source.

```go
actor := identity.Actor{ID: "person-1", Type: "human"}
executionScope := scope.Must("workspace", "default")

decision, err := authorizer.Authorize(ctx, authz.Request{
    Actor: actor, Scope: executionScope,
    OperationID: "records.list", Permission: "records.list",
    Phase: authz.PhaseIntent,
})
```

The Local Identity forward migration removes `scope_kind` and `scope_id` from
`modary_identity_principal`. Back up and restore-test PostgreSQL before applying
the migration. Do not modify the published `0001_identity.sql` migration.

## Password And Session Contracts

Replace the combined `identity.Authenticator` contract with the narrow
facades:

- `application.Passwords().AuthenticatePassword` verifies a credential and
  returns an `identity.Authentication` freshness envelope;
- `application.Sessions().CreateSession` issues a revocable application
  session after revalidating that envelope;
- `ResolveSession` and `RevokeSession` replace `Session` and `Logout`;
- bearer authentication remains independent through `application.Tokens()`.

Modules that provide local login declare and publish
`module.CapabilityPasswords`, `module.PasswordAuthenticator()`,
`module.CapabilitySessions`, and `module.SessionManager()` separately.

## Browser Login Selection

Password login is opt-in at the HTTP boundary:

```go
sessions, err := sessionhttp.New(application, sessionhttp.Options{
    EnablePasswordLogin: true,
})
```

Leave `EnablePasswordLogin` false for redirect-based identity such as OIDC.
Session restoration, logout, protected handlers, and CSRF middleware remain
available without the password capability.

## Execution Scope Resolution

`httpapi.NewAPI` and `httpapi.NewMCP` require a `ScopeResolver`. The resolver
must return a validated consumer-owned scope for the authenticated request. It
must not trust an arbitrary identity claim as a role or scope grant.

CLI Action execution requires both `--scope-kind` and `--scope-id`:

```bash
app action run records.rebuild \
  --token-file token --input request.json \
  --scope-kind workspace --scope-id default
```

## Process And Deployment

Generated applications expose `/livez` for local process progress and
`/readyz` for lifecycle plus bounded selected-dependency checks. Replace
orchestration references to a combined health endpoint. Close admission before
graceful drain, and run schema changes with the generated `migrate` command
when serving instances must not migrate concurrently.

The v0.3 Dockerfile and Compose files are consumer-owned references. Review the
generated diff rather than overwriting an existing application. Build identity
is injected through linker values; the final image runs as non-root and does
not include frontend tooling.

## Optional OIDC And OpenTelemetry

Use `components/oidc` and `oidchttp.Contribution` when replacing local password
login. Provision exact issuer/subject mappings to local principals, then retain
RBAC as the sole role and scope authority. Do not map email, groups, roles, or
tenant claims implicitly.

Use `components/otel` only in compositions that require OTLP/HTTP traces and
metrics. Configure a collector base endpoint, bounded resource identity, and
secret headers. The component owns its providers and does not install global
OpenTelemetry state. Removing it must also remove its configuration, readiness
check, and dependency.

## Verification

After changing composition and transports, run unit, race, PostgreSQL,
copied-out Profile, container, OIDC-provider, OTLP-collector, and
dependency-absence gates. Add a focused authorization test proving that one
actor can access two bound scopes and that an unbound third scope remains
denied. Rehearse the forward identity migration against a restored backup
before production rollout.
