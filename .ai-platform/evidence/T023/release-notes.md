# Modary v0.1.0-alpha.2

Modary is a Go-first framework for governed modular applications. This first
supported Alpha publishes the F0 Kernel, AppKit, application commands, HTTP/MCP
projection, project tooling, official SQLite/local-identity/RBAC/audit adapters,
and the independent Counter example.

## Install

```bash
go get github.com/iiwish/modary@v0.1.0-alpha.2
```

Pin the exact prerelease. Go 1.26 or newer is required; Node.js is not required.

## Validation

- Candidate and annotated-tag preflight passed at
  `a4700f1c7ef53fe058a50fd43d65b906c3be89c4`.
- GitHub tag CI passed Linux quality, Darwin arm64 native, source-stability, and
  release jobs: https://github.com/iiwish/modary/actions/runs/30688981095
- The public Go proxy resolved the exact tag and commit.
- The copied-out Counter application removed its local replacement, resolved
  the tag, and passed verify, generated drift, tests, build, and version checks.

## Compatibility

The framework runtime and public API are unchanged from the rejected Alpha 1
tag. Alpha contracts may still change before v1; review `CHANGELOG.md` and
generated diffs on every upgrade. Native project build is supported on Linux
and Darwin. Windows amd64/arm64 is compile-only.

## Boundaries

The durable F0 profile is one process and one SQLite database. Public-internet
OAuth/SSO/MFA, TLS and proxy policy, high availability, distributed
transactions, hostile plugin isolation, schedulers, containers, and downstream
product release artifacts remain consumer responsibilities or out of scope.

Read `docs/f0-known-limitations.md`, `docs/reference/support-matrix.md`, and
`docs/operations/security.md` before deployment. Report vulnerabilities
privately at https://github.com/iiwish/modary/security/advisories/new.
