# T022 Test Results

- Result: Rejected
- Date: 2026-08-01

## Passed

```text
make release-readiness VERSION=v0.1.0-alpha.1
pass

make release-preflight VERSION=v0.1.0-alpha.1 RELEASE_MODE=tag
pass, commit f57e3adda9a5f0e7335e821ef0b69eaf75c3548b

go list -m -json github.com/iiwish/modary@v0.1.0-alpha.1
pass, exact tag and commit resolved through the public Go proxy

GitHub tag CI quality and darwin-arm64 jobs
pass
```

## Failed Stop Condition

```text
make remote-consumer VERSION=v0.1.0-alpha.1
fail: TestCopiedOutConsumerGate and
TestDevelopmentReplaceIsRelativeAndGOWORKIndependent expected the checkout-only
replace after the remote gate intentionally removed it

GitHub tag CI release job
fail at the same remote-consumer command
```

The next irreversible transition, GitHub release creation, did not occur.
