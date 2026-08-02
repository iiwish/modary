# T024 PostgreSQL And Task Contracts

- Status: Completed
- Completed at: 2026-08-01T14:39:05Z

Implemented the official `adapters/postgres` durable profile and public `task`
contract. The adapter validates and owns separate application and River
schemas, keeps write authority inside governed transactions, inserts River jobs
through the exact active `*sql.Tx`, and owns immutable runner lifecycle,
retry-policy, recovery, and shutdown behavior without exposing River types.

The schema pair is durably bound one-to-one. Advisory locks serialize schema,
River, and Module migrations across concurrent process startup; a queue schema
cannot be shared by a different application profile. Active duplicate
suppression is verified by logical task kind and unique key.

Focused review covered schema identifier injection, credential-safe errors,
transaction provenance, rollback-only nesting, enqueue atomicity, payload and
configuration bounds, duplicate suppression, runner races, and restart.
