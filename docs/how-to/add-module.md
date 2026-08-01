# Add A Consumer Module

This guide adds one consumer-owned feature module. Use the
[external Counter module](../../examples/counter/modules/counter/module.go)
as the complete executable reference.

## 1. Choose Stable Identities

Define one stable module ID, semantic module version, Action IDs, permissions,
and public error codes. Treat published IDs as persisted contracts. Do not derive
them from filenames, UI labels, or translated text.

## 2. Define The Manifest

Create a `module.Manifest` with `module.SchemaVersion`, the module ID and
version, a module type, dependencies, and capability requirements. A feature
that reads or writes consumer state through the official durable profile
requires `module.CapabilityDatabase`.

Declare only capabilities the module actually resolves. Missing, undeclared,
duplicated, cyclic, or ambiguously provided dependencies fail during graph
validation before startup.

## 3. Add Forward-Only Migrations

Embed SQLite migrations in the consumer package and expose the migration
directory as an `fs.FS`. Register it as a `module.MigrationSource` for driver
`sqlite`. Migration names and contents are bounded and validated before database
effects.

Never rewrite a migration that reached a released consumer. Add a new migration
for each correction. Migration SQL must remain inside the supported policy and
must not contain transaction control or temporary-schema access.

## 4. Define Actions

Add `module.ActionBinding` values containing a complete `action.Descriptor` and
a Handler factory. Construct schemas with the `action` schema helpers or supply
valid bounded Draft 7 JSON. Declare Preview, channels, permission, audit,
idempotency, and every consumer public error.

In the factory, resolve typed services and retain their values:

```go
NewHandler: func(_ context.Context, services module.Resolver) (action.Handler, error) {
    db, err := module.Resolve(services, module.Database())
    if err != nil {
        return nil, fmt.Errorf("resolve database: %w", err)
    }
    return &handler{db: db}, nil
}
```

The HandlerFactory Resolver is valid only during the factory call. Do not save
it in the Handler or pass it to another goroutine.

## 5. Add Runtime Resources Only When Needed

If the module provides a capability or owns a process resource, add a `Start`
callback. Publish services through the startup Scope and register cleanup with
`module.OnStop`. Join every goroutine using the Scope before `Start` returns.
Callbacks and cleanup must be concurrency-safe and cancellation-cooperative.

A module that only declares migrations and Actions can omit `Start`.

## 6. Register In The Composition Root

Return the Registration from a pure constructor and append it explicitly to the
consumer's `appkit.Definition.Modules`. Do not introduce source scanning,
`init` registration, a database module catalog, or generated Go composition.

## 7. Verify

```bash
GOWORK=off go run ./tools/modary verify
GOWORK=off go run ./tools/modary generate
GOWORK=off go test ./...
```

Review the generated module graph and Action catalog. Then add module-specific
tests described by the [consumer testing guide](test-application.md).
