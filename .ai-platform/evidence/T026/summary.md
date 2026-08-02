# T026 Consumer, Deletion, And Acceptance

- Status: Completed
- Completed at: 2026-08-02T01:58:49Z

The copied-out Counter consumer uses the public PostgreSQL profile and task
contract. A governed Counter Action persists consumer control state and enqueues
one durable task in the same transaction; after the producer application stops,
a restarted application consumes that task through a public immutable Runner.
All active embedded-database code and dependencies are absent, and canonical
English and Chinese documentation explains the control database, River schema,
transactional enqueue, at-least-once semantics, external business-data boundary,
deployment, backup, security, and first task workflow.

The reviewed source passes framework, copied-out consumer, PostgreSQL, race,
repetition, fuzz, build, cross-build, documentation, neutrality, and
source-stability gates. The current T026 task definitions and execution packet
also pass the generic delivery-artifact validator in strict mode. Modary
acceptance has no downstream product repository dependency.
