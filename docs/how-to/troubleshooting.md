# Troubleshoot Modary Applications

Start with the exact failing command and keep `GOWORK=off`. Do not bypass a
failure by deleting generated artifacts, weakening a descriptor, skipping
migrations, or importing framework internals.

## The Modary Version Is Empty Or Replaced

Inspect the selected module:

```bash
GOWORK=off go list -m -json github.com/iiwish/modary
```

For a released application, `Version` must be the intended exact tag and
`Replace` must be absent. Remove a committed replacement with `go mod edit
-dropreplace=github.com/iiwish/modary`, require the exact release, and run
`go mod tidy`.

## Verify Rejects The Definition

`verify` evaluates the pure application Definition without starting modules.
Read the first reported manifest, capability, metadata, Action descriptor, or
project-path error. Common causes are duplicate module IDs, a missing required
capability provider, a dependency cycle, invalid namespaced identifiers, and
metadata disagreement between `appcmd.Options` and `appkit.Definition`.

Do not add runtime I/O to Definition creation. Filesystem access, database
opening, migrations, handler construction, password hashing, random values, and
goroutines belong to the runtime assembly path.

## Generate Check Reports Drift

Run the write form, inspect every changed generated artifact, and check again:

```bash
GOWORK=off go run ./tools/modary generate
git diff -- internal/generated
GOWORK=off go run ./tools/modary generate --check
```

Generated files are consumer-owned review artifacts. Commit them with the
Definition change and never edit them manually.

## A Capability Cannot Be Resolved

Provider and consumer modules must share the same package-level typed
`module.Key`. Recreating a key with the same name does not reproduce its
identity. Put custom capability names and keys in one small consumer contract
package, declare the capability in both manifests, publish it during provider
startup, and resolve it only inside the Handler factory.

## A Migration Is Rejected

Use ordered forward-only SQLite files. The supported policy rejects transaction
control, temporary schemas, unsupported statement kinds, excessive source
counts or sizes, and changed content for an applied migration. Add a new
migration rather than rewriting released history.

## An Action Request Is Rejected

Check the Action ID, channel, input presence, JSON document limits, schema,
permission, execution scope, Preview plan hash, idempotency key, and declared
public error code. A required-Preview Action must execute the exact bound plan;
repeat Preview when state has changed.

Framework errors intentionally hide private dependency details at public
transport boundaries. Reproduce the failure in a consumer module test when the
public result is insufficient for diagnosis.

## Build Is Unsupported On This Host

F0 native project builds support Linux and Darwin under the documented
filesystem policy. Other support-matrix targets are cross-build claims. On a
supported host, verify ownership and permissions for the project, output path,
and temporary directory; on Darwin, remove unexpected extended ACLs instead of
disabling the policy.

## SQLite Files Are Rejected

Use a regular database path whose final directory is owned by the effective
user and is not writable by group or others. Writable ancestors are accepted
only under the documented root-owned sticky-directory rule. Do not use a
symlinked database file or weaken permissions to make startup succeed.

Continue with the [testing guide](test-application.md), [security boundary](../operations/security.md),
and [known limitations](../f0-known-limitations.md) before deployment.
