# T012 Test Results

- Date: 2026-07-30
- Result: Passed

```text
go test ./appkit ./appcmd ./transport/httpapi                 pass
go test -race ./appkit ./appcmd ./transport/httpapi           pass
go vet ./appkit ./appcmd ./transport/httpapi                  pass
go test -race ./action ./identity ./module ./scope            pass
go test -count=20 ./appkit ./appcmd ./transport/httpapi       pass
public external-package boundary tests                         pass
transport and Module-ID neutrality scan                        zero matches
git diff --check                                               pass
```

The suites cover pure preflight, post-start rollback, opaque Application
projections, command parsing before startup, cancellable input, HTTP drain,
strict session/CSRF/JSON behavior, MCP authentication and protocol boundaries,
explicit mounts, SPA path/cache behavior, callback panic containment, and
exactly-once shutdown.
