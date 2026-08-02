# Support Matrix

This matrix describes the F0 Alpha contract. A future release note may narrow or
extend it, but absence from this table is not implied support.

## Runtime And Build

| Surface | F0 status | Boundary |
| --- | --- | --- |
| Go | 1.26 or newer | `go.mod` is authoritative; project build uses the installed local toolchain. |
| Linux amd64/arm64 | Supported and CI/cross-build covered | Native filesystem and process policy is implemented; release CI runs Ubuntu. |
| Darwin amd64/arm64 | Supported and cross-build covered | Native Darwin/arm64 acceptance includes ACL policy; Darwin/amd64 is cross-built. |
| Windows amd64/arm64 | Compile-only | Native project Build, token-path ACL, and rename behavior are not claimed. |
| Other Unix | Not supported for native Build | F0 has no validated ACL policy and fails closed. |
| Node.js | Not required | Framework, external consumer, project generation, and build are Node-free. |

## Persistence And Topology

| Capability | F0 status |
| --- | --- |
| PostgreSQL 17 | Supported and used by integration acceptance |
| One official PostgreSQL control database | Supported |
| Owned application and River schemas | Required; schemas must be distinct |
| Isolated PostgreSQL schemas for tests | Supported by explicit configuration |
| Multiple concurrent application writers | Supported by PostgreSQL transactions |
| Multiple River worker processes | Supported; handlers remain at-least-once and idempotent |
| Transactional Action write plus task enqueue | Supported in the same control database |
| High availability | PostgreSQL and deployment responsibility; no framework failover controller |
| Distributed transaction | Not provided |
| Arbitrary privileged storage adapter | Not a public consumer extension |
| Durable background queue | Supported through the public `task` contract and internal River adapter |
| MySQL or another official durable profile | Not provided |
| Business data in another database or API | Consumer-owned Connector; outside the local Action transaction |

## Identity And Exposure

| Capability | F0 status |
| --- | --- |
| Explicit local principals, optional password logins, sessions, and bearer tokens | Supported for local/private deployment profiles; credentialless service principals are first-class |
| Explicit bearer tokens | Supported with documented file/stdin boundaries |
| Scoped RBAC | Supported |
| HTTP session and CSRF projection | Supported through explicit HTTP mounting |
| Public-internet IAM, OAuth, SSO, MFA | Not provided |
| TLS, reverse proxy, rate limiting | Consumer deployment responsibility |
| Hostile plugin sandbox | Not provided |

## Project Tooling

| Command | Side effects |
| --- | --- |
| `verify` | Pure Definition and project validation; no module start or persistent state. |
| `generate` | Writes configured consumer-owned generated files. |
| `generate --check` | Reads and compares generated outputs; fails on drift. |
| `check` | Verifies project and generated state. |
| `build` | Builds one configured consumer package into one configured output on supported native platforms. |

## Trusted Cooperation

Consumer Handlers, callbacks, cleanup, identity, authorization, audit,
database dependencies, clocks, and output writers must be concurrency-safe and
honor context cancellation. Modary bounds its own orchestration; it cannot make
non-cooperative Go code safe or interrupt a blocked `io.Writer.Write`.

Read [known limitations](../f0-known-limitations.md) for exact limits and the
[deployment guide](../operations/deployment.md) before production use.
