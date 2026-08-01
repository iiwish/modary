# Counter Console

Counter Console is Modary's public executable example and neutral independent
consumer conformance application. It is a separate Go module with its own composition root,
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

After a version is published, `scripts/remote-consumer.sh <version>` performs a
different release gate: it copies this consumer outside the repository, removes
the replacement, resolves the exact version through normal Go module behavior,
and runs verify, generated drift, test, build, and version commands with
`GOWORK=off`. Local copied-out conformance and remote version conformance are
separate claims.

## Run From A Source Checkout

From this directory:

```bash
GOWORK=off go run ./tools/modary verify
GOWORK=off go run ./tools/modary generate --check
GOWORK=off go test ./...
GOWORK=off go run ./cmd/counter-console version
```

Continue with the repository [quickstart](../../docs/getting-started/quickstart.md)
to build the executable, preview the governed Counter Action, and make a first
contract change.
