# Admin Profile Tutorial

The Admin Profile is a quiet, usable back-office starting point. It combines
ordinary PostgreSQL business transactions, development Identity, RBAC,
session/CSRF handling, and a generated React work surface. It deliberately omits
River and governed Actions.

## Create

From a Modary checkout:

```bash
export MODARY_STARTER_REPLACE="$(pwd)"
go run ./cmd/modary new ../operations-admin \
  --profile admin \
  --module example.com/acme/operations-admin \
  --name "Operations Admin"
cd ../operations-admin
go mod tidy
```

## Configure PostgreSQL

Create or select one PostgreSQL database. Admin uses one application schema and
does not create a River queue schema.

```bash
export DATABASE_URL='postgres://user:password@127.0.0.1:5432/app?sslmode=disable'
export MODARY_DATABASE_SCHEMA=operations_admin
export MODARY_ADMIN_USERNAME=admin
export MODARY_ADMIN_PASSWORD='development-password'
export MODARY_ALLOW_INSECURE_COOKIE=true
```

`MODARY_ALLOW_INSECURE_COOKIE=true` is only for local HTTP. Production keeps the
Secure cookie default and terminates TLS before the application.

## Test And Run

```bash
DATABASE_URL="$DATABASE_URL" go test ./...
go run ./cmd/operations-admin
```

Open `http://127.0.0.1:8080` and sign in with the configured development
credential. The records work surface supports loading, filtering, create,
optimistic update, and delete. Backend RBAC authorizes every route; hiding a
button is never treated as authorization.

## Understand The Backend

`internal/app/application.go` selects:

- `adapters/postgresdb` for the provider-neutral `database.Store`;
- `adapters/localidentity` for explicit development principals;
- `adapters/rbac` for backend policy;
- `transport/sessionhttp` for login/current/logout and mutation middleware;
- the consumer-owned records Module and routes.

`database.Store.WithinTransaction` owns ordinary callback transactions.
Repositories cannot commit, roll back, or access a raw connection. The
governed `database.Access` path remains separate.

## Understand The Frontend

The frontend is an ordinary React 19, TypeScript, Vite, and React Router
application owned by the generated project:

- `web/src/main.tsx` only mounts the application;
- `web/src/App.tsx` composes providers, session initialization, protected
  routes, and module routes;
- `web/src/stores/` contains small typed contexts and hooks for app metadata,
  authentication, and toasts;
- `web/src/modules/index.ts` is the explicit frontend composition root;
- `web/src/modules/records/` owns its API state, screens, dialogs, and route.

The module registry is static source composition. Unselected task, audit,
Action, MCP, and marketplace surfaces contribute no route, navigation item,
state provider, or dependency. The baseline does not require a global state
library; a product can introduce one when shared product state genuinely needs
it.

The API client sends the CSRF token on mutations. Any authenticated request
that receives `401` clears local session state and returns the user to sign-in.
A `403` renders a dedicated permission state. These are usability behaviors,
not authorization: the backend authorizes every protected route.

Frontend development:

```bash
cd web
pnpm install --frozen-lockfile
pnpm lint
pnpm typecheck
pnpm test
pnpm build
pnpm assets:check
pnpm audit:prod
```

`pnpm build` updates `internal/web/dist`. `assets:check` rebuilds in a temporary
directory and requires byte-identical production assets. The Go server uses
`Cache-Control: no-cache` for bootstrap HTML. JavaScript and CSS use
content-hashed names, ETags, and `Cache-Control: public, max-age=31536000,
immutable`, so changed bundles receive new URLs while unchanged assets remain
cacheable.
`audit:prod` blocks known high- or critical-severity production dependency
advisories against the public npm advisory service.

## Production Replacement Points

Before an internet-facing deployment:

1. Replace local Identity or put it behind the product's trusted identity
   boundary.
2. Use TLS and secure cookies.
3. Add request and account rate limiting at the edge.
4. Define real roles, scope resolution, and credential rotation.
5. Replace the records example with product-owned Modules and migrations.
6. Add backup, restore, observability, and deployment-specific health checks.
