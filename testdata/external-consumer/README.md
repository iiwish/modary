# Counter Console

Counter Console is the neutral external-consumer conformance application for
Modary F0. It is an independent Go module with its own composition root,
consumer migration, governed write Action, static UI, application command, and
project tool. Both commands receive the same error-returning Definition provider;
the pure command metadata is shared without evaluating that provider.
An independent System Clock Module provides a consumer-owned capability through
one package-level typed key, and the Counter feature declares and resolves that
capability without importing framework internals.

The relative `replace` in `go.mod` is only the source-checkout development
binding. `TestCopiedOutConsumerGate` copies this module to a temporary
directory, rewrites that binding to the absolute framework checkout, disables
`go.work` discovery, installs failing Node-family shims, and executes the
complete external gate there.
