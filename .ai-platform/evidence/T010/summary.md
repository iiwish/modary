# T010 Evidence Summary

- Status: Completed
- Date: 2026-07-30

## Changed Files

- Canonical delivery constitution and current product/technology/task pointers.
- Framework-decoupling spec, plan, checklist, analysis, work graph, and packets.
- `/Users/iiwish/self/rulary/prototype/README.md`.
- `/Users/iiwish/self/rulary/prototype/modary-integrated-f0/` snapshot and
  `modary-integrated-f0.sha256` manifest.
- `/Users/iiwish/self/rulary/prototype/modary-integrated-f0-governance/`
  snapshot and `modary-integrated-f0-governance.sha256` manifest.
- Versioned source and governance path/mode/hash inventories plus
  `.ai-platform/evidence/T010/check-archives.sh`.

## Validation

- Delivery artifact validator: passed with zero errors and zero warnings.
- Source-to-destination SHA-256 comparison: empty diff, 167 files on both sides.
- Governance archive comparison: empty diff, 51 files on both sides.
- Exact-set and file-mode inventory comparison: passed for both archives.
- `git diff --check`: passed.

## Review

The contract reflects the approved independent-framework boundary, uses one
explicit Go composition source, distinguishes lifecycle cleanup from committed
migration rollback, and defines measurable consumer, neutrality, and release
gates. The snapshot was captured before any consumer-owned source removal.

The versioned evidence anchors both external manifests and exact-set inventories:

```text
source manifest       143d934c29693fe799f230fd04c1647b1f9ff41ac8b292fd7ca965f7e83d43fa
governance manifest   68593d9ad930e4e5c12fef8d6fb0abe55ac11e10a423563d43c96233fd586745
source inventory      119c2280023a59bcb2ac548ed9fba4ffcad0860a13991bea6bca228a1a6e9c38
governance inventory  fa15a300998571ba703f04e49294339b86212af2caf03ff183153293ad0086b0
```

Inventories record every preserved relative path, SHA-256 digest, and file mode.
Exact-set verification excludes only `.DS_Store`, which is platform metadata and
not part of the source preservation contract.

The snapshot represents the T010 working tree rather than every path in Git
history. `docs/f1-proposal.md` and the two superseded hashed Web bundles were
already absent before capture and remain recoverable from baseline commit
`970a2d4b32193562bbbd60dbd53bd1d0857523b7`. The current compiled UI index and
its replacement hashed bundles are present in the 167-file snapshot, so the
executable prototype is complete.

`make archive-check` repeats both manifest checks, verifies the four anchored
digests, rejects duplicate inventory entries, symbolic or special filesystem
entries, and uncontracted directories, and compares the exact path, mode, and
digest contract with each external directory. Environment variables named in
`.ai-platform/evidence/T010/check-archives.sh` can relocate the archives without
weakening the expected anchors.

## Residual Risk

The canonical GitHub path is locally testable through `replace` but cannot be
claimed remotely installable until a repository remote and tag are published.
The external archive directory itself is writable and is not a Git repository;
the evidence-anchored exact inventories detect later byte, mode, membership, or
manifest drift but do not replace an immutable backup.
