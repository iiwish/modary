# Governed Profile Tutorial

The Governed Profile is for high-impact commands that need an authorized
Preview, exact execution plan, idempotent retry, detailed audit, and durable
work committed with business state.

## Create

```bash
export MODARY_STARTER_REPLACE="$(pwd)"
go run ./cmd/modary new ../policy-control \
  --profile governed \
  --module example.com/acme/policy-control \
  --name "Policy Control"
cd ../policy-control
go mod tidy
```

## Configure One Database And Two Schemas

River does not require a separate database. The application and queue schemas
must be different and owned by the configured database role.

```bash
export DATABASE_URL='postgres://user:password@127.0.0.1:5432/app?sslmode=disable'
export MODARY_APPLICATION_SCHEMA=policy_control
export MODARY_QUEUE_SCHEMA=policy_control_queue
export MODARY_OPERATOR_USERNAME=operator
export MODARY_OPERATOR_PASSWORD='development-password'
export MODARY_OPERATOR_TOKEN='development-bearer-token-000000000001'
export MODARY_ALLOW_INSECURE_COOKIE=true
```

The generated Identity is a development adapter. The bearer token must contain
at least 32 non-whitespace bytes; production uses randomly generated secrets and
a product IAM boundary.

## Preview

```bash
printf '%s' "$MODARY_OPERATOR_TOKEN" > token
chmod 600 token
printf '%s\n' '{"value":25,"expected_version":0}' > input.json

go run ./cmd/policy-control action run limits.set \
  --token-file token \
  --input input.json \
  --preview \
  --request-id preview-1
```

Preview reads the current scope state, authorizes intent and impact, and returns
`plan_hash`, a structured current/next summary, affected resources, and expiry.
It does not mutate the limit or enqueue work.

## Execute

Use the exact input and returned plan hash:

```bash
go run ./cmd/policy-control action run limits.set \
  --token-file token \
  --input input.json \
  --plan 'sha256:replace-with-preview-hash' \
  --idempotency-key workspace-default-limit-v1 \
  --request-id execute-1
```

Inside one framework-owned PostgreSQL transaction, the Runtime reauthorizes,
reserves idempotency, executes the consumer handler, inserts the River task,
persists the result, and writes the required allowed audit record. Failure rolls
the unit back. Retrying the same request with the same idempotency key returns
the stored result without repeating the mutation.

## Run The API And Worker

```bash
go run ./cmd/policy-control serve --listen 127.0.0.1:8080
go run ./cmd/policy-control-worker
```

The server mounts `/healthz`, `/api/`, and `/mcp` explicitly. The worker starts
one `task.Runner` and strictly decodes `limits.changed`. Replace its logging
callback with an idempotent product effect. Delivery is at least once.

## Test The Contract

```bash
DATABASE_URL="$DATABASE_URL" go test ./...
go vet ./...
go build ./cmd/policy-control ./cmd/policy-control-worker
```

The integration test proves required Preview, RBAC default deny, Execute,
idempotent replay, SQL Audit, shutdown/restart recovery, and post-restart task
consumption.

## When Not To Use Governed

Do not route ordinary list/edit forms through Preview merely for consistency.
Use the Admin/Store path for normal CRUD. Select Governed per operation when
impact, automation, audit, or retry semantics justify its extra database and
workflow cost.
