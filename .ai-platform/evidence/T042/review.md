# T042 Review

- Verdict: Pass
- P0: 0
- P1: 0
- P2: 0

Spec compliance: principal identity is scope-independent; request scope is
validated separately; RBAC remains exact and default deny; password, session,
bearer, and transport responsibilities are explicit.

Security: session creation revalidates current actor type and password
credential freshness inside the persistence transaction. Password rotation,
actor-type change, revocation, expiry, malformed credentials, CSRF, cookie
policy, and lifecycle cancellation remain covered.

Engineering: Core retains no PostgreSQL dependency, OIDC is not introduced
into the root graph, published migration `0001_identity.sql` remains immutable,
and the forward migration removes the obsolete physical scope columns.

External acceptance: copied-out Admin and Governed consumers compile and run
against real PostgreSQL, and unselected components remain absent.
