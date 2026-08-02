# Optional Project Manifest Tooling

Starter projects do not require `modary.yaml`. The optional `projecttool`
package remains available for consumers that want deterministic Module graph,
Action catalog, TypeScript contract, and constrained build workflows.

## Manifest

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

Metadata must exactly match the consumer Definition and command. Go source
remains the only Module composition source; the graph is a review artifact, not
a runtime registry.

## Commands

- `verify`: validate project, pure Definition, graph, catalog, and paths.
- `generate`: render complete configured outputs, then install changed files.
- `generate --check`: compare without writing.
- `check`: verify project and generated state.
- `build`: build one configured package into one configured output on supported
  native hosts under the documented filesystem policy.

Output paths are bounded canonical relative paths. Absolute paths, `..`,
backslashes, aliases, overlaps, symlink traversal, and device names fail closed.
Generation is not a filesystem-wide crash-atomic transaction; review and commit
generated diffs.

The build path disables Go work-file discovery and ambient module flags, uses
the local toolchain with readonly module metadata, and stages outside the
project. It is not a compiler or hostile-source sandbox.
