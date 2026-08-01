# Contributing To Modary

Modary is an independent governed-application framework. Contributions must
preserve the framework/consumer boundary and must not embed one product's
domain, bootstrap data, UI, policy, credentials, or release process.

## Before Starting

1. Read `docs/index.md`, `docs/framework-f0.md`, and the relevant ADRs.
2. Search existing issues or delivery specifications before proposing a second
   abstraction for an established contract.
3. For a public API, lifecycle, persistence, security, or release change, write
   or update a specification and acceptance criteria before implementation.
4. Keep unrelated work out of the change and preserve an existing dirty
   worktree.

## Local Requirements

- Go 1.26 or newer, with the exact minimum governed by `go.mod`.
- Git, Make, a POSIX shell, `find`, `xargs`, and `rg`.
- No Node.js toolchain is required for the framework or public-example gate.

Bootstrap and run the normal acceptance suite:

```bash
make bootstrap
make acceptance
```

Before requesting review, run the complete local CI gate:

```bash
make ci
git diff --check
```

## Change Design

- Compose modules explicitly in consumer Go code.
- Keep Definitions pure and inspectable; runtime effects belong to lifecycle
  installation and startup.
- Route governed business mutation through `action.Runtime`.
- Expose narrow capabilities and read-only projections rather than raw
  databases, transactions, handlers, or registries.
- Add focused RED/GREEN tests for behavior changes. Documentation-only changes
  must still pass documentation and link validation.
- Update canonical current-state documentation. Use historical wording only in
  changelogs, migration guides, ADRs, or release records.

## Public API Changes

Before v1, public APIs can change, but a release must document the change and an
upgrade path. Prefer additive evolution and deprecation over unnecessary
breakage. A public contract change requires:

- external-package tests using only public imports;
- copied-out consumer coverage when adoption behavior changes;
- generated-format compatibility analysis when applicable;
- an entry under `Unreleased` in `CHANGELOG.md`;
- updated versioning or upgrade documentation.

## Security Changes

Do not put suspected vulnerabilities, real credentials, tokens, private keys,
or exploitable reproduction details in a public issue. Follow `SECURITY.md`.
Security-sensitive fixes must preserve fail-closed behavior and include focused
tests for malformed, unauthorized, cancellation, and dependency-failure paths.

## Contribution License

Modary is licensed under Apache-2.0. Unless explicitly marked otherwise before
submission, every contribution intentionally submitted for inclusion is
provided under Apache-2.0 without additional terms. Contributors must have the
right to submit the work and must preserve applicable third-party license,
copyright, and attribution notices.

## Review Standard

A change is ready for acceptance only when its contract, implementation,
focused tests, full required gates, documentation, and residual risks agree.
Passing tests do not override a broken ownership or security boundary.
