# T013 Test Results

- Date: 2026-07-30
- Result: Passed

```text
go test -count=1 ./projecttool ./action                       pass
go test -race -count=1 ./projecttool ./action                 pass
go vet ./projecttool ./action                                 pass
go test -count=20 ./projecttool                               pass
TypeScript focused regressions -count=100                     pass
manifest parser fuzz, 20 seconds                              pass
Linux and Windows cross-build                                 pass
external consumer test and vet                                pass
projecttool purity scan                                       zero forbidden calls
git diff --check                                              pass
```

The manifest fuzz completed without crash or contract escape. Root-swap,
pre-commit cancellation, exact duplicate tie, artifact rollback, JavaScript
escape, exact-number, and deterministic nested-error regressions are included in
the focused suites.
