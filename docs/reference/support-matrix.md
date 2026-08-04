# Support Matrix

This matrix describes the v0.3 F0 source contract. Absence is not implied
support.

## Toolchain And Platforms

| Surface | Status |
|---|---|
| Go | 1.26.5 or newer |
| Linux amd64/arm64 | Runtime and cross-build covered |
| Darwin arm64 | Native acceptance; amd64 cross-build covered |
| Windows amd64/arm64 | Compile-only for F0 project-tool filesystem/token-path policy |
| PostgreSQL | 17 used by integration acceptance |
| pnpm | 11.1 used for generated Admin source |
| React / React Router | React 19.2 and React Router 8.3 generated baseline |
| Node.js | Needed to change/build Admin frontend; not needed by API/Governed or deployed Admin Go binary |
| OCI runtime | Generated Linux amd64/arm64 static non-root image source |
| OpenTelemetry | SDK/exporters 1.44 in the optional component; OTLP/HTTP |

## Profiles

| Capability | API | Admin | Governed |
|---|:---:|:---:|:---:|
| Database-free startup | yes | no | no |
| Ordinary PostgreSQL Store | no | yes | no |
| Local Identity and RBAC | no | yes | yes |
| Session HTTP | no | yes | governed Action API session |
| React Admin source/bundle | no | yes | no |
| Action Runtime | no | no | yes |
| SQL Audit | no | no | yes |
| River task service/worker | no | no | yes |
| MCP | no | no | yes |
| OIDC browser login | no | optional replacement for local password | no |
| OTLP traces and metrics | no | optional | no |
| `/livez` and `/readyz` | yes | yes | yes |
| Migration-only command | yes, no-op without migrations | yes | yes |

## Persistence

| Capability | Status |
|---|---|
| PostgreSQL ordinary application schema | Supported through `postgresdb` |
| PostgreSQL governed application + River schema pair | Supported through `governedpostgres` |
| Same-database Action write plus task insertion | Supported |
| Multiple API and River worker processes | Supported; jobs are at least once |
| MySQL official Adapter | Not provided |
| Embedded database official Adapter | Not provided |
| Distributed transaction | Not provided |
| External product database/API | Consumer-owned; outside governed local transaction |
| High availability/failover | Deployment and PostgreSQL responsibility |

## Identity And Exposure

| Capability | Status |
|---|---|
| Explicit local principals/passwords/sessions/bearer tokens | Development and controlled internal use |
| Scope-independent principal with multi-scope RBAC | Supported |
| CSRF-protected Admin mutations | Supported |
| Secure cookie default | Supported |
| Generic OIDC Authorization Code + PKCE | Optional Admin component |
| MFA, enrollment, account recovery, SCIM | Not provided; identity-provider/product responsibility |
| TLS, proxy trust, WAF, rate limiting | Deployment responsibility |
| Hostile plugin sandbox | Not provided |

## Tooling

| Tool | Status |
|---|---|
| `modary new` | Create-only API/Admin/Governed project generation |
| Generated Admin assets | Reproducible byte comparison and Go embed |
| `projecttool verify/generate/check/build` | Optional advanced workflow |
| Starter patch/upgrade command | Not provided |
| Runtime plugin/component marketplace | Not provided |
| Generated Dockerfile and local PostgreSQL Compose | Supported reference source |
| Hosted runtime, Kubernetes operator, TLS automation | Not provided |

Trusted consumer callbacks, handlers, repositories, task consumers, and output
writers remain concurrency-safe and cancellation-cooperative responsibilities.
