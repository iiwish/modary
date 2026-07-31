# T012 Evidence Summary

- Status: Completed
- Date: 2026-07-30
- Packet: `.ai-platform/specs/002-framework-decoupling/packets/T012.yaml`

## Changed Files

- Public application assembly and metadata: `appkit/**`.
- Consumer-owned command orchestration: `appcmd/**`.
- Explicit health, Action API, MCP, and SPA handlers: `transport/httpapi/**`.
- Coordinated Action identifier and Identity error contracts: `action/**`,
  `identity/identity.go`, and `adapters/localidentity/service.go`.
- T012 execution packet, work graph, and this evidence record.

## RED Result

The boundary tests represented missing behavior before implementation: opaque
application assembly, pure preflight, post-start rollback, request drain before
Module cleanup, external-package composition, explicit handler mounts, strict
HTTP and JSON-RPC decoding, identity failure classification, cancellable body and
CLI input reads, panic containment, and consumer-owned command execution.

## GREEN Result

- `appkit.Start` validates the complete Definition before side effects, resolves
  every governance service fail-closed, exposes only immutable metadata/catalog,
  governed Runtime, and narrow identity views, and owns bounded shutdown.
- `appcmd` parses before startup, executes CLI Actions only through Runtime,
  requires a cancellation-safe `io.ReadCloser` for stdin, and drains HTTP before
  releasing application resources.
- HTTP session authentication distinguishes invalid credentials from operational
  Identity failures; only invalid sessions clear cookies. Request bodies are
  bounded, strict, UTF-8 JSON and are closed exactly once on request deadline.
- MCP is an explicitly mounted, bearer-authenticated, non-streaming JSON profile.
  It enforces the declared protocol version, Origin and media boundaries,
  string/integer request IDs, UTF-8, strict single-object JSON-RPC, notification
  HTTP semantics, bounded concurrency, and governed Action-only tools.
- SPA assets remain consumer-owned and are served through an explicit filesystem
  with strict paths, cache policy, conditional requests, and fallback behavior.

## Validation

- `go test ./appkit ./appcmd ./transport/httpapi`: pass.
- `go test -race ./appkit ./appcmd ./transport/httpapi`: pass.
- `go vet ./appkit ./appcmd ./transport/httpapi`: pass.
- `go test -race ./action ./identity ./module ./scope`: pass.
- T012 neutrality scan: zero matches.
- `git diff --check`: pass.

External-package tests perform `Start -> Runtime.Execute -> Shutdown`, invoke
`RunAction`, start and query an explicitly mounted health handler through
`Serve`, and construct the public transport surface without internal imports.

## Public API Review

`Application` has no exported fields and exposes no Host, Registry, Handler,
database, installation Scope, or generic service resolver. Definition inspection
and metadata validation are side-effect free. The Go APIs, command syntax, and
Modary-specific wire schemas are explicitly documented as alpha contracts;
consumers are instructed to pin exact pre-v1 versions.

## Transport Review

All HTTP, MCP, and CLI Action paths call the same governed Runtime. No route or
tool depends on a concrete Module ID. Context deadlines interrupt conforming
request and stdin readers, callback panics are contained before a response is
committed, public errors redact operational causes, and lifecycle tests verify
cleanup and close behavior exactly once.

## Residual Risk

The F0 MCP handler intentionally implements the application/json response
profile without SSE or server-assigned sessions. It does not claim streaming,
resumability, or server-initiated requests. No task-local P0, P1, or P2 risk is
accepted by this record. Final independent review passed the focused, repeated,
race, vet, neutrality, direct-import, and diff checks without a P0-P2 finding.
