# Quickstart

> [简体中文](../zh-CN/getting-started/quickstart.md)

This tutorial creates the database-free API Profile from the current checkout.
It is the shortest path for understanding Modary's ownership model.

## 1. Prepare The Framework

```bash
git clone https://github.com/iiwish/modary.git
cd modary
make bootstrap
export MODARY_STARTER_REPLACE="$(pwd)"
```

## 2. Create An API Project

Choose an existing parent directory and a new lowercase project ID:

```bash
go run ./cmd/modary new ../inventory-api \
  --profile api \
  --module example.com/acme/inventory-api \
  --name "Inventory API"
```

The command returns JSON containing the destination, selected Profile, and
sorted file list. Running it again against the same destination fails without
changing any file.

## 3. Inspect The Composition

Open `../inventory-api/internal/app/application.go`. The generated Definition
contains only the consumer `ping` Module. HTTP routes are mounted explicitly;
there is no database or feature scanning.

Confirm absence from the package graph:

```bash
cd ../inventory-api
GOWORK=off go mod tidy
GOWORK=off go list -deps ./... | rg 'river|postgres|localidentity|sqlaudit'
```

The command should print nothing.

## 4. Test And Run

```bash
GOWORK=off go test ./...
GOWORK=off go build ./cmd/inventory-api
go run ./cmd/inventory-api
```

In another terminal:

```bash
curl -fsS http://127.0.0.1:8080/healthz
curl -fsS http://127.0.0.1:8080/api/ping
```

Stop the process with `Ctrl-C`. The command drains the HTTP server and calls the
Host-owned exactly-once shutdown sequence.

## 5. Choose The Next Step

- Add a consumer feature with [Add a Module](../how-to/add-module.md).
- Create a back office with the [Admin Profile](admin-profile.md).
- Learn high-impact commands with the [Governed Profile](governed-profile.md).
- Compare exact component boundaries in [Choose a Profile](choose-profile.md).
