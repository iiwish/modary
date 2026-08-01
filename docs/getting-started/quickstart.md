# Quickstart

This tutorial runs the public Counter Console example, previews one governed
Action, and shows where the framework and application responsibilities meet.
It uses the same independent Go module exercised by copied-out and remote
consumer conformance.

## 1. Get The Tagged Source

```bash
git clone --depth 1 --branch v0.1.0-alpha.1 https://github.com/iiwish/modary.git
cd modary
make bootstrap
```

`make bootstrap` downloads the framework and example Go modules with Go work
files disabled. Go 1.26 or newer is required; Node.js is not required.

When working from an existing source checkout, start at the repository root and
run the same `make bootstrap` command.

## 2. Verify The Public Example

```bash
cd examples/counter
GOWORK=off go run ./tools/modary verify
GOWORK=off go run ./tools/modary generate --check
GOWORK=off go run ./tools/modary check
GOWORK=off go test ./...
```

Expected result: every command exits successfully and generated files remain
unchanged. `verify` inspects the pure Definition without opening the database,
applying migrations, constructing handlers, or starting modules.

## 3. Build And Inspect The Command

```bash
GOWORK=off go run ./tools/modary build
GOWORK=off ./dist/counter-console version
GOWORK=off ./dist/counter-console help
```

The version command prints `counter-console 0.1.0`. Help and version are pure
paths and do not create `data/counter-console.db`.

## 4. Preview A Governed Action

Create local tutorial input and a protected token file:

```bash
printf '%s' 'counter-primary-bearer-token-000000000001' > /tmp/modary-counter-token
chmod 0600 /tmp/modary-counter-token
printf '%s\n' '{"amount":1,"expected_version":0}' > /tmp/modary-counter-input.json
```

Preview the Action:

```bash
GOWORK=off ./dist/counter-console action run counter.increment \
  --token-file /tmp/modary-counter-token \
  --input /tmp/modary-counter-input.json \
  --preview
```

The JSON result contains the current and next Counter state plus a bound plan
hash. Preview authenticates the actor, validates input, authorizes intent, reads
state, and binds the proposed effect; it does not perform the mutation.

Delete the tutorial files after use:

```bash
rm -f /tmp/modary-counter-token /tmp/modary-counter-input.json
```

The included token and password are public local-example credentials. Never use
them in another application or deployment.

## 5. Follow The Composition

The [composition root](../../examples/counter/internal/project/project.go)
constructs official SQLite, local Identity, RBAC, and SQL Audit adapters, then
registers consumer modules. Both the
[application command](../../examples/counter/cmd/counter-console/main.go) and
[project tool](../../examples/counter/tools/modary/main.go) use the same pure
Definition provider.

The [Counter module](../../examples/counter/modules/counter/module.go) owns its
migration, Action descriptor, Handler factory, Preview plan, execution logic,
and public conflict error. The framework owns lifecycle, capability validation,
authorization, plan binding, idempotency, transaction boundaries, and required
audit behavior.

## 6. Make A First Change

Continue with [Create Your First Independent Application](first-application.md).
It copies this example outside the Modary checkout, removes the development
replacement, proves exact remote module resolution, and walks through a
generated Action contract change.

For the repository shape, read [Consumer Project Layout](project-layout.md).
For the runtime semantics, read [Governed Actions](../concepts/governed-actions.md).
Use [Troubleshooting](../how-to/troubleshooting.md) when a command fails.
