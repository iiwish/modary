# Consumer Upgrade Guide

Use this process for every pre-v1 Modary upgrade. Alpha versions may contain
consumer-visible API or generated-format changes.

## 1. Read The Release Contract

Review the target release notes, `CHANGELOG.md`,
[versioning policy](versioning.md), [support matrix](../reference/support-matrix.md),
and [known limitations](../f0-known-limitations.md). Identify changes to public
Go APIs, Go version, manifest fields, generated outputs, Action contracts,
official adapters, migrations, identity, transport behavior, and deployment
policy.

## 2. Create An Upgrade Branch

Start from a clean consumer checkout. Record the current consumer version,
Modary version, generated files, database schema state, and representative test
results. Do not combine the framework upgrade with unrelated product features or
large refactors.

## 3. Update The Exact Version

```bash
go get github.com/iiwish/modary@v0.1.0-alpha.3
go mod tidy
```

Replace the example with the actual target version. Remove any committed local
Modary replacement. Local cross-repository work may use an uncommitted Go work
file, but validation uses `GOWORK=off`.

## 4. Adapt Source Contracts

Compile before changing behavior. Update the composition root, module manifests,
capability keys, Action descriptors, Handler contracts, command options,
transport mounting, and adapter options only where the release requires it.

Preserve stable consumer Action and module IDs unless the product intentionally
introduces a new contract. A framework upgrade does not justify silently
renaming persisted consumer identities.

## 5. Regenerate

```bash
GOWORK=off go run ./tools/modary verify
GOWORK=off go run ./tools/modary generate
GOWORK=off go run ./tools/modary check
```

Review every generated diff. Regeneration can change graph or contract format;
it must not hide an unintended Action, permission, schema, channel, error, or
module dependency change.

## 6. Test Without Persistent Data

Run unit, integration, transport, race, and repeated tests against a new database.
Exercise denied and stale-plan paths, idempotent replay, audit failure,
cancellation, shutdown, and restart.

```bash
GOWORK=off go test ./...
GOWORK=off go test -race ./...
```

## 7. Rehearse Data Upgrade

Create and verify a production-representative backup. Restore it into a private
staging path, run the target binary once, inspect migration and startup results,
exercise representative Actions, restart, and verify durable state and audit.
Follow [PostgreSQL backup and restore](../operations/postgresql-backup-restore.md).

## 8. Build The Release Artifact

```bash
GOWORK=off go run ./tools/modary build
```

Build on a supported native platform. Record source commit, Modary version, Go
version, generated state, test results, and artifact checksum in the consumer's
own release process.

## 9. Deploy And Observe

Back up immediately before rollout, deploy one process, verify readiness and a
small governed workflow, inspect error and audit signals, and only then complete
rollout. Keep the pre-upgrade restore set until the rollback window closes.

## Rollback

Do not edit or reverse a published migration in place. Stop the new application,
restore the verified pre-upgrade database, and run the matching previous binary.
Publish a follow-up prerelease or patch for a defective Modary version rather
than moving its tag.
