# Requirements Quality Checklist

- [x] Control data, queue data, and external business data have distinct owners.
- [x] The same-database requirement for transactional enqueue is explicit.
- [x] PostgreSQL schema selection and identifier injection are bounded.
- [x] Transaction nesting, rollback, commit, and authority constraints are testable.
- [x] Public task API scope and River encapsulation are testable.
- [x] At-least-once semantics and consumer idempotency responsibility are explicit.
- [x] SQLite deletion and lack of compatibility are explicit.
- [x] Real PostgreSQL, concurrency, recovery, and shutdown validation are required.
- [x] Copied-out Counter conformance proves public consumer adoption without
      making a downstream product repository an acceptance dependency.
- [x] No unresolved Critical or High ambiguity remains.
