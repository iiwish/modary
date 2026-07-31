# T010 Test Results

```text
validate_delivery_artifacts: 0 errors, 0 warnings
source files:                167
snapshot files:              167
checksum diff:               empty
governance files:             51
governance checksum diff:     empty
exact-set diff:               empty for both archives
file mode/hash inventory:     pass for all 218 files
git diff --check:            pass
make archive-check:          pass
```

The source inventory excludes Git metadata, `.ai-platform`, dependency caches,
runtime data, top-level build output, and `.DS_Store` platform metadata as
declared in the preservation record. The versioned inventories anchor exact path,
mode, and digest sets for both external archives.
