# T039 HTTP And Admin Contribution Contracts

- Status: Completed
- Date: 2026-08-02
- Packet: `.ai-platform/specs/009-component-boundary-closure/packets/T039.yaml`

`appkit.Preflight` produces an immutable callback-free application Contract.
`httpkit.NewPlan` binds bounded HTTP/Admin contributions to that Contract,
validates capabilities, route ownership, Admin metadata, and permission
inventories before startup, and defers handler builders until the matching
application is ready. Generated API and Admin composition use this contract.

Browser-session authentication has its own `identity.sessions` capability.
Session-aware contributions declare it directly; broad Identity availability
cannot satisfy or bypass that dependency. Contribution, requirement, path, and
aggregate route sizes are bounded before defensive copying or handler work.
