# T027 Test Results

- Result: Passed
- Date: 2026-08-02

Required sequence:

- candidate preflight RED: expected rejection because the candidate is not yet
  committed and the worktree is dirty
- strict T027 artifact validation: passed
- first complete CI pass: correctly rejected a downstream product name in the
  release governance; the wording was replaced with a domain-neutral boundary
- complete candidate `make ci`: passed after the neutrality repair
- clean-worktree `make release-readiness VERSION=v0.1.0-alpha.3`: passed at
  `f39457a52c10ceecd8defb77e0def1b331c45dd2`
- hosted main CI: passed at https://github.com/iiwish/modary/actions/runs/30728949673
- annotated-tag preflight and push: passed; tag object
  `55e0a5b0d7b8a8422f4e9bd2e504b7d61d50d9c0`
- hosted tag CI and hosted remote consumer: passed at
  https://github.com/iiwish/modary/actions/runs/30729209127
- `GOWORK=off go list -m -json github.com/iiwish/modary@v0.1.0-alpha.3`:
  resolved commit `f39457a52c10ceecd8defb77e0def1b331c45dd2`
- local `make remote-consumer VERSION=v0.1.0-alpha.3`: passed without a
  replacement or Go work file
- GitHub prerelease: https://github.com/iiwish/modary/releases/tag/v0.1.0-alpha.3
- final strict artifact, documentation, and diff validation: passed
