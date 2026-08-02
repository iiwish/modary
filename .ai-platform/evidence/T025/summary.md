# T025 PostgreSQL Standard Persistence

- Status: Completed
- Completed at: 2026-08-01T14:39:05Z

Ported Module migration history, Action plans and idempotency, local Identity,
RBAC, and SQL Audit to PostgreSQL-native DDL, placeholders, booleans, JSONB,
constraints, and bounded read projections. Standard adapters retain explicit
empty provisioning, revocation, transaction participation, corruption
rejection, credential verification bounds, and secret-safe public errors.

Local Identity provisions principals independently from password and bearer
credentials. Credentialless service principals remain resolvable without
creating an unused login secret.
