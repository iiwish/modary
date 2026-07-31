# T015 Test Results

- Date: 2026-07-30
- Result: Passed
- Toolchain: Go 1.26.3

## Full Acceptance

```text
make acceptance                                                  pass
  gofmt check                                                    pass
  root and consumer go mod tidy -diff                            pass
  git diff --check                                               pass
  canonical documentation check                                 pass
  root go test ./...                                             pass
  external consumer go test -v ./...                             pass
  root and consumer go vet ./...                                 pass
  consumer verify/generate --check/check                         pass
  active-tree neutrality                                        pass
  native root and consumer build                                pass
  Linux/amd64 root and consumer cross-build                      pass
  Windows/amd64 root and consumer cross-build                    pass
```

The root suite includes the public Kernel, lifecycle, adapters, AppKit,
transports, project tooling, and public API documentation checks.

## Copied-Out Consumer

`TestCopiedOutConsumerGate` copied the consumer to a temporary directory outside
the Modary checkout, rewrote only its local development `replace`, set
`GOWORK=off`, and installed failing Node-family shims. It passed:

```text
go mod tidy -diff                                                pass
go test ./...                                                    pass
go vet ./...                                                     pass
go run ./tools/modary verify                                     pass (5 modules, 1 action)
go run ./tools/modary generate --check                           pass (current: true)
go run ./tools/modary check                                      pass (current: true)
go run ./tools/modary build                                      pass (dist/counter-console)
dist/counter-console version                                     pass (0.1.0)
Node-family shim invocations                                     zero
```

The behavioral suite passed Runtime, CLI, HTTP, MCP, restart, drain, scope
isolation, stale-plan, idempotency, audit, explicit provisioning, and empty
default checks. Structured import inspection found no Modary private, former
`core`, domain, test, or fixture import.

## Archive Verification

```text
source external manifest                                        167/167 OK
governance external manifest                                     51/51 OK
source exact path/mode/hash inventory                            pass
governance exact path/mode/hash inventory                        pass
source manifest digest anchor                                    pass
governance manifest digest anchor                                pass
source inventory digest anchor                                   pass
governance inventory digest anchor                               pass
make archive-check                                               pass
```

Exact-set generation excludes only declared `.DS_Store` platform metadata.

## Stability And Boundary Gates

```text
GOWORK=off go test -race ./...                                   pass
GOWORK=off go test -count=20 ./...                               pass
consumer race and count-20 suites                                pass
scripts/check-neutrality.sh                                      pass
scripts/check-docs.sh                                            pass
git diff --check                                                 pass
```

Race and repeated results were established in the component reviews and are
rerun repository-wide by T016 `make ci`; T015 completion does not substitute
for that final full gate.
