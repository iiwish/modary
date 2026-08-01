# T023 Test Results

- Result: Passed
- Date: 2026-08-01
- Toolchain: Go 1.26.3 darwin/arm64 locally; GitHub-hosted Linux and macOS 15

## RED And Focused GREEN

```text
make remote-consumer VERSION=v0.1.0-alpha.1
RED: failed in checkout-only replacement assertions after remote resolution

go test ./scripts -run TestRemoteConsumer -count=1
GREEN: passed and verified MODARY_EXTERNAL_CONSUMER_COPIED_OUT=1 on go test

make remote-consumer VERSION=v0.1.0-alpha.1
GREEN: passed against the same immutable framework tag after the gate repair
```

## Candidate

```text
make release-readiness VERSION=v0.1.0-alpha.2
pass, including complete acceptance, race, count-20, fuzz, neutrality,
generated drift, cross-platform contracts, docs, and source stability

GitHub main CI 30688748867
pass: Linux quality and Darwin arm64 native jobs
```

## Tag And Distribution

```text
make release-preflight VERSION=v0.1.0-alpha.2 RELEASE_MODE=tag
pass, commit a4700f1c7ef53fe058a50fd43d65b906c3be89c4

GitHub tag CI 30688981095
pass: quality, darwin-arm64, release preflight, remote consumer, source stability

go list -m -json github.com/iiwish/modary@v0.1.0-alpha.2
pass: public proxy resolved exact tag and commit

make remote-consumer VERSION=v0.1.0-alpha.2
pass: no replace; verify, generated drift, tests, build, and version succeeded
```

Both GitHub prerelease records exist, are non-draft, and identify their exact
immutable tags. Alpha 2 is the supported release.
