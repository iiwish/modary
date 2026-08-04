# Deployment

Deployment follows the selected component graph. Modary provides a portable
process contract and consumer-owned reference source; it does not operate
PostgreSQL, terminate TLS, schedule containers, distribute secrets, or run an
identity provider or telemetry collector.

## Generated Artifacts

Every Profile contains a multi-stage `Dockerfile` and `.dockerignore`. Admin and
Governed also contain a PostgreSQL `compose.yaml`. The final image:

- contains a statically linked application and CA roots, not Go, Node.js,
  package caches, VCS data, frontend source, or application source;
- runs as numeric user and group `65532:65532` on a read-only root filesystem;
- receives signals directly through the application entrypoint;
- carries OCI version, revision, and created labels and injects the same bounded
  build identity into structured process diagnostics;
- builds for Linux amd64 or arm64 through `TARGETOS` and `TARGETARCH`.

The generated source is a reviewed baseline. Pin base-image digests in the
consumer repository, run the image scanner used by that organization, and
record the resolved digest in each release.

## Build

From a generated project:

```bash
docker build \
  --build-arg VERSION=v1.4.0 \
  --build-arg REVISION="$(git rev-parse HEAD)" \
  --build-arg CREATED="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -t registry.example.com/acme/service:v1.4.0 .
```

`CREATED` must be canonical RFC3339 when passed to the process. Local builds
may retain the explicit `dev`, `unknown`, `unknown` defaults.

## Probes And Traffic

Generated servers expose:

- `GET` or `HEAD /livez`: local lifecycle only, with no network dependency;
- `GET` or `HEAD /readyz`: false before listener/application readiness, during
  drain, and when a selected bounded dependency check fails.

Probe requests reject query parameters and bodies and return small `no-store`
JSON. Do not treat `/livez` as dependency readiness or add a second ambiguous
health authority. Configure the orchestrator to remove an unready instance from
traffic without restarting a locally live process for a dependency outage.
Generated servers also bound header bytes, header reads, complete reads, writes,
idle connections, and shutdown; product-specific upload and streaming routes
must deliberately replace those defaults rather than disabling them globally.

## Migrations

Database Profiles expose a separate `migrate` operation. Run it once for the
candidate image before starting new serving instances:

```bash
./application migrate
./application serve
```

Generated Compose models the same order with a one-shot migration service and
`service_completed_successfully`. Serving defaults to no migration; set
`MODARY_MIGRATE_ON_START=true` only for an explicitly chosen single-instance or
development policy. Migrations are forward-only. A committed migration is not
reversed when later startup fails.

The migration process loads only the database URL and selected schema names.
Do not grant it Admin passwords, OIDC client secrets, operator credentials, or
OTLP exporter headers. Runtime provisioning and external-provider startup occur
only in serving or worker processes.

## Profile Topology

### API

The API Profile requires no database or worker. Run the single HTTP binary under
supervision and add only consumer-specific readiness checks.

### Admin

Run one or more Go API/UI instances against the same PostgreSQL application
schema. The React bundle is embedded. Default Admin uses local development
identity; `--with oidc` replaces that login boundary. `--with otel` connects to
an external OTLP/HTTP collector. `--with tasks` selects governed PostgreSQL and
River, while `--with audit` adds the bounded audit inspection surface.

OIDC ceremonies are process-local in this Alpha. Use one login instance or
ingress affinity from login start through callback; established PostgreSQL
application sessions do not require that affinity.

### Governed

Run API and worker binaries as separate supervised processes against one
physical PostgreSQL database with distinct exclusive application and River
schemas. The API may enqueue when workers are stopped. A separate queue database
cannot share the governed Action transaction.

## Graceful Shutdown

On SIGINT or SIGTERM the generated process changes readiness before rejecting
new application work, waits for accepted requests, shuts down HTTP, releases
application Modules in dependency order, and enters the stopped phase under one
bounded timeout. Configure the platform termination grace period longer than
that timeout. Callbacks and task handlers must honor cancellation; Go cannot
forcibly stop trusted non-cooperative code.

## Secrets And Network

Supply `DATABASE_URL`, password/OIDC credentials, and OTLP headers through the
deployment secret mechanism. Do not bake them into source, Compose, image
layers, labels, commands, or logs. Production owns TLS, trusted proxy and host
policy, origins, security headers, WAF, rate limits, egress rules, database
roles, and collector access.

## Release Checklist

- Pin exact Modary component, Go, base-image, PostgreSQL, frontend, OIDC, and
  OTel versions used by the selected graph.
- Apply and audit migrations before traffic; back up and restore-test every
  selected schema.
- Verify numeric non-root user, read-only filesystem, dropped capabilities,
  no Node runtime, no source/VCS/cache/secret files, and expected OCI labels.
- Exercise startup, PostgreSQL failure, Collector failure, unready routing,
  active-request SIGTERM drain, forced timeout, and restart.
- Validate secure cookie, TLS, host/origin/proxy, identity-provider callback,
  and rate policies at the public boundary.
- Monitor readiness, request failures and latency, PostgreSQL capacity, River
  lag/retry/discard/oldest-job age, and telemetry export health without user or
  scope metric labels.
- Keep the platform deadline longer than application, worker, and exporter
  shutdown budgets.
