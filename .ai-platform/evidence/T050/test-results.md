# T050 Test Results

- Result: Passed
- Date: 2026-08-27

## Candidate And Publication

- Clean-worktree `make release-readiness VERSION=v0.3.0-alpha.2` passed under
  the exact Go 1.26.7 toolchain for candidate
  `762f76dd54f8b2d8a0f9490e1c41e10876a3aae4`.
- Hosted candidate main CI run `33041481326`: passed all quality,
  copied-profile, operational-provider, and Darwin ARM64 jobs.
- Tag-mode preflight rejected the missing refs before publication and passed
  after all five annotated tags existed at the accepted candidate.
- Hosted tag CI run `33042244822`: passed all jobs. Its release job passed
  tag-mode preflight, replacement-free remote consumption, released-source
  container acceptance, and source-stability verification.

## Independent Remote Verification

- `make remote-consumer VERSION=v0.3.0-alpha.2`: passed locally for all five
  public modules without a local replacement.
- `make released-container-acceptance VERSION=v0.3.0-alpha.2`: passed locally
  for released API, Admin, and Governed Profiles with Go 1.26.7 build images,
  non-root runtime, migrations, probes, and graceful termination.
- Five `GOWORK=off go list -m -json` queries through the public Go proxy
  returned exact version `v0.3.0-alpha.2`, Go version 1.26.7, and origin hash
  `762f76dd54f8b2d8a0f9490e1c41e10876a3aae4` for root, PostgreSQL, Governed
  PostgreSQL, OIDC, and OpenTelemetry modules.
- `gh release view v0.3.0-alpha.2`: returned a published, non-draft GitHub
  prerelease with the accepted tag, scope, compatibility, security baseline,
  verification result, and alpha limitations.

## Final Record

- `go test ./scripts`, `./scripts/check-docs.sh`,
  `./scripts/check-acceptance-evidence.sh`, strict T050 artifact validation, and
  `git diff --check`: passed for the release record.
- Goal closure requires the hosted `main` CI for the committed release record,
  exact remote tag identity, synchronized `origin/main`, and a clean worktree.
