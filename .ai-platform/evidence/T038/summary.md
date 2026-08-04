# T038 Heavy Module Isolation

- Status: Completed
- Date: 2026-08-02
- Packet: `.ai-platform/specs/009-component-boundary-closure/packets/T038.yaml`
- Execution: direct Codex execution; subagent delegation was not authorized

The root Modary module contains only provider-neutral contracts and lightweight
composition. Ordinary PostgreSQL components live in
`github.com/iiwish/modary/components/postgres`; governed PostgreSQL and River
live in `github.com/iiwish/modary/components/governedpostgres`. Cross-module
conformance owns a separate integration module. Generated API and default Admin
graphs prove heavy dependency absence, while selected Governed and operational
Admin consumers require the concrete modules explicitly.

The root and both published component modules use one prerelease version and
one commit, but publish through three annotated tags. Tag-mode preflight rejects
an incomplete or split tag train; normal remote-consumer acceptance removes
every local replacement and resolves all three module versions.
