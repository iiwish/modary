# T040 Operational Admin Surface

- Status: Completed
- Date: 2026-08-02
- Packet: `.ai-platform/specs/009-component-boundary-closure/packets/T040.yaml`

Repeatable `--with tasks` and `--with audit` selections generate the exact Go,
configuration, HTTP, React source, and production assets they need. Public task
and audit readers expose bounded metadata only; audit reading is bound to the
authenticated actor scope. `/api/admin/context` returns selected inert
descriptors and current grants. React fails closed for unknown or unauthorized
modules, hides ungranted record commands, and shares focused page, table,
pagination, loading, empty, and error primitives across operational modules.

Task inspection exposes the closed provider-neutral states `queued`, `pending`,
`scheduled`, `running`, `retrying`, `succeeded`, `failed`, and `cancelled`.
River lifecycle values are translated inside the governed component and cannot
leak into generated HTTP or React contracts.
