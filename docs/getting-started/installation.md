# Installation And Version Pinning

## Requirements

- Go 1.26.5 or newer.
- PostgreSQL 17 for the Admin and Governed Profile integration paths.
- pnpm 11 and a supported Node.js runtime only when changing Admin frontend
  source. The generated production Go binary does not need Node.js.

## Current Version State

The current supported release is `v0.3.0-alpha.1`. The immutable
`v0.2.0-alpha.1` tag is the preceding React component-framework baseline.

Pre-v1 consumers pin exact versions. Do not use `latest`, a branch, or a broad
version range in production.

## Use The Current Checkout

```bash
git clone https://github.com/iiwish/modary.git
cd modary
make bootstrap
export MODARY_STARTER_REPLACE="$(pwd)"
go run ./cmd/modary new ../sample-api --profile api \
  --module example.com/acme/sample-api
```

`MODARY_STARTER_REPLACE` is a development and conformance hook. It writes a
local `replace` directive into the new project. Remove that directive and pin a
published exact version before distributing the consumer.

## Use The Published v0.3 Starter

```bash
go run github.com/iiwish/modary/cmd/modary@v0.3.0-alpha.1 \
  new sample-api --profile api --module example.com/acme/sample-api
```

The global command is create-only. It accepts a nonexistent or empty real
directory, validates all templates first, and refuses to merge, overwrite, or
patch a non-empty destination.

Admin selections are repeatable: `--with tasks`, `--with audit`, `--with oidc`,
and `--with otel`. OIDC replaces local password login. OTel changes only the
backend module graph and process readiness; it adds no UI surface.

## Verify A New Project

```bash
cd sample-api
GOWORK=off go mod tidy
GOWORK=off go test ./...
GOWORK=off go build ./...
```

`GOWORK=off` proves that the project does not rely on the framework checkout's
workspace configuration.
