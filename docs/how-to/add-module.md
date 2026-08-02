# Add A Consumer Module

A consumer Module owns one coherent product capability. Start with the smallest
manifest and add database or governed dependencies only when the feature uses
them.

## 1. Choose Stable Identity

Use a stable lowercase Module ID and semantic version. Persisted migration and
Action ownership refers to this ID; do not derive it from a UI label or package
directory that is likely to move.

## 2. Define The Manifest

A database-free feature:

```go
func Registration() module.Registration {
    return module.Registration{
        Definition: module.Definition{
            Manifest: module.Manifest{
                SchemaVersion: module.SchemaVersion,
                ID: "invoices",
                Version: "0.1.0",
                Type: module.ModuleTypeFeature,
            },
        },
    }
}
```

Declare only capabilities actually resolved. Missing providers, duplicate
providers, undeclared access, dependency cycles, and ambiguous graphs fail
before startup side effects.

## 3. Add A Typed Service When Modules Share Behavior

Put a namespaced capability and one package-level `module.Key[T]` in a small
consumer contract package. A provider publishes it during Start and consumers
resolve the exact same key. Do not use a global variable or string service
locator.

## 4. Add Ordinary Persistence

When an Admin/business feature needs PostgreSQL:

1. require `module.CapabilityDatabase`;
2. declare forward-only PostgreSQL migrations;
3. resolve `module.Database()` in the Module's Start callback or route factory;
4. use `database.Store.WithinTransaction` for mutations.

The Store is for normal repository work. It does not require an Action or River.

## 5. Add A Governed Action Only When Needed

A high-impact feature declares an `action.Descriptor` and factory. Resolve
governed Access and Tasks during the factory call:

```go
NewHandler: func(_ context.Context, resolver module.Resolver) (action.Handler, error) {
    access, err := module.Resolve(resolver, module.ActionDatabase())
    if err != nil {
        return nil, fmt.Errorf("resolve governed database: %w", err)
    }
    tasks, err := module.Resolve(resolver, module.Tasks())
    if err != nil {
        return nil, fmt.Errorf("resolve tasks: %w", err)
    }
    return &handler{database: access, tasks: tasks}, nil
}
```

The Resolver expires after the factory. Retain resolved values, not the
Resolver. The Handler never opens or controls the transaction.

## 6. Own Routes And Frontend Registration

Return `[]httpkit.Route` or another consumer router composition from the feature
package. For Admin, add a frontend module entry to
`web/src/modules/index.ts`. Backend authorization remains mandatory even when a
route or command is not visible in the UI.

## 7. Register Explicitly

Append the Registration in the application's `appkit.Definition.Modules`.
Static Go composition is the only Module list. Do not add `init` registration,
source scanning, a database plugin catalog, or runtime imports.

## 8. Test

Cover pure construction, graph failure, lifecycle cancellation, migrations,
scope isolation, authorization, and copied-out `GOWORK=off` operation as
appropriate. For governed features also cover Preview, stale plans,
idempotency, rollback, audit, restart, and task consumption.
