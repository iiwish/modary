# Expose A Governed Action

Define business behavior once as an Action and project it through explicit
consumer-owned routes or commands. A surface never receives or calls the raw
Handler.

## Application Assembly

The composition root returns one pure `appkit.Definition`. `appcmd.Run` starts
the modules and assembles an opaque application. A custom application path may
use the public AppKit assembly API, but it must preserve the same lifetime and
shutdown behavior.

## CLI

Use `appcmd.Run` for standard `serve`, Action execution, version, and help
orchestration. Runtime commands authenticate through the installed identity
adapter. Bearer tokens can come from an explicitly supported token file or
standard input; do not put tokens in command arguments, source, or logs.

The Action descriptor must include `action.ChannelCLI`. The Runtime still
enforces scope, permission, Preview policy, idempotency, transaction, and audit.

## HTTP

Create the API only after the application is ready:

```go
api, err := httpapi.NewAPI(application, httpapi.APIOptions{})
```

Mount it under a consumer-selected route using `http.ServeMux` or another
router. The consumer owns TLS termination, trusted proxy policy, host filtering,
request logging, rate limits, and public network exposure. Secure cookies are
the default; an insecure-cookie option is suitable only for explicit local
development.

The Action descriptor must include `action.ChannelHTTP`. HTTP request envelopes
and extracted Action documents have independent bounded validation.

## MCP

Create and mount `httpapi.NewMCP` explicitly. The MCP projection uses the same
Action catalog and Runtime. It does not grant an Agent different authorization,
idempotency, transaction, or audit semantics.

The Action descriptor must include `action.ChannelMCP`. Treat MCP clients as
untrusted callers, not trusted module extensions.

## Static UI

Pass consumer-owned assets to `httpapi.NewSPA`, or serve UI through the
consumer's own stack. The Admin Starter copies source and a prebuilt bundle into
the consumer project; the generated Go package embeds those consumer-owned
assets. Modary does not select product routes from Module IDs, generate UI at
runtime, or define application branding.

When the SPA shares a server with API or health endpoints, set
`SPAOptions.FallbackExcludedPaths` to their canonical namespace roots, such as
`/api` and `/readyz`. Requests under those roots never fall back to the SPA,
regardless of the `Accept` header, so unknown backend paths retain their HTTP
404 semantics.

## Custom Surfaces

A consumer may define another non-empty `action.Channel` and call the public
Runtime facade. The adapter must preserve caller identity, execution scope,
request ID, exact input, idempotency key, and plan hash. It must project public
errors without exposing private dependency details.

## Shutdown

Build routes from the active application and stop accepting traffic before or
during application shutdown. The lifecycle gate rejects new Runtime and identity
leases, cancels active contexts, and begins cleanup after cooperative calls
drain. Bound the server shutdown in the consumer process as well.

See the generated Governed `internal/project/project.go` for explicit health,
API, MCP, and command wiring.
