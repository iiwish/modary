# F0 Known Limitations

1. Deployment is single-node SQLite. High availability, distributed locks,
   failover, and multi-writer operation are outside F0.
2. Local identity is a bootstrap implementation. Demo users share the configured
   initial password; password management, MFA, SSO, and recovery are not present.
3. The `rulary.ruleset.f0` schema contains one deterministic address operator
   and fixed F0 source and target tables. It is a kernel acceptance fixture;
   Rulary MVP records use the separately governed RuleSet v1 contract.
4. MCP exposes initialize, tool discovery, and tool calls needed by F0. Streaming,
   resumability, and a broader MCP resource surface are not implemented.
5. Action plans expire after five minutes and remain persisted; automated plan
   retention cleanup is not part of F0.
6. Audit retention, export, signing, and external SIEM delivery are not present.
7. The UI is English-only. Product localization is a later product decision.
8. SQLite schema evolution is forward-only in F0; automated rollback migrations
   are not provided.
9. F0 is developed as a monorepo application. A standalone application scaffold
   and packaged module SDK are outside the current CLI contract.
