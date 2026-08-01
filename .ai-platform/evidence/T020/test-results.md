# T020 Test Results

- Result: Passed
- Completed at: 2026-07-31T13:08:59Z
- Toolchain: Go 1.26.3 darwin/arm64

## Focused Release And Documentation Tests

```text
sh -n scripts/release-preflight.sh scripts/remote-consumer.sh scripts/check-doc-links.sh
pass

go test ./scripts -run 'TestRelease|TestRemote|TestCheckDocLinks|TestCheckDocs|TestMakeRelease|TestCITag' -count=1
pass
```

Covered success and failure contracts include semantic prerelease parsing,
license and private-reporting inputs, canonical origin, clean state, annotated
tag identity, remote replacement removal, exact version resolution, Node-family
tool rejection, required documentation, local links, index reachability, and
accepted historical evidence lineage.

## Acceptance

```text
make acceptance
pass
```

The gate passed formatting, root and consumer module drift, source diff,
canonical and linked documentation, framework and copied-out consumer tests,
`panic(nil)` compatibility, vet, project verify, generated drift, neutrality,
native build, and Linux/Darwin/Windows amd64/arm64 cross-build coverage.

## Complete CI

```text
make ci
pass
```

CI reran acceptance, framework and consumer race tests, shuffled count-20
high-risk suites, project/JSON/schema/protocol/Darwin ACL fuzz smoke, neutrality,
generated checks, source diff, and complete before/after source-state equality.

## Expected External Gate

```text
./scripts/release-preflight.sh v0.1.0-alpha.1 candidate
expected fail: owner-selected redistribution license is absent
```

The networked `make remote-consumer VERSION=v0.1.0-alpha.1` gate is not run
because no such remote version exists. The fixture test proves its command and
failure contract without claiming a remote distribution.
