# Generated Project Layout

The Starter emits ordinary source. Paths vary by Profile, but each project has
one command, one explicit composition root, consumer feature packages, tests,
and an exact Modary module requirement.

## API

```text
sample-api/
  cmd/sample-api/main.go
  internal/app/application.go
  internal/ping/component.go
  go.mod
  README.md
```

## Admin

```text
sample-admin/
  cmd/sample-admin/main.go
  internal/app/application.go
  internal/config/config.go
  internal/records/component.go
  internal/records/migrations/postgres/0001_records.sql
  internal/web/dist/...
  internal/web/web.go
  web/src/App.tsx
  web/src/stores/...
  web/src/modules/index.ts
  web/src/modules/records/...
  web/package.json
  web/pnpm-lock.yaml
```

`web/src/App.tsx` composes React providers and protected routes;
`web/src/modules/index.ts` is the frontend Module composition source. The
production bundle is checked in and embedded so a deployed Go process needs no
Node.js.

## Governed

```text
sample-governed/
  cmd/sample-governed/main.go
  cmd/sample-governed-worker/main.go
  internal/config/config.go
  internal/project/project.go
  internal/limits/module.go
  internal/limits/worker.go
  internal/limits/migrations/postgres/0001_limits.sql
```

The API command uses `appcmd`; the worker uses `task.Runner`. Both import the
same explicit `project.NewDefinition` composition.

## Optional Project Tooling

The `projecttool` package and `modary.yaml` remain available for consumers that
want verified generate/check/build workflows. They are not required by a
Starter project and are not a second Module-discovery mechanism. See the
[project manifest reference](../reference/project-manifest.md).
