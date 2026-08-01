# Project Manifest And Generated Files

Every consumer project has one `modary.yaml` at its verified root. It contains
project metadata and output policy, not module discovery.

## Schema

```yaml
application:
  id: example-console
  name: Example Console
  version: 0.1.0
outputs:
  graph: internal/generated/module_graph.json
  actions: internal/generated/action_catalog.json
  typescript: internal/generated/action_contracts.ts
build:
  package: ./cmd/example-console
  output: dist/example-console
```

`application` must exactly match the pure metadata returned by the consumer's
Definition and command options. Metadata drift fails verification or runtime
preflight rather than silently publishing two identities.

## Output Paths

Graph and Action catalog paths are required; TypeScript is optional. Paths are
canonical relative paths using portable ASCII components. Absolute paths,
backslashes, `.` or `..`, aliases, overlapping outputs, manifest overwrite,
build-package overlap, symlink traversal, and platform device names fail closed.

Generation validates the entire bounded output set before installing changed
files. Each file uses a sibling rename where the host filesystem guarantees
rename atomicity. The set is not a filesystem-wide or crash-atomic transaction,
so CI should run `generate --check` and consumers should review generated diffs.

## Build Target

`build.package` identifies one non-root relative Go package. Recursive package
patterns are not supported. `build.output` is one consumer-owned executable
path and must not overlap source, manifest, or generated outputs.

The build command disables Go work-file discovery and ambient module flags,
requires the local toolchain, builds with readonly module metadata, and stages
outside the project under the documented filesystem policy. It is not a
compiler sandbox; consumer source, the selected Go executable, remaining
environment, and cooperative output writers are trusted.

## Generated Graph

The graph records validated module identity, type, version, dependencies, and
capabilities. It is useful for review and tooling; it does not become the runtime
composition source.

## Action Catalog

The catalog contains public Action descriptors and contract hashes. It excludes
Handlers and private service bindings. Consumers may use it to build reviewed
UI or automation surfaces.

## TypeScript Contracts

The optional TypeScript output projects Action input, preview, output, and error
contracts for consumer frontend use. It is generated from the same Definition
as the runtime catalog. Pin the Modary version and regenerate on every upgrade.
