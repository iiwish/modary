# Modary JSON Schema Engine

This directory is a source fork of `github.com/xeipuuv/gojsonschema` v1.2.0.
The upstream source is licensed under Apache-2.0; the original license and
attribution are kept in `LICENSE-APACHE-2.0.txt` and `NOTICE`.

Modary keeps the engine private so validation semantics and resource accounting
cannot be changed through dependency package globals. The engine accepts one
already-decoded root schema. `$ref` resolution is limited to `#` and `#/...`
JSON Pointers inside that root, including their valid URI-fragment encodings;
there is no external schema registry and schema compilation performs no file or
network I/O.

The local engine provides:

- flag-only single-root and shared-budget multi-root validation;
- independent work, mismatch, and active-evaluation-frame budgets;
- a cached Draft 7 metaschema verified against a pinned SHA-256 digest;
- indexed `properties` lookup and precompiled `patternProperties` expressions;
- byte-aware accounting for object keys and numeric operands; and
- exact JSON-number semantics for numeric comparison, `const`, `enum`, and
  `uniqueItems`.

The public framework owns schema parsing, Draft 7 policy, and static complexity
limits. This engine only compiles an admitted document and evaluates an admitted
JSON value.
