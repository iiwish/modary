# T041 External Acceptance And Engineering Readiness

- Status: Completed
- Date: 2026-08-04
- Packet: `.ai-platform/specs/009-component-boundary-closure/packets/T041.yaml`
- Source digest: git-hash:6ef290f863f6647097268d9b1556844d9e4a30fc
- Release state: Remote verified as v0.2.0-alpha.1

Component Boundary Closure F0 passes corrective and external review. Fresh API,
default Admin, operations Admin, and Governed consumers build and test outside
the repository with `GOWORK=off`. The copied-profile gate requires PostgreSQL,
requires the generated Admin and Governed integration tests to report `pass`
rather than `skip`, and validates consumer-owned source rather than checkout-only
helpers. Admin consumers additionally pass their frozen React pipelines and
embedded-asset parity checks.

Starter derives application, River queue, and integration-test schemas through
one deterministic role-prefixed contract. The namespaces remain distinct and
outside PostgreSQL's reserved schemas, PostgreSQL names stay within 63 bytes,
River names stay within 46 bytes, and truncated names include a stable SHA-256
fragment. Copied-out task-enabled Admins with the maximum 63-byte project ID and
reserved-sensitive `public` and `pg` IDs pass real PostgreSQL/River integration
and reach runtime readiness using generated defaults. Go Module Paths containing
a `vendor` segment fail validation before project files are written.

Generated backend acceptance compares every Admin descriptor field, verifies the
complete and restricted grant sets, and revokes each records permission before
calling its real HTTP route. List, create, update, and delete return HTTP 403 and
leave the stored record unchanged. Task and audit permission denials remain
covered through their real selected endpoints.

The closure keeps Core free of PostgreSQL and River, separates ordinary and
governed implementations into published component modules, coordinates physical
schema roles, and preserves bounded provider-neutral task and audit contracts.
The React Admin resolves only generated source modules that match immutable
backend metadata and current grants. Its primary interface language is Chinese;
technical identifiers and inspected source data retain their original values.

The repository quality gate pins its document root to the current checkout,
canonicalizes that path, and verifies the recorded source digest before accepting
T041 or running release preflight. Caller-supplied alternate roots, relative
paths, and symlink aliases cannot bypass the digest. Direct script tests retain a
separate fixture hook. Any implementation, automation, generated asset, or
canonical-document change outside T041 evidence reopens the evidence until the
digest and review are refreshed.

Accepted commit `3600a38345380401f36958970f82cc93e2c29cd2` is published through
the root tag and both component tags at `v0.2.0-alpha.1`. Hosted main and tag CI,
normal module resolution, and local and hosted copied-out remote consumers pass.
The GitHub prerelease is
https://github.com/iiwish/modary/releases/tag/v0.2.0-alpha.1.
