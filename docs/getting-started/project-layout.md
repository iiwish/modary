# Consumer Project Layout

A Modary application is an ordinary independent Go module. The framework does
not scan source code, own the consumer repository, or generate the composition
root.

```text
consumer/
  go.mod
  go.sum
  modary.yaml
  cmd/
    application/main.go
  tools/
    modary/main.go
  internal/
    project/project.go
    generated/
    ui/
  modules/
    feature/module.go
    feature/migrations/postgres/*.sql
    adapter/module.go
```

## `internal/project`

This is the single composition root. It owns application metadata, typed
configuration, official adapter options, consumer module registration, and
explicit HTTP route composition. Its Definition provider is used by both the
application command and project tool.

Keep Definition creation side-effect-free. It may construct values and validate
options, but it must not open files, connect to a database, apply migrations,
hash passwords, generate randomness, construct handlers, or start goroutines.

## `cmd/application`

The executable owns signal handling, process exit behavior, and consumer
command options. It delegates standard application commands to `appcmd.Run`.
The consumer decides which transports and assets to mount.

## `tools/modary`

The pinned project tool calls `projecttool.Run` with the same Definition
provider. Keeping it in consumer source ensures verify, generate, check, and
build use the same Modary dependency as the application.

## `modules`

Each module owns one stable manifest ID, its consumer migrations, Action
descriptors, handler factories, and optional lifecycle resources. A shared
contract package owns every custom capability's typed key so providers and
consumers use the same identity-bearing value.

## `internal/generated`

Generated graph, Action catalog, and TypeScript contracts are consumer-owned
review artifacts. They are deterministic and should normally be committed.
Regenerate them with the pinned project tool; do not edit them manually.

## `modary.yaml`

The manifest contains application identity, generated output paths, and one Go
build target. Go code remains the only module-composition source. See the
[manifest reference](../reference/project-manifest.md).

## Configuration And Secrets

Local examples may use explicit development credentials. Real consumers load
configuration and secrets through their own reviewed boundary before creating
the Definition. Do not put production credentials, bearer tokens, encryption
keys, or private data in source, `modary.yaml`, generated files, or command
output.
