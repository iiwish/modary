# T044 Delivery Summary

- Status: Completed
- Task: Process Runtime, Probes, Drain, And Migrations
- Date: 2026-08-04

`processkit` defines local liveness, bounded dependency readiness, pre-shutdown
admission drain, accepted-request waiting, canonical build identity, and one
HTTP server lifecycle. `appkit.Migrate` applies the validated graph's forward
migrations without starting feature modules or binding handlers. Generated
Profiles expose `serve` and `migrate` operations and can disable migrations at
serve startup.
