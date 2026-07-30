# Modary

Modary is a lightweight modular application kernel in which human-facing UI,
HTTP clients, CLI commands, scheduled work, and AI agents execute the same
typed business actions through one governed runtime.

The accepted F0 application is a Rulary address-label vertical slice. Its
authoritative product, delivery, and acceptance records are:

- `docs/modary-ssot-v0.1.md`
- `docs/modary-f0-rulary-v0.1.md`
- `docs/f0-acceptance-report.md`

## Development

```bash
make bootstrap
make acceptance
make release-acceptance
make build
./dist/modary-rulary serve
```

The local server listens on `http://127.0.0.1:8080` by default. Runtime data is
stored under `data/` unless overridden with environment variables.

The bootstrap users are `author`, `publisher`, `operator`, `auditor`, and
`admin`. Local development uses password `modary-demo`; set
`MODARY_DEMO_PASSWORD` and `MODARY_AGENT_TOKEN` explicitly outside local
development. The server refuses a non-loopback listener while either demo
credential is active. HTTPS deployments mark the session cookie Secure through
direct TLS or `X-Forwarded-Proto: https`.

## Release

```bash
make release-linux
docker build --platform linux/amd64 -f Dockerfile.release -t modary-f0:amd64 .
make release-acceptance
```

The container is a scratch image containing only the release binary. The
runtime needs one writable data volume for SQLite and no Node.js, Redis,
PostgreSQL, queue, or object store.

`make release-acceptance` runs the contract, race, static, browser, performance,
Linux arm64/amd64 resource, and clean-container workflow checks.
