# ADR-007: F0 Runtime Implementation

- Status: Accepted
- Date: 2026-07-30
- Scope: Modary F0

## Decision

Modary F0 uses the following concrete implementation:

| Concern | Decision |
|---|---|
| HTTP runtime | `net/http` with Chi routing and middleware |
| Database | SQLite through the pure-Go `modernc.org/sqlite` driver |
| Schema validation | JSON Schema Draft 7 through `gojsonschema` |
| Plans | Canonical, SHA-256-bound plans persisted in SQLite with a five-minute TTL |
| UI composition | Generated build-time module route and navigation registries |
| Agent channel | A narrow MCP JSON-RPC adapter over the shared Action Runtime |
| Password KDF | Argon2id with per-user random salts |
| Audit boundary | Successful audit records commit with business writes; rejected and failed attempts are appended after rollback |
| Source snapshot | SHA-256 over the published RuleSpec hash and the ordered source rows, including source update timestamps |

## Consequences

The release is a statically linked Go binary with embedded React assets and no
runtime Node.js or external service dependency. SQLite keeps deployment and
transaction semantics small enough for F0. Static composition requires a
rebuild when the module list changes, which is intentional for this milestone.

The MCP surface implements only the protocol methods needed by F0. A broader
SDK, streaming transport, distributed database, external identity provider,
and general policy language remain outside this decision.
