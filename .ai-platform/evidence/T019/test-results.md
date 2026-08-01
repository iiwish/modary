# T019 Test Results

- Result: Passed
- Completed at: 2026-07-31T12:49:36Z

## RED

```text
go test ./scripts -run 'TestRelease|TestRemote' -count=1
FAIL: release-preflight.sh and remote-consumer.sh did not exist
```

## GREEN

```text
sh -n scripts/release-preflight.sh scripts/remote-consumer.sh
pass

go test ./scripts -run 'TestRelease|TestRemote|TestMakeRelease|TestCITag|TestMakeDocsCheck' -count=1
ok github.com/iiwish/modary/scripts
```

The fixtures cover a complete candidate and annotated tag, invalid stable
version, missing license, pending security contact, wrong origin, dirty state,
missing tag, complete remote command flow, source-fixture preservation, and
replacement-resolution rejection.

## Full Acceptance

```text
make acceptance
pass
```

Formatting, module drift, source diff, canonical and linked documentation,
framework and copied-out consumer tests, panicnil, vet, project verify,
generated drift, neutrality, native build, and the full cross-build matrix pass.

## Real Candidate Owner Gate

```text
./scripts/release-preflight.sh v0.1.0-alpha.1 candidate
expected fail: owner-selected redistribution license is absent
```
