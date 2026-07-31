# T016 Test Results

- Result: Passed
- Frozen tree: sha256:f3e127a3c957a9dab2dba06342f08b7f7c9ae24bc2dfcbe4729aa7924ea052f4
- Completed at: 2026-07-31T08:24:29Z
- Toolchain: Go 1.26.3 darwin/arm64

## Post-Freeze Full Component Gate

The following exact command completed successfully after the implementation
freeze, and a subsequent live source-state digest remained byte-identical:

```text
make format-check tidy-check diff-check test panicnil vet verify \
  check-generated neutrality build cross-build race repeat fuzz-smoke
```

It passed:

```text
gofmt and root/consumer go mod tidy -diff                     pass
complete source diff and unsupported-node checks              pass
root go test -count=1 ./...                                   pass
external consumer go test -count=1 -v ./...                   pass
root GODEBUG=panicnil=1 go test -count=1 ./...                pass
root and consumer go vet ./...                                pass
consumer verify, generate --check, and check                  pass
active-tree neutrality                                        pass
root and consumer native build                                pass
linux/darwin/windows amd64/arm64 root and consumer builds     pass
unsupported-platform test compilation matrix                  pass
root and copied-out consumer go test -race                    pass
curated root and complete consumer go test -count=20          pass
projecttool, JSON, schema, HTTP, and Darwin filepolicy fuzz   pass
```

## Focused Regressions

```text
temporary-schema SQL policy normal, race, and vet             pass
typed-nil provider boundary normal, race, and vet             pass
local Identity and Action Runtime synchronization race tests  pass
project build cancellation focused count-50                   pass
project build cancellation focused race count-20              pass
projecttool full count-20                                      pass
Windows amd64/arm64 projecttool test compilation              pass
```

The pre-repair repeated suite reproduced an empty PID read after file creation.
The final test waits for a valid positive PID; no iteration failed after the
repair.

## Exact Closure Gates

After all five T016 evidence files were present and passed the canonical
documentation checker, both required top-level commands completed
successfully:

```text
make acceptance                                                  pass
make ci                                                          pass
```

`make acceptance` reran formatting, dependency drift, complete source diff,
documentation, root and external consumer tests, panicnil, vet, consumer
verification and generated checks, neutrality, native builds, and the complete
cross-build matrix.

`make ci` reran acceptance, race, count-20, and fuzz gates, then repeated
neutrality, generated checks, and source diff checks. Its before/after complete
source snapshots were byte-identical, including T016 evidence.

## Frozen-State Check

```text
stored diff.patch SHA-256  f3e127a3c957a9dab2dba06342f08b7f7c9ae24bc2dfcbe4729aa7924ea052f4
live review state SHA-256  f3e127a3c957a9dab2dba06342f08b7f7c9ae24bc2dfcbe4729aa7924ea052f4
```
