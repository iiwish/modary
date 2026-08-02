# Installation And Version Pinning

## Requirements

- Go 1.26.5 or newer.
- PostgreSQL 17 for the Admin and Governed Profile integration paths.
- pnpm 11 and a supported Node.js runtime only when changing Admin frontend
  source. The generated production Go binary does not need Node.js.

## Current Version State

The current branch targets `v0.2.0-alpha.1` and is accepted as a source F0. It
is not a published tag. The immutable published baseline is
`v0.1.0-alpha.3`, whose product surface is the Governed stack and Counter
consumer rather than the v0.2 Starter Profiles.

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

## Use A Published v0.2 Starter

After the target tag is published:

```bash
go run github.com/iiwish/modary/cmd/modary@v0.2.0-alpha.1 \
  new sample-api --profile api --module example.com/acme/sample-api
```

The global command is create-only. It accepts a nonexistent or empty real
directory, validates all templates first, and refuses to merge, overwrite, or
patch a non-empty destination.

## Verify A New Project

```bash
cd sample-api
GOWORK=off go mod tidy
GOWORK=off go test ./...
GOWORK=off go build ./...
```

`GOWORK=off` proves that the project does not rely on the framework checkout's
workspace configuration.
