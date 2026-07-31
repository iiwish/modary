# JSON Schema Test Suite snapshot

This directory vendors the mandatory JSON Schema Draft 7 cases from
`json-schema-org/JSON-Schema-Test-Suite` at commit
`0c7b65dc16dd8eaa7bd83e21099c76610c3b246a`.

Source: <https://github.com/json-schema-org/JSON-Schema-Test-Suite>

The vendored JSON bytes are unchanged; the repository's narrow
`.gitattributes` rule preserves upstream whitespace instead of normalizing the
snapshot. The upstream MIT license is preserved in `LICENSE`. Modary's conformance
runner executes every vendored case or requires an exact, reviewed exclusion
for framework policy that is intentionally narrower than general Draft 7. The
snapshot contains 37 files, 257 cases, and 927 instance tests. Modary executes
223 cases and 856 instance tests; the remaining 34 cases and 71 instance tests
are exact exclusions for schema identifiers, URI bases, anchors, and non-local
resources that F0 deliberately prohibits. The runner verifies the snapshot
digest, counts, exclusion identity, and expected policy rejection.
