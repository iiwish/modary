# T030 Create-Only CLI And API Profile

- Status: Completed
- Date: 2026-08-02

Modary provides a reusable create-only `starter.Create` API and the global
`cmd/modary` command. `modary new <destination> --profile api` validates the
project identity, Go module path, exact framework version, Profile, replacement
hook, destination type, and empty ownership boundary before writing. Templates
render and format completely in memory; files use exclusive creation; failures
roll back only paths created by the current operation; final tree and directory
identity checks reject concurrent or symlink substitution without deleting
external content.

The API Profile contains visible consumer-owned composition: one `ping` Module,
its explicit route contribution, AppKit lifecycle, standard health, bounded
`httpkit` route assembly, and a graceful standard-library server. It requires
no database, queue, identity, RBAC, audit, governed Action, MCP, frontend, or
Node.js surface. The generated README starts it with one Go command.

The command derives its default framework version from released Go build
metadata and retains explicit environment overrides for checkout conformance.
Re-running creation against the result fails before alteration; there is no
regeneration or source-patching command.
