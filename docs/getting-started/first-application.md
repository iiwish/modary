# Create Your First Independent Application

This guide turns the tagged Counter example into an ordinary application
directory that resolves Modary through the public Go module tag. Complete the
[quickstart](quickstart.md) first so the framework model and expected commands
are familiar.

## 1. Copy The Example

From the parent directory of a tagged Modary checkout:

```bash
cp -R modary/examples/counter my-counter
cd my-counter
```

The copied directory is already an independent Go module. It contains its own
composition root, project command, application command, feature module,
migration, generated contracts, tests, and static UI.

## 2. Select The Published Framework

Remove the source-checkout development binding and retain the exact Alpha:

```bash
GOWORK=off go mod edit -dropreplace=github.com/iiwish/modary
GOWORK=off go mod edit -require=github.com/iiwish/modary@v0.1.0-alpha.2
GOWORK=off go mod tidy
```

Verify that normal module resolution selected the release and no replacement:

```bash
GOWORK=off go list -m -f '{{.Path}} {{.Version}} {{if .Replace}}replaced{{end}}' github.com/iiwish/modary
```

Expected output:

```text
github.com/iiwish/modary v0.1.0-alpha.2
```

Record the remotely resolved starting point before making the tutorial change:

```bash
git init
git add .
git commit -m "Bootstrap Modary application"
```

## 3. Verify The Application Contract

```bash
GOWORK=off go run ./tools/modary verify
GOWORK=off go run ./tools/modary generate --check
GOWORK=off go test ./...
GOWORK=off go run ./tools/modary build
GOWORK=off ./dist/counter-console version
```

The final command prints `counter-console 0.1.0`. No command needs Node.js or a
Modary source checkout.

## 4. Adopt A Real Module Path

Before publishing the application, replace
`example.com/modary-counter-consumer` in `go.mod` and the consumer imports with
the application's canonical module path. Use the Go-aware rename support in the
editor, then run `go mod tidy` and the complete command sequence above. Do not
change imports beginning with `github.com/iiwish/modary`.

## 5. Make The First Contract Change

Open `modules/counter/module.go` and change the Action `Title` or `Description`
inside `descriptor()`. The checked generated catalog is intentionally stale:

```bash
GOWORK=off go run ./tools/modary generate --check
```

The command fails and identifies generated drift. Regenerate, inspect the diff,
and restore green checks:

```bash
GOWORK=off go run ./tools/modary generate
git diff -- internal/generated
GOWORK=off go run ./tools/modary check
GOWORK=off go test ./...
```

This is the normal contract workflow: change pure Definition metadata, generate
reviewable artifacts, and commit the source and generated result together.

## 6. Replace The Example Domain

Use the [project layout](project-layout.md) as the stable repository shape.
Create a consumer-owned module with the [module guide](../how-to/add-module.md),
then remove Counter code only after the replacement module, migration, Action,
composition registration, generated artifacts, and tests pass together.

The development `replace` is useful only while Modary and the application are
checked out together. Never commit a local filesystem replacement in an
application release branch. See [troubleshooting](../how-to/troubleshooting.md)
for module, generation, capability, migration, and platform failures.
