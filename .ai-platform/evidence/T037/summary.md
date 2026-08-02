# T037 Generated Consumer And Release Readiness

- Status: Completed
- Date: 2026-08-02
- Packet: `.ai-platform/specs/008-react-admin-starter/packets/T037.yaml`
- Execution: direct Codex execution; subagent delegation was not authorized
- Release state: Engineering ready
- Distribution state: Not released

The Admin Profile is React-only from template source through the generated
consumer and embedded production bundle. React 19.2.8, React Router 8.3.0,
TypeScript, Vite, Lucide React, React contexts, and the explicit frontend Module
registry form the complete first-party frontend. Source, dependency, lockfile,
generated-output, and embedded-asset checks reject Vue residue.

A fresh project generated at
`/tmp/modary-react-t037-final.1qfC4q/react-admin` passed the frozen frontend
pipeline, 8-file/19-test suite, byte-identical asset build, production dependency
audit, Go tidy/test/vet/build, and two consecutive real PostgreSQL integration
runs against one schema outside the Modary checkout with `GOWORK=off`. The same
consumer is running at `http://127.0.0.1:8084`; browser acceptance passed login,
create, update, delete, successful logout, responsive navigation, zero horizontal
overflow, and zero console warnings or errors.

Record updates return the complete database-authoritative row, failed logout
keeps the authenticated workspace, initialization failures are explicit and
retryable, generated integration fixtures are repeatable, normal text satisfies
WCAG AA contrast, and production assets use content-hashed immutable URLs.

Repository acceptance and the clean committed-candidate release-readiness gate
pass with Go 1.26.5, including race, 20-repeat, fuzz-smoke, and cross-build.
Reachable Go vulnerability scanning and production frontend dependency auditing
report no known reachable vulnerability. English and Chinese Admin onboarding
describe the architecture, development loop, security boundary, deterministic
production build, and consumer ownership.

The candidate is engineering-ready but is not tagged or published. The immutable
`v0.1.0-alpha.3` release is unchanged.
