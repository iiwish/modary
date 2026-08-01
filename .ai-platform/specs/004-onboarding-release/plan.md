# Modary Onboarding And Alpha Publication Plan

- Version: 1.0
- Status: Confirmed
- Last updated: 2026-08-01

## Decisions

1. Use Apache-2.0 for Modary-owned source and documentation. Preserve embedded
   third-party terms and aggregate attribution in a root `NOTICE`.
2. Promote the existing independently tested consumer to `examples/counter`.
   Do not duplicate it or create a second tutorial-only application.
3. Make the first-run journey `run -> inspect -> change -> generate -> test ->
   build`. Keep deep contracts in concepts, reference, and F0 documentation.
4. Treat `v0.1.0-alpha.1` as a GitHub prerelease and an exact Go module tag.
   Publish no generic binary, container, UI package, or downstream application.
5. Create the canonical public repository at `github.com/iiwish/modary`, use its
   private vulnerability-reporting surface as the security channel, and publish
   only the final accepted commit.

## Implementation Sequence

1. Add T021 contract, checklist, packet, and failing checks for stale example
   location and missing onboarding/license artifacts.
2. Move the Counter consumer to `examples/counter`, update current scripts,
   tests, docs, Make targets, and ignore rules, and preserve historical evidence.
3. Add Apache-2.0 root licensing and third-party attribution; tighten README,
   quickstart, first-change guidance, troubleshooting, and navigation.
4. Run focused onboarding tests, complete acceptance and CI, review the diff,
   and record T021 evidence.
5. Configure the canonical public remote and private reporting URL, finalize the
   changelog and release report, commit, and run clean candidate preflight.
6. Push the candidate, create the annotated tag, wait for tag CI, run remote
   consumer conformance, create the GitHub prerelease, and record T022 evidence.

## Risk Controls

- Path promotion is mechanical but cross-cutting; current code, scripts, tests,
  docs, Make, and CI must resolve the same canonical example path.
- Historical evidence patches are immutable and may retain the path that existed
  at their accepted commit.
- The root `NOTICE` must not replace or remove embedded third-party licenses.
- The release tag is immutable. Every mutable correction happens before tag
  creation; a published defect requires a subsequent version.
- Publication claims are updated only after the corresponding external result.
