# Create Your First Application

> [简体中文](../zh-CN/getting-started/first-application.md)

Use `modary new` instead of copying a framework example. The Starter creates an
independent Go module, and the result contains no copied Modary implementation.

## Create

```bash
export MODARY_CHECKOUT=/absolute/path/to/modary
export MODARY_STARTER_REPLACE="$MODARY_CHECKOUT"
cd "$MODARY_CHECKOUT"
go run ./cmd/modary new /absolute/path/to/billing-api \
  --profile api \
  --module company.example/platform/billing-api \
  --name "Billing API"
```

The destination base must be a lowercase project ID. The parent must exist.
Symlink destinations and non-empty directories are rejected. The Go Module Path
must not contain a path segment named `vendor`; Go reserves that segment for
vendored imports.

## Establish The Independent Boundary

```bash
cd /absolute/path/to/billing-api
GOWORK=off go mod tidy
GOWORK=off go test ./...
GOWORK=off go build ./...
```

Before release, remove the local `replace` directive and select an exact
published Modary version:

```bash
go mod edit -dropreplace github.com/iiwish/modary
go get github.com/iiwish/modary@v0.3.0-alpha.1
go mod tidy
```

Run this only after that version is published.

## Add The First Domain Module

Create `internal/invoices` or another domain-owned package. It should own:

- its manifest identity and capability requirements;
- migrations when persistence is selected;
- repositories and service types;
- HTTP routes or governed Action descriptors;
- focused unit and copied-out integration tests.

Register it explicitly in `internal/app/application.go`. Do not add a registry
scanner or import it from Modary itself.

## Keep Product Decisions Outside Core

Business table names, status values, roles, navigation, validation messages,
and deployment configuration belong to this project. Framework packages expose
bounded mechanics, not a base product schema.
