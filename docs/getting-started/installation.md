# Installation And Version Pinning

## Requirements

Consumers need Go 1.26 or newer for the current F0 contract. `go.mod` is the
authoritative minimum. Install a local toolchain that satisfies it; Modary
project builds set `GOTOOLCHAIN=local` and do not rely on automatic toolchain
download.

Running applications and integration tests also need PostgreSQL 17. The
configured role must be able to create and own the application and River
schemas. Docker is optional; the quickstart uses it only to provide a repeatable
local database.

Framework contributors also need Git, Make, a POSIX shell, `find`, `xargs`, and
`rg`. Node.js, npm, and pnpm are not part of the headless framework workflow.

## Release Consumption

Add an exact release to the consumer:

```bash
go get github.com/iiwish/modary@v0.1.0-alpha.3
go mod tidy
```

Commit the resulting `go.mod` and `go.sum`. Applications should pin
`v0.1.0-alpha.3` exactly rather than `latest`, a branch, or a moving
pseudo-version.

## Current Source Checkout

For framework development from a source checkout, use the public Counter
example in this repository:

```bash
make bootstrap
make acceptance
```

For local development across two sibling repositories, use a developer-owned
Go work file rather than committing a local filesystem replacement to the
consumer module:

```bash
go work init ./modary ./consumer
```

Keep `go.work` outside committed consumer release state. CI and release builds
set `GOWORK=off`; they must prove the declared module version alone is enough.

## Verify The Dependency Boundary

From the consumer repository:

```bash
GOWORK=off go list -m all
GOWORK=off go mod tidy -diff
GOWORK=off go test ./...
```

The module graph must identify the intended Modary version and must contain no
unexpected `replace github.com/iiwish/modary` directive. Consumer code imports
only public packages listed in the [package map](../reference/packages.md).

## Stability

F0 is pre-v1 Alpha. Review the [versioning policy](../releases/versioning.md),
[support matrix](../reference/support-matrix.md), and
[known limitations](../f0-known-limitations.md) before depending on it.
