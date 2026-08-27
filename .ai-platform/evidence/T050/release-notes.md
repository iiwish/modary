# Modary v0.3.0-alpha.2

This security maintenance prerelease keeps the accepted v0.3 Production
Foundation behavior and raises the supported Go baseline to 1.26.7.

## Security Maintenance

- Root, PostgreSQL, Governed PostgreSQL, OIDC, and OpenTelemetry modules use
  Go 1.26.7.
- Generated Profiles and their container build stages use the same Go 1.26.7
  baseline.
- The pinned vulnerability gate reports zero reachable findings across the
  published modules.

## Compatibility

Framework APIs, migrations, Profiles, routes, and generated product behavior
are unchanged from `v0.3.0-alpha.1`. Existing Alpha 1 consumers can update the
five selected module requirements without a migration step.

## Published Modules

- `github.com/iiwish/modary`
- `github.com/iiwish/modary/components/postgres`
- `github.com/iiwish/modary/components/governedpostgres`
- `github.com/iiwish/modary/components/oidc`
- `github.com/iiwish/modary/components/otel`

All five annotated tags resolve to candidate commit
`762f76dd54f8b2d8a0f9490e1c41e10876a3aae4`. Hosted tag CI and independent
replacement-free consumer and released-source container checks pass.

## Release Boundary

This is an alpha source release. It does not provide a hosted product,
container registry, database service, identity provider, collector, or
stable-v1 compatibility promise.
