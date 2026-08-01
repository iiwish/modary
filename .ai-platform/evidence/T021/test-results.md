# T021 Test Results

- Result: Passed
- Completed at: 2026-08-01T05:49:36Z
- Toolchain: Go 1.26.3 darwin/arm64

## Focused RED

```text
go test ./scripts -run 'TestRepositoryUsesApache|TestMakeUsesPublicCounter|TestCheckDocLinksRejectsRetired' -count=1
fail: LICENSE absent, public example path absent, retired path accepted
```

## Focused GREEN

```text
go test ./scripts -run 'TestRepositoryUsesApache|TestMakeUsesPublicCounter|TestCheckDocLinks' -count=1
pass

go test ./scripts -count=1
pass

go test ./internal/quality -count=1
pass
```

## Public Example

```text
make neutrality verify check-generated
pass

make test-consumer
pass, including copied-out verify/generate/test/vet/build/version

./dist/counter-console action run counter.increment --token-file <protected-file> --input <input-file> --preview
pass, returned plan_hash, summary, impact, and expiry
```

## Complete Gates

```text
make ci
pass
```

The complete CI run includes acceptance, root and public-example tests, vet,
native and cross builds, race, count-20 repetition, fuzz smoke, generated drift,
neutrality, documentation, source diff, and before/after source-state equality.

```text
git diff --check
pass

rg 'testdata/external-consumer' .github
pass, no retired CI cache path remains
```
