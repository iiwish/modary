# T018 Evidence Summary

- Status: Completed
- Date: 2026-07-31
- Packet: `.ai-platform/specs/003-release-readiness/packets/T018.yaml`

## Changed Files

- `README.md` now routes readers to one documentation entry point.
- `docs/index.md` lists every user-facing guide and every canonical F0/ADR
  reference.
- `docs/getting-started/**` covers installation, local versus released version
  pinning, executable quickstart, and canonical consumer layout.
- `docs/concepts/**` explains ownership, module/capability lifecycle, and the
  governed Action path.
- `docs/how-to/**` covers module construction, transport projection, and
  application testing.
- `docs/reference/**` provides the public package map, support matrix, and
  project-manifest/generated-output contract.
- `docs/operations/**` provides deployment, security, and SQLite recovery
  guidance.
- `docs/releases/upgrade-guide.md` defines the consumer upgrade and rollback
  workflow.
- `scripts/check-doc-links.sh`, focused tests, and the Make target validate
  required documents, local links, H1 structure, and index reachability.

## Navigation Audit

The index separates application developers, operators, security reviewers, and
framework contributors by task. Every document under `docs/` is directly
reachable from the index. Detailed guides link back to the executable external
consumer or canonical F0 contract rather than copying unverified APIs.

## Contract Review

- Remote installation is described as available only after the tag resolves.
- Local development uses an uncommitted Go work file; release validation uses
  `GOWORK=off` and no committed local replacement.
- F0 platform, SQLite, identity, process, callback, filesystem, and network
  boundaries remain explicit.
- Documentation contains no downstream product vocabulary or implementation.

## Residual Risk

- The private security reporting address cannot be finalized before the owner
  selects the canonical repository host and reporting channel.
- Remote installation commands cannot be executed before publication; T019
  supplies the gate and T020 records the unfulfilled external proof honestly.
