# T017 Test Results

- Result: Passed
- Completed at: 2026-07-31T10:04:00Z

## RED

```text
go test ./scripts -run TestCheckDocsTreatsCompletedF0EvidenceAsHistorical -count=1
FAIL: post-F0 documentation invalidated historical T016 evidence
```

The failure reproduced the governance defect: the checker compared a completed
historical freeze with the evolving current worktree.

## GREEN

```text
go test ./scripts -run 'TestCheckDocsTreatsCompletedF0EvidenceAsHistorical|TestCheckDocsFinalModeFailsClosed' -count=1
ok github.com/iiwish/modary/scripts
```

The checker validates the stored T016 source-state artifact against its recorded
digest, while the existing matrix still rejects missing evidence, digest
tampering, inconsistent findings, invalid timestamps, and task/report drift.

## Documentation

```text
make docs-check
pass

git diff --check
pass
```
