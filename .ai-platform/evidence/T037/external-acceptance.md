# T037 Copied-Out React Admin Acceptance

- Result: Passed
- Date: 2026-08-02

The final consumer passes two consecutive integration runs against one schema,
proving that generated tests and persistent fixtures are repeatable.

## Isolation

The final Admin Profile was generated outside the Modary checkout at
`/tmp/modary-react-t037-final.1qfC4q/react-admin`. Generation selected exact
candidate version `v0.2.0-alpha.1` and used an explicit local replacement only
because the candidate is not published. Every Go validation command used
`GOWORK=off`, `GOTOOLCHAIN=local`, and Go 1.26.5.

## React Acceptance

- The generated project owns conventional `main.tsx`, `App.tsx`, providers,
  hooks, route registry, records Module, styles, tests, and Vite configuration.
- Frozen install, lint, strict typecheck, 8-file/19-test Vitest suite,
  production build, asset parity, and high-severity production audit passed.
- Source, dependency, lockfile, and embedded bundle scans found no `.vue` file,
  Vue, Pinia, Vue Router, Vue build plugin, or compatibility package.

## Go And PostgreSQL Acceptance

- Generated `go.mod` requires Go 1.26.5 and binds the unreleased candidate only
  through the documented development replacement.
- Tidy, test, vet, and build passed with work-file discovery disabled.
- The integration suite ran twice against PostgreSQL 17.10 database
  `modary_react_final_20260802_2008` and schema `react_admin_live`. It exercised
  health and content-hashed embedded React delivery, login, CSRF denial, complete
  create/update response timestamps, scoped create/list/update/delete,
  default-deny RBAC, shutdown, restart, persisted state, and fixture cleanup.

## Runtime Acceptance

The copied-out application is the process served at `http://127.0.0.1:8084`.
Browser acceptance passed sign-in, create, update, delete, successful logout,
re-login, mobile navigation, and zero-overflow checks on the final Router 8
bundle with no warning or error. HTML revalidates; content-hashed JavaScript and
CSS use immutable caching, and the legacy fixed asset URL is not served.

This proves independent source consumption and runtime operation. It does not
claim remote distribution: no v0.2 tag or push was created.
