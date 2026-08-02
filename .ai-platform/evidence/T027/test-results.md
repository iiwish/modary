# T027 Test Results

- Result: In_Progress
- Date: 2026-08-02

Required sequence:

- candidate preflight RED: expected rejection because the candidate is not yet
  committed and the worktree is dirty
- strict T027 artifact validation: passed
- first complete CI pass: correctly rejected a downstream product name in the
  release governance; the wording was replaced with a domain-neutral boundary
- complete candidate `make ci`: passed after the neutrality repair
- complete clean-worktree release readiness
- hosted main CI
- annotated-tag preflight and push
- hosted tag CI
- normal Go module resolution
- copied-out remote consumer
- final strict artifact, documentation, and diff validation
