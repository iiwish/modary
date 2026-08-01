# T021 Evidence Summary

- Status: Completed
- Date: 2026-08-01
- Packet: `.ai-platform/specs/004-onboarding-release/packets/T021.yaml`

## Changed Files

- Root `LICENSE` and `NOTICE` establish Apache-2.0 licensing for Modary-owned
  source and documentation and aggregate retained third-party attribution.
- The independent Counter consumer is promoted without duplication from the
  hidden test fixture to the public `examples/counter` path; Make, scripts,
  fixtures, documentation, and conformance gates use that canonical path.
- `README.md` is a short adoption surface. The documentation index, quickstart,
  first-application tutorial, troubleshooting guide, installation guidance,
  framework contract, limitations, ADR, contribution policy, and release
  records describe the same current candidate.
- Documentation tests reject the retired public path and missing onboarding
  documents. Quality walkers recognize any regular nested Go module boundary
  rather than hard-coding the public example as framework production code.
- GitHub Actions cache keys follow the promoted `examples/counter` module, so
  local and hosted release gates use the same canonical dependency inputs.

## Red And Green

- RED: focused tests observed the missing Apache license and notice, missing
  public example path, and absent retired-path rejection.
- GREEN: the focused license, Make, and documentation tests pass after the
  implementation.
- The documented Counter Action Preview command was executed from the built
  public example and returned a bound plan, summary, and impact.

## Onboarding Journey

The supported journey is one artifact and one dependency path: tagged source,
`examples/counter`, project verify, generated drift check, tests, build, Action
Preview, independent copy, removal of the development replacement, exact remote
version resolution, and a first generated contract change. Users need no
framework source reading or Node.js toolchain.

## License Review

- Root Apache-2.0 text contains the complete copyright, patent, redistribution,
  warranty, and liability terms.
- Root `NOTICE` identifies the Modary work and aggregates xeipuuv,
  johandorland, MongoDB, and JSON Schema Test Suite attribution.
- Embedded license, notice, file headers, and test-suite license remain present.
- `CONTRIBUTING.md` states the Apache-2.0 contribution default and provenance
  responsibility.

## Full Validation

- `make docs-check`, neutrality, project verify, generated checks, public
  example tests, and the copied-out example gate passed.
- `make acceptance` passed inside the final `make ci` run.
- `make ci` passed after the last implementation repair, including race,
  count-20 repetition, fuzz smoke, source stability, and the repeated final
  neutrality/generated/source-diff checks.
- `git diff --check` passed.

## Residual Risk

- Remote tagged consumption is intentionally unproved until T022 creates and
  pushes the exact release tag.
- The public example is deliberately comprehensive rather than a generated
  scaffold. F0 does not add a global CLI or project generator.
- Apache-2.0 compatibility and attribution do not replace legal review where a
  contributor or organization has separate ownership obligations.
